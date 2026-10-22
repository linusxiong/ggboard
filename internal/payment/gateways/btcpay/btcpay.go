// Package btcpay implements BTCPay Server Greenfield invoices (docs/PAYMENTS_GATEWAYS.md §7): invoice creation
// with a hosted checkout link and BTCPay-Sig verified webhooks, confirmed by fetching the invoice. Only a
// Settled invoice counts as paid.
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
	if err := gwutil.Required(cfg, Name, "server_url", "store_id", "api_key"); err != nil {
		return nil, err
	}
	if err := gwutil.RequireAmount(Name, req.TotalAmount); err != nil {
		return nil, err
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
		Currency: strings.ToUpper(req.Currency),
		Metadata: map[string]string{"orderId": req.TradeNo, "itemDesc": subject},
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
	if inv.Status != "Settled" {
		if inv.Status == "Expired" || inv.AdditionalStatus == "PaidLate" {
			slog.Warn("btcpay invoice paid late; not fulfilled automatically", "invoice_id", wh.InvoiceID, "status", inv.Status, "additional_status", inv.AdditionalStatus)
		}
		return nil, ack, nil
	}
	tradeNo := metaString(inv.Metadata, "orderId")
	if tradeNo == "" {
		tradeNo = metaString(wh.Metadata, "orderId")
	}
	if tradeNo == "" {
		return nil, nil, errors.New("btcpay: invoice without orderId")
	}
	res := &payment.NotifyResult{TradeNo: tradeNo, CallbackNo: wh.InvoiceID}
	if c, err := gwutil.ParseCents(inv.Amount); err == nil {
		res.Amount = c
	}
	return res, ack, nil
}
