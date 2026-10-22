// Package alipayf2f implements Alipay Face-to-Face (支付宝当面付) in public-key mode
// (docs/PAYMENTS_GATEWAYS.md §4): RSA2-signed alipay.trade.precreate returning a QR code, and RSA2-verified
// form notifications. CNY only.
package alipayf2f

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
)

// Name is the registry id.
const Name = "alipay_f2f"

// Gateway endpoints.
const (
	ProductionEndpoint = "https://openapi.alipay.com/gateway.do"
	SandboxEndpoint    = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
)

// MinKeyBits is the smallest accepted RSA key size.
const MinKeyBits = 2048

var cst = time.FixedZone("CST", 8*3600)

func init() { payment.Register(&Gateway{}) }

// Gateway is the Alipay F2F plugin. Client and Endpoint override the HTTP client and gateway URL (tests);
// Now overrides the clock.
type Gateway struct {
	Client   *http.Client
	Endpoint string
	Now      func() time.Time
}

// Name implements payment.Gateway.
func (*Gateway) Name() string { return Name }

// Label implements payment.Gateway.
func (*Gateway) Label() string { return "支付宝当面付 (Alipay F2F)" }

// Fields implements payment.Gateway.
func (*Gateway) Fields() []payment.Field {
	return []payment.Field{
		{Key: "app_id", Label: "APPID", Type: payment.FieldText, Required: true, Description: "Open Platform app id with 当面付 signed"},
		{Key: "private_key", Label: "应用私钥", Type: payment.FieldTextarea, Required: true,
			Description: "RSA >= 2048 app private key (PKCS#1 or PKCS#8, PEM or bare base64). Public-key mode only; certificate mode is not supported."},
		{Key: "alipay_public_key", Label: "支付宝公钥", Type: payment.FieldTextarea, Required: true,
			Description: "The Alipay public key (not the app public key), PEM or bare base64"},
		{Key: "sandbox", Label: "沙箱", Type: payment.FieldSelect, Options: []string{"0", "1"}, Default: "0", Description: "1 = sandbox gateway"},
	}
}

// ValidateConfig parses both keys and requires a CNY site.
func (*Gateway) ValidateConfig(cfg map[string]string, siteCurrency string) error {
	if !strings.EqualFold(strings.TrimSpace(siteCurrency), "CNY") {
		return errors.New("alipay_f2f: Alipay Face-to-Face only supports CNY (site currency is " + siteCurrency + ")")
	}
	if _, err := ParsePrivateKey(cfg["private_key"]); err != nil {
		return err
	}
	if _, err := ParsePublicKey(cfg["alipay_public_key"]); err != nil {
		return err
	}
	return nil
}

func keyDER(raw string) (block *pem.Block, der []byte, err error) {
	s := strings.TrimSpace(strings.ReplaceAll(raw, "\r", ""))
	if s == "" {
		return nil, nil, errors.New("empty key")
	}
	if strings.Contains(s, "-----BEGIN") {
		b, _ := pem.Decode([]byte(s))
		if b == nil {
			return nil, nil, errors.New("invalid PEM")
		}
		return b, b.Bytes, nil
	}
	der, err = base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
	if err != nil {
		return nil, nil, errors.New("invalid base64")
	}
	return nil, der, nil
}

// ParsePrivateKey accepts PKCS#1 or PKCS#8 RSA private keys as PEM or bare base64.
func ParsePrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, der, err := keyDER(raw)
	if err != nil {
		return nil, fmt.Errorf("alipay_f2f: private key: %w", err)
	}
	var key *rsa.PrivateKey
	switch {
	case block != nil && block.Type == "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(der)
	case block != nil && block.Type == "PRIVATE KEY":
		key, err = pkcs8RSA(der)
	case block != nil:
		err = fmt.Errorf("unsupported PEM type %q", block.Type)
	default:
		if key, err = pkcs8RSA(der); err != nil {
			key, err = x509.ParsePKCS1PrivateKey(der)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("alipay_f2f: private key: %w", err)
	}
	if key.N.BitLen() < MinKeyBits {
		return nil, fmt.Errorf("alipay_f2f: private key must be at least %d bits", MinKeyBits)
	}
	return key, nil
}

