package ordersvc

import (
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Review 2026-10-22: Stripe sessions live at least 30 minutes, so with the default 30-minute timeout every
// order checked out after creation stayed payable after order_timeout cancelled it ("paid after cancel").
// Checked-out orders now get CheckoutGrace, and orders past their deadline cannot start a new payment.
func TestReviewTimeoutSparesRecentCheckout(t *testing.T) {
	f := newFixture(t, map[string]any{"order_timeout_minutes": 30})
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	p := f.payment(true)
	u := f.user("stripe@example.com")
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	// Created 20 minutes ago, checked out now.
	f.a.DB.Model(&model.Order{}).Where("id = ?", o.ID).UpdateColumn("created_at", f.now.Add(-20*time.Minute))
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &p.ID}); err != nil {
		t.Fatal(err)
	}
	// 11 minutes later the order is past its deadline but the payment page is still open.
	f.now = f.now.Add(11 * time.Minute)
	if n, err := f.svc.CancelExpired(ctx); err != nil || n != 0 {
		t.Fatalf("cancelled a just-checked-out order: n=%d err=%v", n, err)
	}
	// No new payment page past the deadline.
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &p.ID}); status(err) != 409 {
		t.Fatalf("checkout past deadline: %v", err)
	}
	// Once the grace has passed, the job cancels it.
	f.now = f.now.Add(CheckoutGrace)
	if n, err := f.svc.CancelExpired(ctx); err != nil || n != 1 {
		t.Fatalf("after grace: n=%d err=%v", n, err)
	}
}

// Review 2026-10-22: a replayed notification for a cancelled order alerted the administrator every time.
func TestReviewPaidAfterCancelAlertsOnce(t *testing.T) {
	f := newFixture(t)
	admin := testutil.CreateAdmin(t, f.a, "root@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	pay := f.payment(true)
	u := f.user("late@example.com")
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &pay.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Cancel(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	body := `{"trade_no":"` + o.TradeNo + `","callback_no":"late-1","amount":1000}`
	for range 3 {
		if resp := f.notify(pay, fakeName, "s3cret", body); resp.Status != 200 {
			t.Fatalf("notify = %+v", resp)
		}
	}
	n := 0
	for _, m := range testutil.Mails(t, f.a) {
		if m.To == admin.Email && m.Template == mail.TplNotify {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("admin alerts = %d, want 1", n)
	}
}
