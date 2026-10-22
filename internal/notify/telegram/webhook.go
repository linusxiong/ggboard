package telegram

import (
	"context"
	"crypto/subtle"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/notify"
)

// WebhookPath is the webhook route under the API prefix; the last segment is telegram_webhook_secret.
const WebhookPath = "/guest/telegram/webhook/"

// Update is the subset of a Telegram Update the bot handles (text messages).
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

// Message is a Telegram message.
type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
}

// User is a Telegram user.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// Chat is a Telegram chat; Type is private, group, supergroup or channel.
type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

// Reply is a sendMessage call returned as the webhook response body (Telegram performs it).
type Reply struct {
	Method             string          `json:"method"`
	ChatID             int64           `json:"chat_id"`
	Text               string          `json:"text"`
	ParseMode          string          `json:"parse_mode"`
	LinkPreviewOptions map[string]bool `json:"link_preview_options"`
}

func reply(chat int64, htmlText string) *Reply {
	return &Reply{Method: "sendMessage", ChatID: chat, Text: htmlText, ParseMode: "HTML",
		LinkPreviewOptions: map[string]bool{"is_disabled": true}}
}

// ValidSecret compares the webhook path secret with telegram_webhook_secret in constant time. It is false
// while the bot token or the secret is unset.
func ValidSecret(a *app.App, got string) bool {
	want := a.Settings.String(KeyWebhookSecret)
	if want == "" || !Enabled(a) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

var replies = map[string]map[string]string{
	notify.LocaleZH: {
		"bound":      "绑定成功（%s）。订单、工单回复和到期提醒会发送到这里。",
		"bad_code":   "绑定码无效或已过期，请在面板重新获取。",
		"private":    "请在与机器人的私聊中操作。",
		"unbound":    "已解除绑定，此后不再向这里发送通知。",
		"not_bound":  "当前 Telegram 账号未绑定面板账户。",
		"chat_id":    "Chat ID：%s",
		"help":       "可用命令：\n/start <绑定码> — 绑定面板账户（在面板「个人中心 → Telegram」获取）\n/unbind — 解除绑定\n/id — 查看当前 Chat ID",
		"discuss":    "交流群：%s",
		"error":      "暂时无法处理，请稍后再试。",
		"bind_first": "绑定码只能在私聊中使用。",
	},
	notify.LocaleEN: {
		"bound":      "Linked to %s. Orders, ticket replies and reminders will arrive here.",
		"bad_code":   "This code is invalid or expired. Get a new one in the panel.",
		"private":    "Please use this command in a private chat with the bot.",
		"unbound":    "Unlinked. No more notifications will be sent here.",
		"not_bound":  "This Telegram account is not linked to a panel account.",
		"chat_id":    "Chat ID: %s",
		"help":       "Commands:\n/start <code> — link your panel account (get the code under Profile → Telegram)\n/unbind — unlink\n/id — show this chat's ID",
		"discuss":    "Community: %s",
		"error":      "Something went wrong. Please try again later.",
		"bind_first": "Binding codes only work in a private chat.",
	},
}

func text(a *app.App, key string) string { return replies[notify.Locale(a)][key] }

// HandleUpdate runs a bot command and returns the reply to send, or nil:
//
//	/start <code>  bind the account that issued code (private chats)
//	/unbind        unbind every account bound to this chat (private chats)
//	/id            reply with the chat id (any chat: admins use it to fill telegram_admin_chat_ids)
//	/help, other   short help (/help answers in any chat; other text is answered in private chats only)
//
// Commands addressed to another bot (/id@other_bot) and messages from bots are ignored.
func HandleUpdate(ctx context.Context, a *app.App, up Update) *Reply {
	m := up.Message
	if m == nil || m.From == nil || m.From.IsBot {
		return nil
	}
	private := m.Chat.Type == "private"
	fields := strings.Fields(m.Text)
	if len(fields) == 0 {
		return nil
	}
	cmd := strings.ToLower(fields[0])
	if !strings.HasPrefix(cmd, "/") {
		if private {
			return reply(m.Chat.ID, helpText(a))
		}
		return nil
	}
	if name, target, ok := strings.Cut(cmd, "@"); ok {
		if bot := cachedBotUsername(a); bot != "" && !strings.EqualFold(target, bot) {
			return nil
		}
		cmd = name
	}
	switch cmd {
	case "/start":
		if len(fields) < 2 {
			if !private {
				return nil
			}
			return reply(m.Chat.ID, helpText(a))
		}
		if !private {
			return reply(m.Chat.ID, html.EscapeString(text(a, "bind_first")))
		}
		u, err := bind(ctx, a, fields[1], m.Chat.ID, m.From.Username)
		if err != nil {
			a.Log.Error("telegram: bind failed", "err", err)
			return reply(m.Chat.ID, html.EscapeString(text(a, "error")))
		}
		if u == nil {
			return reply(m.Chat.ID, html.EscapeString(text(a, "bad_code")))
		}
		return reply(m.Chat.ID, html.EscapeString(fmt.Sprintf(text(a, "bound"), maskEmail(u.Email))))
	case "/unbind":
		if !private {
			return reply(m.Chat.ID, html.EscapeString(text(a, "private")))
		}
		n, err := unbindChat(ctx, a, m.Chat.ID)
		if err != nil {
			a.Log.Error("telegram: unbind failed", "err", err)
			return reply(m.Chat.ID, html.EscapeString(text(a, "error")))
		}
		if n == 0 {
			return reply(m.Chat.ID, html.EscapeString(text(a, "not_bound")))
		}
		return reply(m.Chat.ID, html.EscapeString(text(a, "unbound")))
	case "/id":
		id := "<code>" + strconv.FormatInt(m.Chat.ID, 10) + "</code>"
		return reply(m.Chat.ID, fmt.Sprintf(html.EscapeString(text(a, "chat_id")), id))
	default:
		if !private && cmd != "/help" {
			return nil
		}
		return reply(m.Chat.ID, helpText(a))
	}
}

func helpText(a *app.App) string {
	s := html.EscapeString(text(a, "help"))
	if link := strings.TrimSpace(a.Settings.String(KeyDiscussLink)); link != "" {
		s += "\n\n" + html.EscapeString(fmt.Sprintf(text(a, "discuss"), link))
	}
	return s
}

// maskEmail keeps the first two characters of the local part: "al***@example.com".
func maskEmail(email string) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return notify.Mask(email)
	}
	r := []rune(local)
	if len(r) > 2 {
		r = r[:2]
	}
	return string(r) + "***@" + domain
}

