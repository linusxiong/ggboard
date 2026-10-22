// Package coinbase implements Coinbase Business Checkouts (USDC on Base; docs/PAYMENTS_GATEWAYS.md §6):
// CDP JWT (ES256) authenticated checkout creation and HMAC-SHA256 signed webhooks. The legacy Coinbase
// Commerce Charge API closed on 2026-03-31 and is not implemented.
package coinbase

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
)

// Name is the registry id.
const Name = "coinbase"

// DefaultBaseURL is the Coinbase Business API host.
const DefaultBaseURL = "https://business.coinbase.com"

// SignatureTolerance is the accepted webhook timestamp skew.
const SignatureTolerance = 300 * time.Second

// idempotencyNS namespaces the deterministic X-Idempotency-Key.
var idempotencyNS = uuid.MustParse("6f1c9a52-6a2e-4c41-9f0b-5b0d7c3e2a11")

func init() { payment.Register(&Gateway{}) }

// Gateway is the Coinbase plugin. Client, BaseURL and Now override the HTTP client, API host and clock (tests).
type Gateway struct {
	Client  *http.Client
	BaseURL string
	Now     func() time.Time
}

// Name implements payment.Gateway.
func (*Gateway) Name() string { return Name }

// Label implements payment.Gateway.
func (*Gateway) Label() string { return "Coinbase Business (USDC)" }

// Fields implements payment.Gateway.
func (*Gateway) Fields() []payment.Field {
	return []payment.Field{
		{Key: "api_key_name", Label: "CDP API key name", Type: payment.FieldText, Required: true, Description: "organizations/{org_id}/apiKeys/{key_id}"},
		{Key: "api_key_secret", Label: "CDP API key secret", Type: payment.FieldTextarea, Required: true, Description: "ECDSA P-256 private key (PEM; literal \\n accepted). Create the key as ECDSA."},
		{Key: "webhook_secret", Label: "Webhook secret", Type: payment.FieldPassword, Required: true,
			Description: "Secret of the webhook subscription (events checkout.payment.success/failed/expired, checkout.refund.*) targeting the notify URL"},
		{Key: "exchange_rate", Label: "Exchange rate", Type: payment.FieldNumber, Default: "1", Description: "USDC per site-currency unit; 1 is only valid for a USD site"},
	}
}

// ValidateConfig parses the EC key and requires an explicit rate for non-USD sites.
func (*Gateway) ValidateConfig(cfg map[string]string, siteCurrency string) error {
	if _, err := ParseKey(cfg["api_key_secret"]); err != nil {
		return err
	}
	if _, err := gwutil.ParseRate(cfg["exchange_rate"]); err != nil {
		return fmt.Errorf("coinbase: %w", err)
	}
	if !strings.EqualFold(siteCurrency, "USD") && strings.TrimSpace(cfg["exchange_rate"]) == "" {
		return errors.New("coinbase: exchange_rate is required when the site currency is not USD")
	}
	return nil
}

// ParseKey parses a SEC1 or PKCS#8 ECDSA private key in PEM (literal "\n" sequences accepted).
func ParseKey(raw string) (*ecdsa.PrivateKey, error) {
	s := strings.TrimSpace(strings.ReplaceAll(raw, `\n`, "\n"))
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("coinbase: api_key_secret must be a PEM EC private key")
	}
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("coinbase: api_key_secret: %w", err)
	}
	ek, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("coinbase: api_key_secret is not an ECDSA key (Ed25519 keys are not supported)")
	}
	return ek, nil
}

func (g *Gateway) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

func (g *Gateway) base() string {
	if g.BaseURL != "" {
		return strings.TrimRight(g.BaseURL, "/")
	}
	return DefaultBaseURL
}

