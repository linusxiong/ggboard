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
//   - verified payments: unknown order, payment_id or amount mismatch → ERROR + admin alert, acknowledged but
//     not paid; an already-paid order with another callback number → WARN (possible double payment); a
//     cancelled order → callback_no stored, ERROR + admin alert; otherwise the order is paid and fulfilled
//     (idempotent). Database failures → 500 so the gateway retries.
func (s *Service) HandleNotify(ctx context.Context, gatewayParam, uuid string, r *http.Request) *payment.NotifyResponse {
	log := s.a.Log.With("component", "payment_notify", "gateway", gatewayParam, "payment_uuid", uuid)
	var p model.Payment
	if err := s.a.DB.WithContext(ctx).Where("uuid = ?", uuid).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn("payment notify for unknown payment")
			return payment.FailResponse()
		}
		log.Error("payment notify: load payment", "err", err)
		return internalFail()
	}
	log = log.With("payment_id", p.ID)
	if !GatewayMatches(gatewayParam, p.Gateway) {
		log.Warn("payment notify gateway mismatch", "expected", p.Gateway)
		return payment.FailResponse()
	}
	if !p.Enabled {
		log.Warn("payment notify for disabled payment")
		return payment.FailResponse()
	}
	gw, ok := payment.Get(p.Gateway)
	if !ok {
		log.Error("payment notify: gateway not registered", "expected", p.Gateway)
		return payment.FailResponse()
	}
	res, resp, err := gw.Notify(ctx, map[string]string(p.Config), r)
	if err != nil {
		log.Warn("payment notify verification failed", "err", err)
		return payment.FailResponse()
	}
	if resp == nil {
		resp = &payment.NotifyResponse{Status: http.StatusOK, Body: "success", ContentType: "text/plain; charset=utf-8"}
	}
	if res == nil {
		log.Info("payment notify acknowledged (not a payment)")
		return resp
	}
	log = log.With("trade_no", res.TradeNo, "callback_no", res.CallbackNo, "reported_amount", res.Amount)
	var o model.Order
	if err := s.a.DB.WithContext(ctx).Where("trade_no = ?", res.TradeNo).First(&o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Error("verified payment for an unknown order")
			s.alertAdmin("Payment received for an unknown order",
				fmt.Sprintf("Payment method %q (#%d) confirmed a payment for order %s (gateway transaction %s), but the order does not exist. Check the payment provider and refund or credit the customer manually.",
					p.Name, p.ID, res.TradeNo, res.CallbackNo))
			return resp
		}
		log.Error("payment notify: load order", "err", err)
		return internalFail()
	}
	switch o.Status {
	case model.OrderStatusCompleted, model.OrderStatusDiscounted, model.OrderStatusProcessing:
		if o.CallbackNo != res.CallbackNo {
			log.Warn("payment notify for an already paid order with another transaction: possible double payment", "order_callback_no", o.CallbackNo)
		}
		return resp
	case model.OrderStatusCancelled:
		return s.paidAfterCancel(ctx, log, &p, &o, res, resp)
	}
	if o.PaymentID == nil || *o.PaymentID != p.ID {
		log.Error("verified payment through a payment method the order was not checked out with", "order_payment_id", o.PaymentID)
		s.alertAdmin("Payment received through an unexpected payment method",
			fmt.Sprintf("Order %s was paid through payment method %q (#%d), gateway transaction %s, but the order was checked out with another method. It was NOT fulfilled; verify the payment and confirm the order manually.",
				o.TradeNo, p.Name, p.ID, res.CallbackNo))
		return resp
	}
	if expected := o.TotalAmount + o.HandlingAmount; res.Amount != 0 && res.Amount != expected {
		log.Error("verified payment amount mismatch; order not fulfilled", "expected", expected)
		s.alertAdmin("Payment amount mismatch",
			fmt.Sprintf("Order %s expected %s but payment method %q reported %s (gateway transaction %s). It was NOT fulfilled; verify the payment and confirm the order manually.",
				o.TradeNo, payment.FormatAmount(expected), p.Name, payment.FormatAmount(res.Amount), res.CallbackNo))
		return resp
	}
	if _, already, err := s.Paid(ctx, o.TradeNo, res.CallbackNo); err != nil {
		if errors.Is(err, ErrOrderCancelled) {
			return s.paidAfterCancel(ctx, log, &p, &o, res, resp)
		}
		log.Error("payment notify: fulfil failed", "err", err)
		return internalFail()
	} else if !already {
		log.Info("order paid through gateway")
	}
	return resp
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
	amount := res.Amount
	if amount == 0 {
		amount = o.TotalAmount + o.HandlingAmount
	}
	s.alertAdmin("Payment received for a cancelled order",
		fmt.Sprintf("Order %s was cancelled but payment method %q (#%d) confirmed a payment of %s (gateway transaction %s). Fulfil it with a new assigned order or refund the customer.",
			o.TradeNo, p.Name, p.ID, payment.FormatAmount(amount), res.CallbackNo))
	return resp
}
