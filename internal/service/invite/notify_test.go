package invite_test

import (
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/notifytest"
	invitesvc "github.com/linusxiong/ggboard/internal/service/invite"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestWithdrawNotification(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"commission_withdraw_limit": 1}))
	rec := notifytest.Attach(t, a)
	u := testutil.CreateUser(t, a, "alice@example.com")
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("commission_balance", 12345).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := invitesvc.New(a).Withdraw(t.Context(), u.ID, "USDT", "TQx1234567890abcdef9aZ"); err != nil {
		t.Fatal(err)
	}
	fs := notifytest.Fields(rec.Last(t, notify.TypeWithdraw))
	if fs["user"] != "alice@example.com" || fs["amount"] != "¥123.45" || fs["method"] != "USDT" || fs["account"] != "TQx***9aZ" {
		t.Fatalf("withdraw = %v", fs)
	}
	rec.None(t, notify.TypeTicketNew) // one notification per request, not two

	// A refused request notifies nothing.
	if _, err := invitesvc.New(a).Withdraw(t.Context(), u.ID, "PayPal", "x"); err == nil {
		t.Fatal("unknown method accepted")
	}
	if n := len(rec.Events(t, notify.TypeWithdraw)); n != 1 {
		t.Fatalf("withdraw events = %d", n)
	}
}
