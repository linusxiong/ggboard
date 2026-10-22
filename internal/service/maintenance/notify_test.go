package maintenance_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/telegram/telegramtest"
	"github.com/linusxiong/ggboard/internal/service/maintenance"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Users who bound Telegram get their reminders there too, alongside the mail; with mail reminders off they still
// get the Telegram message.
func TestRemindersReachBoundTelegramUsers(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"default_locale": "en"}))
	srv := telegramtest.Enable(t, a)
	if _, err := notify.Start(a, notify.Options{Backoff: func(int) time.Duration { return time.Millisecond }}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := local(2026, 10, 22, 10, 0)
	mk := func(email string, chat int64, mut map[string]any) *model.User {
		u := testutil.CreateUser(t, a, email, usersvc.CreateOptions{ExpiredAt: ptr(now.Add(6 * time.Hour)), TransferEnable: 100 << 30})
		cols := map[string]any{"remind_expire": true, "remind_traffic": true}
		if chat != 0 {
			cols["telegram_id"] = chat
		}
		for k, v := range mut {
			cols[k] = v
		}
		if err := a.DB.Model(u).Updates(cols).Error; err != nil {
			t.Fatal(err)
		}
		return u
	}
	mk("bound@example.com", 1001, map[string]any{"u": int64(85) << 30})
	mk("plain@example.com", 0, map[string]any{"u": int64(90) << 30})
	mk("optout@example.com", 1003, map[string]any{"remind_expire": false, "remind_traffic": false})

	drain := func() []telegramtest.Call {
		t.Helper()
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := notify.Wait(c, a); err != nil {
			t.Fatal(err)
		}
		calls := srv.Calls("sendMessage")
		srv.Reset()
		return calls
	}

	if n, err := maintenance.ExpireRemind(ctx, a, clock(now)); err != nil || n != 2 {
		t.Fatalf("expire_remind = %d, %v", n, err)
	}
	calls := drain()
	if len(calls) != 1 || calls[0].ChatID() != "1001" || !strings.Contains(calls[0].Text(), "<b>Subscription expires soon</b>") ||
		!strings.Contains(calls[0].Text(), now.Add(6*time.Hour).Format("2006-01-02 15:04")) {
		t.Fatalf("telegram expire reminders = %+v", calls)
	}
	if got := mailsTo(testutil.Mails(t, a), mail.TplRemindExpire); len(got) != 2 {
		t.Fatalf("expire mails = %v", got)
	}

	// Mail reminders off: Telegram-bound users are still reminded, nobody is mailed.
	if err := a.Settings.Set(map[string]any{"remind_mail_enable": false}); err != nil {
		t.Fatal(err)
	}
	if n, err := maintenance.TrafficRemind(ctx, a, clock(now)); err != nil || n != 1 {
		t.Fatalf("traffic_remind = %d, %v", n, err)
	}
	calls = drain()
	if len(calls) != 1 || calls[0].ChatID() != "1001" || !strings.Contains(calls[0].Text(), "You have used 85%") {
		t.Fatalf("telegram traffic reminders = %+v", calls)
	}
	if got := mailsTo(testutil.Mails(t, a), mail.TplRemindTraffic); len(got) != 0 {
		t.Fatalf("traffic mails with remind_mail_enable off = %v", got)
	}
	// Once per 24 h, whatever the channel.
	if n, err := maintenance.TrafficRemind(ctx, a, clock(now.Add(time.Hour))); err != nil || n != 0 {
		t.Fatalf("second run = %d, %v", n, err)
	}
	// No channel at all: nothing.
	if err := a.Settings.Set(map[string]any{"telegram_bot_token": ""}); err != nil {
		t.Fatal(err)
	}
	if n, err := maintenance.ExpireRemind(ctx, a, clock(now.Add(48*time.Hour))); err != nil || n != 0 {
		t.Fatalf("no channel = %d, %v", n, err)
	}
}
