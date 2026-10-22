package telegram_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
	"github.com/linusxiong/ggboard/internal/notify/telegram/telegramtest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func newApp(t *testing.T, settings ...map[string]any) (*app.App, *telegramtest.Server) {
	t.Helper()
	a := testutil.NewTestApp(t)
	srv := telegramtest.Enable(t, a, settings...)
	if _, err := notify.Start(a, notify.Options{Backoff: func(int) time.Duration { return time.Millisecond }}); err != nil {
		t.Fatal(err)
	}
	return a, srv
}

func drain(t *testing.T, a *app.App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := notify.Wait(ctx, a); err != nil {
		t.Fatal(err)
	}
}

func TestRenderEscapesHTML(t *testing.T) {
	ev := notify.Event{
		Type: notify.TypeTicketNew, Title: "🎫 新工单 <b>", Body: `x < y && "z" > 'w'`,
		Fields: []notify.Field{{Key: "subject", Value: "<script>alert(1)</script>"}, {Key: "trade_no", Value: "A&B"}, {Key: "unknown", Value: "v"}},
	}
	got := telegram.Render(ev, "zh-CN")
	want := "<b>🎫 新工单 &lt;b&gt;</b>\nx &lt; y &amp;&amp; &#34;z&#34; &gt; &#39;w&#39;\n\n主题：&lt;script&gt;alert(1)&lt;/script&gt;\n订单号：<code>A&amp;B</code>\nunknown：v"
	if got != want {
		t.Fatalf("Render =\n%s\nwant\n%s", got, want)
	}
	if en := telegram.Render(notify.Event{Title: "T", Fields: []notify.Field{{Key: "email", Value: "a"}}}, "en"); en != "<b>T</b>\n\nEmail: a" {
		t.Fatalf("en = %q", en)
	}
	// Oversized events stay under Telegram's 4096-character limit.
	big := notify.Event{Title: "T", Body: strings.Repeat("b", 5000)}
	for i := range 40 {
		big.Fields = append(big.Fields, notify.Field{Key: fmt.Sprintf("k%d", i), Value: strings.Repeat("v", 256)})
	}
	if n := len([]rune(telegram.Render(big, "en"))); n > 4096 {
		t.Fatalf("rendered %d runes", n)
	}
}

func TestDriverActiveOnlyWithToken(t *testing.T) {
	a := testutil.NewTestApp(t)
	if telegram.Enabled(a) || notify.Enabled(a) {
		t.Fatal("no token: driver must be off")
	}
	telegramtest.Enable(t, a)
	if !telegram.Enabled(a) || fmt.Sprint(notify.ActiveNames(a)) != "[telegram]" {
		t.Fatalf("drivers = %v", notify.ActiveNames(a))
	}
}

func TestAdminEventsFilteredAndSent(t *testing.T) {
	a, srv := newApp(t, map[string]any{telegram.KeyAdminChats: []string{"111", "-100222", "@ops_channel"}})
	notify.Emit(a, notify.OrderNew(a, notify.Order{Email: "a@example.com", TradeNo: "T1"})) // not in the default events
	notify.Emit(a, notify.OrderPaid(a, notify.Order{UserID: 999, Email: "<b>@example.com", Plan: "Pro", Period: model.PeriodMonthly, Amount: 1000, TradeNo: "T2"}))
	drain(t, a)
	calls := srv.Calls("sendMessage")
	if len(calls) != 3 {
		t.Fatalf("sendMessage calls = %d", len(calls))
	}
	var chats []string
	for _, c := range calls {
		chats = append(chats, c.ChatID())
		if c.Token != telegramtest.Token || c.Params["parse_mode"] != "HTML" {
			t.Fatalf("call = %+v", c)
		}
		if !strings.Contains(c.Text(), "<b>💰 订单已支付</b>") || !strings.Contains(c.Text(), "邮箱：&lt;b&gt;@example.com") ||
			!strings.Contains(c.Text(), "订单号：<code>T2</code>") {
			t.Fatalf("text = %q", c.Text())
		}
	}
	if fmt.Sprint(chats) != "[111 -100222 @ops_channel]" {
		t.Fatalf("chats = %v", chats)
	}
	if _, ok := calls[2].Params["chat_id"].(string); !ok {
		t.Fatal("@channel ids are sent as strings")
	}
	if _, ok := calls[0].Params["chat_id"].(float64); !ok {
		t.Fatal("numeric chat ids are sent as numbers")
	}
	if st := notify.Stats(a); st.Sent != 1 || st.Failed != 0 {
		t.Fatalf("stats = %+v (order_new skipped, order_paid sent once)", st)
	}

	// Subscribing to order_new delivers it.
	srv.Reset()
	if err := a.Settings.Set(map[string]any{telegram.KeyEvents: []string{"order_new"}}); err != nil {
		t.Fatal(err)
	}
	notify.Emit(a, notify.OrderNew(a, notify.Order{Email: "a@example.com", TradeNo: "T3"}))
	drain(t, a)
	if n := len(srv.Calls("sendMessage")); n != 3 {
		t.Fatalf("order_new deliveries = %d", n)
	}
}

