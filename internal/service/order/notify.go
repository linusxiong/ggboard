package ordersvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/payment"
)

// xboardAliases maps lower-cased Xboard payment class names (used in Xboard notify URLs) to gateway ids.
var xboardAliases = map[string]string{
	"epay":           "epay",
	"epusdtpay":      "epusdt",
	"alipayf2f":      "alipay_f2f",
	"stripeallinone": "stripe",
	"stripealipay":   "stripe",
	"stripewepay":    "stripe",
	"stripecredit":   "stripe",
	"stripecheckout": "stripe",
	"coinbase":       "coinbase",
	"btcpay":         "btcpay",
}

// GatewayMatches reports whether the :gateway path segment names gateway (its id, or an Xboard class name).
func GatewayMatches(param, gateway string) bool {
	p := strings.ToLower(strings.TrimSpace(param))
	return p == gateway || xboardAliases[p] == gateway
}

func internalFail() *payment.NotifyResponse {
	return &payment.NotifyResponse{Status: http.StatusInternalServerError, Body: "fail", ContentType: "text/plain; charset=utf-8"}
}

// HandleNotify processes ANY /guest/payment/notify/:gateway/:uuid and returns the response to write
// (docs/PAYMENTS.md §Order lifecycle step 3):
//
//   - unknown/disabled payment, gateway mismatch or failed verification → 400 "fail" (WARN);
//   - verified non-payment events → the gateway's acknowledgement;
//   - verified payments: unknown order or amount mismatch → ERROR + admin alert, acknowledged but not paid (a
//     payment through an earlier checkout's method is charged at that method's fee and re-points the order); an already-paid order with another callback number → WARN (possible double payment); a
//     cancelled order → callback_no stored, ERROR + admin alert; otherwise the order is paid and fulfilled
//     (idempotent). Database failures → 500 so the gateway retries.
func (s *Service) HandleNotify(ctx context.Context, gatewayParam, uuid string, r *http.Request) *payment.NotifyResponse {
	resp, _ := s.HandleNotifyVerified(ctx, gatewayParam, uuid, r)
	return resp
}

