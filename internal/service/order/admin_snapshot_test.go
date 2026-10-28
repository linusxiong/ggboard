package ordersvc

import (
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
)

func TestDemotedAdminCannotChangeOrders(t *testing.T) {
	f := newFixture(t)
	admin := f.user("admin@example.com")
	f.setUser(admin, map[string]any{"is_admin": true})
	actor := testActor(f.reload(admin))
	u := f.user("buyer@example.com")
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	f.setUser(admin, map[string]any{"is_admin": false})
	if _, err := f.svc.MarkPaidAs(ctx, o.ID, actor); status(err) != 401 {
		t.Fatalf("demoted admin marked order paid: %v", err)
	}
	if _, err := f.svc.CancelForAdmin(ctx, o.ID, actor); status(err) != 401 {
		t.Fatalf("demoted admin cancelled order: %v", err)
	}
	if _, err := f.svc.SetCommissionStatusAs(ctx, o.ID, model.CommissionStatusProcessing, actor); status(err) != 401 {
		t.Fatalf("demoted admin approved commission: %v", err)
	}
	other := f.user("other@example.com")
	if _, err := f.svc.Assign(ctx, AssignInput{UserID: other.ID, PlanID: plan.ID, Period: model.PeriodMonthly, Admin: actor}); status(err) != 401 {
		t.Fatalf("demoted admin assigned order: %v", err)
	}
	if got := f.order(o.TradeNo); got.Status != model.OrderStatusPending || got.CommissionStatus != model.CommissionStatusPending || f.reload(u).PlanID != nil {
		t.Fatalf("demoted admin changed order or entitlement: %+v", got)
	}
	var count int64
	if err := f.a.DB.Model(&model.Order{}).Where("user_id = ?", other.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("demoted admin assigned an order: count=%d err=%v", count, err)
	}
	f.setUser(admin, map[string]any{"is_admin": true})
	if paid, err := f.svc.MarkPaidAs(ctx, o.ID, actor); err != nil || paid == nil || paid.Status != model.OrderStatusCompleted {
		t.Fatalf("restored admin could not mark paid: order=%+v err=%v", paid, err)
	}
}
