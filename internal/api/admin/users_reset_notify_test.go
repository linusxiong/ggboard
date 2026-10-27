package admin_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/notifytest"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
	"github.com/linusxiong/ggboard/internal/notify/telegram/telegramtest"
	"github.com/linusxiong/ggboard/internal/service/adminuser"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// resetMails waits for the mail queue and returns the subscription_reset mails.
func resetMails(t *testing.T, a *app.App) []mail.Message {
	t.Helper()
	var out []mail.Message
	for _, m := range testutil.Mails(t, a) {
		if m.Template == mail.TplSubscriptionReset {
			out = append(out, m)
		}
	}
	return out
}

// noSecrets fails when any of the user's subscription credentials (or a subscription URL) appears in texts.
func noSecrets(t *testing.T, what string, users []model.User, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if strings.Contains(s, "token=") {
			t.Errorf("%s contains a subscription URL:\n%s", what, s)
		}
		for _, u := range users {
			for _, secret := range []string{u.Token, u.UUID} {
				if strings.Contains(s, secret) {
					t.Errorf("%s contains a credential %q:\n%s", what, secret, s)
				}
			}
		}
	}
}

func TestAdminResetSecretNotifiesUser(t *testing.T) {
	e := newOpsEnv(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com/"}))
	rec := notifytest.Attach(t, e.a)
	u := testutil.CreateUser(t, e.a, "reset@example.com")
	old := e.reload(t, u.ID)

	v := testutil.Decode[adminuser.View](t, testutil.Do(t, e.r, http.MethodPost, usersPath(u.ID, "/reset-secret"), nil, e.adminTok))
	cur := e.reload(t, u.ID)
	if v.Token == old.Token || v.UUID == old.UUID || cur.PasswordVersion != old.PasswordVersion+1 {
		t.Fatalf("reset: %+v -> %+v", old, cur)
	}

	mails := resetMails(t, e.a)
	if len(mails) != 1 || mails[0].To != "reset@example.com" {
		t.Fatalf("mails = %+v", mails)
	}
	m := mails[0]
	if m.Subject != "【GG Bond】系统已重置您的订阅信息" {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{"系统已重置您的订阅链接和 UUID，旧的订阅链接已失效。", "您也已在所有设备上退出登录。",
		"请重新登录网站，在“订阅”页面复制新的订阅链接", "前往订阅页面: https://panel.example.com/subscription\n"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, m.Text)
		}
	}
	if !strings.Contains(m.HTML, `<a href="https://panel.example.com/subscription"`) {
		t.Errorf("HTML lacks the subscription page button:\n%s", m.HTML)
	}
	noSecrets(t, "mail", []model.User{old, cur}, m.Subject, m.Text, m.HTML)

	// One Telegram event, to the user only (the administrator did it).
	evs := rec.UserEvents(t, u.ID, notify.TypeSubscriptionReset)
	if len(evs) != 1 || evs[0].Admin || evs[0].Field("site") != "https://panel.example.com/subscription" ||
		!strings.HasPrefix(evs[0].Body, "系统已重置你的订阅链接和 UUID，旧链接已失效。\n你也已在所有设备上退出登录。\n") {
		t.Fatalf("user events = %+v", evs)
	}
	rec.None(t, notify.TypeSubscriptionReset)
	n := 0
	for _, d := range rec.Deliveries(t) {
		if d.Event.Type == notify.TypeSubscriptionReset {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("subscription_reset delivered %d times", n)
	}
	noSecrets(t, "telegram", []model.User{old, cur}, telegram.Render(evs[0], notify.LocaleZH))
}

func TestAdminResetSecretEnglishWithoutAppURL(t *testing.T) {
	e := newOpsEnv(t, testutil.WithSettings(map[string]any{"default_locale": "en", "app_name": "Example Panel"}))
	rec := notifytest.Attach(t, e.a)
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPost, usersPath(e.user.ID, "/reset-secret"), nil, e.adminTok), http.StatusOK)

	mails := resetMails(t, e.a)
	if len(mails) != 1 || mails[0].To != e.user.Email || mails[0].Subject != "Your Example Panel subscription was reset by the administrator" {
		t.Fatalf("mails = %+v", mails)
	}
	m := mails[0]
	if !strings.Contains(m.Text, "The administrator reset your subscription link and UUID, so the old link no longer works.") ||
		!strings.Contains(m.Text, "You have also been signed out on all devices.") ||
		strings.Contains(m.Text, "/subscription") || strings.Contains(m.HTML, "Open subscription page") {
		t.Fatalf("without app_url there is no link:\n%s", m.Text)
	}
	evs := rec.UserEvents(t, e.user.ID, notify.TypeSubscriptionReset)
	if len(evs) != 1 || evs[0].Title != "🔄 Subscription reset" || len(evs[0].Fields) != 0 {
		t.Fatalf("events = %+v", evs)
	}
}

