package ordersvc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

var ctx = context.Background()

func TestCreateCheckoutNotifyFulfil(t *testing.T) {
	f := newFixture(t)
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000, model.PeriodYearly: 10000}, func(p *model.Plan) {
		p.SpeedLimit, p.DeviceLimit = ptr(100), ptr(3)
	})
	u := f.user("buyer@example.com", usersvc.CreateOptions{Balance: 150})
	f.setUser(u, map[string]any{"discount": 10, "u": 5, "d": 5})
	coupon := &model.Coupon{Code: "SAVE2", Name: "Save", Type: model.CouponTypeAmount, Value: 200, Show: true, LimitUse: ptr(5),
		StartedAt: f.now.Add(-time.Hour), EndedAt: f.now.Add(time.Hour)}
	if err := f.a.DB.Create(coupon).Error; err != nil {
		t.Fatal(err)
	}
	pay := f.payment(true, func(p *model.Payment) { p.HandlingFeeFixed, p.HandlingFeePercent = 10, 1.5 })

	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly, CouponCode: " SAVE2 "})
	if err != nil {
		t.Fatal(err)
	}
	// 1000 − (200 coupon + 100 user discount) − 150 balance = 550.
	if o.Status != model.OrderStatusPending || o.Type != model.OrderTypeNew || o.DiscountAmount != 300 || o.BalanceAmount != 150 || o.TotalAmount != 550 || o.CouponID == nil {
		t.Fatalf("order = %+v", o)
	}
	if len(o.TradeNo) != 20 || !strings.HasPrefix(o.TradeNo, f.now.Format("20060102")) {
		t.Fatalf("trade_no = %s", o.TradeNo)
	}
	if got := f.reload(u); got.Balance != 0 {
		t.Fatalf("balance = %d", got.Balance)
	}
	var c model.Coupon
	f.a.DB.First(&c, coupon.ID)
	if *c.LimitUse != 4 {
		t.Fatalf("limit_use = %d", *c.LimitUse)
	}
	// One pending order at a time.
	_, err = f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodYearly})
	if status(err) != 409 || !strings.Contains(strings.Join(fieldErrs(err)["trade_no"], ","), o.TradeNo) {
		t.Fatalf("second order: %v", err)
	}

	// Checkout.
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{}); status(err) != 400 {
		t.Fatalf("missing payment_id: %v", err)
	}
	if _, err := f.svc.Checkout(ctx, u.ID+999, o.TradeNo, CheckoutInput{PaymentID: &pay.ID}); status(err) != 404 {
		t.Fatalf("foreign order: %v", err)
	}
	res, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &pay.ID, ClientIP: "203.0.113.5"})
	if err != nil {
		t.Fatal(err)
	}
	// handling = 10 + round(550 × 1.5%) = 10 + 8 = 18.
	req := fake.last()
	if res.Type != payment.PayTypeURL || req.TotalAmount != 568 || req.Subject != "GG Bond - Pro" || req.Currency != "CNY" ||
		req.NotifyURL != "https://panel.example.com/api/v1/guest/payment/notify/"+fakeName+"/"+pay.UUID ||
		req.ReturnURL != "https://panel.example.com/api/v1/guest/payment/return/"+o.TradeNo || req.UserEmail != "buyer@example.com" || req.ClientIP != "203.0.113.5" {
		t.Fatalf("pay request = %+v", req)
	}
	if d := req.ExpiresAt.Sub(f.order(o.TradeNo).CreatedAt); d != 30*time.Minute {
		t.Fatalf("expires after %s", d)
	}
	if got := f.order(o.TradeNo); got.HandlingAmount != 18 || got.PaymentID == nil || *got.PaymentID != pay.ID {
		t.Fatalf("order after checkout = %+v", got)
	}

	// A pending event is acknowledged without paying.
	if resp := f.notify(pay, fakeName, "s3cret", `{"event":"pending"}`); resp.Body != "fake-ok" || f.order(o.TradeNo).Status != model.OrderStatusPending {
		t.Fatalf("pending event: %+v", resp)
	}
	// Paid notify (Xboard class alias accepted in the path).
	resp := f.notify(pay, fakeName, "s3cret", `{"trade_no":"`+o.TradeNo+`","callback_no":"txn-1","amount":568}`)
	if resp.Status != 200 || resp.Body != "fake-ok" {
		t.Fatalf("notify = %+v", resp)
	}
	paid := f.order(o.TradeNo)
	if paid.Status != model.OrderStatusCompleted || paid.CallbackNo != "txn-1" || paid.PaidAt == nil {
		t.Fatalf("paid order = %+v", paid)
	}
	got := f.reload(u)
	wantExp := paid.PaidAt.Local().AddDate(0, 1, 0) // calendar months in the server time zone
	if got.PlanID == nil || *got.PlanID != plan.ID || got.GroupID == nil || *got.GroupID != 7 || got.TransferEnable != 100*model.BytesPerGB ||
		got.ExpiredAt == nil || !got.ExpiredAt.Equal(wantExp) || got.U != 0 || got.D != 0 ||
		got.SpeedLimit == nil || *got.SpeedLimit != 100 || got.DeviceLimit == nil || *got.DeviceLimit != 3 || got.NextResetAt == nil {
		t.Fatalf("user after fulfil = %+v (exp want %s)", got, wantExp)
	}
	msgs := testutil.Mails(t, f.a)
	if len(msgs) != 1 || msgs[0].To != "buyer@example.com" || msgs[0].Template != mail.TplOrderPaid || !strings.Contains(msgs[0].Text, o.TradeNo) || !strings.Contains(msgs[0].Text, "¥7.18") {
		t.Fatalf("mails = %+v", msgs)
	}
	// Idempotent replay and a different transaction (double payment warning) keep the order unchanged.
	f.notify(pay, fakeName, "s3cret", `{"trade_no":"`+o.TradeNo+`","callback_no":"txn-1","amount":568}`)
	f.notify(pay, fakeName, "s3cret", `{"trade_no":"`+o.TradeNo+`","callback_no":"txn-2","amount":568}`)
	if again := f.order(o.TradeNo); again.CallbackNo != "txn-1" || !again.PaidAt.Equal(*paid.PaidAt) {
		t.Fatalf("replay changed the order: %+v", again)
	}
	if got2 := f.reload(u); !got2.ExpiredAt.Equal(*got.ExpiredAt) {
		t.Fatal("replay extended the subscription")
	}
	if n := len(testutil.Mails(t, f.a)); n != 1 {
		t.Fatalf("replay sent mail: %d", n)
	}
	// Checkout of a paid order reports it paid.
	if res, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &pay.ID}); err != nil || res.Type != PayTypePaid {
		t.Fatalf("checkout paid = %+v %v", res, err)
	}

	// Renewal keeps usage and extends from the current expiry.
	f.setUser(u, map[string]any{"u": 1000, "balance": 0})
	o2, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodYearly})
	if err != nil || o2.Type != model.OrderTypeRenew {
		t.Fatalf("renew = %+v %v", o2, err)
	}
	if _, err := f.svc.MarkPaid(ctx, o2.ID); err != nil {
		t.Fatal(err)
	}
	renewed := f.reload(u)
	if renewed.U != 1000 || !renewed.ExpiredAt.Equal(got.ExpiredAt.Local().AddDate(1, 0, 0)) || f.order(o2.TradeNo).CallbackNo != CallbackManual {
		t.Fatalf("renewed = %+v", renewed)
	}
	if _, err := f.svc.MarkPaid(ctx, o2.ID); status(err) != 409 {
		t.Fatalf("mark paid twice: %v", err)
	}
}

