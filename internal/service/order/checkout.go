package ordersvc

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/payment"
)

// PayTypePaid is the checkout result type of an order that needs no gateway (already paid or free).
const PayTypePaid = "paid"

// payTimeout bounds a gateway's Pay call.
const payTimeout = 30 * time.Second

// CheckoutInput is POST /user/orders/:trade_no/checkout.
type CheckoutInput struct {
	PaymentID *uint64
	ClientIP  string
	Actor     *auth.Claims
	// BaseURL (scheme://host of the request) is used when settings.app_url is empty.
	BaseURL string
}

// HandlingAmount is fixed + round(total × percent / 100) (zero when the payment charges no fee).
func HandlingAmount(p *model.Payment, total int64) int64 {
	if p.HandlingFeeFixed == 0 && p.HandlingFeePercent == 0 {
		return 0
	}
	fee := decimal.NewFromInt(total).Mul(decimal.NewFromFloat(p.HandlingFeePercent)).Div(decimal.NewFromInt(100)).
		Add(decimal.NewFromInt(p.HandlingFeeFixed)).Round(0)
	if fee.LessThanOrEqual(decimal.Zero) {
		return 0
	}
	if fee.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return math.MaxInt64
	}
	return fee.IntPart()
}

func badPayment() error {
	return httpx.FieldError("payment_id", "payment method is not available")
}

// userMessager is implemented by gateway errors whose message may be shown to the user.
type userMessager interface{ UserMessage() string }

// Checkout starts paying userID's order tradeNo. Paid orders (and orders with nothing to pay, which are
// fulfilled here) return {type:"paid"}; otherwise the payment's gateway is called and its result returned.
func (s *Service) Checkout(ctx context.Context, userID uint64, tradeNo string, in CheckoutInput) (*payment.PayResult, error) {
	db := s.a.DB.WithContext(ctx)
	var o model.Order
	if err := db.Where("trade_no = ? AND user_id = ?", tradeNo, userID).First(&o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.NotFound("order not found")
		}
		return nil, err
	}
	paid := &payment.PayResult{Type: PayTypePaid, Data: o.TradeNo}
	switch o.Status {
	case model.OrderStatusCompleted, model.OrderStatusDiscounted, model.OrderStatusProcessing:
		return paid, nil
	case model.OrderStatusCancelled:
		return nil, httpx.Conflict("the order is cancelled")
	}
	if o.TotalAmount < 0 {
		return nil, httpx.Conflict("the order amount is invalid")
	}
	if o.TotalAmount == 0 {
		cb := CallbackFree
		if o.BalanceAmount > 0 {
			cb = CallbackBalance
		}
		if _, _, err := s.paidWithActor(ctx, o.TradeNo, cb, nil, userID, in.Actor, nil); err != nil {
			if errors.Is(err, ErrOrderCancelled) {
				return nil, httpx.Conflict("the order is cancelled")
			}
			return nil, err
		}
		return paid, nil
	}
	if in.PaymentID == nil || *in.PaymentID == 0 {
		return nil, httpx.FieldError("payment_id", "is required")
	}
	// No new payment page for an order past its deadline: order_timeout is about to cancel it, and a payment
	// started now would arrive after the cancellation.
	if timeout := time.Duration(s.a.Settings.Int("order_timeout_minutes")) * time.Minute; timeout > 0 &&
		!s.now().Before(o.CreatedAt.Add(timeout)) {
		return nil, httpx.Conflict("the order has expired, please place a new one")
	}
	var p model.Payment
	if err := db.First(&p, *in.PaymentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, badPayment()
		}
		return nil, err
	}
	gw, ok := payment.Get(p.Gateway)
	if !p.Enabled || !ok {
		return nil, badPayment()
	}
	handling := HandlingAmount(&p, o.TotalAmount)
	charge, ok := chargeAmount(o.TotalAmount, handling)
	if !ok {
		return nil, httpx.Conflict("the order amount is invalid")
	}
	var user model.User
	if err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Order{}).Where("id = ? AND user_id = ? AND status = ?", o.ID, userID, model.OrderStatusPending).
			Updates(map[string]any{"payment_id": p.ID, "handling_amount": handling})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return httpx.Conflict("the order is no longer pending")
		}
		u, err := LockActor(tx, userID, in.Actor)
		if err != nil {
			return err
		}
		user = *u
		return nil
	}); err != nil {
		return nil, err
	}
	var plan model.Plan
	if err := db.Select("id", "name").Limit(1).Find(&plan, o.PlanID).Error; err != nil {
		return nil, err
	}
	subject := s.a.Settings.String("app_name")
	if plan.Name != "" {
		subject += " - " + plan.Name
	}
	req := payment.PayRequest{
		TradeNo:     o.TradeNo,
		TotalAmount: charge,
		Currency:    s.a.Settings.String("currency"),
		Subject:     subject,
		ExpiresAt:   o.CreatedAt.Add(time.Duration(s.a.Settings.Int("order_timeout_minutes")) * time.Minute),
		UserID:      o.UserID,
		UserEmail:   user.Email,
		NotifyURL:   s.PaymentNotifyURL(&p, in.BaseURL),
		ReturnURL:   s.SiteURL(in.BaseURL) + ReturnPath + o.TradeNo,
		ClientIP:    in.ClientIP,
	}
	pctx, cancel := context.WithTimeout(ctx, payTimeout)
	defer cancel()
	start := time.Now()
	result, err := gw.Pay(pctx, map[string]string(p.Config), req)
	if err != nil {
		s.a.Log.Warn("payment gateway pay failed", "gateway", p.Gateway, "payment_id", p.ID, "trade_no", o.TradeNo,
			"latency", time.Since(start).String(), "err", err)
		var um userMessager
		if errors.As(err, &um) && um.UserMessage() != "" {
			return nil, httpx.PaymentFailed(um.UserMessage())
		}
		return nil, httpx.PaymentFailed("payment gateway error")
	}
	if result == nil || result.Type == "" {
		return nil, httpx.PaymentFailed("payment gateway error")
	}
	s.a.Log.Info("payment started", "gateway", p.Gateway, "payment_id", p.ID, "trade_no", o.TradeNo,
		"amount", req.TotalAmount, "type", result.Type, "latency", time.Since(start).String())
	return result, nil
}
