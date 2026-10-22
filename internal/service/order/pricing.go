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
	Surplus      int64         // upgrade credit (0 unless an upgrade with surplus_enable)
	Balance      int64         // users.balance available
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
		d = percentOf(price, c.Value)
	}
	return min(max(d, 0), price)
}

// ComputeQuote applies, in order: user discount and coupon (both on the plan price, Xboard), upgrade surplus
// and the account balance.
func ComputeQuote(in QuoteInput) Quote {
	q := Quote{Price: max(in.Price, 0)}
	discount := CouponDiscount(in.Coupon, q.Price)
	if in.UserDiscount != nil {
		discount += percentOf(q.Price, int64(*in.UserDiscount))
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

// ComputeSurplus returns the upgrade credit of u's current plan (upstream Xboard getSurplusValue) from the
// user's completed orders:
//
//   - one-time plan (expired_at NULL): the last completed one-time order of the current plan is worth
//     (total + balance) / transfer_GB per GB; the credit is the unused traffic's worth. Surplus orders are
//     every completed non-reset order.
//   - periodic plan: the completed periodic orders of the current plan since its latest new-purchase or
//     upgrade order cover [first paid_at, first + Σ months); the credit is Σ(total + balance + surplus −
//     refund) × remaining/total time, with the end capped at the user's actual expired_at.
func ComputeSurplus(u *model.User, completed []model.Order, now time.Time) (int64, []uint64) {
	if u.PlanID == nil {
		return 0, nil
	}
	planID := *u.PlanID
	if u.ExpiredAt == nil {
		var last *model.Order
		for i := range completed {
			o := &completed[i]
			if o.Period == model.PeriodOnetime && o.PlanID == planID && (last == nil || o.ID > last.ID) {
				last = o
			}
		}
		if last == nil || u.TransferEnable <= 0 {
			return 0, nil
		}
		paid := last.TotalAmount + last.BalanceAmount
		if paid <= 0 {
			return 0, nil
		}
		gb := float64(u.TransferEnable) / float64(model.BytesPerGB)
		unused := gb - float64(u.U+u.D)/float64(model.BytesPerGB)
		credit := int64(math.Max(0, float64(paid)/gb*unused))
		var ids []uint64
		for _, o := range completed {
			if o.Period != model.PeriodResetTraffic {
				ids = append(ids, o.ID)
			}
		}
		return credit, ids
	}
	var periodic []model.Order
	for _, o := range completed {
		if o.PlanID == planID && orderMonths(&o) > 0 {
			periodic = append(periodic, o)
		}
	}
	if len(periodic) == 0 {
		return 0, nil
	}
	// The streak starts at the latest new purchase or upgrade; earlier orders were superseded.
	var streakStart time.Time
	for i := range periodic {
		o := &periodic[i]
		if (o.Type == model.OrderTypeNew || o.Type == model.OrderTypeUpgrade) && orderPaidAt(o).After(streakStart) {
			streakStart = orderPaidAt(o)
		}
	}
	periodic = slices.DeleteFunc(periodic, func(o model.Order) bool { return orderPaidAt(&o).Before(streakStart) })
	var (
		sum    int64
		months int
		first  time.Time
		ids    []uint64
	)
	for i := range periodic {
		o := &periodic[i]
		sum += o.TotalAmount + o.BalanceAmount + o.SurplusAmount - o.RefundAmount
		months += orderMonths(o)
		if first.IsZero() || orderPaidAt(o).Before(first) {
			first = orderPaidAt(o)
		}
		ids = append(ids, o.ID)
	}
	end := first.AddDate(0, months, 0)
	span := end.Sub(first)
	if u.ExpiredAt.Before(end) {
		end = *u.ExpiredAt
	}
	remain := end.Sub(now)
	if span <= 0 || remain <= 0 || sum <= 0 {
		return 0, ids
	}
	return int64(float64(sum) * float64(remain) / float64(span)), ids
}
