package ordersvc

import (
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
)

func TestImportedOverflowPlanCannotBePurchasedOrAssigned(t *testing.T) {
	f := newFixture(t)
	u := f.user("buyer@example.com")
	plan := f.plan("Corrupt import", map[string]int64{model.PeriodMonthly: 1000}, func(p *model.Plan) {
		p.TransferEnable = 1<<35 + 1 // previously wrapped to one positive GB
	})
	f.setUser(u, map[string]any{"balance": 1000})
	if _, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly}); status(err) != 400 {
		t.Fatalf("invalid plan purchase: %v", err)
	}
	if _, err := f.svc.Assign(ctx, AssignInput{UserID: u.ID, PlanID: plan.ID, Period: model.PeriodMonthly}); status(err) != 400 {
		t.Fatalf("invalid plan assignment: %v", err)
	}
	var orders int64
	if err := f.a.DB.Model(&model.Order{}).Where("user_id = ?", u.ID).Count(&orders).Error; err != nil {
		t.Fatal(err)
	}
	if got := f.reload(u); got.Balance != 1000 || got.PlanID != nil || orders != 0 {
		t.Fatalf("invalid plan charged or granted access: user=%+v orders=%d", got, orders)
	}
}
