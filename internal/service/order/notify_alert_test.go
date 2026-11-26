package ordersvc

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// alertFixture is an order checked out with the fake gateway and a helper counting the administrator alerts with
// a given subject.
func alertFixture(t *testing.T) (f *fixture, pay *model.Payment, o *model.Order, alerts func(subject string) int) {
	f = newFixture(t)
	admin := testutil.CreateAdmin(t, f.a, "root@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	pay = f.payment(true)
	u := f.user("n@example.com")
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &pay.ID}); err != nil {
		t.Fatal(err)
	}
	alerts = func(subject string) int {
		n := 0
		for _, m := range testutil.Mails(t, f.a) {
			if m.To == admin.Email && m.Template == mail.TplNotify && strings.Contains(m.Subject, subject) {
				n++
			}
		}
		return n
	}
	return f, pay, o, alerts
}

func notifyBody(tradeNo, cb string, amount int) string {
	return `{"trade_no":"` + tradeNo + `","callback_no":"` + cb + `","amount":` + strconv.Itoa(amount) + `}`
}

// Security review 2026-11-25: a genuine verified notification (for example an EPay return URL, which carries the
// signed parameters) replayed for an unknown order or with a mismatching amount mailed the administrator on every
// request and could fill the mail queue. Each alert now goes out once per payment, order and transaction.
func TestNotifyAlertsOncePerTransaction(t *testing.T) {
	f, pay, o, alerts := alertFixture(t)
	for range 3 {
		if resp := f.notify(pay, fakeName, "s3cret", notifyBody("20990101000000000000", "imported-1", 1000)); resp.Status != 200 {
			t.Fatalf("unknown order = %+v", resp)
		}
	}
	if n := alerts("unknown order"); n != 1 {
		t.Fatalf("unknown-order alerts after 3 replays = %d, want 1", n)
	}
	// Another transaction is another alert.
	f.notify(pay, fakeName, "s3cret", notifyBody("20990101000000000000", "imported-2", 1000))
	if n := alerts("unknown order"); n != 2 {
		t.Fatalf("unknown-order alerts after a second transaction = %d, want 2", n)
	}

	for range 3 {
		if resp := f.notify(pay, fakeName, "s3cret", notifyBody(o.TradeNo, "short-1", 1)); resp.Status != 200 {
			t.Fatalf("amount mismatch = %+v", resp)
		}
	}
	if n := alerts("amount mismatch"); n != 1 || f.order(o.TradeNo).Status != model.OrderStatusPending {
		t.Fatalf("amount-mismatch alerts after 3 replays = %d, want 1 (order %d)", n, f.order(o.TradeNo).Status)
	}
}

// Security review 2026-11-25: a buyer who pays two checkout pages of one order is charged twice, which used to be a
// WARN line only. The second payment now alerts the administrator once per transaction; a gateway retry of the
// payment that completed the order does not.
func TestNotifyAlertsDoublePayment(t *testing.T) {
	f, pay, o, alerts := alertFixture(t)
	if resp := f.notify(pay, fakeName, "s3cret", notifyBody(o.TradeNo, "page-1", 1000)); resp.Status != 200 || f.order(o.TradeNo).Status != model.OrderStatusCompleted {
		t.Fatalf("first payment = %+v", resp)
	}
	for range 3 {
		f.notify(pay, fakeName, "s3cret", notifyBody(o.TradeNo, "page-1", 1000)) // gateway retry
	}
	if n := alerts("double payment"); n != 0 {
		t.Fatalf("retries of the completing payment alerted %d times", n)
	}
	for range 3 {
		if resp := f.notify(pay, fakeName, "s3cret", notifyBody(o.TradeNo, "page-2", 1000)); resp.Status != 200 {
			t.Fatalf("second payment = %+v", resp)
		}
	}
	if n := alerts("double payment"); n != 1 {
		t.Fatalf("double-payment alerts after 3 deliveries = %d, want 1", n)
	}
	if got := f.order(o.TradeNo); got.CallbackNo != "page-1" {
		t.Fatalf("the second payment changed the order: %+v", got)
	}
}

// The same when the second payment's callback loses the claim: the order was pending when the notification loaded
// it and another page's callback completed it before the claiming update.
func TestNotifyAlertsDoublePaymentWhenClaimLost(t *testing.T) {
	f, pay, o, alerts := alertFixture(t)
	var armed atomic.Bool
	armed.Store(true)
	if err := f.a.DB.Callback().Query().After("gorm:query").Register("test:pay_between_load_and_claim", func(db *gorm.DB) {
		if db.Statement.Table == "orders" && armed.CompareAndSwap(true, false) {
			if _, _, err := f.svc.Paid(ctx, o.TradeNo, "page-1"); err != nil {
				t.Error(err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if resp := f.notify(pay, fakeName, "s3cret", notifyBody(o.TradeNo, "page-2", 1000)); resp.Status != 200 {
		t.Fatalf("second payment = %+v", resp)
	}
	if armed.Load() {
		t.Fatal("the order was never loaded: the hook did not run")
	}
	if got := f.order(o.TradeNo); got.Status != model.OrderStatusCompleted || got.CallbackNo != "page-1" {
		t.Fatalf("order = %+v", got)
	}
	if n := alerts("double payment"); n != 1 {
		t.Fatalf("double-payment alerts = %d, want 1", n)
	}
}
