// Package btcpay implements BTCPay Server Greenfield invoices (docs/PAYMENTS_GATEWAYS.md §7): invoice creation
// with a hosted checkout link and BTCPay-Sig verified webhooks, confirmed by fetching the invoice. Only a
// Settled invoice that carries this panel's binding (an HMAC over payment uuid, trade_no, amount and currency)
// counts as paid.
package btcpay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
)

// Name is the registry id.
const Name = "btcpay"

// Invoice metadata keys written by Pay next to orderId/itemDesc.
const (
	metaPayment = "ggboardPayment" // payment uuid (last segment of the notify URL)
	metaBinding = "ggboardBinding" // binding(webhook_secret, uuid, trade_no, cents, currency)
)

func init() { payment.Register(&Gateway{}) }

// Gateway is the BTCPay plugin. Client overrides the shared HTTP client (tests); Now overrides the clock.
type Gateway struct {
	Client *http.Client
	Now    func() time.Time
}

// Name implements payment.Gateway.
func (*Gateway) Name() string { return Name }

// Label implements payment.Gateway.
func (*Gateway) Label() string { return "BTCPay Server" }

// Fields implements payment.Gateway.
func (*Gateway) Fields() []payment.Field {
	return []payment.Field{
		{Key: "server_url", Label: "Server URL", Type: payment.FieldText, Required: true, Description: "e.g. https://btcpay.example.com"},
		{Key: "store_id", Label: "Store ID", Type: payment.FieldText, Required: true, Description: "Store → Settings → General"},
		{Key: "api_key", Label: "API key", Type: payment.FieldPassword, Required: true,
			Description: "Account → API keys with btcpay.store.cancreateinvoice and btcpay.store.canviewinvoices"},
		{Key: "webhook_secret", Label: "Webhook secret", Type: payment.FieldPassword, Required: true,
			Description: "Store → Settings → Webhooks; payload URL = the notify URL"},
	}
}

// ValidateConfig checks the server URL.
func (*Gateway) ValidateConfig(cfg map[string]string, _ string) error {
	_, err := gwutil.BaseURL(cfg["server_url"], Name, false)
	return err
}

func (g *Gateway) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

type invoiceRequest struct {
	Amount   string            `json:"amount"`
	Currency string            `json:"currency"`
	Metadata map[string]string `json:"metadata"`
	Checkout map[string]any    `json:"checkout"`
}

type invoice struct {
	ID               string         `json:"id"`
	StoreID          string         `json:"storeId"`
	Amount           string         `json:"amount"`
	Currency         string         `json:"currency"`
	Status           string         `json:"status"`
	AdditionalStatus string         `json:"additionalStatus"`
	CheckoutLink     string         `json:"checkoutLink"`
	Metadata         map[string]any `json:"metadata"`
}

func (g *Gateway) do(ctx context.Context, cfg map[string]string, method, path string, body any) (int, []byte, error) {
	base, err := gwutil.BaseURL(cfg["server_url"], Name, false)
	if err != nil {
		return 0, nil, err
	}
	var rd *bytes.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(buf)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, rd)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "token "+strings.TrimSpace(cfg["api_key"]))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return gwutil.Do(ctx, g.Client, req, Name)
}

func storePath(cfg map[string]string) string {
	return "/api/v1/stores/" + url.PathEscape(strings.TrimSpace(cfg["store_id"])) + "/invoices"
}

