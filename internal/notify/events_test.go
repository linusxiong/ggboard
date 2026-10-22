package notify_test

import (
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/settings"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func fieldMap(ev notify.Event) map[string]string {
	m := map[string]string{}
	for _, f := range ev.Fields {
		m[f.Key] = f.Value
	}
	return m
}

func TestBuildersZH(t *testing.T) {
	a := testutil.NewTestApp(t) // default_locale zh-CN, currency_symbol ¥
	ev := notify.OrderPaid(a, notify.Order{
		UserID: 7, Email: "a@example.com", Plan: "Pro", Period: model.PeriodQuarterly, Amount: 12345,
		TradeNo: "T1", Gateway: "Alipay", CallbackNo: "CB9",
	})
	f := fieldMap(ev)
	if ev.Type != notify.TypeOrderPaid || ev.Title != "💰 订单已支付" || !ev.Admin || ev.UserID == nil || *ev.UserID != 7 || ev.At.IsZero() {
		t.Fatalf("event = %+v", ev)
	}
	if f["email"] != "a@example.com" || f["plan"] != "Pro" || f["period"] != "季付" || f["amount"] != "¥123.45" ||
		f["trade_no"] != "T1" || f["gateway"] != "Alipay" || f["callback_no"] != "CB9" {
		t.Fatalf("fields = %v", f)
	}

	ev = notify.OrderNew(a, notify.Order{Email: "a@example.com", Plan: "Pro", Period: model.PeriodMonthly, Amount: 100, TradeNo: "T2"})
	if ev.UserID != nil || !ev.Admin || ev.Title != "🧾 新订单" || len(ev.Fields) != 5 {
		t.Fatalf("order_new = %+v", ev)
	}
	if _, ok := fieldMap(ev)["gateway"]; ok {
		t.Fatal("empty fields are dropped")
	}

	ev = notify.OrderCancel(a, notify.Order{TradeNo: "T3", Reason: notify.ReasonTimeout})
	if fieldMap(ev)["reason"] != "超时未支付" {
		t.Fatalf("order_cancel = %+v", ev)
	}

	ua := strings.Repeat("Mozilla/5.0 ", 20)
	ev = notify.AdminLogin(a, notify.Login{Email: "root@example.com", IsAdmin: false, Method: notify.MethodBackup, IP: "192.0.2.1", UA: ua,
		At: time.Date(2026, 10, 22, 9, 30, 0, 0, time.Local)})
	f = fieldMap(ev)
	if f["role"] != "客服" || f["method"] != "密码 + 备用码" || f["ip"] != "192.0.2.1" || f["time"] != "2026-10-22 09:30:00" {
		t.Fatalf("admin_login = %v", f)
	}
	if n := len([]rune(f["ua"])); n != notify.MaxUALen || !strings.HasSuffix(f["ua"], "…") {
		t.Fatalf("ua = %q (%d runes)", f["ua"], n)
	}

	ev = notify.TicketNew(a, notify.Ticket{Email: "u@example.com", Subject: "Help", Level: 2, Message: strings.Repeat("长", 500)})
	if f := fieldMap(ev); f["level"] != "高" || f["user"] != "u@example.com" || f["subject"] != "Help" {
		t.Fatalf("ticket_new = %v", f)
	}
	if n := len([]rune(ev.Body)); n != notify.MaxExcerptLen {
		t.Fatalf("excerpt = %d runes", n)
	}

	ev = notify.TicketReply(a, notify.Ticket{UserID: 9, Subject: "Help", Message: "Fixed"})
	if ev.UserID == nil || *ev.UserID != 9 || ev.Body != "Fixed" || ev.Title != "🎫 工单有新回复" {
		t.Fatalf("ticket_reply = %+v", ev)
	}

	ev = notify.Withdraw(a, notify.Withdrawal{Email: "u@example.com", Amount: 5000, Method: "USDT", Account: "TQx1234567890abcdef9aZ"})
	if f := fieldMap(ev); f["account"] != "TQx***9aZ" || f["amount"] != "¥50.00" || f["method"] != "USDT" {
		t.Fatalf("withdraw = %v", f)
	}

	ev = notify.RemindTraffic(a, 3, 85, "85.00 GB", "100.00 GB")
	if ev.Admin || ev.UserID == nil || ev.Body != "本周期已用 85% 流量（85.00 GB / 100.00 GB）。" {
		t.Fatalf("remind_traffic = %+v", ev)
	}
	ev = notify.SubscriptionLeak(a, &model.User{ID: 5, Email: "leak@example.com"}, 12)
	if f := fieldMap(ev); ev.Type != notify.TypeSubscriptionLeak || !ev.Admin || ev.UserID != nil || f["user"] != "leak@example.com" ||
		f["distinct"] != "12" || !strings.Contains(ev.Body, "12 个不同 IP") || ev.Title != "🚨 订阅疑似泄露" {
		t.Fatalf("subscription_leak = %+v", ev)
	}
	if notify.Label("zh-CN", "distinct") != "不同 IP 数" {
		t.Fatal("distinct label")
	}
	ev = notify.RemindExpire(a, 3, "2026-10-23 10:00")
	if ev.Admin || !strings.Contains(ev.Body, "2026-10-23 10:00") {
		t.Fatalf("remind_expire = %+v", ev)
	}
}

