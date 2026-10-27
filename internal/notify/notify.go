// Package notify is the pluggable notification system: feature packages describe what happened as an Event and
// call Emit; a bounded in-process queue per App (a priority lane for security events and a normal lane, one worker
// each) hands every event to each configured channel (Notifier) with retries. Channels register a Factory from init() (internal/notify/telegram is the built-in
// Telegram bot) and decide themselves which events they deliver to administrators.
//
//	notify.Emit(a, notify.OrderPaid(a, notify.Order{...}))
//
// Events are localized when they are built (default_locale, zh-CN or en): Title and Body are plain text, Field
// keys are stable identifiers whose labels come from Label.
package notify

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/plugin"
)

// Type identifies an event.
type Type string

// Event types. The first eight can be delivered to administrators (setting telegram_events); the reminders,
// TypeTest and TypeSubscriptionReset are only ever sent to one recipient.
const (
	TypeOrderNew         Type = "order_new"         // a user placed an order
	TypeOrderPaid        Type = "order_paid"        // an order was paid (also sent to its user)
	TypeOrderCancel      Type = "order_cancel"      // a pending order was cancelled by timeout or by an administrator
	TypeAdminLogin       Type = "admin_login"       // an administrator or staff member signed in
	TypeTicketNew        Type = "ticket_new"        // a user opened a ticket
	TypeTicketReply      Type = "ticket_reply"      // staff replied to a ticket (sent to its user)
	TypeWithdraw         Type = "withdraw"          // a user requested a commission withdrawal
	TypeSubscriptionLeak Type = "subscription_leak" // a subscription token looks shared or leaked
	TypeRemindExpire     Type = "remind_expire"     // the user's subscription expires within 24 h
	TypeRemindTraffic    Type = "remind_traffic"    // the user used at least 80 % of the period's traffic
	TypeTest             Type = "test"              // admin "send test message"

	TypeSubscriptionReset Type = "subscription_reset" // an administrator reset the user's subscription token / uuid
)

// AdminTypes lists the event types administrators can subscribe to, in display order (the options of
// telegram_events).
var AdminTypes = []Type{
	TypeOrderNew, TypeOrderPaid, TypeOrderCancel, TypeAdminLogin,
	TypeTicketNew, TypeTicketReply, TypeWithdraw, TypeSubscriptionLeak,
}

// Field is one labelled value of an event. Key is a stable identifier ("email", "trade_no"); drivers show
// Label(locale, Key), which falls back to Key itself for unknown keys.
type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Event is something worth telling administrators and/or one user about.
type Event struct {
	Type   Type
	Title  string  // localized plain text, e.g. "💰 订单已支付"
	Body   string  // localized plain text (may be empty)
	Fields []Field // labelled details, in display order
	UserID *uint64 // recipient user (delivered when the user is reachable on a channel, e.g. bound Telegram)
	Admin  bool    // deliver to administrators (each channel filters by its own settings)
	At     time.Time
}

// Field returns the value of the first field with key ("" when absent).
func (e Event) Field(key string) string {
	for _, f := range e.Fields {
		if f.Key == key {
			return f.Value
		}
	}
	return ""
}

// Notifier is one configured notification channel. Its methods may be called concurrently (one queue worker per
// lane).
type Notifier interface {
	// Name is the driver name ("telegram").
	Name() string
	// SendAdmin delivers ev to the channel's administrator destinations. It returns ErrSkipped when the
	// channel does not deliver this event to administrators (filtered out, no destination configured).
	SendAdmin(ctx context.Context, ev Event) error
	// SendUser delivers ev to one user, addressed by the reference returned by UserAddresser.ChatRef.
	SendUser(ctx context.Context, chatRef string, ev Event) error
}

// UserAddresser is implemented by notifiers that can reach individual users.
type UserAddresser interface {
	// ChatRef returns u's address on the channel, or "" when u cannot be reached (not bound, opted out).
	ChatRef(u *model.User) string
}

// Factory returns the notifier of a driver for a, or nil when the driver is not configured (e.g. no bot token).
// It is called for every event, so it must be cheap and read the current settings.
type Factory func(a *app.App) Notifier

var drivers = plugin.NewRegistry[Factory]("notifier")

// Register adds a driver (call from init). It panics on duplicate names.
func Register(name string, f Factory) { drivers.Register(name, f) }

// Drivers returns the registered driver names.
func Drivers() []string { return drivers.Names() }

// Active returns the notifiers configured for a, in driver-name order.
func Active(a *app.App) []Notifier {
	var out []Notifier
	drivers.Each(func(_ string, f Factory) {
		if n := f(a); n != nil {
			out = append(out, n)
		}
	})
	return out
}

// Enabled reports whether at least one driver is configured; producers check it before doing extra work
// (queries) to build an event.
func Enabled(a *app.App) bool { return len(Active(a)) > 0 }

// ActiveNames returns the names of the configured drivers (never nil).
func ActiveNames(a *app.App) []string {
	out := []string{}
	for _, n := range Active(a) {
		out = append(out, n.Name())
	}
	return out
}

// Reachable reports whether u can be notified on at least one configured channel.
func Reachable(a *app.App, u *model.User) bool {
	for _, n := range Active(a) {
		if ua, ok := n.(UserAddresser); ok && ua.ChatRef(u) != "" {
			return true
		}
	}
	return false
}

// ErrSkipped is returned by a Notifier that does not deliver an event; it counts neither as sent nor failed.
var ErrSkipped = errors.New("notify: skipped")

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent marks err as not worth retrying (e.g. "chat not found").
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsPermanent reports whether err was marked with Permanent.
func IsPermanent(err error) bool {
	var p *permanentError
	return errors.As(err, &p)
}

// RetryAfterError is implemented by errors that tell how long to wait before the next attempt (HTTP 429).
type RetryAfterError interface {
	error
	RetryAfter() time.Duration
}

// progress remembers, across the retries of one delivery, the destinations already handled.
type progress struct {
	mu   sync.Mutex
	done map[string]bool
}

type progressKey struct{}

func withProgress(ctx context.Context, p *progress) context.Context {
	return context.WithValue(ctx, progressKey{}, p)
}

// Done reports whether target was already handled by an earlier attempt of the current delivery. Drivers that
// fan out to several destinations (administrator chats) skip such targets so a retry never sends twice.
func Done(ctx context.Context, target string) bool {
	p, _ := ctx.Value(progressKey{}).(*progress)
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done[target]
}

// MarkDone records that target needs no further attempt (delivered, or failed permanently).
func MarkDone(ctx context.Context, target string) {
	p, _ := ctx.Value(progressKey{}).(*progress)
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done == nil {
		p.done = map[string]bool{}
	}
	p.done[target] = true
}
