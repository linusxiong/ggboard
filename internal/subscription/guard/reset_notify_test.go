package guard_test

import (
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/notifytest"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// The leak auto-rotation keeps its own mail (template notify, with the new link) and sends no subscription_reset.
func TestLeakRotationSendsNoResetNotice(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{
		"subscribe_leak_max_ips": 1, "subscribe_leak_action": "rotate", "app_url": "https://panel.example.com",
	}))
	rec := notifytest.Attach(t, a)
	u := testutil.CreateUser(t, a, "leak@example.com")
	record(a, u.ID, u.Token, "192.0.2.1", time.Now())
	record(a, u.ID, u.Token, "198.51.100.1", time.Now())
	if err := guard.FlushAccess(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	var fresh model.User
	if err := a.DB.First(&fresh, u.ID).Error; err != nil || fresh.Token == u.Token {
		t.Fatalf("token not rotated: %v", err)
	}
	mails := testutil.Mails(t, a)
	if len(mails) != 1 || mails[0].Template != mail.TplNotify {
		t.Fatalf("mails = %+v", mails)
	}
	for _, d := range rec.Deliveries(t) {
		if d.Event.Type == notify.TypeSubscriptionReset {
			t.Fatalf("unexpected delivery %+v", d)
		}
	}
}
