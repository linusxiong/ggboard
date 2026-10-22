package ordersvc

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/model"
)

// Brief is a {id, name} reference.
type Brief struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// UserBrief is a {id, email} reference.
type UserBrief struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
}

// View is the Order shape of docs/API.md (user endpoints).
type View struct {
	ID               uint64                 `json:"id"`
	TradeNo          string                 `json:"trade_no"`
	UserID           uint64                 `json:"user_id"`
	PlanID           uint64                 `json:"plan_id"`
	Plan             *Brief                 `json:"plan"`
	Type             model.OrderType        `json:"type"`
	Period           string                 `json:"period"`
	Status           model.OrderStatus      `json:"status"`
	CommissionStatus model.CommissionStatus `json:"commission_status"`
	TotalAmount      int64                  `json:"total_amount"`
	HandlingAmount   int64                  `json:"handling_amount"`
	DiscountAmount   int64                  `json:"discount_amount"`
	SurplusAmount    int64                  `json:"surplus_amount"`
	RefundAmount     int64                  `json:"refund_amount"`
	BalanceAmount    int64                  `json:"balance_amount"`
	PaymentID        *uint64                `json:"payment_id"`
	Payment          *Brief                 `json:"payment"`
	CallbackNo       string                 `json:"callback_no"`
	PaidAt           *time.Time             `json:"paid_at"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// AdminView adds the commission, invite, coupon and user fields for administrators.
type AdminView struct {
	View
	User              *UserBrief `json:"user"`
	InviteUserID      *uint64    `json:"invite_user_id"`
	CouponID          *uint64    `json:"coupon_id"`
	CommissionBalance int64      `json:"commission_balance"`
	SurplusOrderIDs   []uint64   `json:"surplus_order_ids"`
}

type refs struct {
	plans    map[uint64]*Brief
	payments map[uint64]*Brief
	users    map[uint64]*UserBrief
}

func loadRefs(ctx context.Context, db *gorm.DB, orders []model.Order, withUsers bool) (refs, error) {
	r := refs{plans: map[uint64]*Brief{}, payments: map[uint64]*Brief{}, users: map[uint64]*UserBrief{}}
	if len(orders) == 0 {
		return r, nil
	}
	var planIDs, payIDs, userIDs []uint64
	for _, o := range orders {
		planIDs = append(planIDs, o.PlanID)
		if o.PaymentID != nil {
			payIDs = append(payIDs, *o.PaymentID)
		}
		userIDs = append(userIDs, o.UserID)
	}
	db = db.WithContext(ctx)
	var plans []model.Plan
	if err := db.Select("id", "name").Where("id IN ?", planIDs).Find(&plans).Error; err != nil {
		return r, err
	}
	for _, p := range plans {
		r.plans[p.ID] = &Brief{ID: p.ID, Name: p.Name}
	}
	if len(payIDs) > 0 {
		var pays []model.Payment
		if err := db.Select("id", "name").Where("id IN ?", payIDs).Find(&pays).Error; err != nil {
			return r, err
		}
		for _, p := range pays {
			r.payments[p.ID] = &Brief{ID: p.ID, Name: p.Name}
		}
	}
	if withUsers {
		var users []model.User
		if err := db.Select("id", "email").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return r, err
		}
		for _, u := range users {
			r.users[u.ID] = &UserBrief{ID: u.ID, Email: u.Email}
		}
	}
	return r, nil
}

func (r refs) view(o *model.Order) View {
	v := View{
		ID: o.ID, TradeNo: o.TradeNo, UserID: o.UserID, PlanID: o.PlanID, Plan: r.plans[o.PlanID], Type: o.Type,
		Period: o.Period, Status: o.Status, CommissionStatus: o.CommissionStatus, TotalAmount: o.TotalAmount,
		HandlingAmount: o.HandlingAmount, DiscountAmount: o.DiscountAmount, SurplusAmount: o.SurplusAmount,
		RefundAmount: o.RefundAmount, BalanceAmount: o.BalanceAmount, PaymentID: o.PaymentID, CallbackNo: o.CallbackNo,
		PaidAt: o.PaidAt, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
	if o.PaymentID != nil {
		v.Payment = r.payments[*o.PaymentID]
	}
	return v
}

// Views builds user-facing views (plan and payment names loaded in two queries).
func Views(ctx context.Context, db *gorm.DB, orders []model.Order) ([]View, error) {
	r, err := loadRefs(ctx, db, orders, false)
	if err != nil {
		return nil, err
	}
	out := make([]View, len(orders))
	for i := range orders {
		out[i] = r.view(&orders[i])
	}
	return out, nil
}

// AdminViews builds administrator views (plan, payment and user email loaded in three queries).
func AdminViews(ctx context.Context, db *gorm.DB, orders []model.Order) ([]AdminView, error) {
	r, err := loadRefs(ctx, db, orders, true)
	if err != nil {
		return nil, err
	}
	out := make([]AdminView, len(orders))
	for i := range orders {
		o := &orders[i]
		ids := []uint64(o.SurplusOrderIDs)
		if ids == nil {
			ids = []uint64{}
		}
		out[i] = AdminView{
			View: r.view(o), User: r.users[o.UserID], InviteUserID: o.InviteUserID, CouponID: o.CouponID,
			CommissionBalance: o.CommissionBalance, SurplusOrderIDs: ids,
		}
	}
	return out, nil
}
