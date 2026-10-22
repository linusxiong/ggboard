package ordersvc

import (
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

func TestAssign(t *testing.T) {
	f := newFixture(t)
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	inviter := f.user("inv@example.com")
	u := f.user("Assigned@Example.com", usersvc.CreateOptions{InviteUserID: &inviter.ID, Balance: 5000})

	if _, err := f.svc.Assign(ctx, AssignInput{Email: "nobody@example.com", PlanID: plan.ID, Period: model.PeriodMonthly}); status(err) != 400 {
		t.Fatalf("unknown email: %v", err)
	}
	if _, err := f.svc.Assign(ctx, AssignInput{UserID: 9999, PlanID: plan.ID, Period: model.PeriodMonthly}); status(err) != 400 {
		t.Fatalf("unknown user: %v", err)
	}
	if _, err := f.svc.Assign(ctx, AssignInput{UserID: u.ID, PlanID: plan.ID, Period: model.PeriodYearly}); status(err) != 400 {
		t.Fatalf("no price: %v", err)
	}
	if _, err := f.svc.Assign(ctx, AssignInput{UserID: u.ID, PlanID: plan.ID, Period: model.PeriodYearly, TotalAmount: ptr(int64(-1))}); status(err) != 400 {
		t.Fatalf("negative total: %v", err)
	}
	o, err := f.svc.Assign(ctx, AssignInput{Email: " assigned@example.com ", PlanID: plan.ID, Period: model.PeriodYearly, TotalAmount: ptr(int64(2500))})
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != model.OrderStatusPending || o.TotalAmount != 2500 || o.BalanceAmount != 0 || o.Type != model.OrderTypeNew ||
		o.InviteUserID == nil || *o.InviteUserID != inviter.ID {
		t.Fatalf("assigned = %+v", o)
	}
	if f.reload(u).Balance != 5000 {
		t.Fatal("assign must not touch the balance")
	}
	if _, err := f.svc.Assign(ctx, AssignInput{UserID: u.ID, PlanID: plan.ID, Period: model.PeriodMonthly}); status(err) != 409 {
		t.Fatalf("pending conflict: %v", err)
	}
	paid, err := f.svc.MarkPaid(ctx, o.ID)
	if err != nil || paid.CallbackNo != CallbackManual || paid.CommissionBalance != 250 {
		t.Fatalf("mark paid = %+v %v", paid, err)
	}

	views, err := AdminViews(ctx, f.a.DB, []model.Order{*paid})
	if err != nil || len(views) != 1 || views[0].User == nil || views[0].User.Email != "assigned@example.com" || views[0].Plan == nil ||
		views[0].Plan.Name != "Pro" || views[0].CommissionBalance != 250 || views[0].SurplusOrderIDs == nil {
		t.Fatalf("admin views = %+v %v", views, err)
	}
	uv, err := Views(ctx, f.a.DB, []model.Order{*paid})
	if err != nil || len(uv) != 1 || uv[0].Payment != nil || uv[0].Plan.ID != plan.ID {
		t.Fatalf("views = %+v %v", uv, err)
	}
}

func TestCheckCoupon(t *testing.T) {
	f := newFixture(t)
	pro := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000, model.PeriodYearly: 9000})
	basic := f.plan("Basic", map[string]int64{model.PeriodMonthly: 500})
	u := f.user("c@example.com")
	mk := func(code string, mut func(*model.Coupon)) {
		c := &model.Coupon{Code: code, Name: code, Type: model.CouponTypePercent, Value: 10, Show: true,
			StartedAt: f.now.Add(-time.Hour), EndedAt: f.now.Add(time.Hour)}
		if mut != nil {
			mut(c)
		}
		if err := f.a.DB.Create(c).Error; err != nil {
			t.Fatal(err)
		}
	}
	mk("OK", nil)
	mk("HIDDEN", func(c *model.Coupon) { c.Show = false })
	mk("USED", func(c *model.Coupon) { c.LimitUse = ptr(0) })
	mk("LATER", func(c *model.Coupon) { c.StartedAt = f.now.Add(time.Hour); c.EndedAt = f.now.Add(2 * time.Hour) })
	mk("OVER", func(c *model.Coupon) { c.StartedAt = f.now.Add(-2 * time.Hour); c.EndedAt = f.now.Add(-time.Hour) })
	mk("PROONLY", func(c *model.Coupon) { c.LimitPlanIDs = model.JSONSlice[uint64]{pro.ID} })
	mk("YEARLY", func(c *model.Coupon) { c.LimitPeriods = model.JSONSlice[string]{model.PeriodYearly} })
	mk("ONCE", func(c *model.Coupon) { c.LimitUseWithUser = ptr(1) })
	for code, want := range map[string]int{
		"OK": 0, "HIDDEN": 400, "USED": 400, "LATER": 400, "OVER": 400, "NOPE": 400, "": 400, "ONCE": 0,
	} {
		if _, err := f.svc.CheckCoupon(ctx, u.ID, code, pro.ID, model.PeriodMonthly); status(err) != want {
			t.Errorf("%q: status %d (%v), want %d", code, status(err), err, want)
		}
	}
	if _, err := f.svc.CheckCoupon(ctx, u.ID, "PROONLY", basic.ID, model.PeriodMonthly); status(err) != 400 {
		t.Fatalf("plan restriction: %v", err)
	}
	if _, err := f.svc.CheckCoupon(ctx, u.ID, "PROONLY", pro.ID, model.PeriodMonthly); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CheckCoupon(ctx, u.ID, "YEARLY", pro.ID, model.PeriodMonthly); status(err) != 400 {
		t.Fatalf("period restriction: %v", err)
	}
	if _, err := f.svc.CheckCoupon(ctx, u.ID, "OK", 9999, model.PeriodMonthly); status(err) != 400 {
		t.Fatalf("unknown plan: %v", err)
	}
	if _, err := f.svc.CheckCoupon(ctx, u.ID, "OK", basic.ID, model.PeriodYearly); status(err) != 400 {
		t.Fatalf("unknown period: %v", err)
	}
	// limit_use_with_user counts paid orders only.
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: pro.ID, Period: model.PeriodMonthly, CouponCode: "ONCE"})
	if err != nil || o.DiscountAmount != 100 {
		t.Fatalf("order = %+v %v", o, err)
	}
	if _, err := f.svc.CheckCoupon(ctx, u.ID, "ONCE", pro.ID, model.PeriodMonthly); err != nil {
		t.Fatalf("pending order does not count: %v", err)
	}
	if _, err := f.svc.MarkPaid(ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CheckCoupon(ctx, u.ID, "ONCE", pro.ID, model.PeriodMonthly); status(err) != 400 {
		t.Fatalf("per-user limit: %v", err)
	}
	var once model.Coupon
	f.a.DB.Where("code = ?", "ONCE").First(&once)
	usage, err := CouponUsage(ctx, f.a.DB, []uint64{once.ID, 9999})
	if err != nil || usage[once.ID] != 1 || usage[9999] != 0 {
		t.Fatalf("usage = %v %v", usage, err)
	}
}