func TestNoAdminChatsSkips(t *testing.T) {
	a, srv := newApp(t)
	notify.Emit(a, notify.AdminLogin(a, notify.Login{Email: "root@example.com", IsAdmin: true, Method: notify.MethodPassword}))
	drain(t, a)
	if len(srv.Calls("sendMessage")) != 0 {
		t.Fatal("no admin chat: nothing to send")
	}
	if st := notify.Stats(a); st.Sent != 0 || st.Failed != 0 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestRateLimit429HonoursRetryAfter(t *testing.T) {
	a, srv := newApp(t, map[string]any{telegram.KeyAdminChats: []string{"1", "2"}})
	var n atomic.Int32
	srv.Respond("sendMessage", func(c telegramtest.Call) (int, string) {
		if c.ChatID() == "2" && n.Add(1) == 1 {
			return http.StatusTooManyRequests, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`
		}
		return telegramtest.OK(`{"message_id":1}`)
	})
	start := time.Now()
	notify.Emit(a, notify.New(a, notify.TypeWithdraw, "", notify.F("user", "u@example.com")))
	drain(t, a)
	if d := time.Since(start); d < time.Second {
		t.Fatalf("retried after %s, before retry_after (1 s)", d)
	}
	var chats []string
	for _, c := range srv.Calls("sendMessage") {
		chats = append(chats, c.ChatID())
	}
	if fmt.Sprint(chats) != "[1 2 2]" {
		t.Fatalf("calls = %v: chat 1 must not get the event twice", chats)
	}
	if st := notify.Stats(a); st.Sent != 1 || st.Failed != 0 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestClientErrors(t *testing.T) {
	srv := telegramtest.NewServer(t)
	srv.Respond("sendMessage", func(telegramtest.Call) (int, string) {
		return http.StatusTooManyRequests, `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":3}}`
	})
	c := telegram.NewClient(telegramtest.Token, srv.URL)
	err := c.SendMessage(context.Background(), "1", "hi")
	var ae *telegram.APIError
	if !errors.As(err, &ae) || ae.Code != 429 || ae.RetryAfter() != 3*time.Second || !ae.Temporary() {
		t.Fatalf("429 = %#v", err)
	}
	var ra notify.RetryAfterError
	if !errors.As(err, &ra) {
		t.Fatal("APIError must satisfy notify.RetryAfterError")
	}

	srv.Respond("sendMessage", func(telegramtest.Call) (int, string) { return telegramtest.Fail(400, "Bad Request: chat not found") })
	err = c.SendMessage(context.Background(), "1", "hi")
	if !errors.As(err, &ae) || ae.Temporary() || !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("400 = %v", err)
	}
	srv.Respond("sendMessage", func(telegramtest.Call) (int, string) { return http.StatusBadGateway, "<html>bad gateway</html>" })
	if err = c.SendMessage(context.Background(), "1", "hi"); !errors.As(err, &ae) || !ae.Temporary() {
		t.Fatalf("502 = %v", err)
	}
	if me, err := c.GetMe(context.Background()); err != nil || me.Username != telegramtest.BotUsername {
		t.Fatalf("getMe = %+v, %v", me, err)
	}

	// Transport errors never contain the token (it is part of the request URL).
	dead := telegram.NewClient(telegramtest.Token, "http://127.0.0.1:1")
	err = dead.SendMessage(context.Background(), "1", "hi")
	if err == nil || strings.Contains(err.Error(), telegramtest.Token) || strings.Contains(err.Error(), "TEST-fake") {
		t.Fatalf("transport error = %v", err)
	}
}

func TestPermanentErrorsAreNotRetriedAndNotLeaked(t *testing.T) {
	a, srv := newApp(t, map[string]any{telegram.KeyAdminChats: []string{"1"}})
	srv.Respond("sendMessage", func(telegramtest.Call) (int, string) {
		return telegramtest.Fail(403, "Forbidden: bot was blocked by the user")
	})
	notify.Emit(a, notify.New(a, notify.TypeWithdraw, ""))
	drain(t, a)
	if n := len(srv.Calls("sendMessage")); n != 1 {
		t.Fatalf("a 403 must not be retried: %d calls", n)
	}
	if st := notify.Stats(a); st.Failed != 1 {
		t.Fatalf("stats = %+v", st)
	}
	// Unreachable API: retried, then failed; the token and the webhook secret never reach the logs.
	telegram.SetAPIBase(a, "http://127.0.0.1:1")
	notify.Emit(a, notify.New(a, notify.TypeWithdraw, ""))
	drain(t, a)
	secret := a.Settings.String(telegram.KeyWebhookSecret)
	for _, e := range a.Logs.Entries(-8, 0) {
		line := fmt.Sprint(e.Msg, e.Attrs)
		if strings.Contains(line, telegramtest.Token) || strings.Contains(line, secret) {
			t.Fatalf("secret in log: %s", line)
		}
	}
	if st := notify.Stats(a); st.Failed != 2 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestUserNotifications(t *testing.T) {
	a, srv := newApp(t, map[string]any{telegram.KeyAdminChats: []string{"500"}})
	u := testutil.CreateUser(t, a, "bound@example.com")
	setTelegram(t, a, u.ID, 777)
	admin := testutil.CreateAdmin(t, a, "root@example.com")
	setTelegram(t, a, admin.ID, 500) // the admin's own chat is also an admin chat

	notify.Emit(a, notify.TicketReply(a, notify.Ticket{UserID: u.ID, Subject: "Help", Message: "Done"})) // not subscribed for admins
	notify.Emit(a, notify.OrderPaid(a, notify.Order{UserID: admin.ID, TradeNo: "T1"}))                   // admin chat 500 + user chat 500
	notify.Emit(a, notify.RemindExpire(a, u.ID, "2026-10-23 10:00"))
	drain(t, a)
	var got []string
	for _, c := range srv.Calls("sendMessage") {
		got = append(got, c.ChatID())
	}
	if fmt.Sprint(got) != "[777 500 777]" {
		t.Fatalf("chats = %v (500 must get order_paid once)", got)
	}

	// telegram_user_notify off: only administrator chats.
	srv.Reset()
	if err := a.Settings.Set(map[string]any{telegram.KeyUserNotify: false}); err != nil {
		t.Fatal(err)
	}
	notify.Emit(a, notify.RemindExpire(a, u.ID, "2026-10-23 10:00"))
	drain(t, a)
	if len(srv.Calls("sendMessage")) != 0 {
		t.Fatal("telegram_user_notify=false must silence user messages")
	}
	if notify.Reachable(a, &model.User{TelegramID: ptr(int64(777))}) {
		t.Fatal("Reachable with user notifications off")
	}
}

func setTelegram(t *testing.T, a *app.App, id uint64, chat int64) {
	t.Helper()
	if err := a.DB.Model(&model.User{}).Where("id = ?", id).Update("telegram_id", chat).Error; err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }
