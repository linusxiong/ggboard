package notify

import (
	"strings"

	"github.com/linusxiong/ggboard/internal/app"
)

// Supported locales.
const (
	LocaleZH = "zh-CN"
	LocaleEN = "en"
)

// NormalizeLocale maps any zh* tag to zh-CN and everything else to en.
func NormalizeLocale(l string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(l)), "zh") {
		return LocaleZH
	}
	return LocaleEN
}

// Locale returns the site's notification locale (setting default_locale).
func Locale(a *app.App) string { return NormalizeLocale(a.Settings.String("default_locale")) }

// texts holds every string of the notifications: event titles ("title.<type>"), field labels ("label.<key>")
// and the remaining phrases. Keep them short; emoji only on the titles that carry one.
//
//nolint:gosec // G101: UI strings; "password"/"passkey" are names of sign-in methods, not credentials
var texts = map[string]map[string]string{
	LocaleZH: {
		"title.order_new":         "🧾 新订单",
		"title.order_paid":        "💰 订单已支付",
		"title.order_cancel":      "🧾 订单已取消",
		"title.admin_login":       "🔐 管理员登录",
		"title.ticket_new":        "🎫 新工单",
		"title.ticket_reply":      "🎫 工单有新回复",
		"title.withdraw":          "💸 提现申请",
		"title.subscription_leak": "🚨 订阅疑似泄露",
		"title.remind_expire":     "订阅即将到期",
		"title.remind_traffic":    "流量即将用尽",
		"title.test":              "测试消息",

		"label.email":        "邮箱",
		"label.user":         "用户",
		"label.plan":         "套餐",
		"label.period":       "周期",
		"label.amount":       "金额",
		"label.trade_no":     "订单号",
		"label.gateway":      "支付方式",
		"label.callback_no":  "流水号",
		"label.reason":       "原因",
		"label.role":         "角色",
		"label.method":       "方式",
		"label.ip":           "IP",
		"label.ua":           "设备",
		"label.time":         "时间",
		"label.subject":      "主题",
		"label.level":        "优先级",
		"label.account":      "账号",
		"label.expired_at":   "到期时间",
		"label.used":         "已用",
		"label.total":        "总量",
		"label.site":         "站点",
		"label.count":        "次数",
		"label.subscription": "订阅",
		"label.distinct":     "不同 IP 数",

		"role.admin": "管理员",
		"role.staff": "客服",

		"method.password": "密码",
		"method.totp":     "密码 + 两步验证",
		"method.backup":   "密码 + 备用码",
		"method.passkey":  "通行密钥",

		"level.0": "低",
		"level.1": "中",
		"level.2": "高",

		"reason.timeout": "超时未支付",
		"reason.admin":   "管理员取消",

		"body.order_paid":        "套餐已生效。",
		"body.admin_login":       "如非本人操作，请立即修改密码。",
		"body.remind_expire":     "你的订阅将于 %s 到期，请及时续费。",
		"body.remind_traffic":    "本周期已用 %d%% 流量（%s / %s）。",
		"body.test":              "Telegram 通知已正确配置。",
		"body.subscription_leak": "该订阅短时间内被 %d 个不同 IP 拉取，可能已被分享。",
	},
	LocaleEN: {
		"title.order_new":         "🧾 New order",
		"title.order_paid":        "💰 Order paid",
		"title.order_cancel":      "🧾 Order cancelled",
		"title.admin_login":       "🔐 Admin sign-in",
		"title.ticket_new":        "🎫 New ticket",
		"title.ticket_reply":      "🎫 New reply to your ticket",
		"title.withdraw":          "💸 Withdrawal request",
		"title.subscription_leak": "🚨 Possible subscription leak",
		"title.remind_expire":     "Subscription expires soon",
		"title.remind_traffic":    "Traffic almost used up",
		"title.test":              "Test message",

		"label.email":        "Email",
		"label.user":         "User",
		"label.plan":         "Plan",
		"label.period":       "Period",
		"label.amount":       "Amount",
		"label.trade_no":     "Order",
		"label.gateway":      "Payment",
		"label.callback_no":  "Transaction",
		"label.reason":       "Reason",
		"label.role":         "Role",
		"label.method":       "Method",
		"label.ip":           "IP",
		"label.ua":           "Device",
		"label.time":         "Time",
		"label.subject":      "Subject",
		"label.level":        "Priority",
		"label.account":      "Account",
		"label.expired_at":   "Expires",
		"label.used":         "Used",
		"label.total":        "Total",
		"label.site":         "Site",
		"label.count":        "Count",
		"label.subscription": "Subscription",
		"label.distinct":     "Distinct IPs",

		"role.admin": "Administrator",
		"role.staff": "Staff",

		"method.password": "Password",
		"method.totp":     "Password + 2FA code",
		"method.backup":   "Password + backup code",
		"method.passkey":  "Passkey",

		"level.0": "Low",
		"level.1": "Medium",
		"level.2": "High",

		"reason.timeout": "Not paid in time",
		"reason.admin":   "Cancelled by an administrator",

		"body.order_paid":        "The plan is now active.",
		"body.admin_login":       "If this wasn't you, change your password now.",
		"body.remind_expire":     "Your subscription expires on %s. Renew to keep using it.",
		"body.remind_traffic":    "You have used %d%% of this period's traffic (%s / %s).",
		"body.test":              "Telegram notifications are set up correctly.",
		"body.subscription_leak": "This subscription was fetched from %d different IPs in a short time; it may be shared.",
	},
}

// T returns the text for key in locale (English, then the key itself, as fallbacks).
func T(locale, key string) string {
	if s, ok := texts[NormalizeLocale(locale)][key]; ok {
		return s
	}
	if s, ok := texts[LocaleEN][key]; ok {
		return s
	}
	return key
}

// Title returns the localized title of an event type.
func Title(locale string, t Type) string { return T(locale, "title."+string(t)) }

// Label returns the localized label of a field key (the key itself when unknown).
func Label(locale, key string) string {
	if s := T(locale, "label."+key); s != "label."+key {
		return s
	}
	return key
}