// HandleNotifyVerified is HandleNotify that also reports whether the request passed the gateway's verification. It
// is false for unknown or disabled payments, gateway mismatches and failed signatures: requests anyone can send,
// which the guest route charges to its per-IP miss bucket (a verified callback, retried or not, never is).
func (s *Service) HandleNotifyVerified(ctx context.Context, gatewayParam, uuid string, r *http.Request) (resp *payment.NotifyResponse, verified bool) {
	log := s.a.Log.With("component", "payment_notify", "gateway", gatewayParam, "payment_uuid", uuid)
	var p model.Payment
	if err := s.a.DB.WithContext(ctx).Where("uuid = ?", uuid).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn("payment notify for unknown payment")
			return payment.FailResponse(), false
		}
		log.Error("payment notify: load payment", "err", err)
		return internalFail(), false
	}
	log = log.With("payment_id", p.ID)
	if !GatewayMatches(gatewayParam, p.Gateway) {
		log.Warn("payment notify gateway mismatch", "expected", p.Gateway)
		return payment.FailResponse(), false
	}
	if !p.Enabled {
		log.Warn("payment notify for disabled payment")
		return payment.FailResponse(), false
	}
	gw, ok := payment.Get(p.Gateway)
	if !ok {
		log.Error("payment notify: gateway not registered", "expected", p.Gateway)
		return payment.FailResponse(), false
	}
	res, resp, err := gw.Notify(ctx, map[string]string(p.Config), r)
	if err != nil {
		log.Warn("payment notify verification failed", "err", err)
		return payment.FailResponse(), false
	}
	if resp == nil {
		resp = &payment.NotifyResponse{Status: http.StatusOK, Body: "success", ContentType: "text/plain; charset=utf-8"}
	}
	if res == nil {
		log.Info("payment notify acknowledged (not a payment)")
		return resp, true
	}
	if res.Amount <= 0 {
		log.Warn("payment notify without a positive amount")
		return payment.FailResponse(), true
	}
	log = log.With("trade_no", res.TradeNo, "callback_no", res.CallbackNo, "reported_amount", res.Amount)
	var o model.Order
	if err := s.a.DB.WithContext(ctx).Where("trade_no = ?", res.TradeNo).First(&o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Error("verified payment for an unknown order")
			s.alertAdmin("Payment received for an unknown order",
				fmt.Sprintf("Payment method %q (#%d) confirmed a payment for order %s (gateway transaction %s), but the order does not exist. Check the payment provider and refund or credit the customer manually.",
					p.Name, p.ID, res.TradeNo, res.CallbackNo))
			return resp, true
		}
		log.Error("payment notify: load order", "err", err)
		return internalFail(), true
	}
	switch o.Status {
	case model.OrderStatusCompleted, model.OrderStatusDiscounted, model.OrderStatusProcessing:
		if o.CallbackNo != res.CallbackNo {
			log.Warn("payment notify for an already paid order with another transaction: possible double payment", "order_callback_no", o.CallbackNo)
		}
		return resp, true
	case model.OrderStatusCancelled:
		return s.paidAfterCancel(ctx, log, &p, &o, res, resp), true
	}
	handling := o.HandlingAmount
	if o.PaymentID == nil || *o.PaymentID != p.ID {
		// The buyer paid the page of an earlier checkout with another method: they switched methods, or two checkouts
		// raced, and the order now points at the later one. Only a checkout can open a page for this order at p's
		// merchant account, so the verified payment counts, charged at p's own fee; the claiming update re-points the
		// order to p. A fee that changed since that checkout fails the amount check below (alert, not fulfilled).
		handling = HandlingAmount(&p, o.TotalAmount)
		log.Warn("verified payment through an earlier checkout's payment method; charging it at that method's fee", "order_payment_id", o.PaymentID)
	}
	expected, validCharge := chargeAmount(o.TotalAmount, handling)
	var amountErr error
	if !validCharge {
		amountErr = errors.New("invalid stored payment charge")
	} else if validator, ok := gw.(interface {
		ValidateAmount(map[string]string, string, int64, *payment.NotifyResult) error
	}); ok {
		amountErr = validator.ValidateAmount(map[string]string(p.Config), s.a.Settings.String("currency"), expected, res)
	} else if res.Amount != expected {
		amountErr = errors.New("payment amount mismatch")
	}
	if amountErr != nil {
		log.Error("verified payment amount mismatch; order not fulfilled", "expected", expected, "err", amountErr, "reported_currency", res.Currency)
		s.alertAdmin("Payment amount mismatch",
			fmt.Sprintf("Order %s expected a charge for %s %s but payment method %q reported %s (gateway transaction %s). It was NOT fulfilled; verify the payment and confirm the order manually.",
				o.TradeNo, payment.FormatAmount(expected), s.a.Settings.String("currency"), p.Name, notifyAmount(res), res.CallbackNo))
		return resp, true
	}
	if _, already, err := s.paidVia(ctx, o.TradeNo, res.CallbackNo, &o, p.ID, handling); err != nil {
		if errors.Is(err, ErrOrderCancelled) {
			return s.paidAfterCancel(ctx, log, &p, &o, res, resp), true
		}
		log.Error("payment notify: fulfil failed", "err", err)
		return internalFail(), true
	} else if !already {
		log.Info("order paid through gateway")
	}
	return resp, true
}

// paidAfterCancel records a verified payment for an order that order_timeout (or the user) already
// cancelled: the callback number is stored, the event logged at ERROR and the administrator alerted.
func (s *Service) paidAfterCancel(ctx context.Context, log *slog.Logger, p *model.Payment, o *model.Order, res *payment.NotifyResult, resp *payment.NotifyResponse) *payment.NotifyResponse {
	// Conditional on a new callback number: gateways retry and EPay hands the buyer the signed parameters on the
	// return redirect, so the same notification can be replayed any number of times; alert once per transaction.
	q := s.a.DB.WithContext(ctx).Model(&model.Order{}).Where("id = ? AND status = ?", o.ID, model.OrderStatusCancelled)
	if res.CallbackNo != "" {
		q = q.Where("(callback_no IS NULL OR callback_no <> ?)", res.CallbackNo)
	}
	upd := q.UpdateColumn("callback_no", res.CallbackNo)
	if upd.Error != nil {
		log.Error("payment notify: store callback_no of cancelled order", "err", upd.Error)
		return internalFail()
	}
	if upd.RowsAffected == 0 {
		log.Warn("repeated payment notify for a cancelled order; already reported")
		return resp
	}
	log.Error("verified payment for a cancelled order; not fulfilled")
	s.alertAdmin("Payment received for a cancelled order",
		fmt.Sprintf("Order %s was cancelled but payment method %q (#%d) confirmed a payment of %s (gateway transaction %s). Fulfil it with a new assigned order or refund the customer.",
			o.TradeNo, p.Name, p.ID, notifyAmount(res), res.CallbackNo))
	return resp
}

func notifyAmount(res *payment.NotifyResult) string {
	if res.Currency != "" {
		return fmt.Sprintf("%d minor units of %s", res.Amount, res.Currency)
	}
	return payment.FormatAmount(res.Amount)
}
