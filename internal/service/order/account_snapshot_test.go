package ordersvc

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
)

func testActor(u *model.User) *auth.Claims {
	return &auth.Claims{UUID: u.UUID, Email: u.Email, Ver: u.PasswordVersion,
		RegisteredClaims: jwt.RegisteredClaims{Subject: strconv.FormatUint(u.ID, 10)}}
}

func TestCheckoutUsesEmailFromVerifiedAccountSnapshot(t *testing.T) {
	f := newFixture(t)
	u := f.user("buyer@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	p := f.payment(true)
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	const hook = "test:replace_after_checkout"
	if err := f.a.DB.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table != "plans" || !changed.CompareAndSwap(false, true) {
			return
		}
		if err := f.a.DB.Model(&model.User{}).Where("id = ?", u.ID).
			Updates(map[string]any{"uuid": "replacement-account-uuid", "email": "replacement@example.com"}).Error; err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.a.DB.Callback().Query().Remove(hook) })
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &p.ID, Actor: testActor(u)}); err != nil {
		t.Fatal(err)
	}
	if !changed.Load() || fake.last().UserEmail != u.Email {
		t.Fatalf("checkout exposed replacement account email: changed=%t sent=%q", changed.Load(), fake.last().UserEmail)
	}
}

func TestOldAccountSnapshotCannotSpendBalanceOrAlterOrder(t *testing.T) {
	f := newFixture(t)
	u := f.user("buyer@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	p := f.payment(true)
	actor := testActor(u)
	f.setUser(u, map[string]any{"balance": 1000, "password_version": u.PasswordVersion + 1})
	if _, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly, Actor: actor}); status(err) != 401 {
		t.Fatalf("stale create: %v", err)
	}
	var count int64
	if err := f.a.DB.Model(&model.Order{}).Where("user_id = ?", u.ID).Count(&count).Error; err != nil || count != 0 || f.reload(u).Balance != 1000 {
		t.Fatalf("stale create changed orders or balance: count=%d err=%v", count, err)
	}

	f.setUser(u, map[string]any{"balance": 500})
	current := f.reload(u)
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	if o.TotalAmount != 500 || o.BalanceAmount != 500 {
		t.Fatalf("order quote: %+v", o)
	}
	actor = testActor(current)
	f.setUser(u, map[string]any{"uuid": "replacement-account-instance"})
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{PaymentID: &p.ID, Actor: actor}); status(err) != 401 {
		t.Fatalf("stale checkout: %v", err)
	}
	if got := f.order(o.TradeNo); got.PaymentID != nil || got.Status != model.OrderStatusPending {
		t.Fatalf("stale checkout changed order: %+v", got)
	}
	if _, err := f.svc.CancelForUser(ctx, o.ID, actor); status(err) != 401 {
		t.Fatalf("stale cancel: %v", err)
	}
	if got := f.order(o.TradeNo); got.Status != model.OrderStatusPending || f.reload(u).Balance != 0 {
		t.Fatalf("stale cancel changed order or balance: %+v", got)
	}
}

func TestOldAccountSnapshotCannotFulfilPendingFreeOrder(t *testing.T) {
	f := newFixture(t)
	u := f.user("buyer@example.com")
	plan := f.plan("Free", map[string]int64{model.PeriodMonthly: 0})
	o := &model.Order{TradeNo: "20261028000000999999", UserID: u.ID, PlanID: plan.ID,
		Type: model.OrderTypeNew, Period: model.PeriodMonthly, Status: model.OrderStatusPending}
	if err := f.a.DB.Create(o).Error; err != nil {
		t.Fatal(err)
	}
	actor := testActor(u)
	f.setUser(u, map[string]any{"email": "new@example.com"})
	if _, err := f.svc.Checkout(ctx, u.ID, o.TradeNo, CheckoutInput{Actor: actor}); status(err) != 401 {
		t.Fatalf("stale free checkout: %v", err)
	}
	if f.order(o.TradeNo).Status != model.OrderStatusPending || f.reload(u).PlanID != nil {
		t.Fatal("stale checkout fulfilled free order")
	}
}

func TestOldAccountSnapshotCannotReadCouponEligibility(t *testing.T) {
	f := newFixture(t)
	u := f.user("buyer@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	coupon := &model.Coupon{Code: "SAVE", Name: "Private", Type: model.CouponTypeAmount, Value: 100,
		Show: true, StartedAt: f.now.Add(-time.Hour), EndedAt: f.now.Add(time.Hour)}
	if err := f.a.DB.Create(coupon).Error; err != nil {
		t.Fatal(err)
	}
	actor := testActor(u)
	f.setUser(u, map[string]any{"uuid": "replacement-account-uuid"})
	if _, err := f.svc.CheckCouponForUser(ctx, u.ID, coupon.Code, plan.ID, model.PeriodMonthly, actor); status(err) != 401 {
		t.Fatalf("stale coupon check: %v", err)
	}
}

// Security review 2026-10-29: a user's cancel used to flip any pending order to cancelled first and only then check the
// owner, relying on the rollback. It now matches only the actor's own orders: another user's order is never updated.
func TestCancelForUserNeverTouchesAnotherUsersOrder(t *testing.T) {
	f := newFixture(t)
	owner := f.user("owner@example.com")
	other := f.user("other@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	o, err := f.svc.Create(ctx, owner.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	var touched atomic.Int64
	const cb = "test:count_order_updates"
	if err := f.a.DB.Callback().Update().After("gorm:update").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "orders" {
			touched.Add(tx.RowsAffected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.a.DB.Callback().Update().Remove(cb) })

	if _, err := f.svc.CancelForUser(ctx, o.ID, testActor(other)); status(err) != 404 {
		t.Fatalf("foreign cancel: %v, want 404", err)
	}
	if n := touched.Load(); n != 0 {
		t.Fatalf("foreign cancel updated %d order rows before failing", n)
	}
	if got := f.order(o.TradeNo); got.Status != model.OrderStatusPending {
		t.Fatalf("foreign cancel changed the order: %+v", got)
	}
	if _, err := f.svc.CancelForUser(ctx, o.ID, testActor(owner)); err != nil {
		t.Fatalf("own cancel: %v", err)
	}
	if touched.Load() == 0 {
		t.Fatal("the update counter never saw the owner's cancel; the assertion above proves nothing")
	}
}
