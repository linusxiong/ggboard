package ordersvc

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/model"
	_ "github.com/linusxiong/ggboard/internal/payment/gateways/epay"
)

func TestCheckoutSignatureCannotBecomePaidCallback(t *testing.T) {
	f := newFixture(t, map[string]any{"app_url": ""})
	u := f.user("buyer@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	p := f.payment(true, func(p *model.Payment) {
		p.Gateway = "epay"
		p.Config = model.JSONObject[string]{"url": "https://pay.example.com", "pid": "1001", "key": "test-merchant-key", "type": "alipay"}
	})
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	// The HTTP Host can contain these characters; checkout must not become a signing oracle.
	result, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{
		PaymentID: &p.ID,
		BaseURL:   "http://panel.example.com&trade_no=forged&trade_status=TRADE_SUCCESS&type=",
	})
	if err == nil {
		redirect, err := url.Parse(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		q := redirect.Query()
		before, after, ok := strings.Cut(q.Get("return_url"), "&trade_no=forged&trade_status=TRADE_SUCCESS&type=")
		if ok {
			q.Set("return_url", before)
			q.Set("trade_no", "forged")
			q.Set("trade_status", "TRADE_SUCCESS")
			q.Set("type", after+"&type="+q.Get("type"))
			r := httptest.NewRequest(http.MethodGet, NotifyPath+p.Gateway+"/"+p.UUID+"?"+q.Encode(), nil)
			f.svc.HandleNotify(ctx, p.Gateway, p.UUID, r)
		}
	}
	if f.order(o.TradeNo).Status != model.OrderStatusPending || f.reload(u).PlanID != nil {
		t.Fatal("checkout signature granted a paid plan without a payment")
	}
}

func TestNotifyRechecksPaymentAfterConcurrentCheckout(t *testing.T) {
	f := newFixture(t)
	u := f.user("buyer@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	p := f.payment(true)
	other := f.payment(true, func(p *model.Payment) {
		p.UUID = strings.Repeat("cd", 16)
		p.HandlingFeeFixed = 200
	})
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &p.ID}); err != nil {
		t.Fatal(err)
	}
	// Change checkout immediately after Notify reads its order snapshot, before it claims fulfilment.
	var changed atomic.Bool
	const hook = "test:concurrent_checkout"
	if err := f.a.DB.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table != "orders" || tx.Error != nil || !changed.CompareAndSwap(false, true) {
			return
		}
		_, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &other.ID})
		if err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.a.DB.Callback().Query().Remove(hook); err != nil {
			t.Error(err)
		}
	})
	resp := f.notify(p, fakeName, "s3cret", `{"trade_no":"`+o.TradeNo+`","callback_no":"first-checkout","amount":1000}`)
	if !changed.Load() {
		t.Fatal("concurrent checkout did not run")
	}
	if resp.Status != http.StatusInternalServerError || f.order(o.TradeNo).Status != model.OrderStatusPending || f.reload(u).PlanID != nil {
		t.Fatal("callback fulfilled after its payment method and charge changed")
	}
}

func TestPaidRejectsChangedVerifiedCharge(t *testing.T) {
	for _, field := range []string{"payment_id", "total_amount", "handling_amount"} {
		t.Run(field, func(t *testing.T) {
			f := newFixture(t)
			u := f.user("buyer@example.com")
			plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
			p := f.payment(true)
			o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &p.ID}); err != nil {
				t.Fatal(err)
			}
			snapshot := f.order(o.TradeNo)
			if err := f.a.DB.Model(o).UpdateColumn(field, 2000).Error; err != nil {
				t.Fatal(err)
			}
			if _, _, err := f.svc.paid(ctx, o.TradeNo, "stale-quote", snapshot); err == nil {
				t.Fatal("fulfilled a different charge than the one verified")
			}
			if f.order(o.TradeNo).Status != model.OrderStatusPending || f.reload(u).PlanID != nil {
				t.Fatal("stale verification granted plan entitlement")
			}
		})
	}
}

func TestNotifyRejectsUnreportedOrNonPositiveAmount(t *testing.T) {
	for _, amount := range []string{"", `,"amount":0`, `,"amount":-1`} {
		t.Run(amount, func(t *testing.T) {
			f := newFixture(t)
			u := f.user("buyer@example.com")
			plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
			p := f.payment(true)
			o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &p.ID}); err != nil {
				t.Fatal(err)
			}
			resp := f.notify(p, fakeName, "s3cret", `{"trade_no":"`+o.TradeNo+`","callback_no":"fixture-transaction"`+amount+`}`)
			if resp.Status != http.StatusBadRequest || f.order(o.TradeNo).Status != model.OrderStatusPending || f.reload(u).PlanID != nil {
				t.Fatal("callback without a positive amount granted a paid plan")
			}
		})
	}
}
