package adminuser

import (
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/notifytest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// The notice goes out only when the token or the uuid really changed, and says exactly what changed.
func TestNotifySubscriptionResetOnlyOnChange(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	rec := notifytest.Attach(t, a)
	u := testutil.CreateUser(t, a, "notice@example.com")
	s := New(a)

	same := *u
	s.notifySubscriptionReset(u, &same)
	sessions := *u
	sessions.PasswordVersion++ // a password change ends sessions but keeps the subscription
	s.notifySubscriptionReset(u, &sessions)
	if evs := rec.UserEvents(t, u.ID, notify.TypeSubscriptionReset); len(evs) != 0 {
		t.Fatalf("events without a credential change: %+v", evs)
	}
	if mails := testutil.Mails(t, a); len(mails) != 0 {
		t.Fatalf("mails without a credential change: %+v", mails)
	}

	rotated := *u
	rotated.Token = "fedcba9876543210fedcba9876543210"
	s.notifySubscriptionReset(u, &rotated)
	evs := rec.UserEvents(t, u.ID, notify.TypeSubscriptionReset)
	if len(evs) != 1 || !strings.HasPrefix(evs[0].Body, "系统已重置你的订阅链接，旧链接已失效。\n请重新登录") {
		t.Fatalf("token only = %+v", evs)
	}
	mails := testutil.Mails(t, a)
	if len(mails) != 1 || mails[0].Template != mail.TplSubscriptionReset || mails[0].To != u.Email ||
		!strings.Contains(mails[0].Text, "系统已重置您的订阅链接，旧的订阅链接已失效。") || strings.Contains(mails[0].Text, "退出登录") ||
		strings.Contains(mails[0].Text, rotated.Token) || !strings.Contains(mails[0].Text, "https://panel.example.com/subscription") {
		t.Fatalf("token only mail = %+v", mails)
	}
}
