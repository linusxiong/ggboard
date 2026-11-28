package admin_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
	"github.com/linusxiong/ggboard/internal/notify/telegram/telegramtest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const mailPath = "/api/v1/admin/users/mail"

func TestAdminUsersMailMarkdown(t *testing.T) {
	e := newOpsEnv(t)
	testutil.CreateUser(t, e.a, "md@example.com")
	rec := testutil.Do(t, e.r, http.MethodPost, mailPath, map[string]any{
		"email": "md@", "subject": "Maintenance", "markdown": true,
		"content": "Tonight **23:00**.\n\n- one\n- two\n\n[Status page](https://status.example.com)",
	}, e.adminTok)
	if out := testutil.Decode[map[string]int](t, rec); out["queued"] != 1 || out["telegram"] != 0 {
		t.Fatalf("result = %v", out)
	}
	msgs := testutil.Mails(t, e.a)
	if len(msgs) != 1 {
		t.Fatalf("mails = %d", len(msgs))
	}
	m := msgs[0]
	if m.Template != mail.TplNotify || !strings.Contains(m.HTML, "<strong") || !strings.Contains(m.HTML, "<li") ||
		!strings.Contains(m.HTML, `href="https://status.example.com"`) || strings.Contains(m.HTML, "**") {
		t.Fatalf("html = %s", m.HTML)
	}
	if strings.Contains(m.Text, "**") || !strings.Contains(m.Text, "Tonight 23:00.") || !strings.Contains(m.Text, "https://status.example.com") {
		t.Fatalf("text = %q", m.Text)
	}
}

func TestAdminUsersMailTelegram(t *testing.T) {
	e := newOpsEnv(t)
	body := map[string]any{"email": "tg", "subject": "公告", "content": "**今晚** 维护", "markdown": true, "send_telegram": true}

	// Without the bot: a field error, in the request's language.
	rec := testutil.DoWithHeaders(t, e.r, http.MethodPost, mailPath, body, e.adminTok, map[string]string{"Accept-Language": "zh-CN"})
	testutil.ExpectStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "未配置 Telegram 机器人") {
		t.Fatalf("no bot = %s", rec.Body.String())
	}

	srv := telegramtest.Enable(t, e.a)
	bound := testutil.CreateUser(t, e.a, "tg-bound@example.com")
	testutil.CreateUser(t, e.a, "tg-unbound@example.com")
	if err := e.a.DB.Model(&model.User{}).Where("id = ?", bound.ID).Update("telegram_id", 4242).Error; err != nil {
		t.Fatal(err)
	}

	// E-mail and Telegram: both unbound and bound users get the mail, only the bound one the message.
	rec = testutil.Do(t, e.r, http.MethodPost, mailPath, body, e.adminTok)
	if out := testutil.Decode[map[string]int](t, rec); out["queued"] != 2 || out["telegram"] != 1 {
		t.Fatalf("result = %v", out)
	}
	drainNotify(t, e)
	calls := srv.Calls("sendMessage")
	if len(calls) != 1 || calls[0].ChatID() != "4242" || calls[0].Text() != "<b>公告</b>\n\n<b>今晚</b> 维护" {
		t.Fatalf("telegram calls = %+v", calls)
	}
	if len(testutil.Mails(t, e.a)) != 2 {
		t.Fatal("mails not sent")
	}

	// Telegram only.
	srv.Reset()
	only := map[string]any{"email": "tg", "subject": "s", "content": "plain <text>", "send_email": false, "send_telegram": true}
	rec = testutil.Do(t, e.r, http.MethodPost, mailPath, only, e.adminTok)
	if out := testutil.Decode[map[string]int](t, rec); out["queued"] != 0 || out["telegram"] != 1 {
		t.Fatalf("telegram only = %v", out)
	}
	drainNotify(t, e)
	if calls := srv.Calls("sendMessage"); len(calls) != 1 || calls[0].Text() != "<b>s</b>\n\nplain &lt;text&gt;" {
		t.Fatalf("plain telegram = %+v", calls)
	}

	// No channel at all, or user notifications switched off.
	none := map[string]any{"subject": "s", "content": "c", "send_email": false}
	rec = testutil.DoWithHeaders(t, e.r, http.MethodPost, mailPath, none, e.adminTok, map[string]string{"Accept-Language": "zh-CN"})
	testutil.ExpectStatus(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "请至少选择邮件或 Telegram 其中一种") {
		t.Fatalf("no channel = %s", rec.Body.String())
	}
	if err := e.a.Settings.Set(map[string]any{telegram.KeyUserNotify: false}); err != nil {
		t.Fatal(err)
	}
	e.expect(t, http.MethodPost, mailPath, body, e.adminTok, http.StatusBadRequest)
}

func drainNotify(t *testing.T, e *opsEnv) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := notify.Wait(ctx, e.a); err != nil {
		t.Fatal(err)
	}
}
