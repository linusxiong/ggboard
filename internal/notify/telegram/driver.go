// Package telegram is the Telegram bot driver of internal/notify: administrator notifications to the chats in
// telegram_admin_chat_ids, personal notifications to users who bound their Telegram account, account binding
// (one-time codes and t.me deep links), the bot webhook (/start <code>, /unbind, /id) and the admin helpers
// (setWebhook, test message). It is active only when telegram_bot_token is set; the token and the webhook
// secret never appear in logs or errors.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
)

// DriverName is the notify driver name.
const DriverName = "telegram"

// Settings keys (group telegram).
const (
	KeyToken         = "telegram_bot_token" //nolint:gosec // G101: a settings key name, not a credential
	KeyAdminChats    = "telegram_admin_chat_ids"
	KeyEvents        = "telegram_events"
	KeyUserNotify    = "telegram_user_notify"
	KeyDiscussLink   = "telegram_discuss_link"
	KeyWebhookSecret = "telegram_webhook_secret" //nolint:gosec // G101: a settings key name, not a credential
)

func init() { notify.Register(DriverName, newNotifier) }

// Token returns the configured bot token ("" = the driver is off).
func Token(a *app.App) string { return strings.TrimSpace(a.Settings.String(KeyToken)) }

// Enabled reports whether a bot token is configured.
func Enabled(a *app.App) bool { return Token(a) != "" }

// AdminChats returns the administrator chat ids.
func AdminChats(a *app.App) []string { return a.Settings.Strings(KeyAdminChats) }

// Subscribed reports whether administrators receive events of type t (telegram_events).
func Subscribed(a *app.App, t notify.Type) bool {
	return slices.Contains(a.Settings.Strings(KeyEvents), string(t))
}

const kvAPIBase = "telegram:api_base"

// SetAPIBase points a's Bot API client at base (tests use an httptest server; "" restores the default).
func SetAPIBase(a *app.App, base string) {
	if base == "" {
		a.KV.Delete(kvAPIBase)
		return
	}
	a.KV.Set(kvAPIBase, base, -1)
}

// ClientFor returns a Bot API client with a's token.
func ClientFor(a *app.App) *Client {
	base, _ := a.KV.Get(kvAPIBase)
	s, _ := base.(string)
	return NewClient(Token(a), s)
}

// Notifier delivers notify events through the bot.
type Notifier struct {
	a      *app.App
	client *Client
}

func newNotifier(a *app.App) notify.Notifier {
	if !Enabled(a) {
		return nil
	}
	return &Notifier{a: a, client: ClientFor(a)}
}

// Name implements notify.Notifier.
func (n *Notifier) Name() string { return DriverName }

// ChatRef implements notify.UserAddresser: the bound chat of u while telegram_user_notify is on.
func (n *Notifier) ChatRef(u *model.User) string {
	if !n.a.Settings.Bool(KeyUserNotify) || u.TelegramID == nil || *u.TelegramID == 0 {
		return ""
	}
	return strconv.FormatInt(*u.TelegramID, 10)
}

// SendAdmin sends ev to every administrator chat when its type is listed in telegram_events. A retry skips
// the chats already served; chats that failed permanently are logged and not retried.
func (n *Notifier) SendAdmin(ctx context.Context, ev notify.Event) error {
	if !Subscribed(n.a, ev.Type) {
		return notify.ErrSkipped
	}
	chats := AdminChats(n.a)
	if len(chats) == 0 {
		return notify.ErrSkipped
	}
	text := Render(ev, notify.Locale(n.a))
	var errs []error
	permanent := true
	for _, chat := range chats {
		if notify.Done(ctx, chat) {
			continue
		}
		err := classify(n.client.SendMessage(ctx, chat, text))
		switch {
		case err == nil:
			notify.MarkDone(ctx, chat)
			continue
		case notify.IsPermanent(err):
			notify.MarkDone(ctx, chat)
			n.a.Log.Warn("telegram: admin chat failed", "chat", chat, "type", ev.Type, "err", err)
		default:
			permanent = false
		}
		errs = append(errs, fmt.Errorf("chat %s: %w", chat, err))
		var ra notify.RetryAfterError
		if errors.As(err, &ra) && ra.RetryAfter() > 0 {
			break // rate limited: the queue waits retry_after, then resumes with the remaining chats
		}
	}
	if len(errs) == 0 {
		return nil
	}
	err := errors.Join(errs...)
	if permanent {
		return notify.Permanent(err)
	}
	return err
}

// SendUser sends ev to a user's bound chat. A chat that is also an administrator chat subscribed to ev.Type
// already got the event and is skipped.
func (n *Notifier) SendUser(ctx context.Context, chatRef string, ev notify.Event) error {
	if ev.Admin && Subscribed(n.a, ev.Type) && slices.Contains(AdminChats(n.a), chatRef) {
		return notify.ErrSkipped
	}
	return classify(n.client.SendMessage(ctx, chatRef, Render(ev, notify.Locale(n.a))))
}

// classify marks Bot API refusals (4xx other than 429: chat not found, bot blocked, bad token) as permanent.
func classify(err error) error {
	var ae *APIError
	if errors.As(err, &ae) && !ae.Temporary() {
		return notify.Permanent(err)
	}
	return err
}
