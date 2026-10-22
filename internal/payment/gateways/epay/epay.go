// Package epay implements the 易支付 (Caihong EPay) protocol, also served by GM Pay v2's EPay-compatible
// endpoint (docs/PAYMENTS_GATEWAYS.md §2): MD5-signed submit.php redirect and GET/POST notify.
package epay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
)

// Name is the registry id.
const Name = "epay"

func init() { payment.Register(&Gateway{}) }

// Gateway is the EPay plugin. The zero value is ready to use.
type Gateway struct{}

// Name implements payment.Gateway.
func (*Gateway) Name() string { return Name }

// Label implements payment.Gateway.
func (*Gateway) Label() string { return "易支付 (EPay)" }

// Fields implements payment.Gateway.
func (*Gateway) Fields() []payment.Field {
	return []payment.Field{
		{Key: "url", Label: "网关地址", Type: payment.FieldText, Required: true,
			Description: "Base URL, e.g. https://pay.example.com (no /submit.php, no trailing slash). GM Pay v2: https://<host>/payments/epay/v1/order/create-transaction"},
		{Key: "pid", Label: "商户 ID", Type: payment.FieldText, Required: true, Description: "Merchant id"},
		{Key: "key", Label: "商户密钥", Type: payment.FieldPassword, Required: true, Description: "MD5 key"},
		{Key: "type", Label: "支付方式", Type: payment.FieldText, Options: []string{"alipay", "wxpay", "qqpay"},
			Description: "alipay / wxpay / qqpay or a provider value such as usdt.tron; empty lets the cashier ask. EPay settles in CNY."},
	}
}

// ValidateConfig checks the base URL (called by the admin API when a payment is saved).
func (*Gateway) ValidateConfig(cfg map[string]string, _ string) error {
	if _, err := gwutil.BaseURL(cfg["url"], Name, true); err != nil {
		return err
	}
	return nil
}

// Sign returns md5(signString + key) over params except sign/sign_type with empty values dropped.
func Sign(params map[string]string, key string) string {
	return gwutil.MD5Hex(gwutil.SignString(params, "sign", "sign_type") + key)
}

// Pay implements payment.Gateway: it returns the signed submit.php URL.
func (*Gateway) Pay(_ context.Context, cfg map[string]string, req payment.PayRequest) (*payment.PayResult, error) {
	if err := gwutil.Required(cfg, Name, "url", "pid", "key"); err != nil {
		return nil, err
	}
	if err := gwutil.RequireAmount(Name, req.TotalAmount); err != nil {
		return nil, err
	}
	base, err := gwutil.BaseURL(cfg["url"], Name, true)
	if err != nil {
		return nil, err
	}
	name := req.Subject
	if name == "" {
		name = req.TradeNo
	}
	params := map[string]string{
		"pid":          strings.TrimSpace(cfg["pid"]),
		"type":         strings.TrimSpace(cfg["type"]),
		"out_trade_no": req.TradeNo,
		"notify_url":   req.NotifyURL,
		"return_url":   req.ReturnURL,
		"name":         name,
		"money":        payment.FormatAmount(req.TotalAmount),
	}
	params["sign"] = Sign(params, cfg["key"])
	params["sign_type"] = "MD5"
	q := url.Values{}
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	return &payment.PayResult{Type: payment.PayTypeURL, Data: base + "/submit.php?" + q.Encode()}, nil
}

// Notify implements payment.Gateway.
func (*Gateway) Notify(_ context.Context, cfg map[string]string, r *http.Request) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	if err := gwutil.Required(cfg, Name, "pid", "key"); err != nil {
		return nil, nil, err
	}
	params, err := gwutil.Values(r)
	if err != nil {
		return nil, nil, fmt.Errorf("epay: parse callback: %w", err)
	}
	if params["sign"] == "" {
		return nil, nil, errors.New("epay: missing sign")
	}
	if !gwutil.EqualHex(params["sign"], Sign(params, cfg["key"])) {
		return nil, nil, errors.New("epay: signature mismatch")
	}
	if params["pid"] != strings.TrimSpace(cfg["pid"]) {
		return nil, nil, errors.New("epay: pid mismatch")
	}
	ok := gwutil.TextResponse(http.StatusOK, "success")
	if params["trade_status"] != "TRADE_SUCCESS" {
		return nil, ok, nil
	}
	if params["out_trade_no"] == "" {
		return nil, nil, errors.New("epay: missing out_trade_no")
	}
	amount, err := gwutil.ParsePaidCents(params["money"])
	if err != nil {
		return nil, nil, err
	}
	return &payment.NotifyResult{TradeNo: params["out_trade_no"], CallbackNo: params["trade_no"], Amount: amount}, ok, nil
}
