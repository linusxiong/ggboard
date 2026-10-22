package ordersvc

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

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
	// BaseURL (scheme://host of the request) is used when settings.app_url is empty.
	BaseURL string
}

// HandlingAmount is fixed + round(total × percent / 100) (zero when the payment charges no fee).
func HandlingAmount(p *model.Payment, total int64) int64 {
	if p.HandlingFeeFixed == 0 && p.HandlingFeePercent == 0 {
		return 0
	}
	fee := decimal.NewFromInt(total).Mul(decimal.NewFromFloat(p.HandlingFeePercent)).Div(decimal.NewFromInt(100)).
		Add(decimal.NewFromInt(p.HandlingFeeFixed)).Round(0).IntPart()
	return max(fee, 0)
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
	if o.TotalAmount <= 0 {
		cb := CallbackFree
		if o.BalanceAmount > 0 {
			cb = CallbackBalance
		}
		if _, _, err := s.Paid(ctx, o.TradeNo, cb); err != nil {
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
	res := db.Model(&model.Order{}).Where("id = ? AND status = ?", o.ID, model.OrderStatusPending).
		Updates(map[string]any{"payment_id": p.ID, "handling_amount": handling})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, httpx.Conflict("the order is no longer pending")
	}
	var (
		plan model.Plan
		user model.User
	)
	if err := db.Select("id", "name").Limit(1).Find(&plan, o.PlanID).Error; err != nil {
		return nil, err
	}
	if err := db.Select("id", "email").Limit(1).Find(&user, o.UserID).Error; err != nil {
		return nil, err
	}
	subject := s.a.Settings.String("app_name")
	if plan.Name != "" {
		subject += " - " + plan.Name
	}
	req := payment.PayRequest{
		TradeNo:     o.TradeNo,
		TotalAmount: o.TotalAmount + handling,
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
