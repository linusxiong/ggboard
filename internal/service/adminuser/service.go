package adminuser

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	statsvc "github.com/linusxiong/ggboard/internal/service/stat"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// MaxTransferGB bounds GB inputs so the byte value fits in int64.
const MaxTransferGB = 1_000_000_000

// GBToBytes converts a (possibly fractional) GB amount to bytes.
func GBToBytes(gb float64) int64 { return int64(math.Round(gb * float64(model.BytesPerGB))) }

func (s *Service) load(ctx context.Context, id uint64) (*model.User, error) {
	return loadUser(s.a.DB.WithContext(ctx), id)
}

func loadUser(db *gorm.DB, id uint64) (*model.User, error) {
	var u model.User
	err := db.First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.NotFound("user not found")
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func loadPlan(db *gorm.DB, id uint64, field string) (*model.Plan, error) {
	var p model.Plan
	err := db.First(&p, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.FieldError(field, "plan not found")
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// optionalPlan loads plan id, returning nil (no error) when it no longer exists.
func optionalPlan(db *gorm.DB, id *uint64) (*model.Plan, error) {
	if id == nil {
		return nil, nil
	}
	var p model.Plan
	err := db.First(&p, *id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func checkGroup(db *gorm.DB, id uint64) error {
	var n int64
	if err := db.Model(&model.ServerGroup{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return httpx.FieldError("group_id", "server group not found")
	}
	return nil
}

func checkInviter(db *gorm.DB, id, self uint64) error {
	if id == self {
		return httpx.FieldError("invite_user_id", "a user cannot invite themselves")
	}
	var n int64
	if err := db.Model(&model.User{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return httpx.FieldError("invite_user_id", "user not found")
	}
	return nil
}

// otherAdmins counts administrators other than id.
func otherAdmins(db *gorm.DB, id uint64) (int64, error) {
	var n int64
	err := db.Model(&model.User{}).Where("is_admin = ? AND id <> ?", true, id).Count(&n).Error
	return n, err
}

func nonZero(p *uint64) *uint64 {
	if p == nil || *p == 0 {
		return nil
	}
	return p
}

func (s *Service) invalidate(id uint64) {
	auth.InvalidateUser(s.a, id)
	usersvc.New(s.a).InvalidateNodeUsers()
}

// ---------------------------------------------------------------- create

// CreateInput is POST /admin/users. transfer_enable is in GB. A plan fills group_id, transfer_enable,
// speed_limit and device_limit when they are omitted; 0 ids mean none.
type CreateInput struct {
	Email          string     `json:"email" binding:"required,email,max=191"`
	Password       string     `json:"password" binding:"required,min=8,max=128"`
	PlanID         *uint64    `json:"plan_id"`
	GroupID        *uint64    `json:"group_id"`
	TransferEnable *float64   `json:"transfer_enable" binding:"omitempty,gte=0,lte=1000000000"`
	ExpiredAt      *time.Time `json:"expired_at"`
	IsAdmin        bool       `json:"is_admin"`
	IsStaff        bool       `json:"is_staff"`
	SpeedLimit     *int       `json:"speed_limit" binding:"omitempty,gte=0"`
	DeviceLimit    *int       `json:"device_limit" binding:"omitempty,gte=0"`
	Balance        int64      `json:"balance" binding:"gte=0"`
	Remarks        string     `json:"remarks" binding:"max=16000"` // runes: ≤ 64000 bytes, fits a MySQL TEXT
	InviteUserID   *uint64    `json:"invite_user_id"`
}

// Create registers a user on behalf of an administrator.
func (s *Service) Create(ctx context.Context, in CreateInput, now time.Time) (*model.User, error) {
	db := s.a.DB.WithContext(ctx)
	opts := usersvc.CreateOptions{
		ExpiredAt: in.ExpiredAt, SpeedLimit: in.SpeedLimit, DeviceLimit: in.DeviceLimit, Balance: in.Balance,
		IsAdmin: in.IsAdmin, IsStaff: in.IsStaff, Remarks: in.Remarks,
	}
	var plan *model.Plan
	if id := nonZero(in.PlanID); id != nil {
		p, err := loadPlan(db, *id, "plan_id")
		if err != nil {
			return nil, err
		}
		plan, opts.PlanID = p, &p.ID
	}
	if id := nonZero(in.GroupID); id != nil {
		if err := checkGroup(db, *id); err != nil {
			return nil, err
		}
		opts.GroupID = id
	} else if plan != nil {
		gid := plan.GroupID
		opts.GroupID = &gid
	}
	switch {
	case in.TransferEnable != nil:
		opts.TransferEnable = GBToBytes(*in.TransferEnable)
	case plan != nil:
		opts.TransferEnable = plan.TransferBytes()
	}
	if plan != nil {
		if opts.SpeedLimit == nil {
			opts.SpeedLimit = plan.SpeedLimit
		}
		if opts.DeviceLimit == nil {
			opts.DeviceLimit = plan.DeviceLimit
		}
	}
	if id := nonZero(in.InviteUserID); id != nil {
		if err := checkInviter(db, *id, 0); err != nil {
			return nil, err
		}
		opts.InviteUserID = id
	}
	svc := usersvc.New(s.a)
	u, err := svc.Create(ctx, in.Email, in.Password, opts)
	if errors.Is(err, usersvc.ErrEmailTaken) {
		return nil, httpx.Conflict("email already registered").WithField("email", "is already registered")
	}
	if err != nil {
		return nil, err
	}
	if next := svc.ComputeNextResetAt(u, plan, now); next != nil {
		if err := db.Model(&model.User{}).Where("id = ?", u.ID).Update("next_reset_at", next).Error; err != nil {
			return nil, fmt.Errorf("adminuser: set next_reset_at: %w", err)
		}
		u.NextResetAt = next
	}
	return u, nil
}

// ---------------------------------------------------------------- update

// UpdateInput is PATCH /admin/users/:id. Absent fields are unchanged; Nullable fields accept null to clear
// (plan_id, group_id, invite_user_id: null or 0). transfer_enable, u and d are bytes. Setting a plan without
// group_id also moves the user to the plan's group. next_reset_at is recomputed whenever plan_id or expired_at
// is present.
type UpdateInput struct {
	Email             *string             `json:"email,omitempty" binding:"omitempty,email,max=191"`
	Password          *string             `json:"password,omitempty" binding:"omitempty,min=8,max=128"`
	Banned            *bool               `json:"banned,omitempty"`
	U                 *int64              `json:"u,omitempty" binding:"omitempty,gte=0"`
	D                 *int64              `json:"d,omitempty" binding:"omitempty,gte=0"`
	TransferEnable    *int64              `json:"transfer_enable,omitempty" binding:"omitempty,gte=0"`
	ExpiredAt         Nullable[time.Time] `json:"expired_at,omitzero"`
	PlanID            Nullable[uint64]    `json:"plan_id,omitzero"`
	GroupID           Nullable[uint64]    `json:"group_id,omitzero"`
	InviteUserID      Nullable[uint64]    `json:"invite_user_id,omitzero"`
	IsAdmin           *bool               `json:"is_admin,omitempty"`
	IsStaff           *bool               `json:"is_staff,omitempty"`
	SpeedLimit        Nullable[int]       `json:"speed_limit,omitzero"`
	DeviceLimit       Nullable[int]       `json:"device_limit,omitzero"`
	Balance           *int64              `json:"balance,omitempty" binding:"omitempty,gte=0"`
	CommissionBalance *int64              `json:"commission_balance,omitempty" binding:"omitempty,gte=0"`
	CommissionType    *int                `json:"commission_type,omitempty" binding:"omitempty,oneof=0 1 2"`
	CommissionRate    Nullable[int]       `json:"commission_rate,omitzero"`
	Discount          Nullable[int]       `json:"discount,omitzero"`
	Remarks           *string             `json:"remarks,omitempty" binding:"omitempty,max=16000"`
	RemindExpire      *bool               `json:"remind_expire,omitempty"`
	RemindTraffic     *bool               `json:"remind_traffic,omitempty"`
	TelegramID        Nullable[int64]     `json:"telegram_id,omitzero"`
}

// Validate checks the Nullable fields (binding tags cover the others).
func (in *UpdateInput) Validate() error {
	e := httpx.BadRequest("validation failed")
	ranged := []struct {
		name     string
		v        Nullable[int]
		min, max int
	}{
		{"speed_limit", in.SpeedLimit, 0, math.MaxInt32},
		{"device_limit", in.DeviceLimit, 0, math.MaxInt32},
		{"commission_rate", in.CommissionRate, 0, 100},
		{"discount", in.Discount, 0, 100},
	}
	for _, r := range ranged {
		if r.v.Valid && (r.v.Value < r.min || r.v.Value > r.max) {
			e = e.WithField(r.name, fmt.Sprintf("must be between %d and %d", r.min, r.max))
		}
	}
	if len(e.Errors) > 0 {
		return e
	}
	return nil
}

// Update applies in to user id on behalf of actor. Administrators cannot ban themselves or drop their own
// admin role, and the last administrator cannot be demoted.
func (s *Service) Update(ctx context.Context, actor *model.User, id uint64, in UpdateInput, now time.Time) (*model.User, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	var out *model.User
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, err := loadUser(tx, id)
		if err != nil {
			return err
		}
		set, err := s.changes(tx, actor, u, &in, now)
		if err != nil {
			return err
		}
		if len(set) > 0 {
			err := tx.Model(&model.User{}).Where("id = ?", id).Updates(set).Error
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return httpx.Conflict("email already registered").WithField("email", "is already registered")
			}
			if err != nil {
				return err
			}
		}
		out, err = loadUser(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	if in.Password != nil {
		if err := usersvc.New(s.a).SetPassword(ctx, out, *in.Password); err != nil {
			return nil, err
		}
	}
	s.invalidate(id)
	return out, nil
}

// changes builds the column updates of in for u.
func (s *Service) changes(tx *gorm.DB, actor, u *model.User, in *UpdateInput, now time.Time) (map[string]any, error) {
	set := map[string]any{}
	self := actor != nil && actor.ID == u.ID
	if in.Email != nil {
		if e := usersvc.NormalizeEmail(*in.Email); e != u.Email {
			set["email"] = e
		}
	}
	if in.Banned != nil {
		if *in.Banned && self {
			return nil, httpx.Conflict("you cannot ban yourself")
		}
		set["banned"] = *in.Banned
	}
	if in.IsAdmin != nil {
		if !*in.IsAdmin && u.IsAdmin {
			if self {
				return nil, httpx.Conflict("you cannot remove your own administrator role")
			}
			if n, err := otherAdmins(tx, u.ID); err != nil {
				return nil, err
			} else if n == 0 {
				return nil, httpx.Conflict("the last administrator cannot be demoted")
			}
		}
		set["is_admin"] = *in.IsAdmin
	}
	ints := []struct {
		col string
		v   *int64
	}{{"u", in.U}, {"d", in.D}, {"transfer_enable", in.TransferEnable}, {"balance", in.Balance}, {"commission_balance", in.CommissionBalance}}
	for _, f := range ints {
		if f.v != nil {
			set[f.col] = *f.v
		}
	}
	bools := []struct {
		col string
		v   *bool
	}{{"is_staff", in.IsStaff}, {"remind_expire", in.RemindExpire}, {"remind_traffic", in.RemindTraffic}}
	for _, f := range bools {
		if f.v != nil {
			set[f.col] = *f.v
		}
	}
	nullInts := []struct {
		col string
		v   Nullable[int]
	}{{"speed_limit", in.SpeedLimit}, {"device_limit", in.DeviceLimit}, {"commission_rate", in.CommissionRate}, {"discount", in.Discount}}
	for _, f := range nullInts {
		if f.v.Set {
			set[f.col] = f.v.Ptr()
		}
	}
	if in.CommissionType != nil {
		set["commission_type"] = *in.CommissionType
	}
	if in.Remarks != nil {
		set["remarks"] = *in.Remarks
	}
	if in.TelegramID.Set {
		set["telegram_id"] = in.TelegramID.Ptr()
	}

	if in.GroupID.Set {
		gid := nonZero(in.GroupID.Ptr())
		if gid != nil {
			if err := checkGroup(tx, *gid); err != nil {
				return nil, err
			}
		}
		set["group_id"] = gid
	}
	if in.InviteUserID.Set {
		iid := nonZero(in.InviteUserID.Ptr())
		if iid != nil {
			if err := checkInviter(tx, *iid, u.ID); err != nil {
				return nil, err
			}
		}
		set["invite_user_id"] = iid
	}

	// Plan, expiry and the derived next_reset_at.
	plan, planID, expiredAt := (*model.Plan)(nil), u.PlanID, u.ExpiredAt
	if in.PlanID.Set {
		planID = nonZero(in.PlanID.Ptr())
		set["plan_id"] = planID
	}
	if in.PlanID.Set && planID != nil {
		p, err := loadPlan(tx, *planID, "plan_id")
		if err != nil {
			return nil, err
		}
		plan = p
		if !in.GroupID.Set {
			gid := p.GroupID
			set["group_id"] = &gid
		}
	} else if in.ExpiredAt.Set {
		p, err := optionalPlan(tx, planID) // a deleted plan no longer resets traffic
		if err != nil {
			return nil, err
		}
		plan = p
	}
	if in.ExpiredAt.Set {
		expiredAt = in.ExpiredAt.Ptr()
		set["expired_at"] = expiredAt
	}
	if in.PlanID.Set || in.ExpiredAt.Set {
		probe := model.User{ExpiredAt: expiredAt}
		set["next_reset_at"] = usersvc.New(s.a).ComputeNextResetAt(&probe, plan, now)
	}
	return set, nil
}

// ---------------------------------------------------------------- delete

// Delete removes user id and its dependants (orders, tickets and their messages, stat_users, invite codes,
// commission logs earned as inviter, rule profile, passkeys and the user_mfa row). Users and orders that reference the user as inviter get
// invite_user_id = null. Actors cannot delete themselves, and the last administrator cannot be deleted.
func (s *Service) Delete(ctx context.Context, actor *model.User, id uint64) error {
	if actor != nil && actor.ID == id {
		return httpx.Conflict("you cannot delete your own account")
	}
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, err := loadUser(tx, id)
		if err != nil {
			return err
		}
		if u.IsAdmin {
			if n, err := otherAdmins(tx, id); err != nil {
				return err
			} else if n == 0 {
				return httpx.Conflict("the last administrator cannot be deleted")
			}
		}
		steps := []func() error{
			func() error {
				return tx.Where("ticket_id IN (?)", tx.Model(&model.Ticket{}).Select("id").Where("user_id = ?", id)).
					Delete(&model.TicketMessage{}).Error
			},
			func() error { return tx.Where("user_id = ?", id).Delete(&model.Ticket{}).Error },
			func() error { return tx.Where("user_id = ?", id).Delete(&model.Order{}).Error },
			func() error {
				return tx.Model(&model.Order{}).Where("invite_user_id = ?", id).Update("invite_user_id", nil).Error
			},
			func() error { return tx.Where("user_id = ?", id).Delete(&model.StatUser{}).Error },
			func() error { return tx.Where("user_id = ?", id).Delete(&model.InviteCode{}).Error },
			func() error { return tx.Where("invite_user_id = ?", id).Delete(&model.CommissionLog{}).Error },
			func() error { return tx.Where("user_id = ?", id).Delete(&model.RuleProfile{}).Error },
			// Second factors: an orphaned passkey would sign in as whoever gets this id next (passkey login
			// resolves the owner through these rows), and an orphaned TOTP secret would be forced on them.
			func() error { return tx.Where("user_id = ?", id).Delete(&model.Passkey{}).Error },
			func() error { return tx.Where("user_id = ?", id).Delete(&model.UserMFA{}).Error },
			func() error {
				return tx.Model(&model.User{}).Where("invite_user_id = ?", id).Update("invite_user_id", nil).Error
			},
			func() error { return tx.Delete(&model.User{}, id).Error },
		}
		for _, step := range steps {
			if err := step(); err != nil {
				return fmt.Errorf("adminuser: delete user %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.invalidate(id)
	return nil
}

// ---------------------------------------------------------------- resets, login token, stats

// ResetTraffic sets u = d = 0 and recomputes next_reset_at from the user's plan.
func (s *Service) ResetTraffic(ctx context.Context, id uint64, now time.Time) (*model.User, error) {
	var out *model.User
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, err := loadUser(tx, id)
		if err != nil {
			return err
		}
		plan, err := optionalPlan(tx, u.PlanID)
		if err != nil {
			return err
		}
		next := usersvc.New(s.a).ComputeNextResetAt(u, plan, now)
		if err := tx.Model(&model.User{}).Where("id = ?", id).
			Updates(map[string]any{"u": 0, "d": 0, "next_reset_at": next}).Error; err != nil {
			return err
		}
		out, err = loadUser(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.invalidate(id)
	return out, nil
}

// ResetSecret issues a new uuid and subscription token (and ends the user's sessions).
func (s *Service) ResetSecret(ctx context.Context, id uint64) (*model.User, error) {
	u, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := usersvc.New(s.a).ResetSecurity(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// LoginURL returns app_url + "/login?token=<one-time token>" for user id (the SPA exchanges the token at
// POST /passport/login/token within auth.LoginTokenTTL). Administrators and staff cannot be impersonated (409):
// the token skips their second factor and would let one admin act as, or plant a passkey on, another.
func (s *Service) LoginURL(ctx context.Context, id uint64) (string, error) {
	u, err := s.load(ctx, id)
	if err != nil {
		return "", err
	}
	if u.IsAdmin || u.IsStaff {
		return "", httpx.Conflict("administrators and staff cannot be impersonated")
	}
	tok := auth.IssueLoginToken(s.a, id)
	return strings.TrimRight(s.a.Settings.String("app_url"), "/") + "/login?token=" + tok, nil
}

// Stats returns the paid order total and the commission earned by user id.
func (s *Service) Stats(ctx context.Context, id uint64) (DetailStats, error) {
	var st DetailStats
	db := s.a.DB.WithContext(ctx)
	if err := db.Model(&model.Order{}).Select("COALESCE(SUM(total_amount), 0)").
		Where("user_id = ? AND status IN ?", id, statsvc.PaidStatuses).Scan(&st.OrdersTotal).Error; err != nil {
		return st, fmt.Errorf("adminuser: order total: %w", err)
	}
	if err := db.Model(&model.CommissionLog{}).Select("COALESCE(SUM(get_amount), 0)").
		Where("invite_user_id = ?", id).Scan(&st.CommissionTotal).Error; err != nil {
		return st, fmt.Errorf("adminuser: commission total: %w", err)
	}
	return st, nil
}
