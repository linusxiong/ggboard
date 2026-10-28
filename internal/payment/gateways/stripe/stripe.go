// Package stripe implements Stripe Checkout (docs/PAYMENTS_GATEWAYS.md §5) with stripe-go v87: hosted
// Checkout Sessions in payment mode and signed webhooks (checkout.session.completed /
// checkout.session.async_payment_succeeded).
package stripe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	sdk "github.com/stripe/stripe-go/v87"

	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
)

// Name is the registry id.
const Name = "stripe"

// Payment method choices (payment_methods field).
const (
	MethodAuto      = "auto"
	MethodCard      = "card"
	MethodAlipay    = "alipay"
	MethodWeChatPay = "wechat_pay"
)

var (
	zeroDecimal  = set("bif", "clp", "djf", "gnf", "jpy", "kmf", "krw", "mga", "pyg", "rwf", "vnd", "vuv", "xaf", "xof", "xpf")
	wholeUnits   = set("isk", "ugx")
	threeDecimal = set("bhd", "jod", "kwd", "omr", "tnd")
)

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func init() { payment.Register(&Gateway{}) }

// Gateway is the Stripe plugin. Client and BackendURL override the HTTP client and API base URL (tests);
// Tolerance overrides the webhook timestamp tolerance (0 = Stripe's default 300 s, negative = ignore).
type Gateway struct {
	Client     *http.Client
	BackendURL string
	Tolerance  time.Duration
}

// Name implements payment.Gateway.
func (*Gateway) Name() string { return Name }

// Label implements payment.Gateway.
func (*Gateway) Label() string { return "Stripe Checkout" }

// Fields implements payment.Gateway.
func (*Gateway) Fields() []payment.Field {
	return []payment.Field{
		{Key: "secret_key", Label: "Secret key", Type: payment.FieldPassword, Required: true, Description: "sk_live_… / sk_test_… or a restricted rk_… key with Checkout Sessions write"},
		{Key: "webhook_secret", Label: "Webhook signing secret", Type: payment.FieldPassword, Required: true,
			Description: "whsec_… of the endpoint targeting the notify URL; subscribe to checkout.session.completed, checkout.session.async_payment_succeeded, checkout.session.async_payment_failed and checkout.session.expired"},
		{Key: "currency", Label: "Currency", Type: payment.FieldText, Description: "ISO 4217 code; defaults to the site currency"},
		{Key: "exchange_rate", Label: "Exchange rate", Type: payment.FieldNumber, Description: "Units of currency per site-currency unit; required when the currency differs from the site currency"},
		{Key: "payment_methods", Label: "Payment methods", Type: payment.FieldSelect, Default: MethodAuto,
			Options:     []string{MethodAuto, MethodCard, MethodAlipay, MethodWeChatPay},
			Description: "auto = every method enabled in the Dashboard; Alipay / WeChat Pay must be enabled there"},
	}
}

type pricing struct {
	currency string
	rate     decimal.Decimal
}

func resolvePricing(cfg map[string]string, siteCurrency string) (pricing, error) {
	site := strings.ToLower(strings.TrimSpace(siteCurrency))
	cur := strings.ToLower(strings.TrimSpace(cfg["currency"]))
	if cur == "" {
		cur = site
	}
	if len(cur) != 3 {
		return pricing{}, fmt.Errorf("stripe: invalid currency %q", cur)
	}
	if threeDecimal[cur] {
		return pricing{}, fmt.Errorf("stripe: three-decimal currency %s is not supported", strings.ToUpper(cur))
	}
	p := pricing{currency: cur, rate: decimal.NewFromInt(1)}
	if cur != site {
		if strings.TrimSpace(cfg["exchange_rate"]) == "" {
			return pricing{}, fmt.Errorf("stripe: exchange_rate is required when the currency (%s) differs from the site currency (%s)", strings.ToUpper(cur), strings.ToUpper(site))
		}
		rate, err := gwutil.ParseRate(cfg["exchange_rate"])
		if err != nil {
			return pricing{}, fmt.Errorf("stripe: %w", err)
		}
		p.rate = rate
	}
	return p, nil
}

// minorAmount converts site cents to Stripe's minor units of p.currency.
func (p pricing) minorAmount(cents int64) (int64, error) {
	minor := gwutil.ConvertCents(cents, p.rate)
	if zeroDecimal[p.currency] {
		minor = decimal.NewFromInt(cents).Mul(p.rate).Div(decimal.NewFromInt(100)).Round(0).IntPart()
	}
	if wholeUnits[p.currency] && minor%100 != 0 {
		return 0, &gwutil.UserError{Msg: fmt.Sprintf("amount must be a whole number of %s", strings.ToUpper(p.currency))}
	}
	if minor <= 0 {
		return 0, &gwutil.UserError{Msg: "amount is too small for Stripe"}
	}
	return minor, nil
}

