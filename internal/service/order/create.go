package ordersvc

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// CreateInput is a user's purchase request.
type CreateInput struct {
	PlanID     uint64
	Period     string
	CouponCode string
	Actor      *auth.Claims
}

func loadPlan(tx *gorm.DB, id uint64) (*model.Plan, error) {
	var p model.Plan
	if err := tx.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.NotFound("plan not found")
		}
		return nil, err
	}
	if p.TransferEnable < 0 || p.TransferEnable > model.MaxPlanTransferGB {
		return nil, httpx.FieldError("plan_id", "plan traffic is invalid; contact support")
	}
	return &p, nil
}

// Create prices and stores a pending order for userID (POST /user/orders). Balance used by the order is
// deducted immediately (restored on cancel) and a limited coupon loses one use. An order with nothing left to
// pay is fulfilled before Create returns (callback_no "free" or "balance").
func (s *Service) Create(ctx context.Context, userID uint64, in CreateInput) (*model.Order, error) {
	now := s.now()
	var (
		order           *model.Order
		email, planName string
	)
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, err := LockActor(tx, userID, in.Actor)
		if err != nil {
			return err
		}
		if p, err := pendingOrder(tx, u.ID); err != nil {
			return err
		} else if p != nil {
			return pendingConflict(p)
		}
		plan, err := loadPlan(tx, in.PlanID)
		if err != nil {
			return err
		}
		email, planName = u.Email, plan.Name
		if !plan.Show && (u.PlanID == nil || *u.PlanID != plan.ID) {
			return httpx.NotFound("plan not found")
		}
		price, err := validatePurchase(ctx, tx, u, plan, in.Period, now)
		if err != nil {
			return err
		}
		typ := OrderType(u, plan.ID, in.Period, now)
		if typ == model.OrderTypeUpgrade && !s.a.Settings.Bool("plan_change_enable") {
			return httpx.Forbidden("changing plans is disabled; contact support")
		}
		var coupon *model.Coupon
		if code := strings.TrimSpace(in.CouponCode); code != "" {
			if coupon, err = findCoupon(tx, code, "coupon_code"); err != nil {
				return err
			}
			if err := validateCoupon(tx, coupon, u.ID, plan.ID, in.Period, "coupon_code", now); err != nil {
				return err
			}
		}
		var (
			surplus    int64
			surplusIDs []uint64
		)
		if typ == model.OrderTypeUpgrade && s.a.Settings.Bool("surplus_enable") {
			var completed []model.Order
			if err := tx.Where("user_id = ? AND status = ?", u.ID, model.OrderStatusCompleted).Find(&completed).Error; err != nil {
				return err
			}
			surplus, surplusIDs = ComputeSurplus(u, completed, now)
		}
		q := ComputeQuote(QuoteInput{Price: price, UserDiscount: u.Discount, Coupon: coupon, Surplus: surplus, Balance: u.Balance})
		if coupon != nil {
			if err := consumeCoupon(tx, coupon, "coupon_code"); err != nil {
				return err
			}
		}
		if q.Balance > 0 {
			res := tx.Model(&model.User{}).Where("id = ? AND balance >= ?", u.ID, q.Balance).
				UpdateColumn("balance", gorm.Expr("balance - ?", q.Balance))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return httpx.Conflict("insufficient balance")
			}
		}
		tradeNo, err := uniqueTradeNo(tx, now)
		if err != nil {
			return err
		}
		order = &model.Order{
			TradeNo:         tradeNo,
			UserID:          u.ID,
			PlanID:          plan.ID,
			Type:            typ,
			Period:          in.Period,
			TotalAmount:     q.Total,
			DiscountAmount:  q.Discount,
			SurplusAmount:   q.Surplus,
			RefundAmount:    q.Refund,
			BalanceAmount:   q.Balance,
			SurplusOrderIDs: surplusIDs,
			Status:          model.OrderStatusPending,
		}
		if coupon != nil {
			order.CouponID = &coupon.ID
		}
		if u.InviteUserID != nil && q.Total > 0 {
			order.InviteUserID = u.InviteUserID
		}
		return tx.Create(order).Error
	})
	if err != nil {
		return nil, err
	}
	if order.BalanceAmount > 0 {
		s.invalidateUser(order.UserID)
	}
	s.a.Log.Info("order created", "trade_no", order.TradeNo, "user_id", order.UserID, "plan_id", order.PlanID,
		"type", order.Type, "period", order.Period, "total", order.TotalAmount, "balance", order.BalanceAmount)
	notify.Emit(s.a, notify.OrderNew(s.a, notify.Order{
		UserID: order.UserID, Email: email, Plan: planName, Period: order.Period,
		Amount: order.TotalAmount + order.BalanceAmount, TradeNo: order.TradeNo,
	}))
	if order.TotalAmount == 0 {
		cb := CallbackFree
		if order.BalanceAmount > 0 {
			cb = CallbackBalance
		}
		paid, _, err := s.paidWithActor(ctx, order.TradeNo, cb, nil, userID, in.Actor, nil)
		if err != nil {
			return nil, err
		}
		return paid, nil
	}
	return order, nil
}

