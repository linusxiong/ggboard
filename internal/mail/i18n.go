package mail

import (
	"fmt"
	"strings"
)

// NormalizeLocale maps any zh* locale to zh-CN and everything else to en.
func NormalizeLocale(l string) string {
	if strings.HasPrefix(strings.ToLower(l), "zh") {
		return "zh-CN"
	}
	return "en"
}

var dictionaries = map[string]map[string]string{
	"zh-CN": {
		"footer.auto":        "此邮件由系统自动发送，请勿直接回复。",
		"button.open":        "打开面板",
		"button.renew":       "立即续费",
		"button.view_order":  "查看订单",
		"button.view_ticket": "查看工单",

		"verify_code.subject": "【{{.AppName}}】邮箱验证码",
		"verify_code.title":   "邮箱验证码",
		"verify_code.body":    "您正在进行邮箱验证，验证码如下，%v 分钟内有效：",
		"verify_code.ignore":  "如果这不是您本人的操作，请忽略此邮件。",

		"remind_expire.subject": "【{{.AppName}}】您的服务即将到期",
		"remind_expire.title":   "服务即将到期",
		"remind_expire.body":    "您的服务将于 %v 到期。为避免服务中断，请及时续费。",

		"remind_traffic.subject": "【{{.AppName}}】流量使用提醒",
		"remind_traffic.title":   "流量即将用尽",
		"remind_traffic.body":    "本周期您已使用 %v%% 的流量（%v / %v）。",

		"order_paid.subject": "【{{.AppName}}】订单支付成功",
		"order_paid.title":   "支付成功",
		"order_paid.body":    "感谢您的购买，订单已处理完成。",
		"order.trade_no":     "订单号",
		"order.plan":         "套餐",
		"order.period":       "周期",
		"order.amount":       "金额",

		"ticket_reply.subject": "【{{.AppName}}】您的工单有新回复",
		"ticket_reply.title":   "工单有新回复",
		"ticket_reply.body":    "您的工单「%v」收到了新的回复：",

		"subscription_reset.subject":    "【{{.AppName}}】系统已重置您的订阅信息",
		"subscription_reset.title":      "订阅信息已重置",
		"subscription_reset.link":       "系统已重置您的订阅链接，旧的订阅链接已失效。",
		"subscription_reset.link_uuid":  "系统已重置您的订阅链接和 UUID，旧的订阅链接已失效。",
		"subscription_reset.uuid":       "系统已重置您的 UUID，客户端中现有的节点配置已失效。",
		"subscription_reset.signed_out": "您也已在所有设备上退出登录。",
		"subscription_reset.action":     "请重新登录网站，在“订阅”页面复制新的订阅链接，并重新导入到您的客户端中。",
		"button.subscription":           "前往订阅页面",

		"notify.subject": "{{if .Subject}}{{.Subject}}{{else}}【{{.AppName}}】通知{{end}}",
	},
	"en": {
		"footer.auto":        "This email was sent automatically. Please do not reply.",
		"button.open":        "Open dashboard",
		"button.renew":       "Renew now",
		"button.view_order":  "View order",
		"button.view_ticket": "View ticket",

		"verify_code.subject": "{{.AppName}} verification code",
		"verify_code.title":   "Verify your email",
		"verify_code.body":    "Use the code below to verify your email. It expires in %v minutes.",
		"verify_code.ignore":  "If you did not request this, you can safely ignore this email.",

		"remind_expire.subject": "Your {{.AppName}} subscription expires soon",
		"remind_expire.title":   "Subscription expiring soon",
		"remind_expire.body":    "Your subscription expires on %v. Renew now to avoid interruption.",

		"remind_traffic.subject": "{{.AppName}} data usage alert",
		"remind_traffic.title":   "You're running low on data",
		"remind_traffic.body":    "You have used %v%% of your data this cycle (%v of %v).",

		"order_paid.subject": "{{.AppName}} payment received",
		"order_paid.title":   "Payment received",
		"order_paid.body":    "Thank you for your purchase. Your order is complete.",
		"order.trade_no":     "Order number",
		"order.plan":         "Plan",
		"order.period":       "Period",
		"order.amount":       "Amount",

		"ticket_reply.subject": "New reply to your {{.AppName}} ticket",
		"ticket_reply.title":   "Your ticket has a new reply",
		"ticket_reply.body":    "Your ticket \"%v\" received a new reply:",

		"subscription_reset.subject":    "Your {{.AppName}} subscription was reset by the administrator",
		"subscription_reset.title":      "Your subscription was reset",
		"subscription_reset.link":       "The administrator reset your subscription link, so the old link no longer works.",
		"subscription_reset.link_uuid":  "The administrator reset your subscription link and UUID, so the old link no longer works.",
		"subscription_reset.uuid":       "The administrator reset your UUID, so the node settings in your clients no longer work.",
		"subscription_reset.signed_out": "You have also been signed out on all devices.",
		"subscription_reset.action":     "Sign in to the website again, copy the new subscription link from the Subscription page and import it into your clients.",
		"button.subscription":           "Open subscription page",

		"notify.subject": "{{if .Subject}}{{.Subject}}{{else}}{{.AppName}} notification{{end}}",
	},
}

func lookup(locale, key string) string {
	if s, ok := dictionaries[locale][key]; ok {
		return s
	}
	if s, ok := dictionaries["en"][key]; ok {
		return s
	}
	return key
}

func translator(locale string) func(string, ...any) string {
	return func(key string, args ...any) string {
		s := lookup(locale, key)
		if len(args) > 0 {
			return fmt.Sprintf(s, args...)
		}
		return s
	}
}