// Pay implements payment.Gateway: it creates an invoice and returns its checkout link.
func (g *Gateway) Pay(ctx context.Context, cfg map[string]string, req payment.PayRequest) (*payment.PayResult, error) {
	// webhook_secret also keys the invoice binding, so an invoice is never created without it.
	if err := gwutil.Required(cfg, Name, "server_url", "store_id", "api_key", "webhook_secret"); err != nil {
		return nil, err
	}
	if err := gwutil.RequireAmount(Name, req.TotalAmount); err != nil {
		return nil, err
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		return nil, errors.New("btcpay: site currency is not set")
	}
	payUUID := gwutil.PaymentUUID(req.NotifyURL)
	if payUUID == "" {
		return nil, errors.New("btcpay: notify URL without a payment uuid")
	}
	subject := req.Subject
	if subject == "" {
		subject = req.TradeNo
	}
	checkout := map[string]any{"redirectURL": req.ReturnURL, "redirectAutomatically": true}
	if !req.ExpiresAt.IsZero() {
		checkout["expirationMinutes"] = max(1, int(math.Ceil(req.ExpiresAt.Sub(g.now()).Minutes())))
	}
	status, raw, err := g.do(ctx, cfg, http.MethodPost, storePath(cfg), invoiceRequest{
		Amount:   payment.FormatAmount(req.TotalAmount),
		Currency: currency,
		Metadata: map[string]string{
			"orderId": req.TradeNo, "itemDesc": subject,
			metaPayment: payUUID, metaBinding: binding(cfg["webhook_secret"], payUUID, req.TradeNo, req.TotalAmount, currency),
		},
		Checkout: checkout,
	})
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, fmt.Errorf("btcpay: create invoice failed (HTTP %d)", status)
	}
	var inv invoice
	if err := json.Unmarshal(raw, &inv); err != nil || inv.CheckoutLink == "" {
		return nil, errors.New("btcpay: invalid invoice response")
	}
	return &payment.PayResult{Type: payment.PayTypeURL, Data: inv.CheckoutLink}, nil
}

// binding is the hex HMAC-SHA256, keyed with the webhook secret, over the payment uuid, trade_no, amount (cents)
// and currency of an invoice Pay creates. Notify recomputes it from the fetched invoice, which proves the invoice
// was created by this panel for exactly that order, amount and currency: a store's public invoice surfaces
// (Point of Sale custom amounts, Pay Button, crowdfunds, top-ups) let anyone create and pay an invoice with a chosen
// orderId, amount and currency on the same store, and its InvoiceSettled webhook is signed like ours.
func binding(secret, paymentUUID, tradeNo string, cents int64, currency string) string {
	msg, _ := json.Marshal([]any{"ggboard/btcpay/v1", paymentUUID, tradeNo, cents, strings.ToUpper(currency)})
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(msg)
	return hex.EncodeToString(mac.Sum(nil))
}

// Signature returns "sha256=<hex HMAC-SHA256(secret, body)>".
func Signature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type webhook struct {
	Type            string         `json:"type"`
	StoreID         string         `json:"storeId"`
	InvoiceID       string         `json:"invoiceId"`
	Metadata        map[string]any `json:"metadata"`
	AfterExpiration bool           `json:"afterExpiration"`
	PartiallyPaid   bool           `json:"partiallyPaid"`
	ManuallyMarked  bool           `json:"manuallyMarked"`
}

