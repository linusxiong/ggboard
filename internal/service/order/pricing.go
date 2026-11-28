package ordersvc

import (
	"context"
	"math"
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	plansvc "github.com/linusxiong/ggboard/internal/service/plan"
)

// QuoteInput is everything pricing depends on (cents unless noted).
type QuoteInput struct {
	Price        int64
	UserDiscount *int          // users.discount, percent
	Coupon       *model.Coupon // validated coupon, or nil
	Surplus      int64         // upgrade credit from paid orders (0 unless an upgrade with surplus_enable)
	// ListedSurplus is upgrade credit estimated from list prices (Surplus.Listed). It lowers the price at most to
	// zero: an estimate is never refunded to the balance.
	ListedSurplus int64
	Balance       int64 // users.balance available
}

// Quote is the price breakdown of an order.
type Quote struct {
	Price    int64 `json:"price"`
	Discount int64 `json:"discount_amount"` // user discount + coupon, capped at Price
	Surplus  int64 `json:"surplus_amount"`
	Refund   int64 `json:"refund_amount"`  // surplus exceeding the price, credited to the balance on fulfilment
	Balance  int64 `json:"balance_amount"` // paid from the balance (deducted at creation)
	Total    int64 `json:"total_amount"`   // left to pay through a gateway
}

// CouponDiscount is the coupon's discount on price (fixed cents or percent), capped at price.
func CouponDiscount(c *model.Coupon, price int64) int64 {
	if c == nil || price <= 0 {
		return 0
	}
	var d int64
	switch c.Type {
	case model.CouponTypeAmount:
		d = c.Value
	case model.CouponTypePercent:
		d = percentOf(price, min(c.Value, 100))
	}
	return min(max(d, 0), price)
}

// ComputeQuote applies, in order: user discount and coupon (both on the plan price, Xboard), upgrade surplus
// (paid, then listed) and the account balance.
func ComputeQuote(in QuoteInput) Quote {
	q := Quote{Price: max(in.Price, 0)}
	discount := CouponDiscount(in.Coupon, q.Price)
	if in.UserDiscount != nil {
		discount += min(percentOf(q.Price, int64(*in.UserDiscount)), q.Price-discount)
	}
	q.Discount = min(discount, q.Price)
	total := q.Price - q.Discount
	if in.Surplus > 0 {
		q.Surplus = in.Surplus
		if in.Surplus >= total {
			q.Refund = in.Surplus - total
			total = 0
		} else {
			total -= in.Surplus
		}
	}
	if listed := min(max(in.ListedSurplus, 0), total); listed > 0 {
		q.Surplus = saturatingAmount(q.Surplus, listed)
		total -= listed
	}
	if total > 0 && in.Balance > 0 {
		q.Balance = min(in.Balance, total)
		total -= q.Balance
	}
	q.Total = total
	return q
}

// OrderType classifies a purchase (Xboard setOrderType): reset_traffic; upgrade (another plan while the
// current subscription is active); renew (same plan, active); otherwise a new purchase.
func OrderType(u *model.User, planID uint64, period string, now time.Time) model.OrderType {
	if period == model.PeriodResetTraffic {
		return model.OrderTypeResetTraffic
	}
	active := u.ExpiredAt == nil || u.ExpiredAt.After(now)
	if u.PlanID != nil && active {
		if *u.PlanID != planID {
			return model.OrderTypeUpgrade
		}
		return model.OrderTypeRenew
	}
	return model.OrderTypeNew
}

// subscriptionActive is Xboard's UserService::isAvailable: not banned, has traffic and not expired.
func subscriptionActive(u *model.User, now time.Time) bool {
	return !u.Banned && u.TransferEnable > 0 && !u.IsExpired(now)
}

