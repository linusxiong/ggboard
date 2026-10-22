package notify

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/payment"
	plansvc "github.com/linusxiong/ggboard/internal/service/plan"
)

// Limits applied by the builders (runes).
const (
	MaxFieldLen   = 256 // field values
	MaxExcerptLen = 200 // ticket messages quoted in Body
	MaxUALen      = 80  // admin_login user agent
)

// F returns a field.
func F(key, value string) Field { return Field{Key: key, Value: value} }

// New returns an event of type t with its localized title, the given body, At = now and the non-empty fields
// (values are cut to MaxFieldLen). Producers outside this package build their events with New, e.g.
//
//	notify.Emit(a, notify.New(a, notify.TypeSubscriptionLeak, body, notify.F("email", u.Email), notify.F("ip", ip)))
//
// The result goes to administrators only; set UserID to address a user as well.
func New(a *app.App, t Type, body string, fields ...Field) Event {
	ev := Event{Type: t, Title: Title(Locale(a), t), Body: body, Admin: true, At: time.Now()}
	for _, f := range fields {
		if f.Value = strings.TrimSpace(f.Value); f.Value != "" {
			f.Value = Truncate(f.Value, MaxFieldLen)
			ev.Fields = append(ev.Fields, f)
		}
	}
	return ev
}

func forUser(ev Event, userID uint64, admin bool) Event {
	id := userID
	ev.UserID, ev.Admin = &id, admin
	return ev
}

// Money formats cents with the site's currency symbol ("¥12.00"; "12.00 CNY" without a symbol).
func Money(a *app.App, cents int64) string {
	if sym := a.Settings.String("currency_symbol"); sym != "" {
		return sym + payment.FormatAmount(cents)
	}
	return strings.TrimSpace(payment.FormatAmount(cents) + " " + a.Settings.String("currency"))
}