// Through the real Telegram driver: only the bound user's chat gets the message (never an administrator chat,
// whatever telegram_events lists), nothing while telegram_user_notify is off, and a failed delivery does not
// fail the reset.
func TestAdminResetSecretTelegram(t *testing.T) {
	e := newOpsEnv(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	all := make([]string, len(notify.AdminTypes))
	for i, typ := range notify.AdminTypes {
		all[i] = string(typ)
	}
	srv := telegramtest.Enable(t, e.a, map[string]any{telegram.KeyAdminChats: []string{"111"}, telegram.KeyEvents: all})
	if _, err := notify.Start(e.a, notify.Options{Backoff: func(int) time.Duration { return time.Millisecond }}); err != nil {
		t.Fatal(err)
	}
	drain := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := notify.Wait(ctx, e.a); err != nil {
			t.Fatal(err)
		}
	}
	bound := testutil.CreateUser(t, e.a, "bound@example.com")
	if err := e.a.DB.Model(&model.User{}).Where("id = ?", bound.ID).Update("telegram_id", int64(777)).Error; err != nil {
		t.Fatal(err)
	}
	unbound := testutil.CreateUser(t, e.a, "unbound@example.com")
	for _, u := range []*model.User{bound, unbound} {
		testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPost, usersPath(u.ID, "/reset-secret"), nil, e.adminTok), http.StatusOK)
	}
	drain()
	calls := srv.Calls("sendMessage")
	want := "<b>🔄 订阅信息已重置</b>\n系统已重置你的订阅链接和 UUID，旧链接已失效。\n你也已在所有设备上退出登录。\n" +
		"请重新登录网站，复制新的订阅链接并重新导入客户端。\n\n站点：https://panel.example.com/subscription"
	if len(calls) != 1 || calls[0].ChatID() != "777" || calls[0].Text() != want {
		t.Fatalf("sendMessage = %+v", calls)
	}
	cur := e.reload(t, bound.ID)
	noSecrets(t, "telegram", []model.User{*bound, cur}, calls[0].Text())
	if n := len(resetMails(t, e.a)); n != 2 {
		t.Fatalf("mails = %d, want one per user", n)
	}

	// telegram_user_notify off: the mail still goes out, Telegram stays silent.
	srv.Reset()
	if err := e.a.Settings.Set(map[string]any{telegram.KeyUserNotify: false}); err != nil {
		t.Fatal(err)
	}
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPost, usersPath(bound.ID, "/reset-secret"), nil, e.adminTok), http.StatusOK)
	drain()
	if calls := srv.Calls("sendMessage"); len(calls) != 0 {
		t.Fatalf("telegram_user_notify=false still sent %+v", calls)
	}
	if n := len(resetMails(t, e.a)); n != 3 {
		t.Fatalf("mails = %d", n)
	}

	// Telegram failing: the reset still succeeds; the failure is logged at WARN by the queue.
	if err := e.a.Settings.Set(map[string]any{telegram.KeyUserNotify: true}); err != nil {
		t.Fatal(err)
	}
	srv.Respond("sendMessage", func(telegramtest.Call) (int, string) {
		return telegramtest.Fail(http.StatusInternalServerError, "down")
	})
	before := e.reload(t, bound.ID)
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPost, usersPath(bound.ID, "/reset-secret"), nil, e.adminTok), http.StatusOK)
	if after := e.reload(t, bound.ID); after.Token == before.Token {
		t.Fatal("the token was not reset")
	}
	drain()
	if !warned(e.a, "notification failed", string(notify.TypeSubscriptionReset)) {
		t.Fatal("a failed Telegram delivery must be logged at WARN")
	}
}

// warned reports whether a WARN entry msg mentions s in one of its attributes.
func warned(a *app.App, msg, s string) bool {
	for _, e := range a.Logs.Entries(slog.LevelWarn, 0) {
		if e.Msg == msg && strings.Contains(fmt.Sprint(e.Attrs), s) {
			return true
		}
	}
	return false
}

// A mail that cannot be queued is logged and the reset (and the Telegram event) still succeed.
func TestAdminResetSecretMailFailure(t *testing.T) {
	e := newOpsEnv(t)
	rec := notifytest.Attach(t, e.a)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.a.Mailer.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	old := e.reload(t, e.user.ID)
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPost, usersPath(e.user.ID, "/reset-secret"), nil, e.adminTok), http.StatusOK)
	if cur := e.reload(t, e.user.ID); cur.Token == old.Token || cur.UUID == old.UUID {
		t.Fatalf("not reset: %+v", cur)
	}
	if !warned(e.a, "subscription reset: mail not queued", mail.ErrQueueClosed.Error()) {
		t.Fatalf("no WARN for the mail: %+v", e.a.Logs.Entries(slog.LevelWarn, 0))
	}
	if n := len(rec.UserEvents(t, e.user.ID, notify.TypeSubscriptionReset)); n != 1 {
		t.Fatalf("telegram events = %d", n)
	}
}

// PATCH /admin/users/:id cannot change the token or uuid (unknown fields are ignored), so no edit sends the notice,
// not even one that ends the user's sessions (password).
func TestAdminUserUpdateDoesNotNotifyReset(t *testing.T) {
	e := newOpsEnv(t)
	rec := notifytest.Attach(t, e.a)
	old := e.reload(t, e.user.ID)
	body := map[string]any{
		"uuid": "00000000-0000-4000-8000-000000000000", "token": "0123456789abcdef0123456789abcdef",
		"remarks": "edited", "password": "another-password-1", "email": "renamed@example.com",
	}
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPatch, usersPath(e.user.ID, ""), body, e.adminTok), http.StatusOK)
	cur := e.reload(t, e.user.ID)
	if cur.UUID != old.UUID || cur.Token != old.Token || cur.Remarks != "edited" || cur.Email != "renamed@example.com" {
		t.Fatalf("update: %+v -> %+v", old, cur)
	}
	if mails := resetMails(t, e.a); len(mails) != 0 {
		t.Fatalf("mails = %+v", mails)
	}
	for _, d := range rec.Deliveries(t) {
		if d.Event.Type == notify.TypeSubscriptionReset {
			t.Fatalf("unexpected delivery %+v", d)
		}
	}
}