// validatePurchase applies the shop rules and returns the price for period.
//   - the plan must have a price for period;
//   - reset_traffic: only for the holder of an active subscription to this plan;
//   - other buyers: show && sell, and capacity left (409 when full);
//   - holders: renew must be allowed; hidden or legacy (sell=false) plans only while still active.
func validatePurchase(ctx context.Context, tx *gorm.DB, u *model.User, p *model.Plan, period string, now time.Time) (int64, error) {
	if !model.IsValidPeriod(period) {
		return 0, httpx.FieldError("period", "unknown period")
	}
	price, ok := p.Price(period)
	if !ok {
		return 0, httpx.FieldError("period", "this period is not available for the plan")
	}
	holder := u.PlanID != nil && *u.PlanID == p.ID
	active := subscriptionActive(u, now)
	if period == model.PeriodResetTraffic {
		if !holder || !active {
			return 0, httpx.FieldError("plan_id", "a traffic reset needs an active subscription to this plan")
		}
		return price, nil
	}
	if !holder {
		if !p.Show || !p.Sell {
			return 0, httpx.FieldError("plan_id", "this plan is not for sale")
		}
		ok, err := plansvc.HasCapacity(ctx, tx, p, u.ID, now)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, httpx.Conflict("this plan is sold out").WithField("plan_id", "sold out")
		}
		return price, nil
	}
	if !p.Renew {
		return 0, httpx.FieldError("plan_id", "this plan cannot be renewed; choose another plan")
	}
	if (!p.Show || !p.Sell) && !active {
		return 0, httpx.FieldError("plan_id", "this subscription has expired; choose another plan")
	}
	return price, nil
}

// orderMonths returns the months an order's period covers.
func orderMonths(o *model.Order) int { return model.PeriodMonths(o.Period) }

func orderPaidAt(o *model.Order) time.Time {
	if o.PaidAt != nil {
		return *o.PaidAt
	}
	return o.CreatedAt
}

// Surplus is the upgrade credit of a user's current plan.
type Surplus struct {
	// Paid is the unused share of what completed orders in the panel paid for the current plan.
	Paid int64
	// Listed is the list-price worth of remaining time or traffic that no such order paid for: subscriptions
	// imported from Xboard (whose orders are not imported) or set by an administrator. The panel cannot tell
	// the two apart, so ComputeQuote never refunds this part.
	Listed int64
	// OrderIDs are the completed orders the upgrade supersedes (status 4 on fulfilment).
	OrderIDs []uint64
}

// ComputeSurplus returns the upgrade credit of u's current plan (upstream Xboard getSurplusValue, extended to
// time and traffic the panel holds no order for) from the user's completed orders. current is u's plan, or nil
// when it no longer exists.
//
//   - one-time plan (expired_at NULL): the last completed one-time order of the current plan is worth
//     (total + balance) / transfer_GB per GB, and Paid is the unused traffic's worth. Without such an order,
//     Listed is the unused traffic's worth at the plan's one-time price. Surplus orders are every completed
//     non-reset order.
//   - periodic plan, streak with a new purchase or upgrade: the completed periodic orders of the current plan
//     since its latest new-purchase or upgrade order cover [first paid_at, first + Σ months); Paid is
//     Σ(total + balance + surplus − refund) × remaining/total time, with the end capped at the user's actual
//     expired_at. Time beyond that window was added by an administrator and earns nothing.
//   - periodic plan, otherwise (imported, or the plan was set by an administrator, possibly renewed since):
//     renewals extend the expiry, so the orders cover the last Σ months before expired_at and Paid is their
//     unused share; the remaining time before that window is Listed at the plan's cheapest monthly rate.
//
// Listed values take off the user's discount, as the plan price would have.
func ComputeSurplus(u *model.User, current *model.Plan, completed []model.Order, now time.Time) Surplus {
	if u.PlanID == nil {
		return Surplus{}
	}
	planID := *u.PlanID
	if u.ExpiredAt == nil {
		return onetimeSurplus(u, current, completed)
	}
	var periodic []model.Order
	for _, o := range completed {
		if o.PlanID == planID && orderMonths(&o) > 0 {
			periodic = append(periodic, o)
		}
	}
	// The streak starts at the latest new purchase or upgrade; earlier orders were superseded.
	var streakStart time.Time
	for i := range periodic {
		o := &periodic[i]
		if (o.Type == model.OrderTypeNew || o.Type == model.OrderTypeUpgrade) && orderPaidAt(o).After(streakStart) {
			streakStart = orderPaidAt(o)
		}
	}
	anchored := !streakStart.IsZero()
	periodic = slices.DeleteFunc(periodic, func(o model.Order) bool { return orderPaidAt(&o).Before(streakStart) })
	var (
		sum    int64
		months int
		first  time.Time
		ids    []uint64
	)
	for i := range periodic {
		o := &periodic[i]
		surplus := max(o.SurplusAmount, 0)
		netSurplus := surplus - min(max(o.RefundAmount, 0), surplus)
		sum = saturatingAmount(sum, saturatingAmount(saturatingAmount(o.TotalAmount, o.BalanceAmount), netSurplus))
		months += orderMonths(o)
		if first.IsZero() || orderPaidAt(o).Before(first) {
			first = orderPaidAt(o)
		}
		ids = append(ids, o.ID)
	}
	if anchored {
		end := first.AddDate(0, months, 0)
		span := end.Sub(first)
		if u.ExpiredAt.Before(end) {
			end = *u.ExpiredAt
		}
		return Surplus{Paid: share(sum, end.Sub(now), span), OrderIDs: ids}
	}
	end := *u.ExpiredAt
	covered := end.AddDate(0, -months, 0) // start of the window the orders paid for
	s := Surplus{OrderIDs: ids}
	if months > 0 {
		s.Paid = share(sum, end.Sub(later(now, covered)), end.Sub(covered))
	}
	if covered.After(now) {
		s.Listed = afterDiscount(listValue(current, now, covered), u.Discount)
	}
	return s
}