// Truncate cuts s to at most n runes, ending with "…" when it was longer.
func Truncate(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// Mask hides the middle of an account number or address: "abc***xyz" (at most 3 characters kept at each end,
// fewer for short values).
func Mask(s string) string {
	r := []rune(strings.TrimSpace(s))
	switch n := len(r); {
	case n == 0:
		return ""
	case n <= 2:
		return "***"
	case n <= 6:
		return string(r[:1]) + "***" + string(r[n-1:])
	default:
		return string(r[:3]) + "***" + string(r[n-3:])
	}
}

// Cancel reasons of order_cancel.
const (
	ReasonTimeout = "timeout"
	ReasonAdmin   = "admin"
)

// Order describes an order for the order_* events.
type Order struct {
	UserID     uint64
	Email      string
	Plan       string
	Period     string // model period key, localized by the builders
	Amount     int64  // cents
	TradeNo    string
	Gateway    string // order_paid: payment method name ("" for free, balance or manual payments)
	CallbackNo string // order_paid
	Reason     string // order_cancel: ReasonTimeout or ReasonAdmin
}

func (o Order) fields(a *app.App, locale string) []Field {
	return []Field{
		F("email", o.Email), F("plan", o.Plan), F("period", plansvc.PeriodLabel(o.Period, locale)),
		F("amount", Money(a, o.Amount)), F("trade_no", o.TradeNo),
	}
}

// OrderNew is order_new (administrators): email, plan, period, amount, trade_no.
func OrderNew(a *app.App, o Order) Event {
	return New(a, TypeOrderNew, "", o.fields(a, Locale(a))...)
}

// OrderPaid is order_paid (administrators and the buyer): email, plan, period, amount, trade_no, gateway,
// callback_no.
func OrderPaid(a *app.App, o Order) Event {
	l := Locale(a)
	fs := append(o.fields(a, l), F("gateway", o.Gateway), F("callback_no", o.CallbackNo))
	return forUser(New(a, TypeOrderPaid, T(l, "body.order_paid"), fs...), o.UserID, true)
}

// OrderCancel is order_cancel (administrators): email, plan, period, amount, trade_no, reason.
func OrderCancel(a *app.App, o Order) Event {
	l := Locale(a)
	reason := ""
	if o.Reason != "" {
		reason = T(l, "reason."+o.Reason)
	}
	return New(a, TypeOrderCancel, "", append(o.fields(a, l), F("reason", reason))...)
}

// Sign-in methods of admin_login.
const (
	MethodPassword = "password"
	MethodTOTP     = "totp"
	MethodBackup   = "backup"
	MethodPasskey  = "passkey"
)

// Login describes an administrator or staff sign-in.
type Login struct {
	Email   string
	IsAdmin bool   // false = staff
	Method  string // MethodPassword, MethodTOTP, MethodBackup or MethodPasskey
	IP      string
	UA      string
	At      time.Time
}

// AdminLogin is admin_login (administrators): email, role, method, ip, ua (MaxUALen), time.
func AdminLogin(a *app.App, in Login) Event {
	l := Locale(a)
	role := "staff"
	if in.IsAdmin {
		role = "admin"
	}
	at := in.At
	if at.IsZero() {
		at = time.Now()
	}
	ev := New(a, TypeAdminLogin, T(l, "body.admin_login"),
		F("email", in.Email), F("role", T(l, "role."+role)), F("method", T(l, "method."+in.Method)),
		F("ip", in.IP), F("ua", Truncate(in.UA, MaxUALen)), F("time", at.Local().Format("2006-01-02 15:04:05")))
	ev.At = at
	return ev
}

// Ticket describes a ticket for ticket_new / ticket_reply.
type Ticket struct {
	ID      uint64
	UserID  uint64
	Email   string
	Subject string
	Level   int    // 0 low, 1 medium, 2 high
	Message string // the new message, quoted (MaxExcerptLen) in Body
}

// TicketNew is ticket_new (administrators): subject, level, user; Body quotes the first message.
func TicketNew(a *app.App, t Ticket) Event {
	l := Locale(a)
	return New(a, TypeTicketNew, excerpt(t.Message),
		F("subject", t.Subject), F("level", T(l, "level."+strconv.Itoa(t.Level))), F("user", t.Email))
}

// TicketReply is ticket_reply (the ticket's user; administrators only when they subscribed to it): subject;
// Body quotes the staff reply.
func TicketReply(a *app.App, t Ticket) Event {
	return forUser(New(a, TypeTicketReply, excerpt(t.Message), F("subject", t.Subject)), t.UserID, true)
}

func excerpt(s string) string {
	return Truncate(strings.TrimSpace(s), MaxExcerptLen)
}

// Withdrawal describes a commission withdrawal request.
type Withdrawal struct {
	Email   string
	Amount  int64 // cents
	Method  string
	Account string // masked by the builder
}

// Withdraw is withdraw (administrators): user, amount, method, account (masked).
func Withdraw(a *app.App, w Withdrawal) Event {
	return New(a, TypeWithdraw, "",
		F("user", w.Email), F("amount", Money(a, w.Amount)), F("method", w.Method), F("account", Mask(w.Account)))
}

// RemindExpire is remind_expire for one user; expiresAt is already formatted.
func RemindExpire(a *app.App, userID uint64, expiresAt string) Event {
	body := fmt.Sprintf(T(Locale(a), "body.remind_expire"), expiresAt)
	return forUser(New(a, TypeRemindExpire, body), userID, false)
}

// RemindTraffic is remind_traffic for one user (used/total already formatted).
func RemindTraffic(a *app.App, userID uint64, percent int64, used, total string) Event {
	body := fmt.Sprintf(T(Locale(a), "body.remind_traffic"), percent, used, total)
	return forUser(New(a, TypeRemindTraffic, body), userID, false)
}

// SubscriptionLeak is subscription_leak (administrators): user, distinct (client IPs that fetched the subscription
// within the detector's window). Its signature matches the subscription leak detector, so the integrator can
// wire it with EmitSubscriptionLeak.
func SubscriptionLeak(a *app.App, u *model.User, distinct int) Event {
	body := fmt.Sprintf(T(Locale(a), "body.subscription_leak"), distinct)
	var email string
	if u != nil {
		email = u.Email
	}
	return New(a, TypeSubscriptionLeak, body, F("user", email), F("distinct", strconv.Itoa(distinct)))
}

// EmitSubscriptionLeak emits SubscriptionLeak; it has the shape of subscription.LeakHook
// (func(a *app.App, u *model.User, distinct int)).
func EmitSubscriptionLeak(a *app.App, u *model.User, distinct int) {
	Emit(a, SubscriptionLeak(a, u, distinct))
}

// Test is the administrators' test message: the site name and URL.
func Test(a *app.App) Event {
	site := a.Settings.String("app_name")
	if u := strings.TrimSpace(a.Settings.String("app_url")); u != "" {
		site += " · " + u
	}
	return New(a, TypeTest, T(Locale(a), "body.test"), F("site", site))
}
