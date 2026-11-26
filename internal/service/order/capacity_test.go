package ordersvc

import (
	"sync"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	plansvc "github.com/linusxiong/ggboard/internal/service/plan"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

func capacityPlan(f *fixture, limit int) *model.Plan {
	return f.plan("Limited", map[string]int64{model.PeriodMonthly: 1000}, func(p *model.Plan) { p.CapacityLimit = &limit })
}

// A pending order holds a seat of a capacity-limited plan until it is paid, cancelled or timed out: with one seat
// left, three buyers cannot each create an order and then all pay for it.
func TestCapacityCountsOpenOrders(t *testing.T) {
	f := newFixture(t)
	plan := capacityPlan(f, 1)
	a, b, c := f.user("a@example.com"), f.user("b@example.com"), f.user("c@example.com")
	in := CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly}

	oa, err := f.svc.Create(ctx, a.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, b.ID, in); status(err) != 409 || len(fieldErrs(err)["plan_id"]) == 0 {
		t.Fatalf("second buyer while the first is pending: %v", err)
	}
	// The sold-out flags agree with the purchase check; the buyer's own reservation does not count against them.
	if list, err := plansvc.ListForUser(ctx, f.a.DB, f.reload(b), f.now); err != nil || len(list) != 1 || !list[0].SoldOut {
		t.Fatalf("list for another buyer = %+v %v", list, err)
	}
	if list, err := plansvc.ListForUser(ctx, f.a.DB, f.reload(a), f.now); err != nil || len(list) != 1 || list[0].SoldOut {
		t.Fatalf("list for the reserving buyer = %+v %v", list, err)
	}
	if v, err := plansvc.GetForUser(ctx, f.a.DB, f.reload(b), plan.ID, f.now); err != nil || !v.SoldOut {
		t.Fatalf("plan for another buyer = %+v %v", v, err)
	}
	if guest, err := plansvc.ListForGuest(ctx, f.a.DB, f.now); err != nil || len(guest) != 0 {
		t.Fatalf("guest list shows a fully reserved plan: %+v %v", guest, err)
	}

	// Cancelling (or order_timeout) frees the seat.
	if _, err := f.svc.Cancel(ctx, oa.ID); err != nil {
		t.Fatal(err)
	}
	ob, err := f.svc.Create(ctx, b.ID, in)
	if err != nil {
		t.Fatalf("seat freed by the cancellation: %v", err)
	}
	if _, _, err := f.svc.Paid(ctx, ob.TradeNo, "cb-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, c.ID, in); status(err) != 409 {
		t.Fatalf("buyer after the seat was paid for: %v", err)
	}
	// The holder can still renew.
	renew, err := f.svc.Create(ctx, b.ID, in)
	if err != nil || renew.Type != model.OrderTypeRenew {
		t.Fatalf("holder renewal: %+v %v", renew, err)
	}
	if _, _, err := f.svc.Paid(ctx, renew.TradeNo, "cb-b2"); err != nil {
		t.Fatal(err)
	}
	if n, err := plansvc.HolderCount(ctx, f.a.DB, plan.ID); err != nil || n != 1 {
		t.Fatalf("holders = %d %v", n, err)
	}
}

// An active holder's pending renewal takes no new seat; an expired holder's renewal takes the seat back when paid,
// so it holds one while pending.
func TestCapacityRenewalSeats(t *testing.T) {
	f := newFixture(t)
	plan := capacityPlan(f, 2)
	future, past := f.now.Add(24*time.Hour), f.now.Add(-24*time.Hour)
	holder := f.user("holder@example.com", usersvc.CreateOptions{PlanID: &plan.ID, ExpiredAt: &future, TransferEnable: model.BytesPerGB})
	lapsed := f.user("lapsed@example.com", usersvc.CreateOptions{PlanID: &plan.ID, ExpiredAt: &past, TransferEnable: model.BytesPerGB})
	x, y := f.user("x@example.com"), f.user("y@example.com")
	in := CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly}

	if _, err := f.svc.Create(ctx, holder.ID, in); err != nil {
		t.Fatalf("active holder renewal: %v", err)
	}
	// 1 active subscriber, the active holder's renewal takes no seat: one left.
	if _, err := f.svc.Create(ctx, x.ID, in); err != nil {
		t.Fatalf("buyer with a seat left: %v", err)
	}
	if _, err := f.svc.Create(ctx, y.ID, in); status(err) != 409 {
		t.Fatalf("buyer with no seat left: %v", err)
	}
	// Holders are never blocked, even with every seat taken; the lapsed holder's renewal then holds a seat too.
	lo, err := f.svc.Create(ctx, lapsed.ID, in)
	if err != nil {
		t.Fatalf("lapsed holder renewal: %v", err)
	}
	if n, err := plansvc.ReservedSeats(ctx, f.a.DB, plan.ID, 0, f.now); err != nil || n != 2 {
		t.Fatalf("reserved seats = %d %v (want x and the lapsed holder)", n, err)
	}
	if _, err := f.svc.Cancel(ctx, lo.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := plansvc.ReservedSeats(ctx, f.a.DB, plan.ID, 0, f.now); err != nil || n != 1 {
		t.Fatalf("reserved seats after the cancellation = %d %v", n, err)
	}
}

// Concurrent purchases of the last seat: exactly one order is created.
func TestCapacityConcurrentCreates(t *testing.T) {
	f := newFixture(t)
	plan := capacityPlan(f, 1)
	var buyers []*model.User
	for _, e := range []string{"p1@example.com", "p2@example.com", "p3@example.com", "p4@example.com", "p5@example.com"} {
		buyers = append(buyers, f.user(e))
	}
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		ok, full int
		other    []error
	)
	for _, u := range buyers {
		wg.Go(func() {
			_, err := f.svc.Create(ctx, u.ID, CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
			mu.Lock()
			defer mu.Unlock()
			switch status(err) {
			case 0:
				ok++
			case 409:
				full++
			default:
				other = append(other, err)
			}
		})
	}
	wg.Wait()
	if ok != 1 || full != len(buyers)-1 || len(other) != 0 {
		t.Fatalf("created=%d sold out=%d errors=%v", ok, full, other)
	}
}