// ValidateAmount compares the callback with the same forward quote used by Pay.
func (*Gateway) ValidateAmount(cfg map[string]string, siteCurrency string, expected int64, res *payment.NotifyResult) error {
	pr, err := resolvePricing(cfg, siteCurrency)
	if err != nil {
		return err
	}
	minor, err := pr.minorAmount(expected)
	if err != nil {
		return err
	}
	if !strings.EqualFold(res.Currency, pr.currency) || res.Amount != minor {
		return errors.New("stripe: payment amount or currency mismatch")
	}
	return nil
}

func paymentMethod(cfg map[string]string) (string, error) {
	pm := strings.ToLower(strings.TrimSpace(cfg["payment_methods"]))
	switch pm {
	case "", MethodAuto:
		return MethodAuto, nil
	case MethodCard, MethodAlipay, MethodWeChatPay:
		return pm, nil
	}
	return "", fmt.Errorf("stripe: unsupported payment method %q", pm)
}

// ValidateConfig checks the currency, exchange rate and payment method.
func (*Gateway) ValidateConfig(cfg map[string]string, siteCurrency string) error {
	if _, err := resolvePricing(cfg, siteCurrency); err != nil {
		return err
	}
	_, err := paymentMethod(cfg)
	return err
}

func (g *Gateway) client(secret string) *sdk.Client {
	bc := &sdk.BackendConfig{
		HTTPClient:    gwutil.Client(g.Client),
		LeveledLogger: &sdk.LeveledLogger{Level: sdk.LevelNull},
	}
	if g.BackendURL != "" {
		bc.URL = sdk.String(g.BackendURL)
		bc.MaxNetworkRetries = sdk.Int64(0)
	}
	return sdk.NewClient(secret, sdk.WithBackends(sdk.NewBackendsWithConfig(bc)))
}

// Pay implements payment.Gateway: it creates a Checkout Session and returns its hosted URL.
func (g *Gateway) Pay(ctx context.Context, cfg map[string]string, req payment.PayRequest) (*payment.PayResult, error) {
	if err := gwutil.Required(cfg, Name, "secret_key", "webhook_secret"); err != nil {
		return nil, err
	}
	if err := gwutil.RequireAmount(Name, req.TotalAmount); err != nil {
		return nil, err
	}
	pr, err := resolvePricing(cfg, req.Currency)
	if err != nil {
		return nil, err
	}
	minor, err := pr.minorAmount(req.TotalAmount)
	if err != nil {
		return nil, err
	}
	pm, err := paymentMethod(cfg)
	if err != nil {
		return nil, err
	}
	subject := req.Subject
	if subject == "" {
		subject = req.TradeNo
	}
	now := time.Now()
	exp := req.ExpiresAt
	if exp.Before(now.Add(31 * time.Minute)) {
		exp = now.Add(31 * time.Minute)
	}
	if exp.After(now.Add(24 * time.Hour)) {
		exp = now.Add(24 * time.Hour)
	}
	params := &sdk.CheckoutSessionCreateParams{
		Mode:              sdk.String("payment"),
		ClientReferenceID: sdk.String(req.TradeNo),
		SuccessURL:        sdk.String(req.ReturnURL),
		CancelURL:         sdk.String(req.ReturnURL),
		ExpiresAt:         sdk.Int64(exp.Unix()),
		Metadata:          map[string]string{"trade_no": req.TradeNo, "payment_uuid": gwutil.PaymentUUID(req.NotifyURL)},
		PaymentIntentData: &sdk.CheckoutSessionCreatePaymentIntentDataParams{Metadata: map[string]string{"trade_no": req.TradeNo}},
		LineItems: []*sdk.CheckoutSessionCreateLineItemParams{{
			Quantity: sdk.Int64(1),
			PriceData: &sdk.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:    sdk.String(pr.currency),
				UnitAmount:  sdk.Int64(minor),
				ProductData: &sdk.CheckoutSessionCreateLineItemPriceDataProductDataParams{Name: sdk.String(subject)},
			},
		}},
	}
	if pm != MethodAuto {
		params.AllowedPaymentMethodTypes = sdk.StringSlice([]string{pm})
	}
	if pm == MethodWeChatPay || pm == MethodAuto {
		params.PaymentMethodOptions = &sdk.CheckoutSessionCreatePaymentMethodOptionsParams{
			WeChatPay: &sdk.CheckoutSessionCreatePaymentMethodOptionsWeChatPayParams{Client: sdk.String("web")},
		}
	}
	sc := g.client(cfg["secret_key"])
	sess, err := sc.V1CheckoutSessions.Create(ctx, params)
	var se *sdk.Error
	if err != nil && pm == MethodAuto && errors.As(err, &se) && se.HTTPStatusCode == http.StatusBadRequest &&
		strings.Contains(se.Param+se.Msg, "wechat_pay") {
		// WeChat Pay disabled on the account: retry without its option (docs/PAYMENTS_GATEWAYS.md §5.1).
		params.PaymentMethodOptions = nil
		sess, err = sc.V1CheckoutSessions.Create(ctx, params)
	}
	if err != nil {
		if errors.As(err, &se) {
			slog.Warn("stripe checkout session failed", "status", se.HTTPStatusCode, "code", se.Code, "type", se.Type, "request_id", se.RequestID)
			if se.Code == sdk.ErrorCodeAmountTooSmall {
				return nil, &gwutil.UserError{Msg: "amount is below Stripe's minimum charge", Err: err}
			}
		}
		return nil, fmt.Errorf("stripe: create checkout session: %w", err)
	}
	if sess.URL == "" {
		return nil, errors.New("stripe: checkout session has no URL")
	}
	return &payment.PayResult{Type: payment.PayTypeURL, Data: sess.URL}, nil
}

