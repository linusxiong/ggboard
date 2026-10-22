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
// presence (400), duplicate email (409), invite code validity (400 when forced), email code (400), then
// the user is created with the inviter, the trial plan and the remind defaults.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*model.User, error) {
	st := s.a.Settings
	if st.Bool("stop_register") {
		return nil, httpx.Forbidden("registration is closed")
	}
	email := usersvc.NormalizeEmail(in.Email)
	if st.Bool("email_whitelist_enable") && !EmailAllowed(email, st.Strings("email_whitelist_suffix")) {
		return nil, httpx.FieldError("email", "this email domain is not allowed")
	}
	if err := s.checkRegisterIP(in.IP); err != nil {
		return nil, err
	}
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
	if _, err := s.users.FindByEmail(ctx, email); err == nil {
		return nil, httpx.Conflict("this email is already registered")
	} else if !errors.Is(err, usersvc.ErrNotFound) {
		return nil, err
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
		opts.SpeedLimit, opts.DeviceLimit = plan.SpeedLimit, plan.DeviceLimit
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
	s.countRegisterIP(in.IP)
	return u, nil
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

func (s *Service) checkRegisterIP(ip string) error {
	st := s.a.Settings
	if ip == "" || !st.Bool("register_limit_by_ip_enable") {
		return nil
	}
	key := kvRegisterIP + ip
	v, ok := s.a.KV.Get(key)
	n, _ := v.(int)
	if !ok || n < st.Int("register_limit_count") {
		return nil
	}
	wait, _ := s.a.KV.TTL(key)
	return retryError(fmt.Sprintf("too many registrations from this IP address, try again in %d minutes", waitMinutes(wait)), wait)
}

func (s *Service) countRegisterIP(ip string) {
	st := s.a.Settings
	if ip == "" || !st.Bool("register_limit_by_ip_enable") {
		return
	}
	s.a.KV.Update(kvRegisterIP+ip, minutes(st.Int("register_limit_expire")), true, func(old any, _ bool) any {
		n, _ := old.(int)
		return n + 1
	})
}