func TestCheckoutErrors(t *testing.T) {
	f := newFixture(t)
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	u := f.user("e@example.com")
	disabled := f.payment(false)
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &disabled.ID}); status(err) != 400 {
		t.Fatalf("disabled payment: %v", err)
	}
	missing := uint64(9999)
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &missing}); status(err) != 400 {
		t.Fatalf("missing payment: %v", err)
	}
	enabled := f.payment(true, func(p *model.Payment) { p.UUID = strings.Repeat("cd", 16) })
	fake.mu.Lock()
	fake.payErr = &gwutil.UserError{Msg: "amount is below the minimum"}
	fake.mu.Unlock()
	_, err = f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &enabled.ID})
	fake.mu.Lock()
	fake.payErr = nil
	fake.mu.Unlock()
	if status(err) != 402 || !strings.Contains(err.Error(), "below the minimum") {
		t.Fatalf("gateway error: %v", err)
	}
	if _, err := f.svc.Cancel(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &enabled.ID}); status(err) != 409 {
		t.Fatalf("cancelled checkout: %v", err)
	}
	if _, err := f.svc.Cancel(ctx, o.ID); status(err) != 409 {
		t.Fatalf("cancel twice: %v", err)
	}
	if _, err := f.svc.Cancel(ctx, 424242); status(err) != 404 {
		t.Fatalf("cancel missing: %v", err)
	}
}