// WebhookURL returns <app_url>/api/v1/guest/telegram/webhook/<secret>.
func WebhookURL(a *app.App) string {
	base := strings.TrimRight(strings.TrimSpace(a.Settings.String("app_url")), "/")
	return base + httpx.APIPrefix + WebhookPath + a.Settings.String(KeyWebhookSecret)
}

// validWebhookSecret reports whether s can be used in the URL path and as Telegram's secret_token.
func validWebhookSecret(s string) bool {
	if len(s) < 16 || len(s) > 256 {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// WebhookResult is the answer of POST /admin/settings/telegram/webhook: Telegram's setWebhook answer and the
// URL that was registered.
type WebhookResult struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	ErrorCode   int    `json:"error_code,omitempty"`
	URL         string `json:"url"`
}

// RegisterWebhook calls setWebhook with WebhookURL (and the secret as secret_token). Missing settings are
// returned as field errors; transport failures wrap ErrUnavailable.
func RegisterWebhook(ctx context.Context, a *app.App) (*WebhookResult, error) {
	fields := map[string][]string{}
	if !Enabled(a) {
		fields[KeyToken] = append(fields[KeyToken], "set the bot token first")
	}
	if strings.TrimSpace(a.Settings.String("app_url")) == "" {
		fields["app_url"] = append(fields["app_url"], "set the public URL of the panel first (Telegram calls <app_url>"+httpx.APIPrefix+WebhookPath+"…)")
	}
	if !validWebhookSecret(a.Settings.String(KeyWebhookSecret)) {
		fields[KeyWebhookSecret] = append(fields[KeyWebhookSecret], "must be 16-256 characters: letters, digits, _ or -")
	}
	if len(fields) > 0 {
		e := httpx.BadRequest("telegram webhook settings are incomplete")
		e.Errors = fields
		return nil, e
	}
	url := WebhookURL(a)
	r, err := ClientFor(a).SetWebhook(ctx, url, a.Settings.String(KeyWebhookSecret))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return &WebhookResult{OK: r.OK, Description: r.Description, ErrorCode: r.ErrorCode, URL: url}, nil
}

// TestResult is the outcome of the test message for one administrator chat.
type TestResult struct {
	ChatID string `json:"chat_id"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// TestReport is the answer of POST /admin/settings/telegram/test.
type TestReport struct {
	Sent    int          `json:"sent"`
	Failed  int          `json:"failed"`
	Results []TestResult `json:"results"`
}

// SendTest sends the test message to every administrator chat synchronously, one attempt each.
func SendTest(ctx context.Context, a *app.App) (*TestReport, error) {
	fields := map[string][]string{}
	if !Enabled(a) {
		fields[KeyToken] = []string{"set the bot token first"}
	}
	chats := AdminChats(a)
	if len(chats) == 0 {
		fields[KeyAdminChats] = []string{"add at least one chat id (send /id to the bot to get it)"}
	}
	if len(fields) > 0 {
		e := httpx.BadRequest("telegram test settings are incomplete")
		e.Errors = fields
		return nil, e
	}
	client, msg := ClientFor(a), Render(notify.Test(a), notify.Locale(a))
	rep := &TestReport{Results: make([]TestResult, 0, len(chats))}
	for _, chat := range chats {
		res := TestResult{ChatID: chat, OK: true}
		if err := client.SendMessage(ctx, chat, msg); err != nil {
			res.OK, res.Error = false, err.Error()
			rep.Failed++
		} else {
			rep.Sent++
		}
		rep.Results = append(rep.Results, res)
	}
	return rep, nil
}
