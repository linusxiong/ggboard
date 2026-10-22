package ordersvc

import (
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// buy creates and manually pays an order of price for invitee.
func (f *fixture) buy(u *model.User, plan *model.Plan, period string) *model.Order {
	f.t.Helper()
	o, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: period})
	if err != nil {
		f.t.Fatal(err)
	}
	if o.Status == model.OrderStatusPending {
		if o, err = f.svc.MarkPaid(ctx, o.ID); err != nil {
			f.t.Fatal(err)
		}
	}
	return f.order(o.TradeNo)
}

func TestCommissionRules(t *testing.T) {
	f := newFixture(t)
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000, model.PeriodYearly: 10000})
	inviter := f.user("inviter@example.com")

	// System type + commission_first_time_enable (default true): only the first paid order.
	sys := f.user("sys@example.com", usersvc.CreateOptions{InviteUserID: &inviter.ID})
	o1 := f.buy(sys, plan, model.PeriodMonthly)
	if o1.InviteUserID == nil || *o1.InviteUserID != inviter.ID || o1.CommissionBalance != 100 || o1.CommissionStatus != model.CommissionStatusPending {
		t.Fatalf("first order = %+v", o1)
	}
	if o2 := f.buy(sys, plan, model.PeriodMonthly); o2.CommissionBalance != 0 {
		t.Fatalf("second order commission = %d", o2.CommissionBalance)
	}
	// commission_first_time_enable=false: every order.
	if err := f.a.Settings.Set(map[string]any{"commission_first_time_enable": false}); err != nil {
		t.Fatal(err)
	}
	if o3 := f.buy(sys, plan, model.PeriodMonthly); o3.CommissionBalance != 100 {
		t.Fatalf("every order commission = %d", o3.CommissionBalance)
	}

	// Period type with a rate override: every order at 20 %.
	periodInviter := f.user("period@example.com")
	f.setUser(periodInviter, map[string]any{"commission_type": model.CommissionTypePeriod, "commission_rate": 20})
	pu := f.user("pu@example.com", usersvc.CreateOptions{InviteUserID: &periodInviter.ID})
	if a, b := f.buy(pu, plan, model.PeriodMonthly), f.buy(pu, plan, model.PeriodMonthly); a.CommissionBalance != 200 || b.CommissionBalance != 200 {
		t.Fatalf("period commissions = %d %d", a.CommissionBalance, b.CommissionBalance)
	}

	// One-time type: first order only, even though the setting now says "every order".
	oneInviter := f.user("one@example.com")
	f.setUser(oneInviter, map[string]any{"commission_type": model.CommissionTypeOneTime})
	ou := f.user("ou@example.com", usersvc.CreateOptions{InviteUserID: &oneInviter.ID})
	if a, b := f.buy(ou, plan, model.PeriodMonthly), f.buy(ou, plan, model.PeriodMonthly); a.CommissionBalance != 100 || b.CommissionBalance != 0 {
		t.Fatalf("onetime commissions = %d %d", a.CommissionBalance, b.CommissionBalance)
	}

	// Balance-paid orders carry no commission (only gateway money counts).
	rich := f.user("rich@example.com", usersvc.CreateOptions{InviteUserID: &inviter.ID, Balance: 100000})
	if o := f.buy(rich, plan, model.PeriodMonthly); o.InviteUserID != nil || o.CommissionBalance != 0 {
		t.Fatalf("balance order = %+v", o)
	}
}

