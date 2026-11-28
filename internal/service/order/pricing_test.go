package ordersvc

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
)

func TestComputeQuote(t *testing.T) {
	fixed := &model.Coupon{Type: model.CouponTypeAmount, Value: 300}
	pct := &model.Coupon{Type: model.CouponTypePercent, Value: 25}
	huge := &model.Coupon{Type: model.CouponTypeAmount, Value: 99999}
	cases := []struct {
		name string
		in   QuoteInput
		want Quote
	}{
		{"plain", QuoteInput{Price: 1000}, Quote{Price: 1000, Total: 1000}},
		{"user discount", QuoteInput{Price: 1000, UserDiscount: ptr(10)}, Quote{Price: 1000, Discount: 100, Total: 900}},
		{"user discount rounds half up", QuoteInput{Price: 995, UserDiscount: ptr(10)}, Quote{Price: 995, Discount: 100, Total: 895}},
		{"fixed coupon", QuoteInput{Price: 1000, Coupon: fixed}, Quote{Price: 1000, Discount: 300, Total: 700}},
		{"percent coupon", QuoteInput{Price: 1000, Coupon: pct}, Quote{Price: 1000, Discount: 250, Total: 750}},
		{"coupon + user discount on the price", QuoteInput{Price: 1000, Coupon: pct, UserDiscount: ptr(10)}, Quote{Price: 1000, Discount: 350, Total: 650}},
		{"discount capped", QuoteInput{Price: 1000, Coupon: huge, UserDiscount: ptr(50)}, Quote{Price: 1000, Discount: 1000, Total: 0}},
		{"surplus below total", QuoteInput{Price: 1000, Surplus: 400}, Quote{Price: 1000, Surplus: 400, Total: 600}},
		{"surplus above total refunds", QuoteInput{Price: 1000, Coupon: fixed, Surplus: 900}, Quote{Price: 1000, Discount: 300, Surplus: 900, Refund: 200, Total: 0}},
		{"balance partial", QuoteInput{Price: 1000, Balance: 250}, Quote{Price: 1000, Balance: 250, Total: 750}},
		{"balance covers", QuoteInput{Price: 1000, Coupon: fixed, Balance: 5000}, Quote{Price: 1000, Discount: 300, Balance: 700, Total: 0}},
		{"balance unused after refund", QuoteInput{Price: 1000, Surplus: 1500, Balance: 500}, Quote{Price: 1000, Surplus: 1500, Refund: 500, Total: 0}},
		{"everything", QuoteInput{Price: 2000, UserDiscount: ptr(5), Coupon: pct, Surplus: 300, Balance: 200}, Quote{Price: 2000, Discount: 600, Surplus: 300, Balance: 200, Total: 900}},
		{"free plan", QuoteInput{Price: 0, Balance: 100}, Quote{}},
		{"listed below total", QuoteInput{Price: 1000, Coupon: fixed, ListedSurplus: 400}, Quote{Price: 1000, Discount: 300, Surplus: 400, Total: 300}},
		{"listed above total is not refunded", QuoteInput{Price: 1000, ListedSurplus: 1500, Balance: 500}, Quote{Price: 1000, Surplus: 1000, Total: 0}},
		{"paid then listed then balance", QuoteInput{Price: 1000, Surplus: 300, ListedSurplus: 400, Balance: 500}, Quote{Price: 1000, Surplus: 700, Balance: 300, Total: 0}},
		{"paid refund leaves no room for listed", QuoteInput{Price: 1000, Surplus: 1200, ListedSurplus: 300}, Quote{Price: 1000, Surplus: 1200, Refund: 200, Total: 0}},
		{"negative listed ignored", QuoteInput{Price: 1000, ListedSurplus: -5}, Quote{Price: 1000, Total: 1000}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ComputeQuote(c.in); got != c.want {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
	if CouponDiscount(nil, 100) != 0 || CouponDiscount(&model.Coupon{Type: model.CouponTypePercent, Value: 150}, 100) != 100 {
		t.Fatal("coupon discount bounds")
	}
}

func TestOrderType(t *testing.T) {
	now := time.Now()
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	plan := uint64(1)
	cases := []struct {
		u      model.User
		planID uint64
		period string
		want   model.OrderType
	}{
		{model.User{}, 1, model.PeriodMonthly, model.OrderTypeNew},
		{model.User{PlanID: &plan, ExpiredAt: &future}, 1, model.PeriodMonthly, model.OrderTypeRenew},
		{model.User{PlanID: &plan}, 1, model.PeriodMonthly, model.OrderTypeRenew},
		{model.User{PlanID: &plan, ExpiredAt: &past}, 1, model.PeriodMonthly, model.OrderTypeNew},
		{model.User{PlanID: &plan, ExpiredAt: &future}, 2, model.PeriodYearly, model.OrderTypeUpgrade},
		{model.User{PlanID: &plan}, 2, model.PeriodOnetime, model.OrderTypeUpgrade},
		{model.User{PlanID: &plan, ExpiredAt: &past}, 2, model.PeriodMonthly, model.OrderTypeNew},
		{model.User{PlanID: &plan, ExpiredAt: &future}, 1, model.PeriodResetTraffic, model.OrderTypeResetTraffic},
	}
	for i, c := range cases {
		if got := OrderType(&c.u, c.planID, c.period, now); got != c.want {
			t.Errorf("case %d: got %d, want %d", i, got, c.want)
		}
	}
}

// paidSurplus is ComputeSurplus without the current plan, so there is no list price to fall back on.
func paidSurplus(u *model.User, orders []model.Order, now time.Time) (int64, []uint64) {
	s := ComputeSurplus(u, nil, orders, now)
	return s.Paid, s.OrderIDs
}

func TestComputeSurplus(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	plan := uint64(5)
	at := func(d time.Time) *time.Time { return &d }

	// Periodic: one monthly order 10 days into a 30-day month (Oct 12 → Nov 12 = 31 days).
	paid := now.AddDate(0, 0, -10)
	exp := paid.AddDate(0, 1, 0)
	u := &model.User{PlanID: &plan, ExpiredAt: &exp}
	orders := []model.Order{
		{ID: 1, PlanID: plan, Period: model.PeriodMonthly, Type: model.OrderTypeNew, TotalAmount: 2000, BalanceAmount: 1100, PaidAt: at(paid)},
		{ID: 2, PlanID: plan, Period: model.PeriodResetTraffic, Type: model.OrderTypeResetTraffic, TotalAmount: 500, PaidAt: at(paid)},
		{ID: 3, PlanID: 99, Period: model.PeriodMonthly, Type: model.OrderTypeNew, TotalAmount: 9999, PaidAt: at(paid)},
	}
	got, ids := paidSurplus(u, orders, now)
	want := int64(float64(3100) * float64(exp.Sub(now)) / float64(exp.Sub(paid)))
	if got != want || !slices.Equal(ids, []uint64{1}) {
		t.Fatalf("periodic = %d %v, want %d [1]", got, ids, want)
	}

	// A renewal extends the streak; orders before the latest new purchase are ignored.
	old := now.AddDate(0, -6, 0)
	renew := paid.Add(24 * time.Hour)
	exp2 := paid.AddDate(0, 2, 0)
	u2 := &model.User{PlanID: &plan, ExpiredAt: &exp2}
	streak := []model.Order{
		{ID: 10, PlanID: plan, Period: model.PeriodMonthly, Type: model.OrderTypeNew, TotalAmount: 1000, PaidAt: at(old)},
		{ID: 11, PlanID: plan, Period: model.PeriodMonthly, Type: model.OrderTypeNew, TotalAmount: 1000, PaidAt: at(paid)},
		{ID: 12, PlanID: plan, Period: model.PeriodMonthly, Type: model.OrderTypeRenew, TotalAmount: 1000, PaidAt: at(renew)},
	}
	got, ids = paidSurplus(u2, streak, now)
	want = int64(float64(2000) * float64(exp2.Sub(now)) / float64(exp2.Sub(paid)))
	if got != want || !slices.Equal(ids, []uint64{11, 12}) {
		t.Fatalf("streak = %d %v, want %d [11 12]", got, ids, want)
	}

	// Expiry shortened by an admin caps the credit; an expired window yields 0.
	short := now.Add(24 * time.Hour)
	if got, _ := paidSurplus(&model.User{PlanID: &plan, ExpiredAt: &short}, orders, now); got <= 0 || got >= 3100/10 {
		t.Fatalf("capped = %d", got)
	}
	if got, _ := paidSurplus(u, orders, exp.Add(time.Hour)); got != 0 {
		t.Fatalf("expired = %d", got)
	}

	// One-time: 100 GB bought for 50.00 (40 + 10 from balance), 25 GB used → 37.50 credit.
	one := &model.User{PlanID: &plan, TransferEnable: 100 * model.BytesPerGB, U: 20 * model.BytesPerGB, D: 5 * model.BytesPerGB}
	onetime := []model.Order{
		{ID: 20, PlanID: plan, Period: model.PeriodOnetime, TotalAmount: 4000, BalanceAmount: 1000},
		{ID: 21, PlanID: plan, Period: model.PeriodResetTraffic, TotalAmount: 300},
		{ID: 22, PlanID: plan, Period: model.PeriodMonthly, TotalAmount: 300},
	}
	got, ids = paidSurplus(one, onetime, now)
	if got != 3750 || !slices.Equal(ids, []uint64{20, 22}) {
		t.Fatalf("onetime = %d %v", got, ids)
	}
	if got, _ := paidSurplus(&model.User{}, onetime, now); got != 0 {
		t.Fatal("no plan → 0")
	}
}

// Subscriptions the panel holds no order for (imported from Xboard, whose orders are not imported, or set by an
// administrator) are credited at the plan's cheapest monthly list price, without a refund. Before, imported
// subscribers who upgraded got no credit at all (MISTAKES #323).
func TestComputeSurplusWithoutPanelOrders(t *testing.T) {
	now := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	planID := uint64(1)
	pro := &model.Plan{ID: planID, Prices: map[string]int64{
		model.PeriodQuarterly: 3000, model.PeriodHalfYearly: 5700, model.PeriodYearly: 10800, model.PeriodResetTraffic: 500,
	}}
	yearly := now.AddDate(1, 0, 0).Sub(now)
	exp := now.AddDate(0, 0, 45).Add(6 * time.Hour)
	imported := &model.User{PlanID: &planID, ExpiredAt: &exp}

	// Yearly is the cheapest per month (9.00 vs 10.00 and 9.50): 45.25 days of 108.00 a year.
	want := int64(float64(10800) * float64(exp.Sub(now)) / float64(yearly))
	got := ComputeSurplus(imported, pro, nil, now)
	if got.Paid != 0 || got.Listed != want || got.OrderIDs != nil || want < 1330 || want > 1345 {
		t.Fatalf("imported = %+v, want listed %d", got, want)
	}
	// The user's discount applies, as it would have on the plan price.
	ten := 10
	discounted := &model.User{PlanID: &planID, ExpiredAt: &exp, Discount: &ten}
	if got := ComputeSurplus(discounted, pro, nil, now); got.Listed != want-percentOf(want, 10) {
		t.Fatalf("discounted = %+v", got)
	}
	// Orders of other plans do not change it.
	other := []model.Order{{ID: 7, PlanID: 99, Period: model.PeriodYearly, Type: model.OrderTypeNew, TotalAmount: 5000, PaidAt: &now}}
	if got := ComputeSurplus(imported, pro, other, now); got.Listed != want || got.Paid != 0 {
		t.Fatalf("other plan orders = %+v", got)
	}
	// No price to go by: deleted plan, a free period, or no recurring period.
	free := &model.Plan{ID: planID, Prices: map[string]int64{model.PeriodMonthly: 2000, model.PeriodThreeYearly: 0}}
	traffic := &model.Plan{ID: planID, Prices: map[string]int64{model.PeriodOnetime: 9000, model.PeriodResetTraffic: 100}}
	for name, p := range map[string]*model.Plan{"deleted": nil, "free period": free, "no recurring period": traffic} {
		if got := ComputeSurplus(imported, p, nil, now); got.Listed != 0 || got.Paid != 0 {
			t.Fatalf("%s = %+v", name, got)
		}
	}

	// Imported, then renewed in the panel: the renewal paid for the last 3 months before the expiry; the time
	// before them is listed.
	importedEnd := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	renewed := importedEnd.AddDate(0, 3, 0)
	paidAt := now.AddDate(0, 0, -10)
	renewals := []model.Order{{ID: 30, PlanID: planID, Period: model.PeriodQuarterly, Type: model.OrderTypeRenew, TotalAmount: 2000, BalanceAmount: 1000, PaidAt: &paidAt}}
	u := &model.User{PlanID: &planID, ExpiredAt: &renewed}
	got = ComputeSurplus(u, pro, renewals, now)
	if got.Paid != 3000 || got.Listed != listValue(pro, now, importedEnd) || !slices.Equal(got.OrderIDs, []uint64{30}) {
		t.Fatalf("renewed import = %+v", got)
	}
	// An administrator shortened that expiry into the renewal's window: only its unused share, nothing listed.
	short := now.AddDate(0, 1, 0)
	u = &model.User{PlanID: &planID, ExpiredAt: &short}
	got = ComputeSurplus(u, pro, renewals, now)
	wantPaid := int64(float64(3000) * float64(short.Sub(now)) / float64(short.Sub(short.AddDate(0, -3, 0))))
	if got.Paid != wantPaid || got.Listed != 0 {
		t.Fatalf("shortened renewal = %+v, want paid %d", got, wantPaid)
	}

	// A streak that began with a purchase in the panel: time beyond its orders was added by an administrator and
	// earns nothing, as before.
	bought := now.AddDate(0, -1, 0)
	gift := bought.AddDate(0, 3+6, 0)
	purchase := []model.Order{{ID: 40, PlanID: planID, Period: model.PeriodQuarterly, Type: model.OrderTypeNew, TotalAmount: 3000, PaidAt: &bought}}
	u = &model.User{PlanID: &planID, ExpiredAt: &gift}
	got = ComputeSurplus(u, pro, purchase, now)
	end := bought.AddDate(0, 3, 0)
	if got.Listed != 0 || got.Paid != int64(float64(3000)*float64(end.Sub(now))/float64(end.Sub(bought))) {
		t.Fatalf("gifted time = %+v", got)
	}

	// One-time plan without a one-time order: the unused traffic at the plan's one-time price.
	one := &model.User{PlanID: &planID, TransferEnable: 100 * model.BytesPerGB, U: 20 * model.BytesPerGB, D: 5 * model.BytesPerGB}
	onetime := &model.Plan{ID: planID, Prices: map[string]int64{model.PeriodOnetime: 5000}}
	if got := ComputeSurplus(one, onetime, nil, now); got.Listed != 3750 || got.Paid != 0 {
		t.Fatalf("imported onetime = %+v", got)
	}
	if got := ComputeSurplus(one, pro, nil, now); got.Listed != 0 {
		t.Fatalf("onetime user on a plan without a one-time price = %+v", got)
	}
}

func TestListValuePicksTheCheapestMonthlyRate(t *testing.T) {
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	p := &model.Plan{Prices: map[string]int64{model.PeriodMonthly: 1000, model.PeriodQuarterly: 2700, model.PeriodYearly: 12000}}
	// Quarterly (9.00 a month) beats monthly (10.00) and yearly (10.00): one month is a third of the quarter.
	to := from.AddDate(0, 1, 0)
	want := int64(float64(2700) * float64(to.Sub(from)) / float64(from.AddDate(0, 3, 0).Sub(from)))
	if got := listValue(p, from, to); got != want {
		t.Fatalf("listValue = %d, want %d", got, want)
	}
	if listValue(p, to, from) != 0 || listValue(nil, from, to) != 0 {
		t.Fatal("empty interval or no plan must be worth 0")
	}
	huge := &model.Plan{Prices: map[string]int64{model.PeriodMonthly: math.MaxInt64}}
	if got := listValue(huge, from, from.AddDate(100, 0, 0)); got != math.MaxInt64 {
		t.Fatalf("saturation = %d", got)
	}
}