// BearerJWT builds the per-request CDP JWT for method + host/path.
func BearerJWT(keyName string, key *ecdsa.PrivateKey, method, hostPath string, now time.Time) (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	claims := jwt.MapClaims{
		"iss": "cdp",
		"sub": keyName,
		"nbf": now.Unix(),
		"exp": now.Add(120 * time.Second).Unix(),
		"uri": method + " " + hostPath,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = keyName
	tok.Header["nonce"] = hex.EncodeToString(nonce[:])
	return tok.SignedString(key)
}

type checkoutRequest struct {
	Amount             string            `json:"amount"`
	Currency           string            `json:"currency"`
	Description        string            `json:"description"`
	Metadata           map[string]string `json:"metadata"`
	SuccessRedirectURL string            `json:"successRedirectUrl,omitempty"`
	FailRedirectURL    string            `json:"failRedirectUrl,omitempty"`
	ExpiresAt          string            `json:"expiresAt,omitempty"`
}

// Pay implements payment.Gateway: it creates a checkout and returns its hosted URL.
func (g *Gateway) Pay(ctx context.Context, cfg map[string]string, req payment.PayRequest) (*payment.PayResult, error) {
	if err := gwutil.Required(cfg, Name, "api_key_name", "api_key_secret", "webhook_secret"); err != nil {
		return nil, err
	}
	if err := gwutil.RequireAmount(Name, req.TotalAmount); err != nil {
		return nil, err
	}
	key, err := ParseKey(cfg["api_key_secret"])
	if err != nil {
		return nil, err
	}
	rate, err := gwutil.ParseRate(cfg["exchange_rate"])
	if err != nil {
		return nil, fmt.Errorf("coinbase: %w", err)
	}
	usdc := gwutil.ConvertCents(req.TotalAmount, rate)
	if usdc <= 0 {
		return nil, &gwutil.UserError{Msg: "amount is too small for Coinbase"}
	}
	payUUID := gwutil.PaymentUUID(req.NotifyURL)
	subject := req.Subject
	if subject == "" {
		subject = req.TradeNo
	}
	body := checkoutRequest{
		Amount:             payment.FormatAmount(usdc),
		Currency:           "USDC",
		Description:        subject,
		Metadata:           map[string]string{"trade_no": req.TradeNo, "payment_uuid": payUUID},
		SuccessRedirectURL: req.ReturnURL,
		FailRedirectURL:    req.ReturnURL,
	}
	if !req.ExpiresAt.IsZero() {
		body.ExpiresAt = req.ExpiresAt.UTC().Format(time.RFC3339)
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := g.base() + "/api/v1/checkouts"
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	tok, err := BearerJWT(strings.TrimSpace(cfg["api_key_name"]), key, http.MethodPost, u.Host+u.Path, g.now())
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Authorization", "Bearer "+tok)
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("X-Idempotency-Key", uuid.NewSHA1(idempotencyNS, []byte(strings.Join([]string{payUUID, req.TradeNo, body.Amount, body.ExpiresAt}, "|"))).String())
	status, raw, err := gwutil.Do(ctx, g.Client, hreq, Name)
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, fmt.Errorf("coinbase: create checkout failed (HTTP %d)", status)
	}
	var res struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || res.URL == "" {
		return nil, errors.New("coinbase: invalid checkout response")
	}
	return &payment.PayResult{Type: payment.PayTypeURL, Data: res.URL}, nil
}

// SignedString builds the HMAC input: t + "." + h + "." + header values + "." + body, or t + "." + body
// without h.
func SignedString(t, h string, header http.Header, body []byte) string {
	if h == "" {
		return t + "." + string(body)
	}
	names := strings.Fields(h)
	vals := make([]string, len(names))
	for i, n := range names {
		vals[i] = header.Get(n)
	}
	return t + "." + h + "." + strings.Join(vals, ".") + "." + string(body)
}

// Verify checks an X-Webhook-Signature header ("t=…,h=…,v1=…") against body.
func Verify(sigHeader, secret string, header http.Header, body []byte, now time.Time) error {
	parts := map[string]string{}
	for _, p := range strings.Split(sigHeader, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if ok {
			parts[k] = v
		}
	}
	t, v1 := parts["t"], parts["v1"]
	if t == "" || v1 == "" {
		return errors.New("coinbase: malformed signature header")
	}
	ts, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return errors.New("coinbase: invalid signature timestamp")
	}
	if d := now.Sub(time.Unix(ts, 0)); d > SignatureTolerance || d < -SignatureTolerance {
		return errors.New("coinbase: signature timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(SignedString(t, parts["h"], header, body)))
	if !gwutil.EqualHex(v1, hex.EncodeToString(mac.Sum(nil))) {
		return errors.New("coinbase: signature mismatch")
	}
	return nil
}

type event struct {
	ID              string            `json:"id"`
	EventType       string            `json:"eventType"`
	Status          string            `json:"status"`
	Amount          string            `json:"amount"`
	Currency        string            `json:"currency"`
	Metadata        map[string]string `json:"metadata"`
	TransactionHash string            `json:"transactionHash"`
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
	sig := r.Header.Get("X-Webhook-Signature")
	if sig == "" {
		sig = r.Header.Get("X-Hook0-Signature")
	}
	if sig == "" {
		return nil, nil, errors.New("coinbase: missing signature header")
	}
	if err := Verify(sig, strings.TrimSpace(cfg["webhook_secret"]), r.Header, raw, g.now()); err != nil {
		return nil, nil, err
	}
	var ev event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil, nil, fmt.Errorf("coinbase: decode event: %w", err)
	}
	ack := gwutil.TextResponse(http.StatusOK, "ok")
	if u := gwutil.PaymentUUID(r.URL.Path); u == "" || ev.Metadata["payment_uuid"] != u {
		return nil, ack, nil
	}
	if strings.HasPrefix(ev.EventType, "checkout.refund.") {
		slog.Warn("coinbase refund event", "event", ev.EventType, "checkout_id", ev.ID, "trade_no", ev.Metadata["trade_no"], "status", ev.Status)
		return nil, ack, nil
	}
	if ev.EventType != "checkout.payment.success" || ev.Status != "COMPLETED" {
		return nil, ack, nil
	}
	tradeNo := ev.Metadata["trade_no"]
	if tradeNo == "" || ev.ID == "" {
		return nil, nil, errors.New("coinbase: event without trade_no or id")
	}
	slog.Info("coinbase payment confirmed", "trade_no", tradeNo, "checkout_id", ev.ID, "transaction_hash", ev.TransactionHash)
	res := &payment.NotifyResult{TradeNo: tradeNo, CallbackNo: ev.ID}
	if rate, err := gwutil.ParseRate(cfg["exchange_rate"]); err == nil && rate.Equal(decimal.NewFromInt(1)) {
		if c, err := gwutil.ParseCents(ev.Amount); err == nil {
			res.Amount = c
		}
	}
	return res, ack, nil
}
