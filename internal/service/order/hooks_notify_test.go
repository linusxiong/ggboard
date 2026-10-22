package ordersvc

import (
	"strconv"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/notifytest"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

func TestOrderNotifications(t *testing.T) {
	f := newFixture(t)
	rec := notifytest.Attach(t, f.a)
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000, model.PeriodYearly: 9000})
	pay := f.payment(true)
	u := f.user("buyer@example.com", usersvc.CreateOptions{Balance: 100})

	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	ev := rec.Last(t, notify.TypeOrderNew)
	fs := notifytest.Fields(ev)
	if fs["email"] != "buyer@example.com" || fs["plan"] != "Pro" || fs["period"] != "月付" || fs["amount"] != "¥10.00" || fs["trade_no"] != o.TradeNo {
		t.Fatalf("order_new = %v", fs)
	}
	if ev.UserID != nil {
		t.Fatal("order_new is for administrators only")
	}

	// Paid through a gateway: gateway name + callback number, also sent to the buyer.
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &pay.ID}); err != nil {
		t.Fatal(err)
	}
	paidOrder := f.order(o.TradeNo)
	amount := paidOrder.TotalAmount + paidOrder.HandlingAmount
	if resp := f.notify(pay, fakeName, "s3cret", `{"trade_no":"`+o.TradeNo+`","callback_no":"CB-1","amount":`+strconv.FormatInt(amount, 10)+`}`); resp.Status != 200 {
		t.Fatalf("notify = %+v", resp)
	}
	ev = rec.Last(t, notify.TypeOrderPaid)
	fs = notifytest.Fields(ev)
	if fs["gateway"] != "Fake Pay" || fs["callback_no"] != "CB-1" || fs["trade_no"] != o.TradeNo || fs["email"] != "buyer@example.com" ||
		ev.UserID == nil || *ev.UserID != u.ID {
		t.Fatalf("order_paid = %+v", ev)
	}
	if got := rec.UserEvents(t, u.ID, notify.TypeOrderPaid); len(got) != 1 {
		t.Fatalf("buyer deliveries = %d", len(got))
	}
	// A duplicate callback does not notify twice.
	f.notify(pay, fakeName, "s3cret", `{"trade_no":"`+o.TradeNo+`","callback_no":"CB-1","amount":`+strconv.FormatInt(amount, 10)+`}`)
	if n := len(rec.Events(t, notify.TypeOrderPaid)); n != 1 {
		t.Fatalf("order_paid events = %d", n)
	}

	// A user cancelling their own order is not notified; an administrator or the timeout job is.
	o2, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodYearly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Cancel(ctx, o2.ID); err != nil {
		t.Fatal(err)
	}
	rec.None(t, notify.TypeOrderCancel)

	o3, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodYearly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CancelBy(ctx, o3.ID, CancelByAdmin); err != nil {
		t.Fatal(err)
	}
	fs = notifytest.Fields(rec.Last(t, notify.TypeOrderCancel))
	if fs["reason"] != "管理员取消" || fs["trade_no"] != o3.TradeNo || fs["plan"] != "Pro" || fs["email"] != "buyer@example.com" || fs["period"] != "年付" {
		t.Fatalf("order_cancel (admin) = %v", fs)
	}

	o4, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodYearly})
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(2 * time.Hour)
	if err := f.a.DB.Model(&model.Order{}).Where("id = ?", o4.ID).Update("created_at", f.now.Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := f.svc.CancelExpired(ctx); err != nil || n != 1 {
		t.Fatalf("CancelExpired = %d, %v", n, err)
	}
	fs = notifytest.Fields(rec.Last(t, notify.TypeOrderCancel))
	if fs["reason"] != "超时未支付" || fs["trade_no"] != o4.TradeNo {
		t.Fatalf("order_cancel (timeout) = %v", fs)
	}
	if n := len(rec.Events(t, notify.TypeOrderCancel)); n != 2 {
		t.Fatalf("order_cancel events = %d", n)
	}
}

func TestOrderNotificationsFreeAndManual(t *testing.T) {
	f := newFixture(t)
	rec := notifytest.Attach(t, f.a)
	free := f.plan("Free", map[string]int64{model.PeriodMonthly: 0})
	u := f.user("free@example.com")
	if _, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: free.ID, Period: model.PeriodMonthly}); err != nil {
		t.Fatal(err)
	}
	if len(rec.Events(t, notify.TypeOrderNew)) != 1 {
		t.Fatal("order_new for a free order")
	}
	fs := notifytest.Fields(rec.Last(t, notify.TypeOrderPaid))
	if fs["callback_no"] != CallbackFree || fs["gateway"] != "" || fs["amount"] != "¥0.00" {
		t.Fatalf("free order_paid = %v", fs)
	}

	// Admin-assigned orders are not "new" user orders; confirming them manually is a payment.
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	v := f.user("assigned@example.com")
	o, err := f.svc.Assign(ctx, AssignInput{UserID: v.ID, PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Events(t, notify.TypeOrderNew)); n != 1 {
		t.Fatalf("order_new after Assign = %d", n)
	}
	if _, err := f.svc.MarkPaid(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	ev := rec.Last(t, notify.TypeOrderPaid)
	if notifytest.Fields(ev)["callback_no"] != CallbackManual || *ev.UserID != v.ID {
		t.Fatalf("manual order_paid = %+v", ev)
	}
}
