package notify_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/settings"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestSubscriptionResetBuilder(t *testing.T) {
	a := testutil.NewTestApp(t) // default_locale zh-CN
	ev := notify.SubscriptionReset(a, notify.Reset{
		UserID: 7, Token: true, UUID: true, SignedOut: true, URL: "https://panel.example.com/subscription",
	})
	want := "系统已重置你的订阅链接和 UUID，旧链接已失效。\n你也已在所有设备上退出登录。\n请重新登录网站，复制新的订阅链接并重新导入客户端。"
	if ev.Type != notify.TypeSubscriptionReset || ev.Title != "🔄 订阅信息已重置" || ev.Body != want || ev.Admin ||
		ev.UserID == nil || *ev.UserID != 7 || ev.At.IsZero() {
		t.Fatalf("event = %+v", ev)
	}
	if len(ev.Fields) != 1 || ev.Field("site") != "https://panel.example.com/subscription" {
		t.Fatalf("fields = %+v", ev.Fields)
	}

	// Token only (no sign-out), uuid only; no app_url → no site field.
	ev = notify.SubscriptionReset(a, notify.Reset{UserID: 7, Token: true})
	if ev.Body != "系统已重置你的订阅链接，旧链接已失效。\n请重新登录网站，复制新的订阅链接并重新导入客户端。" || len(ev.Fields) != 0 {
		t.Fatalf("token only = %+v", ev)
	}
	ev = notify.SubscriptionReset(a, notify.Reset{UserID: 7, UUID: true})
	if !strings.HasPrefix(ev.Body, "系统已重置你的 UUID，客户端里现有的节点配置已失效。\n") || strings.Contains(ev.Body, "退出登录") {
		t.Fatalf("uuid only = %+v", ev)
	}

	en := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"default_locale": "en"}))
	ev = notify.SubscriptionReset(en, notify.Reset{UserID: 1, Token: true, UUID: true, SignedOut: true, URL: "https://x.example.com/subscription"})
	if ev.Title != "🔄 Subscription reset" || ev.Body != "Your subscription link and UUID were reset by the administrator; the old link no longer works.\n"+
		"You have also been signed out on all devices.\nSign in to the website again, copy the new subscription link and import it into your clients." {
		t.Fatalf("en = %+v", ev)
	}
}

// subscription_reset is a user-only type: administrators can never subscribe to it in telegram_events.
func TestSubscriptionResetIsUserOnly(t *testing.T) {
	if slices.Contains(notify.AdminTypes, notify.TypeSubscriptionReset) {
		t.Fatal("subscription_reset must not be an administrator event")
	}
	d, ok := settings.Lookup("telegram_events")
	if !ok || slices.Contains(d.Options, string(notify.TypeSubscriptionReset)) {
		t.Fatalf("telegram_events options = %v", d.Options)
	}
	for _, l := range []string{notify.LocaleZH, notify.LocaleEN} {
		if title := notify.Title(l, notify.TypeSubscriptionReset); strings.HasPrefix(title, "title.") {
			t.Errorf("no %s title", l)
		}
	}
	a := testutil.NewTestApp(t)
	if err := a.Settings.Set(map[string]any{"telegram_events": []string{"subscription_reset"}}); err == nil {
		t.Fatal("telegram_events must reject subscription_reset")
	}
}