func metaString(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

// Notify implements payment.Gateway (webhook endpoint).
func (g *Gateway) Notify(ctx context.Context, cfg map[string]string, r *http.Request) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	if err := gwutil.Required(cfg, Name, "server_url", "store_id", "api_key", "webhook_secret"); err != nil {
		return nil, nil, err
	}
	raw, err := gwutil.ReadBody(r.Body)
	if err != nil {
		return nil, nil, err
	}
	sig := strings.TrimSpace(r.Header.Get("BTCPay-Sig"))
	if sig == "" {
		return nil, nil, errors.New("btcpay: missing BTCPay-Sig")
	}
	scheme, hexSig, ok := strings.Cut(sig, "=")
	if !ok || !strings.EqualFold(scheme, "sha256") ||
		!gwutil.EqualHex(hexSig, strings.TrimPrefix(Signature(cfg["webhook_secret"], raw), "sha256=")) {
		return nil, nil, errors.New("btcpay: signature mismatch")
	}
	var wh webhook
	if err := json.Unmarshal(raw, &wh); err != nil {
		return nil, nil, fmt.Errorf("btcpay: decode webhook: %w", err)
	}
	ack := gwutil.TextResponse(http.StatusOK, "ok")
	if wh.StoreID != strings.TrimSpace(cfg["store_id"]) {
		slog.Warn("btcpay webhook for another store", "store_id", wh.StoreID, "type", wh.Type)
		return nil, ack, nil
	}
	switch wh.Type {
	case "InvoiceSettled", "InvoiceProcessing", "InvoiceReceivedPayment", "InvoicePaymentSettled":
	case "InvoiceExpired":
		if wh.PartiallyPaid {
			slog.Warn("btcpay invoice expired partially paid", "invoice_id", wh.InvoiceID, "trade_no", metaString(wh.Metadata, "orderId"))
		}
		return nil, ack, nil
	default:
		return nil, ack, nil
	}
	if wh.AfterExpiration {
		slog.Warn("btcpay payment received after expiration", "invoice_id", wh.InvoiceID, "type", wh.Type)
	}
	if wh.InvoiceID == "" {
		return nil, nil, errors.New("btcpay: webhook without invoiceId")
	}
	status, body, err := g.do(ctx, cfg, http.MethodGet, storePath(cfg)+"/"+url.PathEscape(wh.InvoiceID), nil)
	if err != nil {
		return nil, nil, err
	}
	if status != http.StatusOK {
		return nil, nil, fmt.Errorf("btcpay: fetch invoice failed (HTTP %d)", status)
	}
	var inv invoice
	if err := json.Unmarshal(body, &inv); err != nil {
		return nil, nil, fmt.Errorf("btcpay: decode invoice: %w", err)
	}
	if inv.ID != "" && inv.ID != wh.InvoiceID {
		return nil, nil, fmt.Errorf("btcpay: fetched invoice %q for webhook invoice %q", inv.ID, wh.InvoiceID)
	}
	inv.ID = wh.InvoiceID
	if inv.Status != "Settled" {
		if inv.Status == "Expired" || inv.AdditionalStatus == "PaidLate" {
			slog.Warn("btcpay invoice paid late; not fulfilled automatically", "invoice_id", wh.InvoiceID, "status", inv.Status, "additional_status", inv.AdditionalStatus)
		}
		return nil, ack, nil
	}
	return boundPayment(cfg, r, &inv, ack)
}

// boundPayment turns a fetched Settled invoice into a payment only when it carries this panel's binding for this
// payment method; everything else on the store (Point of Sale sales, other apps, another ggboard payment method,
// invoices whose orderId, amount or currency differ from what Pay created) is acknowledged without a result.
// Only the fetched invoice is trusted, never the webhook body's metadata.
func boundPayment(cfg map[string]string, r *http.Request, inv *invoice, ack *payment.NotifyResponse) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	tradeNo := metaString(inv.Metadata, "orderId")
	if tradeNo == "" {
		slog.Info("btcpay invoice without orderId ignored", "invoice_id", inv.ID)
		return nil, ack, nil
	}
	uuid := gwutil.PaymentUUID(r.URL.Path)
	if p := metaString(inv.Metadata, metaPayment); p != "" && p != uuid {
		slog.Info("btcpay invoice of another payment method ignored", "invoice_id", inv.ID, "trade_no", tradeNo)
		return nil, ack, nil
	}
	cents, err := gwutil.ParsePaidCents(inv.Amount)
	if err != nil {
		cents = 0 // zero (top-up) or unparsable: never bound, Pay rejects non-positive amounts
	}
	currency := strings.ToUpper(strings.TrimSpace(inv.Currency))
	if cents <= 0 || currency == "" || uuid == "" ||
		!gwutil.EqualHex(metaString(inv.Metadata, metaBinding), binding(cfg["webhook_secret"], uuid, tradeNo, cents, currency)) {
		slog.Warn("btcpay settled invoice is not bound to this panel's order; not fulfilled",
			"invoice_id", inv.ID, "trade_no", tradeNo, "amount", inv.Amount, "currency", inv.Currency)
		return nil, ack, nil
	}
	return &payment.NotifyResult{TradeNo: tradeNo, CallbackNo: inv.ID, Amount: cents}, ack, nil
}
