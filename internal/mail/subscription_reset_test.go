package mail

import (
	"strings"
	"testing"
)

// The built-in subscription_reset mail picks its sentences from TokenReset / UUIDReset / SignedOut and links the
// subscription page only when the caller passes one (no app_url → no button).
func TestSubscriptionResetTemplate(t *testing.T) {
	r := customRenderer(t)
	full := Data{"TokenReset": true, "UUIDReset": true, "SignedOut": true, "URL": "https://panel.example.com/subscription"}

	msg, err := r.Render(TplSubscriptionReset, customBrand, full)
	if err != nil {
		t.Fatal(err)
	}
	wantText := "Example Panel\n\n订阅信息已重置\n\n系统已重置您的订阅链接和 UUID，旧的订阅链接已失效。\n\n您也已在所有设备上退出登录。\n\n" +
		"请重新登录网站，在“订阅”页面复制新的订阅链接，并重新导入到您的客户端中。\n\n前往订阅页面: https://panel.example.com/subscription\n\n" +
		"--\n此邮件由系统自动发送，请勿直接回复。\nhttps://panel.example.com\n"
	if msg.Subject != "【Example Panel】系统已重置您的订阅信息" || msg.Text != wantText || msg.Template != TplSubscriptionReset {
		t.Fatalf("zh message:\n%q\n%q", msg.Subject, msg.Text)
	}
	for _, want := range []string{">订阅信息已重置</h1>", ">系统已重置您的订阅链接和 UUID，旧的订阅链接已失效。</p>", ">您也已在所有设备上退出登录。</p>",
		`<a href="https://panel.example.com/subscription" style="display:inline-block;`, ">前往订阅页面</a>"} {
		if !strings.Contains(msg.HTML, want) {
			t.Errorf("zh HTML lacks %q", want)
		}
	}

	// Token only, no sign-out, no URL and no app_url: the link sentence alone and no button.
	msg, err = r.Render(TplSubscriptionReset, Brand{AppName: "Example Panel", Locale: "zh-CN"}, Data{"TokenReset": true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg.Text, "\n\n系统已重置您的订阅链接，旧的订阅链接已失效。\n\n请重新登录网站") || strings.Contains(msg.Text, "退出登录") ||
		strings.Contains(msg.Text, "前往订阅页面") || strings.Contains(msg.HTML, "前往订阅页面") || strings.Contains(msg.HTML, "<a href") {
		t.Fatalf("token only:\n%s\n%s", msg.Text, msg.HTML)
	}
	msg, _ = r.Render(TplSubscriptionReset, customBrand, Data{"UUIDReset": true})
	if !strings.Contains(msg.Text, "系统已重置您的 UUID，客户端中现有的节点配置已失效。") || strings.Contains(msg.Text, "订阅链接和 UUID") {
		t.Fatalf("uuid only:\n%s", msg.Text)
	}

	msg, err = r.Render(TplSubscriptionReset, brandIn("en"), full)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Subject != "Your Example Panel subscription was reset by the administrator" ||
		!strings.Contains(msg.Text, "\n\nYour subscription was reset\n\nThe administrator reset your subscription link and UUID, so the old link no longer works.\n\n"+
			"You have also been signed out on all devices.\n\nSign in to the website again, copy the new subscription link from the Subscription page and import it into your clients.\n\n"+
			"Open subscription page: https://panel.example.com/subscription\n") {
		t.Fatalf("en message:\n%q\n%s", msg.Subject, msg.Text)
	}
}