func pkcs8RSA(der []byte) (*rsa.PrivateKey, error) {
	k, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an RSA key")
	}
	return rk, nil
}

// ParsePublicKey accepts X.509 SPKI or PKCS#1 RSA public keys as PEM or bare base64.
func ParsePublicKey(raw string) (*rsa.PublicKey, error) {
	block, der, err := keyDER(raw)
	if err != nil {
		return nil, fmt.Errorf("alipay_f2f: public key: %w", err)
	}
	var key *rsa.PublicKey
	switch {
	case block != nil && block.Type == "RSA PUBLIC KEY":
		key, err = x509.ParsePKCS1PublicKey(der)
	case block != nil && block.Type == "PUBLIC KEY":
		key, err = pkixRSA(der)
	case block != nil:
		err = fmt.Errorf("unsupported PEM type %q", block.Type)
	default:
		if key, err = pkixRSA(der); err != nil {
			key, err = x509.ParsePKCS1PublicKey(der)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("alipay_f2f: public key: %w", err)
	}
	if key.N.BitLen() < MinKeyBits {
		return nil, fmt.Errorf("alipay_f2f: public key must be at least %d bits", MinKeyBits)
	}
	return key, nil
}

func pkixRSA(der []byte) (*rsa.PublicKey, error) {
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	rk, ok := k.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not an RSA key")
	}
	return rk, nil
}

// SignContent is the request sign string: every param except sign (sign_type included), empty values
// dropped, sorted, raw values.
func SignContent(params map[string]string) string { return gwutil.SignString(params, "sign") }

// VerifyContent is the notify sign string (rsaCheckV1): sign and sign_type removed.
func VerifyContent(params map[string]string) string {
	return gwutil.SignString(params, "sign", "sign_type")
}

// SignRSA2 signs content with SHA256withRSA and returns standard base64.
func SignRSA2(key *rsa.PrivateKey, content string) (string, error) {
	sum := sha256.Sum256([]byte(content))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyRSA2 verifies a base64 SHA256withRSA signature.
func VerifyRSA2(pub *rsa.PublicKey, content, sign string) error {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sign))
	if err != nil {
		return errors.New("alipay_f2f: invalid signature encoding")
	}
	sum := sha256.Sum256([]byte(content))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return errors.New("alipay_f2f: signature mismatch")
	}
	return nil
}

type bizContent struct {
	OutTradeNo     string `json:"out_trade_no"`
	TotalAmount    string `json:"total_amount"`
	Subject        string `json:"subject"`
	TimeoutExpress string `json:"timeout_express,omitempty"`
}

