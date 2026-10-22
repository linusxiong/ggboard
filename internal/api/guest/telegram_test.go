package guest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/api/guest"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
	"github.com/linusxiong/ggboard/internal/notify/telegram/telegramtest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type tgBotEnv struct {
	a    *app.App
	r    *gin.Engine
	hook string
}

func newTgBotEnv(t *testing.T, settings ...map[string]any) *tgBotEnv {
	t.Helper()
	a := testutil.NewTestApp(t)
	telegramtest.Enable(t, a, settings...)
	return &tgBotEnv{a: a, r: testutil.NewRouterWith(t, a, guest.Register),
		hook: "/api/v1/guest/telegram/webhook/" + a.Settings.String(telegram.KeyWebhookSecret)}
}

func tgMessage(chat int64, chatType, text string) map[string]any {
	return map[string]any{"update_id": 1, "message": map[string]any{
		"message_id": 3, "text": text,
		"from": map[string]any{"id": 4242, "is_bot": false, "first_name": "Dee", "username": "dee"},
		"chat": map[string]any{"id": chat, "type": chatType},
	}}
}

// send posts an update and returns the reply text ("" for an empty 200).
func (e *tgBotEnv) send(t *testing.T, update any) string {
	t.Helper()
	rec := testutil.Do(t, e.r, http.MethodPost, e.hook, update, "")
	return tgReplyText(t, rec)
}

func tgReplyText(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook = %d %s", rec.Code, rec.Body)
	}
	if rec.Body.Len() == 0 {
		return ""
	}
	var rep telegram.Reply
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Method != "sendMessage" || rep.ParseMode != "HTML" || rep.ChatID == 0 {
		t.Fatalf("reply = %+v", rep)
	}
	return rep.Text
}

