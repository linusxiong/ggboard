package subscription

import (
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// LeakHook is called once per token and 24 hours when a subscription token was fetched from more than
// subscribe_leak_max_ips distinct IPs (IPv6 counted per /64) within 24 hours (internal/subscription/guard).
// With subscribe_leak_action = "rotate" the token is already rotated when the hook runs, so u carries the new
// token. The default logs at WARN and mails the user (template notify); an integrator can replace it, for
// example to also notify the admins on Telegram, and may call DefaultLeakHook from the replacement. nil
// disables the notification (the rotation still happens). The hook runs on the flush goroutine: queue slow
// work instead of doing it inline.
var LeakHook = DefaultLeakHook

// LeakActionRotate is the subscribe_leak_action value that rotates the token.
const LeakActionRotate = "rotate"

// DefaultLeakHook logs the event and mails the user: a warning with advice to reset the link, or (after a
// rotation) the new subscription link.
func DefaultLeakHook(a *app.App, u *model.User, distinct int) {
	rotated := a.Settings.String("subscribe_leak_action") == LeakActionRotate
	maxIPs := a.Settings.Int("subscribe_leak_max_ips")
	a.Log.Warn("subscription leak suspected", "user_id", u.ID, "distinct_ips", distinct, "max_ips", maxIPs,
		"token_rotated", rotated)
	if a.Mailer == nil {
		return
	}
	data := leakMail(a, u, distinct, maxIPs, rotated)
	if err := a.Mailer.SendTemplate(u.Email, mail.TplNotify, data); err != nil {
		a.Log.Warn("subscription leak: mail not queued", "user_id", u.ID, "err", err)
	}
}

func leakMail(a *app.App, u *model.User, distinct, maxIPs int, rotated bool) mail.Data {
	d, m := strconv.Itoa(distinct), strconv.Itoa(maxIPs)
	page := strings.TrimRight(a.Settings.String("app_url"), "/")
	if page != "" {
		page += "/subscription"
	}
	var subject string
	var parts []string
	if a.Settings.String("default_locale") == "en" {
		subject = "Your subscription link may have leaked"
		parts = append(parts, "In the last 24 hours your subscription link was fetched from "+d+
			" different IP addresses (IPv6 counted per /64), more than the limit of "+m+".")
		if rotated {
			parts = append(parts, "To protect your account the link was reset and the old one no longer works. "+
				"Update your clients with the new link:", usersvc.New(a).SubscribeURL(u))
		} else {
			parts = append(parts, "If these are not your devices, reset your subscription link in the user center "+
				"and only share it with your own clients.")
		}
	} else {
		subject = "订阅链接疑似泄露"
		parts = append(parts, "过去 24 小时内，你的订阅链接被 "+d+" 个不同的 IP 地址获取（IPv6 按 /64 计算），超过了 "+m+" 个的上限。")
		if rotated {
			parts = append(parts, "为保护你的账户，订阅链接已自动重置，旧链接已失效。请在客户端中更新为新的订阅链接：",
				usersvc.New(a).SubscribeURL(u))
		} else {
			parts = append(parts, "如果这些不是你自己的设备，请在用户中心重置订阅链接，并且只在自己的客户端中使用。")
		}
	}
	data := mail.Data{"Subject": subject, "Content": strings.Join(parts, "\n\n")}
	if page != "" {
		data["URL"] = page
	}
	return data
}
