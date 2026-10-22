package ordersvc

import (
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
	got, ids := ComputeSurplus(u, orders, now)
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
	got, ids = ComputeSurplus(u2, streak, now)
	want = int64(float64(2000) * float64(exp2.Sub(now)) / float64(exp2.Sub(paid)))
	if got != want || !slices.Equal(ids, []uint64{11, 12}) {
		t.Fatalf("streak = %d %v, want %d [11 12]", got, ids, want)
	}

	// Expiry shortened by an admin caps the credit; an expired window yields 0.
	short := now.Add(24 * time.Hour)
	if got, _ := ComputeSurplus(&model.User{PlanID: &plan, ExpiredAt: &short}, orders, now); got <= 0 || got >= 3100/10 {
		t.Fatalf("capped = %d", got)
	}
	if got, _ := ComputeSurplus(u, orders, exp.Add(time.Hour)); got != 0 {
		t.Fatalf("expired = %d", got)
	}

	// One-time: 100 GB bought for 50.00 (40 + 10 from balance), 25 GB used → 37.50 credit.
	one := &model.User{PlanID: &plan, TransferEnable: 100 * model.BytesPerGB, U: 20 * model.BytesPerGB, D: 5 * model.BytesPerGB}
	onetime := []model.Order{
		{ID: 20, PlanID: plan, Period: model.PeriodOnetime, TotalAmount: 4000, BalanceAmount: 1000},
		{ID: 21, PlanID: plan, Period: model.PeriodResetTraffic, TotalAmount: 300},
		{ID: 22, PlanID: plan, Period: model.PeriodMonthly, TotalAmount: 300},
	}
	got, ids = ComputeSurplus(one, onetime, now)
	if got != 3750 || !slices.Equal(ids, []uint64{20, 22}) {
		t.Fatalf("onetime = %d %v", got, ids)
	}
	if got, _ := ComputeSurplus(&model.User{}, onetime, now); got != 0 {
		t.Fatal("no plan → 0")
	}
}
