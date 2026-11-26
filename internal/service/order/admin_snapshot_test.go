package ordersvc

import (
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
)

// An administrator demoted or banned while a request is in flight gets 403 from every order action (docs/API.md
// §Admin orders), a changed account binding 401; nothing is changed either way.
func TestDemotedAdminCannotChangeOrders(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  map[string]any
		restore map[string]any
		want    int
	}{
		{"demoted", map[string]any{"is_admin": false}, map[string]any{"is_admin": true}, 403},
		{"banned", map[string]any{"banned": true}, map[string]any{"banned": false}, 403},
		{"password", map[string]any{"password_version": 7}, map[string]any{"password_version": 0}, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			f.setUser(admin, tc.change)
			if _, err := f.svc.MarkPaidAs(ctx, o.ID, actor); status(err) != tc.want {
				t.Fatalf("marked order paid: %v", err)
			}
			if _, err := f.svc.CancelForAdmin(ctx, o.ID, actor); status(err) != tc.want {
				t.Fatalf("cancelled order: %v", err)
			}
			if _, err := f.svc.SetCommissionStatusAs(ctx, o.ID, model.CommissionStatusProcessing, actor); status(err) != tc.want {
				t.Fatalf("approved commission: %v", err)
			}
			other := f.user("other@example.com")
			if _, err := f.svc.Assign(ctx, AssignInput{UserID: other.ID, PlanID: plan.ID, Period: model.PeriodMonthly, Admin: actor}); status(err) != tc.want {
				t.Fatalf("assigned order: %v", err)
			}
			if got := f.order(o.TradeNo); got.Status != model.OrderStatusPending || got.CommissionStatus != model.CommissionStatusPending || f.reload(u).PlanID != nil {
				t.Fatalf("changed order or entitlement: %+v", got)
			}
			var count int64
			if err := f.a.DB.Model(&model.Order{}).Where("user_id = ?", other.ID).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("assigned an order: count=%d err=%v", count, err)
			}
			f.setUser(admin, tc.restore)
			if paid, err := f.svc.MarkPaidAs(ctx, o.ID, actor); err != nil || paid == nil || paid.Status != model.OrderStatusCompleted {
				t.Fatalf("restored admin could not mark paid: order=%+v err=%v", paid, err)
			}
		})
	}
}