func TestCommissionSettlement(t *testing.T) {
	f := newFixture(t, map[string]any{"commission_distribution_enable": true, "commission_distribution_l1": 50,
		"commission_distribution_l2": 30, "commission_distribution_l3": 20})
	plan := f.plan("Pro", map[string]int64{model.PeriodYearly: 10000})
	top := f.user("top@example.com")
	mid := f.user("mid@example.com", usersvc.CreateOptions{InviteUserID: &top.ID})
	direct := f.user("direct@example.com", usersvc.CreateOptions{InviteUserID: &mid.ID})
	buyer := f.user("buyer@example.com", usersvc.CreateOptions{InviteUserID: &direct.ID})
	o := f.buy(buyer, plan, model.PeriodYearly)
	if o.CommissionBalance != 1000 {
		t.Fatalf("commission = %d", o.CommissionBalance)
	}
	// Within the hold window nothing is settled.
	if n, err := f.svc.SettleCommissions(ctx); err != nil || n != 0 {
		t.Fatalf("early settle = %d %v", n, err)
	}
	// After the hold the auto check approves and pays: 500 / 300 / 200 up the chain.
	f.a.DB.Model(&model.Order{}).Where("id = ?", o.ID).UpdateColumn("paid_at", time.Now().Add(-CommissionHold-time.Minute))
	if err := f.a.Scheduler.RunNow(JobCommissionSettle); err != nil {
		t.Fatal(err)
	}
	if got := f.order(o.TradeNo); got.CommissionStatus != model.CommissionStatusValid {
		t.Fatalf("commission status = %d", got.CommissionStatus)
	}
	for u, want := range map[*model.User]int64{direct: 500, mid: 300, top: 200} {
		if got := f.reload(u); got.CommissionBalance != want || got.Balance != 0 {
			t.Fatalf("%s commission = %d, want %d", u.Email, got.CommissionBalance, want)
		}
	}
	var logs []model.CommissionLog
	f.a.DB.Order("id").Find(&logs)
	if len(logs) != 3 || logs[0].InviteUserID != direct.ID || logs[0].UserID != buyer.ID || logs[0].TradeNo != o.TradeNo ||
		logs[0].OrderAmount != 10000 || logs[0].GetAmount != 500 || logs[2].InviteUserID != top.ID {
		t.Fatalf("logs = %+v", logs)
	}
	// Settled commissions are paid once and cannot be changed.
	if n, _ := f.svc.SettleCommissions(ctx); n != 0 {
		t.Fatal("settled twice")
	}
	if _, err := f.svc.SetCommissionStatus(ctx, o.ID, model.CommissionStatusInvalid); status(err) != 409 {
		t.Fatalf("change settled: %v", err)
	}
}

func TestCommissionManualApprovalAndWithdrawClose(t *testing.T) {
	f := newFixture(t, map[string]any{"commission_auto_check_enable": false, "withdraw_close_enable": true})
	plan := f.plan("Pro", map[string]int64{model.PeriodMonthly: 1000})
	inviter := f.user("inv@example.com")
	a := f.user("a@example.com", usersvc.CreateOptions{InviteUserID: &inviter.ID})
	b := f.user("b@example.com", usersvc.CreateOptions{InviteUserID: &inviter.ID})
	oa, ob := f.buy(a, plan, model.PeriodMonthly), f.buy(b, plan, model.PeriodMonthly)
	f.svc.Now = func() time.Time { return f.now.Add(10 * CommissionHold) }
	if n, _ := f.svc.SettleCommissions(ctx); n != 0 {
		t.Fatal("auto check disabled: nothing settles")
	}
	if _, err := f.svc.SetCommissionStatus(ctx, oa.ID, model.CommissionStatusProcessing); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetCommissionStatus(ctx, ob.ID, model.CommissionStatusInvalid); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetCommissionStatus(ctx, ob.ID, model.CommissionStatusValid); status(err) != 400 {
		t.Fatalf("setting 2 directly: %v", err)
	}
	if _, err := f.svc.SetCommissionStatus(ctx, 9999, model.CommissionStatusProcessing); status(err) != 404 {
		t.Fatalf("missing order: %v", err)
	}
	if n, err := f.svc.SettleCommissions(ctx); err != nil || n != 1 {
		t.Fatalf("settle = %d %v", n, err)
	}
	// withdraw_close_enable credits the balance instead of commission_balance.
	if got := f.reload(inviter); got.Balance != 100 || got.CommissionBalance != 0 {
		t.Fatalf("inviter = balance %d commission %d", got.Balance, got.CommissionBalance)
	}
	if f.order(ob.TradeNo).CommissionStatus != model.CommissionStatusInvalid {
		t.Fatal("invalid commission must stay invalid")
	}
}