func TestHelpers(t *testing.T) {
	f := newFixture(t)
	p := &model.Payment{Gateway: "epay", UUID: "u1"}
	if got := f.svc.PaymentNotifyURL(p, "http://fallback.example.com"); got != "https://panel.example.com/api/v1/guest/payment/notify/epay/u1" {
		t.Fatalf("notify url = %s", got)
	}
	p.NotifyDomain = "https://cb.example.com/"
	if got := f.svc.PaymentNotifyURL(p, ""); got != "https://cb.example.com/api/v1/guest/payment/notify/epay/u1" {
		t.Fatalf("notify domain url = %s", got)
	}
	if err := f.a.Settings.Set(map[string]any{"app_url": ""}); err != nil {
		t.Fatal(err)
	}
	if got := f.svc.OrderPageURL("T1", "http://h.example.com/"); got != "http://h.example.com/orders/T1" {
		t.Fatalf("order page = %s", got)
	}
	for _, c := range []struct {
		fixed int64
		pct   float64
		total int64
		want  int64
	}{{0, 0, 1000, 0}, {10, 0, 1000, 10}, {0, 2.5, 1000, 25}, {5, 1.5, 550, 13}, {0, 0.1, 5, 0}} {
		if got := HandlingAmount(&model.Payment{HandlingFeeFixed: c.fixed, HandlingFeePercent: c.pct}, c.total); got != c.want {
			t.Errorf("handling(%d, %v, %d) = %d, want %d", c.fixed, c.pct, c.total, got, c.want)
		}
	}
	a, b := NewTradeNo(time.Now()), NewTradeNo(time.Now())
	if len(a) != 20 || a == b {
		t.Fatalf("trade numbers %s %s", a, b)
	}
}
