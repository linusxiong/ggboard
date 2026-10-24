package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// RegisterInput is a registration request.
type RegisterInput struct {
	Email      string
	Password   string
	InviteCode string
	EmailCode  string
	Captcha    string
	IP         string
}

// EmailDomain returns the part after the last "@" of a normalized email ("" when there is none).
func EmailDomain(email string) string {
	i := strings.LastIndexByte(email, '@')
	if i < 0 {
		return ""
	}
	return email[i+1:]
}

// EmailAllowed reports whether email's domain is exactly one of suffixes (case-insensitive; entries may be
// written as "example.com" or "@example.com").
func EmailAllowed(email string, suffixes []string) bool {
	domain := EmailDomain(usersvc.NormalizeEmail(email))
	if domain == "" {
		return false
	}
	for _, s := range suffixes {
		if strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "@") == domain {
			return true
		}
	}
	return false
}

// Register creates an account following the register settings, in this order: stop_register (403),
// email whitelist (400), per-IP limit (429), captcha (400), invite_force (400), email_verify code
// presence (400), then
//   - email_verify off: duplicate email (409), invite code validity (400 when forced);
//   - email_verify on: invite code validity (400 when forced, checked first so a typo does not burn the
//     code), email code (400), duplicate email (409). Only the mailbox owner can tell a registered address
//     from a new one: without the code both get the same 400 on email_code.
//
// The user is then created with the inviter, the trial plan and the remind defaults; with email_verify on
// email_verified_at is stamped (accounts registered without it are reclaimed on their first password reset,
// see ResetPassword).
func (s *Service) Register(ctx context.Context, in RegisterInput) (*model.User, error) {
	st := s.a.Settings
	if st.Bool("stop_register") {
		return nil, httpx.Forbidden("registration is closed")
	}
	email := usersvc.NormalizeEmail(in.Email)
	if st.Bool("email_whitelist_enable") && !EmailAllowed(email, st.Strings("email_whitelist_suffix")) {
		return nil, httpx.FieldError("email", "this email domain is not allowed")
	}
	release, err := s.reserveRegisterIP(in.IP)
	if err != nil {
		return nil, err
	}
	registered := false
	defer func() {
		if !registered {
			release() // only completed registrations count towards register_limit_count
		}
	}()
	if err := s.VerifyCaptcha(ctx, in.Captcha, in.IP); err != nil {
		return nil, err
	}
	inviteCode := strings.TrimSpace(in.InviteCode)
	forced := st.Bool("invite_force")
	if forced && inviteCode == "" {
		return nil, httpx.FieldError("invite_code", "an invite code is required")
	}
	verify := st.Bool("email_verify")
	emailCode := strings.TrimSpace(in.EmailCode)
	if verify && emailCode == "" {
		return nil, httpx.FieldError(codeFieldName, "is required")
	}
	if !verify {
		if err := s.checkEmailFree(ctx, email); err != nil {
			return nil, err
		}
	}

	var invite *model.InviteCode
	if inviteCode != "" {
		var err error
		if invite, err = s.findInvite(ctx, inviteCode); err != nil {
			return nil, err
		}
		if invite == nil && forced {
			return nil, httpx.FieldError("invite_code", "invalid invite code")
		}
	}
	if verify {
		if err := s.ConsumeCode(PurposeRegister, email, emailCode); err != nil {
			return nil, err
		}
		if err := s.checkEmailFree(ctx, email); err != nil {
			return nil, err
		}
	}

	// Single-use invite codes are claimed with a conditional update so concurrent registrations cannot
	// both use them; the claim is released if the user cannot be created.
	claimed := false
	if invite != nil && !st.Bool("invite_never_expire") {
		res := s.a.DB.WithContext(ctx).Model(&model.InviteCode{}).
			Where("id = ? AND status = ?", invite.ID, false).Update("status", true)
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 0 {
			if forced {
				return nil, httpx.FieldError("invite_code", "invalid invite code")
			}
			invite = nil
		} else {
			claimed = true
		}
	}

	now := time.Now()
	var opts usersvc.CreateOptions
	if invite != nil {
		opts.InviteUserID = &invite.UserID
	}
	plan, err := s.trialPlan(ctx)
	if err != nil {
		return nil, err
	}
	if plan != nil && st.Int("try_out_hour") > 0 {
		exp := now.Add(time.Duration(st.Int("try_out_hour")) * time.Hour)
		groupID := plan.GroupID
		opts.PlanID, opts.GroupID = &plan.ID, &groupID
		opts.TransferEnable = plan.TransferBytes()
		opts.ExpiredAt = &exp
		opts.SpeedLimit, opts.DeviceLimit, opts.ConnLimit = plan.SpeedLimit, plan.DeviceLimit, plan.ConnLimit
	}

	u, err := s.users.Create(ctx, email, in.Password, opts)
	if err != nil {
		if claimed {
			if rerr := s.a.DB.WithContext(ctx).Model(&model.InviteCode{}).Where("id = ?", invite.ID).
				Update("status", false).Error; rerr != nil {
				s.a.Log.Warn("account: release invite code failed", "invite_code_id", invite.ID, "err", rerr)
			}
		}
		if errors.Is(err, usersvc.ErrEmailTaken) {
			return nil, httpx.Conflict("this email is already registered")
		}
		return nil, err
	}
	if verify {
		// As with next_reset_at below, a failure must not fail the registration; the account just stays
		// unverified (its first password reset then rotates its credentials).
		if err := s.users.MarkEmailVerified(ctx, u); err != nil {
			s.a.Log.Error("account: mark email verified failed", "user_id", u.ID, "err", err)
		}
	}
	if plan != nil {
		if next := s.users.ComputeNextResetAt(u, plan, now); next != nil {
			// The account exists at this point: a failure here must not turn the registration into an error
			// (the traffic_reset job would simply skip the user until an admin or a purchase sets the date).
			if err := s.a.DB.WithContext(ctx).Model(&model.User{}).Where("id = ?", u.ID).
				UpdateColumn("next_reset_at", *next).Error; err != nil {
				s.a.Log.Error("account: set trial next_reset_at failed", "user_id", u.ID, "err", err)
			} else {
				u.NextResetAt = next
			}
		}
	}
	registered = true
	return u, nil
}