func TestPurchaseRules(t *testing.T) {
	f := newFixture(t)
	pro := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000, model.PeriodResetTraffic: 300})
	hidden := f.plan("Hidden", map[string]int64{model.PeriodMonthly: 500}, func(p *model.Plan) { p.Show = false })
	legacy := f.plan("Legacy", map[string]int64{model.PeriodMonthly: 500}, func(p *model.Plan) { p.Sell = false })
	noRenew := f.plan("NoRenew", map[string]int64{model.PeriodMonthly: 500}, func(p *model.Plan) { p.Renew = false })
	limited := f.plan("Limited", map[string]int64{model.PeriodMonthly: 500}, func(p *model.Plan) { p.CapacityLimit = ptr(1) })
	future := f.now.AddDate(0, 1, 0)
	past := f.now.AddDate(0, -1, 0)

	newUser := func(email string, planID *uint64, exp *time.Time) *model.User {
		u := f.user(email, usersvc.CreateOptions{PlanID: planID, ExpiredAt: exp, TransferEnable: model.BytesPerGB})
		return u
	}
	check := func(name string, u *model.User, in CreateInput, want int) {
		t.Helper()
		o, err := f.svc.Create(ctx, u.ID, in)
		if status(err) != want {
			t.Fatalf("%s: status %d (%v), want %d", name, status(err), err, want)
		}
		if err == nil {
			if _, err := f.svc.Cancel(ctx, o.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	buyer := newUser("buyer@example.com", nil, nil)
	check("unknown plan", buyer, CreateInput{PlanID: 9999, Period: model.PeriodMonthly}, 404)
	check("unknown period", buyer, CreateInput{PlanID: pro.ID, Period: "weekly"}, 400)
	check("period not sold", buyer, CreateInput{PlanID: pro.ID, Period: model.PeriodYearly}, 400)
	check("hidden plan", buyer, CreateInput{PlanID: hidden.ID, Period: model.PeriodMonthly}, 404)
	check("legacy plan", buyer, CreateInput{PlanID: legacy.ID, Period: model.PeriodMonthly}, 400)
	check("reset without plan", buyer, CreateInput{PlanID: pro.ID, Period: model.PeriodResetTraffic}, 400)
	check("ok", buyer, CreateInput{PlanID: pro.ID, Period: model.PeriodMonthly}, 0)
	check("bad coupon", buyer, CreateInput{PlanID: pro.ID, Period: model.PeriodMonthly, CouponCode: "NOPE"}, 400)

	holder := newUser("holder@example.com", &pro.ID, &future)
	check("reset traffic", holder, CreateInput{PlanID: pro.ID, Period: model.PeriodResetTraffic}, 0)
	hiddenHolder := newUser("hh@example.com", &hidden.ID, &future)
	check("hidden holder renews", hiddenHolder, CreateInput{PlanID: hidden.ID, Period: model.PeriodMonthly}, 0)
	legacyActive := newUser("la@example.com", &legacy.ID, &future)
	check("legacy holder renews", legacyActive, CreateInput{PlanID: legacy.ID, Period: model.PeriodMonthly}, 0)
	legacyExpired := newUser("le@example.com", &legacy.ID, &past)
	check("legacy expired", legacyExpired, CreateInput{PlanID: legacy.ID, Period: model.PeriodMonthly}, 400)
	nr := newUser("nr@example.com", &noRenew.ID, &future)
	check("renew disabled", nr, CreateInput{PlanID: noRenew.ID, Period: model.PeriodMonthly}, 400)

	// Capacity: one active holder fills the plan; the holder may still renew.
	full := newUser("full@example.com", &limited.ID, &future)
	check("sold out", buyer, CreateInput{PlanID: limited.ID, Period: model.PeriodMonthly}, 409)
	check("holder renews full plan", full, CreateInput{PlanID: limited.ID, Period: model.PeriodMonthly}, 0)
	f.setUser(full, map[string]any{"expired_at": past})
	check("expired holder frees capacity", buyer, CreateInput{PlanID: limited.ID, Period: model.PeriodMonthly}, 0)

	// Upgrades need plan_change_enable.
	if err := f.a.Settings.Set(map[string]any{"plan_change_enable": false}); err != nil {
		t.Fatal(err)
	}
	other := f.plan("Other", map[string]int64{model.PeriodMonthly: 100})
	check("plan change disabled", holder, CreateInput{PlanID: other.ID, Period: model.PeriodMonthly}, 403)
}

func TestFreeAndBalanceOrdersArePaidImmediately(t *testing.T) {
	f := newFixture(t)
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000, model.PeriodOnetime: 0})
	rich := f.user("rich@example.com", usersvc.CreateOptions{Balance: 5000})
	o, err := f.svc.Create(ctx, rich.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != model.OrderStatusCompleted || o.CallbackNo != CallbackBalance || o.BalanceAmount != 1000 || o.TotalAmount != 0 {
		t.Fatalf("balance order = %+v", o)
	}
	if got := f.reload(rich); got.Balance != 4000 || got.PlanID == nil {
		t.Fatalf("rich = %+v", got)
	}
	poor := f.user("poor@example.com")
	o, err = f.svc.Create(ctx, poor.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodOnetime})
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != model.OrderStatusCompleted || o.CallbackNo != CallbackFree {
		t.Fatalf("free order = %+v", o)
	}
	if got := f.reload(poor); got.ExpiredAt != nil || got.TransferEnable != 100*model.BytesPerGB || got.NextResetAt != nil {
		t.Fatalf("onetime user = %+v", got)
	}
}

