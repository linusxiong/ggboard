package ordersvc

import (
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
)

func TestHugePriceDiscountNeverWrapsToFreeCheckout(t *testing.T) {
	fifty := 50
	q := ComputeQuote(QuoteInput{Price: math.MaxInt64, UserDiscount: &fifty,
		Coupon: &model.Coupon{Type: model.CouponTypePercent, Value: 50}})
	if q.Discount != math.MaxInt64 || q.Total != 0 {
		t.Fatalf("overflow quote: %+v", q)
	}
	q = ComputeQuote(QuoteInput{Price: math.MaxInt64, UserDiscount: &fifty})
	if q.Discount != 4_611_686_018_427_387_904 || q.Total != 4_611_686_018_427_387_903 {
		t.Fatalf("overflow percentage: %+v", q)
	}

	f := newFixture(t)
	u := f.user("buyer@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	o := &model.Order{TradeNo: "20261028000000999997", UserID: u.ID, PlanID: plan.ID,
		Type: model.OrderTypeNew, Period: model.PeriodMonthly, Status: model.OrderStatusPending, TotalAmount: -1}
	if err := f.a.DB.Create(o).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{}); status(err) != 409 {
		t.Fatalf("negative total accepted as free: %v", err)
	}
	if got := f.reload(u); got.PlanID != nil || f.order(o.TradeNo).Status != model.OrderStatusPending {
		t.Fatalf("negative total granted plan: %+v", got)
	}
}

func TestOverflowedChargeCannotReachGatewayOrFulfil(t *testing.T) {
	f := newFixture(t)
	u := f.user("buyer@example.com")
	plan := f.plan("Expensive", map[string]int64{model.PeriodMonthly: math.MaxInt64})
	p := f.payment(true, func(p *model.Payment) { p.HandlingFeeFixed = 1 })
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &p.ID}); status(err) != 409 {
		t.Fatalf("overflow checkout: %v", err)
	}
	if got := f.order(o.TradeNo); got.PaymentID != nil || got.Status != model.OrderStatusPending || f.reload(u).PlanID != nil {
		t.Fatalf("overflow charge changed entitlement: %+v", got)
	}
	if got := HandlingAmount(&model.Payment{HandlingFeePercent: 100}, math.MaxInt64); got != math.MaxInt64 {
		t.Fatalf("overflow fee became %d", got)
	}
	if err := f.a.DB.Model(o).Updates(map[string]any{"payment_id": p.ID, "handling_amount": int64(1)}).Error; err != nil {
		t.Fatal(err)
	}
	resp := f.notify(p, fakeName, "s3cret", `{"trade_no":"`+o.TradeNo+`","callback_no":"fake","amount":9223372036854775807}`)
	if resp.Status != http.StatusOK || f.order(o.TradeNo).Status != model.OrderStatusPending || f.reload(u).PlanID != nil {
		t.Fatalf("overflow callback fulfilled order: %+v", resp)
	}
}

func TestSurplusIgnoresOverflowedUsageAndMoney(t *testing.T) {
	now := time.Now()
	planID := uint64(1)
	u := &model.User{PlanID: &planID, TransferEnable: model.BytesPerGB, U: math.MaxInt64, D: 1}
	orders := []model.Order{{ID: 1, PlanID: planID, Period: model.PeriodOnetime, TotalAmount: 1000}}
	if credit, _ := ComputeSurplus(u, orders, now); credit != 0 {
		t.Fatalf("overflowed usage yielded %d credit", credit)
	}
	u.U, u.D = 0, 0
	orders[0].TotalAmount = math.MaxInt64
	if credit, _ := ComputeSurplus(u, orders, now); credit != math.MaxInt64 {
		t.Fatalf("maximum paid value yielded %d credit", credit)
	}
}

func TestRestoreCouponAtIntegerLimitDoesNotOverflow(t *testing.T) {
	f := newFixture(t)
	limit := math.MaxInt
	c := &model.Coupon{Code: "MAXLIMIT", Name: "Imported", LimitUse: &limit}
	if err := f.a.DB.Create(c).Error; err != nil {
		t.Fatal(err)
	}
	if err := restoreCoupon(f.a.DB, c.ID); err != nil {
		t.Fatal(err)
	}
	var got model.Coupon
	if err := f.a.DB.First(&got, c.ID).Error; err != nil || got.LimitUse == nil || *got.LimitUse != math.MaxInt {
		t.Fatalf("coupon limit overflowed: %+v err=%v", got, err)
	}
}

func TestMoneyCreditsRollBackAtIntegerLimit(t *testing.T) {
	f := newFixture(t)
	u := f.user("buyer@example.com")
	f.setUser(u, map[string]any{"balance": int64(math.MaxInt64), "commission_balance": int64(math.MaxInt64)})
	for _, column := range []string{"balance", "commission_balance"} {
		if err := creditUser(f.a.DB, u.ID, column, 1); err == nil {
			t.Fatalf("%s overflow credited", column)
		}
	}
	if got := f.reload(u); got.Balance != math.MaxInt64 || got.CommissionBalance != math.MaxInt64 {
		t.Fatalf("overflow changed balance: %+v", got)
	}
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1})
	o := &model.Order{TradeNo: "20261028000000999996", UserID: u.ID, PlanID: plan.ID,
		Type: model.OrderTypeNew, Period: model.PeriodMonthly, Status: model.OrderStatusPending, BalanceAmount: 1}
	if err := f.a.DB.Create(o).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Cancel(ctx, o.ID); err == nil || f.order(o.TradeNo).Status != model.OrderStatusPending {
		t.Fatalf("overflow cancellation did not roll back: %v", err)
	}
}