// checkEmailFree returns 409 when email is already registered. The unique index still decides races (Create
// returns ErrEmailTaken).
func (s *Service) checkEmailFree(ctx context.Context, email string) error {
	if _, err := s.users.FindByEmail(ctx, email); err == nil {
		return httpx.Conflict("this email is already registered")
	} else if !errors.Is(err, usersvc.ErrNotFound) {
		return err
	}
	return nil
}

// findInvite returns the unused invite code whose inviter still exists, or nil.
func (s *Service) findInvite(ctx context.Context, code string) (*model.InviteCode, error) {
	var ic model.InviteCode
	err := s.a.DB.WithContext(ctx).Where("code = ? AND status = ?", code, false).First(&ic).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var n int64
	if err := s.a.DB.WithContext(ctx).Model(&model.User{}).Where("id = ?", ic.UserID).Count(&n).Error; err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return &ic, nil
}

// trialPlan returns the plan of try_out_plan_id, or nil when trials are off or the plan no longer exists.
func (s *Service) trialPlan(ctx context.Context) (*model.Plan, error) {
	id := s.a.Settings.Int64("try_out_plan_id")
	if id <= 0 {
		return nil, nil
	}
	var p model.Plan
	err := s.a.DB.WithContext(ctx).First(&p, uint64(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		s.a.Log.Warn("account: try_out_plan_id points to a missing plan; no trial granted", "plan_id", id)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// reserveRegisterIP reserves one registration of ip (IPv6 per /64) under register_limit_* (account.ReserveAttempt:
// reserved before the work so concurrent requests cannot all pass the check), or returns a *RetryError.
func (s *Service) reserveRegisterIP(ip string) (release func(), err error) {
	st := s.a.Settings
	if ip == "" || !st.Bool("register_limit_by_ip_enable") {
		return func() {}, nil
	}
	key := kvRegisterIP + httpx.IPKey(ip)
	release, ok := ReserveAttempt(s.a, key, st.Int("register_limit_count"), minutes(st.Int("register_limit_expire")))
	if !ok {
		wait, _ := s.a.KV.TTL(key)
		return nil, retryError(fmt.Sprintf("too many registrations from this IP address, try again in %d minutes", waitMinutes(wait)), wait)
	}
	return release, nil
}