func marshalNoEscape(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func (g *Gateway) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

func (g *Gateway) endpoint(cfg map[string]string) string {
	if g.Endpoint != "" {
		return g.Endpoint
	}
	if strings.TrimSpace(cfg["sandbox"]) == "1" {
		return SandboxEndpoint
	}
	return ProductionEndpoint
}

// timeoutExpress maps an absolute expiry to Alipay's relative "Nm" (1m..15d).
func timeoutExpress(expires, now time.Time) string {
	if expires.IsZero() {
		return ""
	}
	minutes := int(expires.Sub(now).Minutes())
	minutes = max(1, min(minutes, 15*24*60))
	return fmt.Sprintf("%dm", minutes)
}

// Pay implements payment.Gateway (alipay.trade.precreate → QR code).
func (g *Gateway) Pay(ctx context.Context, cfg map[string]string, req payment.PayRequest) (*payment.PayResult, error) {
	if err := gwutil.Required(cfg, Name, "app_id", "private_key", "alipay_public_key"); err != nil {
		return nil, err
	}
	if err := gwutil.RequireAmount(Name, req.TotalAmount); err != nil {
		return nil, err
	}
	if !strings.EqualFold(req.Currency, "CNY") {
		return nil, errors.New("alipay_f2f: only CNY is supported")
	}
	priv, err := ParsePrivateKey(cfg["private_key"])
	if err != nil {
		return nil, err
	}
	pub, err := ParsePublicKey(cfg["alipay_public_key"])
	if err != nil {
		return nil, err
	}
	now := g.now()
	subject := req.Subject
	if subject == "" {
		subject = req.TradeNo
	}
	biz, err := marshalNoEscape(bizContent{
		OutTradeNo:     req.TradeNo,
		TotalAmount:    payment.FormatAmount(req.TotalAmount),
		Subject:        subject,
		TimeoutExpress: timeoutExpress(req.ExpiresAt, now),
	})
	if err != nil {
		return nil, err
	}
	params := map[string]string{
		"app_id":      strings.TrimSpace(cfg["app_id"]),
		"method":      "alipay.trade.precreate",
		"format":      "JSON",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"version":     "1.0",
		"timestamp":   now.In(cst).Format(time.DateTime),
		"notify_url":  req.NotifyURL,
		"biz_content": biz,
	}
	sign, err := SignRSA2(priv, SignContent(params))
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	form.Set("sign", sign)
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint(cfg), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	status, raw, err := gwutil.Do(ctx, g.Client, hreq, Name)
	if err != nil {
		return nil, err
	}
	var env struct {
		R    json.RawMessage `json:"alipay_trade_precreate_response"`
		Sign string          `json:"sign"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || len(env.R) == 0 {
		return nil, fmt.Errorf("alipay_f2f: invalid response (HTTP %d)", status)
	}
	var res struct {
		Code    string `json:"code"`
		Msg     string `json:"msg"`
		SubCode string `json:"sub_code"`
		SubMsg  string `json:"sub_msg"`
		QRCode  string `json:"qr_code"`
	}
	if err := json.Unmarshal(env.R, &res); err != nil {
		return nil, fmt.Errorf("alipay_f2f: invalid response body: %w", err)
	}
	if res.Code != "10000" {
		return nil, fmt.Errorf("alipay_f2f: precreate failed: %s %s %s", res.Code, res.SubCode, gwutil.Truncate(res.SubMsg, 200))
	}
	if env.Sign != "" {
		if err := VerifyRSA2(pub, string(env.R), env.Sign); err != nil {
			return nil, fmt.Errorf("alipay_f2f: response %w", err)
		}
	}
	if res.QRCode == "" {
		return nil, errors.New("alipay_f2f: empty qr_code")
	}
	return &payment.PayResult{Type: payment.PayTypeQRCode, Data: res.QRCode}, nil
}

// Notify implements payment.Gateway.
func (*Gateway) Notify(_ context.Context, cfg map[string]string, r *http.Request) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	if err := gwutil.Required(cfg, Name, "app_id", "alipay_public_key"); err != nil {
		return nil, nil, err
	}
	pub, err := ParsePublicKey(cfg["alipay_public_key"])
	if err != nil {
		return nil, nil, err
	}
	params, err := gwutil.Values(r)
	if err != nil {
		return nil, nil, fmt.Errorf("alipay_f2f: parse callback: %w", err)
	}
	if params["sign"] == "" {
		return nil, nil, errors.New("alipay_f2f: missing sign")
	}
	if err := VerifyRSA2(pub, VerifyContent(params), params["sign"]); err != nil {
		return nil, nil, err
	}
	if params["app_id"] != strings.TrimSpace(cfg["app_id"]) {
		return nil, nil, errors.New("alipay_f2f: app_id mismatch")
	}
	ok := gwutil.TextResponse(http.StatusOK, "success")
	switch params["trade_status"] {
	case "TRADE_SUCCESS", "TRADE_FINISHED":
	default:
		return nil, ok, nil
	}
	if params["out_trade_no"] == "" {
		return nil, nil, errors.New("alipay_f2f: missing out_trade_no")
	}
	amount, err := gwutil.ParsePaidCents(params["total_amount"])
	if err != nil {
		return nil, nil, err
	}
	return &payment.NotifyResult{TradeNo: params["out_trade_no"], CallbackNo: params["trade_no"], Amount: amount}, ok, nil
}
