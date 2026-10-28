package invite_test

import (
	"errors"
	"math"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	invitesvc "github.com/linusxiong/ggboard/internal/service/invite"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestTransferRejectsChangedSessionAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change map[string]any
	}{
		{"password", map[string]any{"password_version": 1}},
		{"email", map[string]any{"email": "new@example.com"}},
		{"account", map[string]any{"uuid": "replacement-account"}},
		{"ban", map[string]any{"banned": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"commission_withdraw_limit": 0}))
			u := testutil.CreateUser(t, a, "alice@example.com")
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{"commission_balance": 1000}).Error; err != nil {
				t.Fatal(err)
			}
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(tc.change).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := invitesvc.New(a).Transfer(t.Context(), u, 500); !errors.Is(err, invitesvc.ErrSessionExpired) {
				t.Fatalf("stale session transfer: %v", err)
			}
			if _, err := invitesvc.New(a).GenerateCode(t.Context(), u); !errors.Is(err, invitesvc.ErrSessionExpired) {
				t.Fatalf("stale session invite code: %v", err)
			}
			if _, err := invitesvc.New(a).Withdraw(t.Context(), u, "USDT", "example-account"); !errors.Is(err, invitesvc.ErrSessionExpired) {
				t.Fatalf("stale session withdrawal: %v", err)
			}
			var got model.User
			if err := a.DB.First(&got, u.ID).Error; err != nil || got.Balance != 0 || got.CommissionBalance != 1000 {
				t.Fatalf("balance changed: %+v, %v", got, err)
			}
			var codes, tickets int64
			if err := a.DB.Model(&model.InviteCode{}).Where("user_id = ?", u.ID).Count(&codes).Error; err != nil {
				t.Fatal(err)
			}
			if err := a.DB.Model(&model.Ticket{}).Where("user_id = ?", u.ID).Count(&tickets).Error; err != nil {
				t.Fatal(err)
			}
			if codes != 0 || tickets != 0 {
				t.Fatalf("stale session wrote codes=%d tickets=%d", codes, tickets)
			}
		})
	}
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "rich@example.com")
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{"commission_balance": 1000, "balance": int64(math.MaxInt64 - 100)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := invitesvc.New(a).Transfer(t.Context(), u, 500); !errors.Is(err, invitesvc.ErrInsufficientCommission) {
		t.Fatalf("overflow transfer: %v", err)
	}
}
