package account

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/model"
)

// PlanRef is the {id, name} summary of a plan embedded in user objects.
type PlanRef struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// UserView is the `User` self view of docs/API.md. It never contains the password hash, the subscription
// token, remarks, the last login IP or the Telegram id.
type UserView struct {
	ID                uint64     `json:"id"`
	Email             string     `json:"email"`
	UUID              string     `json:"uuid"`
	PlanID            *uint64    `json:"plan_id"`
	Plan              *PlanRef   `json:"plan"`
	GroupID           *uint64    `json:"group_id"`
	TransferEnable    int64      `json:"transfer_enable"`
	U                 int64      `json:"u"`
	D                 int64      `json:"d"`
	ExpiredAt         *time.Time `json:"expired_at"`
	NextResetAt       *time.Time `json:"next_reset_at"`
	SpeedLimit        *int       `json:"speed_limit"`
	DeviceLimit       *int       `json:"device_limit"`
	Balance           int64      `json:"balance"`
	CommissionBalance int64      `json:"commission_balance"`
	CommissionRate    *int       `json:"commission_rate"`
	Discount          *int       `json:"discount"`
	RemindExpire      bool       `json:"remind_expire"`
	RemindTraffic     bool       `json:"remind_traffic"`
	IsAdmin           bool       `json:"is_admin"`
	IsStaff           bool       `json:"is_staff"`
	Banned            bool       `json:"banned"`
	AvatarURL         string     `json:"avatar_url"`
	CreatedAt         time.Time  `json:"created_at"`
	LastLoginAt       *time.Time `json:"last_login_at"`
}

// AdminUserView is the admin view of docs/API.md: the self view plus admin-only fields (never the password).
type AdminUserView struct {
	UserView
	Token          string               `json:"token"`
	CommissionType model.CommissionType `json:"commission_type"`
	InviteUserID   *uint64              `json:"invite_user_id"`
	LastLoginIP    string               `json:"last_login_ip"`
	TelegramID     *int64               `json:"telegram_id"`
	Remarks        string               `json:"remarks"`
}

// AvatarURL returns the Gravatar URL of email (md5 of the trimmed, lower-cased address; mystery-person fallback).
func AvatarURL(email string) string {
	sum := md5.Sum([]byte(strings.ToLower(strings.TrimSpace(email))))
	return "https://www.gravatar.com/avatar/" + hex.EncodeToString(sum[:]) + "?s=200&d=mp"
}

// RefOf returns the {id, name} of plan, or nil.
func RefOf(plan *model.Plan) *PlanRef {
	if plan == nil {
		return nil
	}
	return &PlanRef{ID: plan.ID, Name: plan.Name}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}

// ToUserView builds the self view of u. plan is u's plan (nil when none or deleted); when plan is nil the
// preloaded u.Plan is used, so admin lists can Preload("Plan") instead of loading plans one by one.
func ToUserView(u *model.User, plan *model.Plan) UserView {
	if plan == nil && u.Plan != nil && u.PlanID != nil && u.Plan.ID == *u.PlanID {
		plan = u.Plan
	}
	return UserView{
		ID:                u.ID,
		Email:             u.Email,
		UUID:              u.UUID,
		PlanID:            u.PlanID,
		Plan:              RefOf(plan),
		GroupID:           u.GroupID,
		TransferEnable:    u.TransferEnable,
		U:                 u.U,
		D:                 u.D,
		ExpiredAt:         utc(u.ExpiredAt),
		NextResetAt:       utc(u.NextResetAt),
		SpeedLimit:        u.SpeedLimit,
		DeviceLimit:       u.DeviceLimit,
		Balance:           u.Balance,
		CommissionBalance: u.CommissionBalance,
		CommissionRate:    u.CommissionRate,
		Discount:          u.Discount,
		RemindExpire:      u.RemindExpire,
		RemindTraffic:     u.RemindTraffic,
		IsAdmin:           u.IsAdmin,
		IsStaff:           u.IsStaff,
		Banned:            u.Banned,
		AvatarURL:         AvatarURL(u.Email),
		CreatedAt:         u.CreatedAt.UTC(),
		LastLoginAt:       utc(u.LastLoginAt),
	}
}

// ToAdminUserView builds the admin view of u (see ToUserView for plan).
func ToAdminUserView(u *model.User, plan *model.Plan) AdminUserView {
	return AdminUserView{
		UserView:       ToUserView(u, plan),
		Token:          u.Token,
		CommissionType: u.CommissionType,
		InviteUserID:   u.InviteUserID,
		LastLoginIP:    u.LastLoginIP,
		TelegramID:     u.TelegramID,
		Remarks:        u.Remarks,
	}
}

// LoadPlan returns the plan with id, or nil when id is nil or the plan no longer exists.
func LoadPlan(ctx context.Context, db *gorm.DB, id *uint64) (*model.Plan, error) {
	if id == nil {
		return nil, nil
	}
	var p model.Plan
	err := db.WithContext(ctx).First(&p, *id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// View loads u's plan and returns the self view.
func (s *Service) View(ctx context.Context, u *model.User) (UserView, error) {
	plan, err := LoadPlan(ctx, s.a.DB, u.PlanID)
	if err != nil {
		return UserView{}, err
	}
	return ToUserView(u, plan), nil
}
