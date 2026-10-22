package user_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/guest"
	"github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
	"github.com/linusxiong/ggboard/internal/notify/telegram/telegramtest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type telegramStatusView struct {
	Enabled     bool    `json:"enabled"`
	Bound       bool    `json:"bound"`
	Username    *string `json:"username"`
	BotUsername string  `json:"bot_username"`
	DiscussLink string  `json:"discuss_link"`
}

func TestTelegramStatusWithoutBot(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"telegram_discuss_link": "https://t.me/example_group"}))
	r := testutil.NewRouterWith(t, a, user.Register)
	u := testutil.CreateUser(t, a, "alice@example.com")
	tok := testutil.Login(t, a, u)

	st := testutil.Decode[telegramStatusView](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/telegram", nil, tok))
	if st.Enabled || st.Bound || st.Username != nil || st.BotUsername != "" || st.DiscussLink != "https://t.me/example_group" {
		t.Fatalf("status = %+v", st)
	}
	rec := testutil.Do(t, r, http.MethodPost, "/api/v1/user/telegram/bind-code", nil, tok)
	if e := testutil.DecodeError(t, rec); e.Status != http.StatusForbidden {
		t.Fatalf("bind-code without a bot = %d", e.Status)
	}
	if rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/telegram", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", rec.Code)
	}
}

// Full flow: bind code → /start <code> through the webhook → status → unbind.
func TestTelegramBindFlow(t *testing.T) {
	a := testutil.NewTestApp(t)
	srv := telegramtest.Enable(t, a)
	r := testutil.NewRouterWith(t, a, user.Register, guest.Register)
	u := testutil.CreateUser(t, a, "alice@example.com")
	tok := testutil.Login(t, a, u)

	code := testutil.Decode[telegram.BindCode](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/telegram/bind-code", nil, tok))
	if len(code.Code) != telegram.BindCodeLength || code.BotUsername != telegramtest.BotUsername ||
		code.DeepLink != "https://t.me/"+telegramtest.BotUsername+"?start="+code.Code {
		t.Fatalf("bind code = %+v", code)
	}
	// getMe is cached for an hour.
	code2 := testutil.Decode[telegram.BindCode](t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/telegram/bind-code", nil, tok))
	if n := len(srv.Calls("getMe")); n != 1 {
		t.Fatalf("getMe calls = %d", n)
	}

	secret := a.Settings.String(telegram.KeyWebhookSecret)
	hook := "/api/v1/guest/telegram/webhook/" + secret
	// Issuing a new code revoked the first one.
	rep := telegramWebhookReply(t, testutil.Do(t, r, http.MethodPost, hook, telegramStartUpdate(9001, "carol", "/start "+code.Code), ""))
	if !strings.Contains(rep.Text, "绑定码无效") {
		t.Fatalf("revoked code reply = %q", rep.Text)
	}
	rec := testutil.Do(t, r, http.MethodPost, hook, telegramStartUpdate(9001, "carol", "/start "+code2.Code), "")
	rep = telegramWebhookReply(t, rec)
	if rep.Method != "sendMessage" || rep.ChatID != 9001 || !strings.HasPrefix(rep.Text, "绑定成功") || !strings.Contains(rep.Text, "al***@example.com") {
		t.Fatalf("reply = %+v", rep)
	}

	st := testutil.Decode[telegramStatusView](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/telegram", nil, tok))
	if !st.Enabled || !st.Bound || st.Username == nil || *st.Username != "carol" || st.BotUsername != telegramtest.BotUsername {
		t.Fatalf("status = %+v", st)
	}
	var got model.User
	if err := a.DB.First(&got, u.ID).Error; err != nil || got.TelegramID == nil || *got.TelegramID != 9001 {
		t.Fatalf("telegram_id = %v, %v", got.TelegramID, err)
	}
	// Codes are single use.
	rep = telegramWebhookReply(t, testutil.Do(t, r, http.MethodPost, hook, telegramStartUpdate(9002, "", "/start "+code2.Code), ""))
	if !strings.Contains(rep.Text, "绑定码无效") {
		t.Fatalf("reused code reply = %q", rep.Text)
	}

	if rec := testutil.Do(t, r, http.MethodDelete, "/api/v1/user/telegram", nil, tok); rec.Code != http.StatusOK {
		t.Fatalf("unbind = %d %s", rec.Code, rec.Body)
	}
	st = testutil.Decode[telegramStatusView](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/telegram", nil, tok))
	if st.Bound || st.Username != nil {
		t.Fatalf("after unbind = %+v", st)
	}
	var after model.User
	if err := a.DB.First(&after, u.ID).Error; err != nil || after.TelegramID != nil || after.TelegramUsername != nil {
		t.Fatalf("after unbind = %+v", after)
	}
}

func TestTelegramBindCodeBotUnreachable(t *testing.T) {
	a := testutil.NewTestApp(t)
	srv := telegramtest.Enable(t, a)
	srv.Respond("getMe", func(telegramtest.Call) (int, string) { return telegramtest.Fail(401, "Unauthorized") })
	r := testutil.NewRouterWith(t, a, user.Register)
	tok := testutil.Login(t, a, testutil.CreateUser(t, a, "alice@example.com"))

	rec := testutil.Do(t, r, http.MethodPost, "/api/v1/user/telegram/bind-code", nil, tok)
	if e := testutil.DecodeError(t, rec); e.Status != http.StatusBadGateway || e.Code != telegram.CodeUnavailable {
		t.Fatalf("bind-code = %+v", e)
	}
	if strings.Contains(rec.Body.String(), telegramtest.Token) {
		t.Fatal("token leaked in the response")
	}
	// The status page does not wait for Telegram again right after a failure.
	before := len(srv.Calls("getMe"))
	st := testutil.Decode[telegramStatusView](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/telegram", nil, tok))
	if !st.Enabled || st.BotUsername != "" || len(srv.Calls("getMe")) != before {
		t.Fatalf("status = %+v, getMe calls %d → %d", st, before, len(srv.Calls("getMe")))
	}
}

func telegramStartUpdate(chat int64, username, text string) map[string]any {
	return map[string]any{
		"update_id": 1,
		"message": map[string]any{
			"message_id": 10,
			"from":       map[string]any{"id": chat, "is_bot": false, "first_name": "C", "username": username},
			"chat":       map[string]any{"id": chat, "type": "private"},
			"text":       text,
		},
	}
}

// webhookReply decodes the webhook answer (a bare sendMessage call, no {"data"} envelope).
func telegramWebhookReply(t *testing.T, rec *httptest.ResponseRecorder) telegram.Reply {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook = %d %s", rec.Code, rec.Body)
	}
	var rep telegram.Reply
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("webhook body %q: %v", rec.Body, err)
	}
	return rep
}