// AssignInput is an administrator's order assignment (POST /admin/orders).
type AssignInput struct {
	UserID      uint64
	Email       string
	PlanID      uint64
	Period      string
	TotalAmount *int64 // nil = the plan's price for Period
	Admin       *auth.Claims
}

// Assign creates a pending order for a user without discounts, coupons, surplus or balance (Xboard admin
// assign); the user pays it through checkout (or it is confirmed with POST /admin/orders/:id/paid).
func (s *Service) Assign(ctx context.Context, in AssignInput) (*model.Order, error) {
	now := s.now()
	var order *model.Order
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if in.Admin != nil {
			if err := lockAdminActor(tx, in.Admin); err != nil {
				return err
			}
		}
		id := in.UserID
		if id == 0 {
			var u model.User
			err := tx.Where("email = ?", usersvc.NormalizeEmail(in.Email)).First(&u).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.FieldError("email", "user not found")
			}
			if err != nil {
				return err
			}
			id = u.ID
		}
		u, err := lockUser(tx, id)
		if err != nil {
			var he *httpx.Error
			if errors.As(err, &he) && he.Status == 404 {
				return httpx.FieldError("user_id", "user not found")
			}
			return err
		}
		if p, err := pendingOrder(tx, u.ID); err != nil {
			return err
		} else if p != nil {
			return httpx.Conflict("the user has an unpaid order").WithField("trade_no", p.TradeNo)
		}
		plan, err := loadPlan(tx, in.PlanID)
		if err != nil {
			return err
		}
		if !model.IsValidPeriod(in.Period) {
			return httpx.FieldError("period", "unknown period")
		}
		var total int64
		if in.TotalAmount != nil {
			total = *in.TotalAmount
		} else {
			price, ok := plan.Price(in.Period)
			if !ok {
				return httpx.FieldError("period", "this period is not available for the plan; set total_amount")
			}
			total = price
		}
		if total < 0 {
			return httpx.FieldError("total_amount", "must be greater than or equal to 0")
		}
		tradeNo, err := uniqueTradeNo(tx, now)
		if err != nil {
			return err
		}
		order = &model.Order{
			TradeNo:     tradeNo,
			UserID:      u.ID,
			PlanID:      plan.ID,
			Type:        OrderType(u, plan.ID, in.Period, now),
			Period:      in.Period,
			TotalAmount: total,
			Status:      model.OrderStatusPending,
		}
		if u.InviteUserID != nil && total > 0 {
			order.InviteUserID = u.InviteUserID
		}
		return tx.Create(order).Error
	})
	if err != nil {
		return nil, err
	}
	s.a.Log.Info("order assigned", "trade_no", order.TradeNo, "user_id", order.UserID, "plan_id", order.PlanID, "total", order.TotalAmount)
	return order, nil
}