// onetimeSurplus is ComputeSurplus for a one-time plan.
func onetimeSurplus(u *model.User, current *model.Plan, completed []model.Order) Surplus {
	if u.TransferEnable <= 0 {
		return Surplus{}
	}
	var last *model.Order
	var ids []uint64
	for i := range completed {
		o := &completed[i]
		if o.Period != model.PeriodResetTraffic {
			ids = append(ids, o.ID)
		}
		if o.Period == model.PeriodOnetime && o.PlanID == *u.PlanID && (last == nil || o.ID > last.ID) {
			last = o
		}
	}
	if last != nil {
		paid := saturatingAmount(last.TotalAmount, last.BalanceAmount)
		if paid <= 0 {
			return Surplus{}
		}
		return Surplus{Paid: unusedTrafficShare(paid, u), OrderIDs: ids}
	}
	if current == nil {
		return Surplus{}
	}
	price, ok := current.Price(model.PeriodOnetime)
	if !ok || price <= 0 {
		return Surplus{}
	}
	return Surplus{Listed: afterDiscount(unusedTrafficShare(price, u), u.Discount), OrderIDs: ids}
}

// unusedTrafficShare is amount × unused / transfer_enable (whole GB as Xboard computes it), never negative.
func unusedTrafficShare(amount int64, u *model.User) int64 {
	gb := float64(u.TransferEnable) / float64(model.BytesPerGB)
	unused := gb - float64(u.Used())/float64(model.BytesPerGB)
	credit := math.Max(0, float64(amount)/gb*unused)
	if credit >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(credit)
}

// share is amount × part / whole, 0 when either duration is not positive, saturating at MaxInt64.
func share(amount int64, part, whole time.Duration) int64 {
	if amount <= 0 || part <= 0 || whole <= 0 {
		return 0
	}
	credit := float64(amount) * float64(part) / float64(whole)
	if credit >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(credit)
}

// listValue is what p's cheapest recurring price per month charges for [from, to): that period's price × the
// share of its length (counted from from) the interval takes. 0 when p is nil or sells no recurring period.
func listValue(p *model.Plan, from, to time.Time) int64 {
	if p == nil || !to.After(from) {
		return 0
	}
	price, months := int64(-1), 0
	for _, period := range model.Periods() {
		m := model.PeriodMonths(period)
		v, ok := p.Price(period)
		if m <= 0 || !ok || v < 0 {
			continue
		}
		if price < 0 || float64(v)*float64(months) < float64(price)*float64(m) {
			price, months = v, m
		}
	}
	if price <= 0 {
		return 0
	}
	return share(price, to.Sub(from), from.AddDate(0, months, 0).Sub(from))
}

// afterDiscount takes the user's percent discount off amount.
func afterDiscount(amount int64, discount *int) int64 {
	if discount == nil {
		return amount
	}
	return amount - percentOf(amount, int64(*discount))
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