func TestBuildersEN(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{
		"default_locale": "en", "currency_symbol": "", "currency": "USD", "app_name": "Panel", "app_url": "https://panel.example.com",
	}))
	ev := notify.OrderNew(a, notify.Order{Email: "a@example.com", Plan: "Pro", Period: model.PeriodMonthly, Amount: 999, TradeNo: "T"})
	if ev.Title != "🧾 New order" || fieldMap(ev)["amount"] != "9.99 USD" || fieldMap(ev)["period"] != "Monthly" {
		t.Fatalf("en order_new = %+v", ev)
	}
	ev = notify.Test(a)
	if ev.Title != "Test message" || fieldMap(ev)["site"] != "Panel · https://panel.example.com" {
		t.Fatalf("test = %+v", ev)
	}
	if notify.Label("en", "trade_no") != "Order" || notify.Label("zh-CN", "trade_no") != "订单号" || notify.Label("en", "custom_key") != "custom_key" {
		t.Fatal("labels")
	}
}

// Every administrator event type has a title in both locales and is a telegram_events option.
func TestTypesAndTexts(t *testing.T) {
	d, ok := settings.Lookup("telegram_events")
	if !ok {
		t.Fatal("telegram_events missing")
	}
	if len(d.Options) != len(notify.AdminTypes) {
		t.Fatalf("options %v vs AdminTypes %v", d.Options, notify.AdminTypes)
	}
	for i, typ := range notify.AdminTypes {
		if d.Options[i] != string(typ) {
			t.Errorf("option %d = %s, want %s", i, d.Options[i], typ)
		}
		for _, l := range []string{notify.LocaleZH, notify.LocaleEN} {
			if title := notify.Title(l, typ); strings.HasPrefix(title, "title.") {
				t.Errorf("%s has no %s title", typ, l)
			}
		}
	}
	if notify.Title("zh-CN", notify.TypeSubscriptionLeak) != "🚨 订阅疑似泄露" || notify.NormalizeLocale("zh-TW") != "zh-CN" || notify.NormalizeLocale("fr") != "en" {
		t.Fatal("subscription_leak title / locale normalization")
	}
}

func TestMaskAndTruncate(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "a": "***", "ab": "***", "abc": "a***c", "abcdef": "a***f", "alice@example.com": "ali***com", " 支付宝账号12345 ": "支付宝***345",
	} {
		if got := notify.Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
	if notify.Truncate("hello", 5) != "hello" || notify.Truncate("hello!", 5) != "hell…" || notify.Truncate("你好世界", 3) != "你好…" {
		t.Fatal("Truncate")
	}
	long := notify.New(testutil.NewTestApp(t), notify.TypeSubscriptionLeak, "", notify.F("note", strings.Repeat("x", 1000)), notify.F("empty", "  "))
	if len(long.Fields) != 1 || len([]rune(long.Fields[0].Value)) != notify.MaxFieldLen || !long.Admin || long.UserID != nil {
		t.Fatalf("New = %+v", long)
	}
	if long.Field("note") == "" || long.Field("missing") != "" {
		t.Fatal("Event.Field")
	}
}