func TestTelegramWebhookSecret(t *testing.T) {
	e := newTgBotEnv(t)
	upd := tgMessage(4242, "private", "/id")
	for _, path := range []string{"/api/v1/guest/telegram/webhook/wrong-secret-0123456789", "/api/v1/guest/telegram/webhook/x"} {
		if rec := testutil.Do(t, e.r, http.MethodPost, path, upd, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("%s = %d", path, rec.Code)
		}
	}
	rec := testutil.DoWithHeaders(t, e.r, http.MethodPost, e.hook, upd, "", map[string]string{"X-Telegram-Bot-Api-Secret-Token": "wrong"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong secret header = %d", rec.Code)
	}
	secret := e.a.Settings.String(telegram.KeyWebhookSecret)
	rec = testutil.DoWithHeaders(t, e.r, http.MethodPost, e.hook, upd, "", map[string]string{"X-Telegram-Bot-Api-Secret-Token": secret})
	if !strings.Contains(tgReplyText(t, rec), "<code>4242</code>") {
		t.Fatal("matching header accepted")
	}
	// No bot token: the webhook is closed.
	if err := e.a.Settings.Set(map[string]any{telegram.KeyToken: nil}); err != nil {
		t.Fatal(err)
	}
	if rec := testutil.Do(t, e.r, http.MethodPost, e.hook, upd, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("webhook without token = %d", rec.Code)
	}
	// The secret never reaches the access log (route pattern only).
	for _, en := range e.a.Logs.Entries(-8, 0) {
		if b, _ := json.Marshal(en); strings.Contains(string(b), secret) {
			t.Fatalf("secret logged: %s", b)
		}
	}
}

func TestTelegramWebhookCommands(t *testing.T) {
	e := newTgBotEnv(t, map[string]any{telegram.KeyDiscussLink: "https://t.me/example_group"})

	if got := e.send(t, tgMessage(-100500, "supergroup", "/id")); got != "Chat ID：<code>-100500</code>" {
		t.Fatalf("/id in a group = %q", got)
	}
	if got := e.send(t, tgMessage(4242, "private", "/ID")); !strings.Contains(got, "<code>4242</code>") {
		t.Fatalf("/ID = %q", got)
	}
	help := e.send(t, tgMessage(4242, "private", "hello"))
	if !strings.Contains(help, "/start &lt;绑定码&gt;") || !strings.Contains(help, "/unbind") || !strings.Contains(help, "https://t.me/example_group") {
		t.Fatalf("help = %q", help)
	}
	if got := e.send(t, tgMessage(4242, "private", "/start")); got != help {
		t.Fatalf("/start without code = %q", got)
	}
	// Groups: no chatter, no binding.
	if got := e.send(t, tgMessage(-100500, "group", "hello everyone")); got != "" {
		t.Fatalf("group text = %q", got)
	}
	if got := e.send(t, tgMessage(-100500, "group", "/start ABC")); !strings.Contains(got, "私聊") {
		t.Fatalf("group /start = %q", got)
	}
	if got := e.send(t, tgMessage(-100500, "group", "/unknown")); got != "" {
		t.Fatalf("group unknown command = %q", got)
	}
	if got := e.send(t, tgMessage(4242, "private", "/start not-a-code")); !strings.Contains(got, "绑定码无效") {
		t.Fatalf("bad code = %q", got)
	}
	if got := e.send(t, tgMessage(4242, "private", "/unbind")); !strings.Contains(got, "未绑定") {
		t.Fatalf("/unbind unbound = %q", got)
	}
	// Bots and undecodable updates are ignored with an empty 200.
	bot := tgMessage(4242, "private", "/id")
	bot["message"].(map[string]any)["from"].(map[string]any)["is_bot"] = true
	if got := e.send(t, bot); got != "" {
		t.Fatalf("bot message = %q", got)
	}
	if got := e.send(t, "{not json"); got != "" {
		t.Fatalf("malformed = %q", got)
	}
	if got := e.send(t, map[string]any{"update_id": 2}); got != "" {
		t.Fatalf("no message = %q", got)
	}
	// Commands for another bot are ignored once the bot's own username is known.
	if _, err := telegram.BotUsername(context.Background(), e.a); err != nil {
		t.Fatal(err)
	}
	if got := e.send(t, tgMessage(-100500, "group", "/id@other_bot")); got != "" {
		t.Fatalf("/id@other_bot = %q", got)
	}
	if got := e.send(t, tgMessage(-100500, "group", "/id@"+telegramtest.BotUsername)); !strings.Contains(got, "-100500") {
		t.Fatalf("/id@own_bot = %q", got)
	}
}

func TestTelegramWebhookBindMovesChat(t *testing.T) {
	e := newTgBotEnv(t, map[string]any{"default_locale": "en"})
	ctx := context.Background()
	alice := testutil.CreateUser(t, e.a, "alice@example.com")
	bob := testutil.CreateUser(t, e.a, "bob@example.com")

	code, err := telegram.IssueBindCode(ctx, e.a, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.send(t, tgMessage(4242, "private", "/start "+code.Code)); !strings.HasPrefix(got, "Linked to al***@example.com") {
		t.Fatalf("bind = %q", got)
	}
	// The same Telegram account binds bob: alice is unbound (one chat ↔ one account).
	code, err = telegram.IssueBindCode(ctx, e.a, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	e.send(t, tgMessage(4242, "private", "/start "+code.Code))
	if tg := tgLoadUser(t, e.a, alice.ID); tg.TelegramID != nil || tg.TelegramUsername != nil {
		t.Fatalf("alice still bound: %+v", tg)
	}
	if tg := tgLoadUser(t, e.a, bob.ID); tg.TelegramID == nil || *tg.TelegramID != 4242 || tg.TelegramUsername == nil || *tg.TelegramUsername != "dee" {
		t.Fatalf("bob = %+v", tg)
	}
	if got := e.send(t, tgMessage(4242, "private", "/unbind")); !strings.HasPrefix(got, "Unlinked") {
		t.Fatalf("/unbind = %q", got)
	}
	if tg := tgLoadUser(t, e.a, bob.ID); tg.TelegramID != nil {
		t.Fatal("bob still bound after /unbind")
	}
	// A code of a deleted account is refused.
	carol := testutil.CreateUser(t, e.a, "carol@example.com")
	code, err = telegram.IssueBindCode(ctx, e.a, carol.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.a.DB.Delete(&model.User{}, carol.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got := e.send(t, tgMessage(4242, "private", "/start "+code.Code)); !strings.Contains(got, "invalid or expired") {
		t.Fatalf("deleted account = %q", got)
	}
}

func tgLoadUser(t *testing.T, a *app.App, id uint64) model.User {
	t.Helper()
	var u model.User
	if err := a.DB.Select("id", "telegram_id", "telegram_username").First(&u, id).Error; err != nil {
		t.Fatal(err)
	}
	return u
}
