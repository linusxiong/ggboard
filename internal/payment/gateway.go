// Package payment defines the pluggable payment gateway interface and its registry (docs/PAYMENTS.md).
// Gateways live in payment/gateways/<name> and call Register from init(); order money flow lives in
// internal/service/order.
package payment

import (
	"context"
	"net/http"
	"time"

	"github.com/linusxiong/ggboard/internal/plugin"
)

// Field describes one admin configuration field of a gateway.
type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`              // text | password | number | select | textarea
	Options     []string `json:"options,omitempty"` // suggestions for select (free text is still accepted)
	Default     string   `json:"default,omitempty"`
	Required    bool     `json:"required"`
}

// Field types.
const (
	FieldText     = "text"
	FieldPassword = "password"
	FieldNumber   = "number"
	FieldSelect   = "select"
	FieldTextarea = "textarea"
)

// PayRequest is what the order service asks a gateway to charge.
type PayRequest struct {
	TradeNo     string
	TotalAmount int64     // cents actually charged = order.total_amount + handling_amount
	Currency    string    // settings.currency, e.g. CNY/USD
	Subject     string    // "<app_name> - <plan name>"
	ExpiresAt   time.Time // order.created_at + order_timeout_minutes (passed through by gateways that support it)
	UserID      uint64
	UserEmail   string
	NotifyURL   string // https://host/api/v1/guest/payment/notify/<gateway>/<uuid>
	ReturnURL   string // https://host/api/v1/guest/payment/return/<trade_no> (302 -> SPA)
	ClientIP    string
}

// Pay result types.
const (
	PayTypeURL    = "url"    // redirect to Data
	PayTypeQRCode = "qrcode" // render a QR code of Data
	PayTypeHTML   = "html"   // render Data (auto-submitting form)
)

// PayResult tells the SPA how to continue the payment.
type PayResult struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

// NotifyResult identifies the paid order from a verified callback.
type NotifyResult struct {
	TradeNo    string // our order no
	CallbackNo string // gateway transaction id
	Amount     int64  // required and > 0; site cents, or provider minor units with ValidateAmount
	Currency   string // provider currency with ValidateAmount
}

// NotifyResponse is the HTTP response the gateway expects after a callback.
type NotifyResponse struct {
	Status      int
	Body        string
	ContentType string
}

// Gateway is a payment provider plugin.
type Gateway interface {
	Name() string    // registry id, e.g. "epay"
	Label() string   // human name
	Fields() []Field // admin form schema
	Pay(ctx context.Context, cfg map[string]string, req PayRequest) (*PayResult, error)
	// Notify verifies the callback:
	//   (res, resp, nil)  verified payment: the caller marks the order paid (idempotent) and then writes resp.
	//   (nil, resp, nil)  verified but not a payment (expired/pending/partial events): the caller only writes resp.
	//   (nil, nil, err)   verification failed: the caller writes 400 text/plain "fail" and logs at WARN.
	Notify(ctx context.Context, cfg map[string]string, r *http.Request) (*NotifyResult, *NotifyResponse, error)
}

var registry = plugin.NewRegistry[Gateway]("payment gateway")

// Register adds a gateway (call from init; panics on duplicates).
func Register(gw Gateway) { registry.Register(gw.Name(), gw) }

// Get returns the gateway registered under name.
func Get(name string) (Gateway, bool) { return registry.Get(name) }

// List returns every registered gateway sorted by name.
func List() []Gateway {
	out := make([]Gateway, 0, registry.Len())
	registry.Each(func(_ string, gw Gateway) { out = append(out, gw) })
	return out
}

// Names returns the registered gateway names sorted.
func Names() []string { return registry.Names() }

// FailResponse is what the notify endpoint writes when Notify returns an error (400 text/plain "fail").
func FailResponse() *NotifyResponse {
	return &NotifyResponse{Status: http.StatusBadRequest, Body: "fail", ContentType: "text/plain; charset=utf-8"}
}
