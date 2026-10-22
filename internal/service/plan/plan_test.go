package plansvc

import (
	"context"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestPlanQueries(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx, now := context.Background(), time.Now()
	one := 1
	mk := func(p model.Plan) *model.Plan {
		if err := a.DB.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
		return &p
	}
	open := mk(model.Plan{Name: "Open", Show: true, Sell: true, Renew: true, Sort: 2, Prices: model.JSONObject[int64]{"monthly": 100}})
	full := mk(model.Plan{Name: "Full", Show: true, Sell: true, Sort: 1, CapacityLimit: &one})
	hidden := mk(model.Plan{Name: "Hidden", Show: false, Sell: true, Sort: 0})
	legacy := mk(model.Plan{Name: "Legacy", Show: true, Sell: false, Sort: 3})
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	holder := testutil.CreateUser(t, a, "h@example.com", usersvc.CreateOptions{PlanID: &full.ID, ExpiredAt: &future})
	testutil.CreateUser(t, a, "x@example.com", usersvc.CreateOptions{PlanID: &full.ID, ExpiredAt: &past})
	testutil.CreateUser(t, a, "l@example.com", usersvc.CreateOptions{PlanID: &open.ID})
	hiddenHolder := testutil.CreateUser(t, a, "hh@example.com", usersvc.CreateOptions{PlanID: &hidden.ID})
	buyer := testutil.CreateUser(t, a, "b@example.com")

	counts, err := ActiveUserCounts(ctx, a.DB, now)
	if err != nil || counts[full.ID] != 1 || counts[open.ID] != 1 || counts[legacy.ID] != 0 {
		t.Fatalf("counts = %v %v", counts, err)
	}
	if n, _ := HolderCount(ctx, a.DB, full.ID); n != 2 {
		t.Fatalf("holders = %d", n)
	}
	if ok, _ := HasCapacity(ctx, a.DB, full, buyer.ID, now); ok {
		t.Fatal("full plan has no capacity")
	}
	if ok, _ := HasCapacity(ctx, a.DB, full, holder.ID, now); !ok {
		t.Fatal("the holder is not counted against themselves")
	}
	if ok, _ := HasCapacity(ctx, a.DB, open, buyer.ID, now); !ok {
		t.Fatal("unlimited plan")
	}

	guest, err := ListForGuest(ctx, a.DB, now)
	if err != nil || len(guest) != 1 || guest[0].ID != open.ID || guest[0].Prices["monthly"] != 100 || guest[0].Tags == nil {
		t.Fatalf("guest = %+v %v", guest, err)
	}
	list, err := ListForUser(ctx, a.DB, hiddenHolder, now)
	if err != nil || len(list) != 4 || list[0].ID != hidden.ID || !list[0].IsCurrent || list[1].ID != full.ID || !list[1].SoldOut {
		t.Fatalf("user list = %+v %v", list, err)
	}
	list, _ = ListForUser(ctx, a.DB, holder, now)
	for _, p := range list {
		if p.ID == full.ID && (p.SoldOut || !p.IsCurrent) {
			t.Fatalf("holder sees own plan sold out: %+v", p)
		}
		if p.ID == hidden.ID {
			t.Fatal("hidden plan leaked")
		}
	}
	if _, err := GetForUser(ctx, a.DB, buyer, hidden.ID, now); err == nil {
		t.Fatal("hidden plan must 404")
	}
	if v, err := GetForUser(ctx, a.DB, hiddenHolder, hidden.ID, now); err != nil || !v.IsCurrent {
		t.Fatalf("hidden holder = %+v %v", v, err)
	}
	if v, err := GetForUser(ctx, a.DB, buyer, full.ID, now); err != nil || !v.SoldOut {
		t.Fatalf("full = %+v %v", v, err)
	}
	admin, err := ListForAdmin(ctx, a.DB, now)
	if err != nil || len(admin) != 4 || admin[0].ID != hidden.ID || admin[1].UserCount != 1 {
		t.Fatalf("admin = %+v %v", admin, err)
	}
	if errs := ValidatePrices(map[string]int64{"monthly": 1, "weekly": 1, "yearly": -1}); len(errs) != 2 || errs["prices.weekly"] == nil || errs["prices.yearly"] == nil {
		t.Fatalf("price errors = %v", errs)
	}
	if PeriodLabel("monthly", "zh-CN") != "月付" || PeriodLabel("yearly", "en") != "Yearly" || PeriodLabel("x", "en") != "x" {
		t.Fatal("labels")
	}
}
