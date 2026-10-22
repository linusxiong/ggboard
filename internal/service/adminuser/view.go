// Package adminuser implements user administration for /admin/users: the admin user view, list filters,
// create/update/delete (with cascades), traffic and secret resets, batch generation, CSV export and bulk mail.
package adminuser

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
)

// PlanRef is the embedded plan summary of a user.
type PlanRef struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// View is the admin user view: the self view (docs/API.md "User") plus admin-only fields. The password hash
// is never included; uuid and token are blanked for staff (Redact).
type View struct {
	ID                uint64               `json:"id"`
	Email             string               `json:"email"`
	UUID              string               `json:"uuid"`
	Token             string               `json:"token"`
	PlanID            *uint64              `json:"plan_id"`
	Plan              *PlanRef             `json:"plan"`
	GroupID           *uint64              `json:"group_id"`
	TransferEnable    int64                `json:"transfer_enable"`
	U                 int64                `json:"u"`
	D                 int64                `json:"d"`
	ExpiredAt         *time.Time           `json:"expired_at"`
	NextResetAt       *time.Time           `json:"next_reset_at"`
	SpeedLimit        *int                 `json:"speed_limit"`
	DeviceLimit       *int                 `json:"device_limit"`
	Balance           int64                `json:"balance"`
	CommissionBalance int64                `json:"commission_balance"`
	CommissionType    model.CommissionType `json:"commission_type"`
	CommissionRate    *int                 `json:"commission_rate"`
	Discount          *int                 `json:"discount"`
	RemindExpire      bool                 `json:"remind_expire"`
	RemindTraffic     bool                 `json:"remind_traffic"`
	IsAdmin           bool                 `json:"is_admin"`
	IsStaff           bool                 `json:"is_staff"`
	Banned            bool                 `json:"banned"`
	AvatarURL         string               `json:"avatar_url"`
	InviteUserID      *uint64              `json:"invite_user_id"`
	InviteUserEmail   *string              `json:"invite_user_email"`
	LastLoginAt       *time.Time           `json:"last_login_at"`
	LastLoginIP       string               `json:"last_login_ip"`
	TelegramID        *int64               `json:"telegram_id"`
	Remarks           string               `json:"remarks"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}

// DetailStats are the money totals of GET /admin/users/:id.
type DetailStats struct {
	OrdersTotal     int64 `json:"orders_total"`     // total_amount of the user's paid orders (cents)
	CommissionTotal int64 `json:"commission_total"` // commission earned as inviter (commission_logs, cents)
}

// Detail is the admin view plus stats.
type Detail struct {
	View
	Stats DetailStats `json:"stats"`
}

// AvatarURL returns the Gravatar URL of email (md5 of the trimmed, lower-cased address).
func AvatarURL(email string) string {
	sum := md5.Sum([]byte(strings.ToLower(strings.TrimSpace(email))))
	return "https://www.gravatar.com/avatar/" + hex.EncodeToString(sum[:]) + "?s=200&d=mp"
}

// Redact blanks the credentials (uuid, token) for viewers who are not administrators.
func (v *View) Redact() { v.UUID, v.Token = "", "" }

func newView(u *model.User) View {
	return View{
		ID: u.ID, Email: u.Email, UUID: u.UUID, Token: u.Token, PlanID: u.PlanID, GroupID: u.GroupID,
		TransferEnable: u.TransferEnable, U: u.U, D: u.D, ExpiredAt: u.ExpiredAt, NextResetAt: u.NextResetAt,
		SpeedLimit: u.SpeedLimit, DeviceLimit: u.DeviceLimit, Balance: u.Balance, CommissionBalance: u.CommissionBalance,
		CommissionType: u.CommissionType, CommissionRate: u.CommissionRate, Discount: u.Discount,
		RemindExpire: u.RemindExpire, RemindTraffic: u.RemindTraffic, IsAdmin: u.IsAdmin, IsStaff: u.IsStaff,
		Banned: u.Banned, AvatarURL: AvatarURL(u.Email), InviteUserID: u.InviteUserID, LastLoginAt: u.LastLoginAt,
		LastLoginIP: u.LastLoginIP, TelegramID: u.TelegramID, Remarks: u.Remarks, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

// Service is cheap to construct: adminuser.New(a) per request is fine.
type Service struct{ a *app.App }

// New returns a Service bound to a.
func New(a *app.App) *Service { return &Service{a: a} }

// Views converts users to admin views, loading plan names and inviter emails with one query each.
func (s *Service) Views(ctx context.Context, users []model.User) ([]View, error) {
	planIDs, inviterIDs := map[uint64]struct{}{}, map[uint64]struct{}{}
	for i := range users {
		if users[i].PlanID != nil {
			planIDs[*users[i].PlanID] = struct{}{}
		}
		if users[i].InviteUserID != nil {
			inviterIDs[*users[i].InviteUserID] = struct{}{}
		}
	}
	plans := map[uint64]string{}
	if len(planIDs) > 0 {
		var rows []model.Plan
		if err := s.a.DB.WithContext(ctx).Select("id", "name").Where("id IN ?", keys(planIDs)).
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("adminuser: load plans: %w", err)
		}
		for _, p := range rows {
			plans[p.ID] = p.Name
		}
	}
	emails := map[uint64]string{}
	if len(inviterIDs) > 0 {
		var rows []model.User
		if err := s.a.DB.WithContext(ctx).Select("id", "email").Where("id IN ?", keys(inviterIDs)).
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("adminuser: load inviters: %w", err)
		}
		for _, u := range rows {
			emails[u.ID] = u.Email
		}
	}
	out := make([]View, len(users))
	for i := range users {
		v := newView(&users[i])
		if v.PlanID != nil {
			if name, ok := plans[*v.PlanID]; ok {
				v.Plan = &PlanRef{ID: *v.PlanID, Name: name}
			}
		}
		if v.InviteUserID != nil {
			if email, ok := emails[*v.InviteUserID]; ok {
				v.InviteUserEmail = &email
			}
		}
		out[i] = v
	}
	return out, nil
}

// View converts one user.
func (s *Service) View(ctx context.Context, u *model.User) (View, error) {
	vs, err := s.Views(ctx, []model.User{*u})
	if err != nil {
		return View{}, err
	}
	return vs[0], nil
}

// Detail loads user id with its view and stats.
func (s *Service) Detail(ctx context.Context, id uint64) (*Detail, error) {
	u, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	v, err := s.View(ctx, u)
	if err != nil {
		return nil, err
	}
	d := &Detail{View: v}
	if d.Stats, err = s.Stats(ctx, id); err != nil {
		return nil, err
	}
	return d, nil
}

func keys(m map[uint64]struct{}) []uint64 {
	out := make([]uint64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