func TestUpgradeWithSurplusRefund(t *testing.T) {
	f := newFixture(t)
	big := f.plan("Big", map[string]int64{model.PeriodYearly: 12000})
	small := f.plan("Small", map[string]int64{model.PeriodMonthly: 500}, func(p *model.Plan) { p.GroupID = 9 })
	u := f.user("up@example.com")
	f.svc.Now = func() time.Time { return f.now.AddDate(0, -1, 0) }
	o1, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: big.ID, Period: model.PeriodYearly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.MarkPaid(ctx, o1.ID); err != nil {
		t.Fatal(err)
	}
	f.svc.Now = func() time.Time { return f.now }
	f.setUser(u, map[string]any{"u": 777})
	o2, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: small.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	// ~11/12 of 120.00 remains: far above 5.00 → free upgrade with the rest refunded to the balance.
	if o2.Type != model.OrderTypeUpgrade || o2.Status != model.OrderStatusCompleted || o2.SurplusAmount < 10000 || o2.RefundAmount != o2.SurplusAmount-500 ||
		len(o2.SurplusOrderIDs) != 1 || o2.SurplusOrderIDs[0] != o1.ID {
		t.Fatalf("upgrade = %+v", o2)
	}
	got := f.reload(u)
	if got.Balance != o2.RefundAmount || *got.PlanID != small.ID || *got.GroupID != 9 || got.U != 0 ||
		!got.ExpiredAt.Equal(o2.PaidAt.Local().AddDate(0, 1, 0)) {
		t.Fatalf("user = %+v", got)
	}
	if f.order(o1.TradeNo).Status != model.OrderStatusDiscounted {
		t.Fatal("surplus order must be discounted")
	}
	// surplus_enable=false: full price.
	if err := f.a.Settings.Set(map[string]any{"surplus_enable": false}); err != nil {
		t.Fatal(err)
	}
	o3, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: big.ID, Period: model.PeriodYearly})
	if err != nil {
		t.Fatal(err)
	}
	if o3.SurplusAmount != 0 || o3.Type != model.OrderTypeUpgrade || o3.BalanceAmount != min(got.Balance, 12000) {
		t.Fatalf("no surplus = %+v", o3)
	}
}

func TestTimeoutJobRestoresBalanceAndCoupon(t *testing.T) {
	f := newFixture(t)
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	u := f.user("late@example.com", usersvc.CreateOptions{Balance: 300})
	coupon := &model.Coupon{Code: "ONCE", Name: "Once", Type: model.CouponTypePercent, Value: 10, Show: true, LimitUse: ptr(1),
		StartedAt: f.now.Add(-time.Hour), EndedAt: f.now.Add(time.Hour)}
	f.a.DB.Create(coupon)
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly, CouponCode: "ONCE"})
	if err != nil {
		t.Fatal(err)
	}
	other := f.user("other@example.com")
	if _, err := f.svc.Create(ctx, other.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly, CouponCode: "ONCE"}); status(err) != 400 {
		t.Fatalf("exhausted coupon: %v", err)
	}
	fresh, err := f.svc.Create(ctx, other.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	f.a.DB.Model(&model.Order{}).Where("id = ?", o.ID).UpdateColumn("created_at", time.Now().Add(-31*time.Minute))
	if err := f.a.Scheduler.RunNow(JobOrderTimeout); err != nil {
		t.Fatal(err)
	}
	if f.order(o.TradeNo).Status != model.OrderStatusCancelled || f.order(fresh.TradeNo).Status != model.OrderStatusPending {
		t.Fatal("only the stale order is cancelled")
	}
	if got := f.reload(u); got.Balance != 300 {
		t.Fatalf("balance = %d", got.Balance)
	}
	var c model.Coupon
	f.a.DB.First(&c, coupon.ID)
	if *c.LimitUse != 1 {
		t.Fatalf("coupon use not restored: %d", *c.LimitUse)
	}
	if n, err := f.svc.CancelExpired(ctx); err != nil || n != 0 {
		t.Fatalf("second run = %d %v", n, err)
	}
}