type session struct {
	ID                string            `json:"id"`
	ClientReferenceID string            `json:"client_reference_id"`
	PaymentStatus     string            `json:"payment_status"`
	AmountTotal       int64             `json:"amount_total"`
	Currency          string            `json:"currency"`
	Metadata          map[string]string `json:"metadata"`
	PaymentIntent     json.RawMessage   `json:"payment_intent"`
}

func (s session) paymentIntentID() string {
	if len(s.PaymentIntent) == 0 || string(s.PaymentIntent) == "null" {
		return ""
	}
	if id, ok := sdk.ParseID(s.PaymentIntent); ok {
		return id
	}
	var obj struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(s.PaymentIntent, &obj) == nil {
		return obj.ID
	}
	return ""
}

// Notify implements payment.Gateway (webhook endpoint).
func (g *Gateway) Notify(_ context.Context, cfg map[string]string, r *http.Request) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	if err := gwutil.Required(cfg, Name, "webhook_secret"); err != nil {
		return nil, nil, err
	}
	raw, err := gwutil.ReadBody(r.Body)
	if err != nil {
		return nil, nil, err
	}
	opts := []sdk.WebhookOption{sdk.WithIgnoreAPIVersionMismatch()}
	switch {
	case g.Tolerance < 0:
		opts = append(opts, sdk.WithIgnoreTolerance())
	case g.Tolerance > 0:
		opts = append(opts, sdk.WithTolerance(g.Tolerance))
	}
	ev, err := sdk.ConstructEvent(raw, r.Header.Get("Stripe-Signature"), strings.TrimSpace(cfg["webhook_secret"]), opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("stripe: webhook verification: %w", err)
	}
	ack := &payment.NotifyResponse{Status: http.StatusOK, Body: `{"received":true}`, ContentType: "application/json"}
	typ := string(ev.Type)
	if typ != "checkout.session.completed" && typ != "checkout.session.async_payment_succeeded" {
		return nil, ack, nil
	}
	if ev.Data == nil || len(ev.Data.Raw) == 0 {
		return nil, ack, nil
	}
	var s session
	if err := json.Unmarshal(ev.Data.Raw, &s); err != nil {
		return nil, nil, fmt.Errorf("stripe: decode session: %w", err)
	}
	if uuid := gwutil.PaymentUUID(r.URL.Path); uuid == "" || s.Metadata["payment_uuid"] != uuid {
		return nil, ack, nil // another site or payment on the same account
	}
	if typ == "checkout.session.completed" && s.PaymentStatus != "paid" {
		return nil, ack, nil // async methods: wait for async_payment_succeeded
	}
	tradeNo := s.ClientReferenceID
	if tradeNo == "" {
		tradeNo = s.Metadata["trade_no"]
	}
	if tradeNo == "" {
		return nil, nil, errors.New("stripe: session without client_reference_id")
	}
	callback := s.paymentIntentID()
	if callback == "" {
		callback = s.ID
	}
	if s.AmountTotal <= 0 || strings.TrimSpace(s.Currency) == "" {
		return nil, nil, errors.New("stripe: missing or non-positive amount or currency")
	}
	return &payment.NotifyResult{TradeNo: tradeNo, CallbackNo: callback, Amount: s.AmountTotal, Currency: s.Currency}, ack, nil
}
