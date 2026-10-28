package coinbase

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/linusxiong/ggboard/internal/payment"
)

const (
	tradeNo   = "202610211234567a8b9c0d1e2f3a4b5c"
	payUUID   = "0123456789abcdef0123456789abcdef"
	notifyURL = "https://panel.example.com/api/v1/guest/payment/notify/coinbase/" + payUUID
	secret    = "cb_test_secret"
	keyName   = "organizations/org-test/apiKeys/key-test"
)

var vectorTime = time.Unix(1792584000, 0)

func TestSignatureVectors(t *testing.T) {
	body := []byte(`{"id":"6750a1b2c3d4e5f6a7b8c9d0","eventType":"checkout.payment.success","status":"COMPLETED","amount":"12.34","currency":"USDC","metadata":{"trade_no":"202610211234567a8b9c0d1e2f3a4b5c"}}`)
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	if got := SignedString("1792584000", "content-type", h, body); got != "1792584000.content-type.application/json."+string(body) {
		t.Fatalf("signed = %s", got)
	}
	withH := "t=1792584000,h=content-type,v1=e8f80395b5b260133c6ea4eacc96c8434ecf5127dd3dbbca33c24833b6c32512"
	if err := Verify(withH, secret, h, body, vectorTime); err != nil {
		t.Fatal(err)
	}
	if err := Verify("t=1792584000,v1=55a059c3184d57fdbdcf555e2d54966a679a3e49f5d1bac2aa6cdae74fdd438c", secret, h, body, vectorTime); err != nil {
		t.Fatal(err)
	}
	if err := Verify("t=1792584000,h=content-type,v1=E8F80395B5B260133C6EA4EACC96C8434ECF5127DD3DBBCA33C24833B6C32512", secret, h, body, vectorTime); err != nil {
		t.Fatalf("uppercase hex: %v", err)
	}
	for name, tc := range map[string]struct {
		sig, secret string
		now         time.Time
	}{
		"flipped": {strings.Replace(withH, "e8f8", "e8f9", 1), secret, vectorTime},
		"key":     {withH, "other", vectorTime},
		"stale":   {withH, secret, vectorTime.Add(301 * time.Second)},
		"future":  {withH, secret, vectorTime.Add(-301 * time.Second)},
		"missing": {"h=content-type", secret, vectorTime},
	} {
		if err := Verify(tc.sig, tc.secret, h, body, tc.now); err == nil {
			t.Fatalf("%s: expected failure", name)
		}
	}
}

func sign(t *testing.T, body string, ts time.Time) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(SignedString(itoa(ts), "content-type", http.Header{"Content-Type": {"application/json"}}, []byte(body))))
	return "t=" + itoa(ts) + ",h=content-type,v1=" + hex.EncodeToString(mac.Sum(nil))
}

func itoa(t time.Time) string { return strconv.FormatInt(t.Unix(), 10) }

func ev(eventType, status, uuid string) string {
	return `{"id":"6750a1b2c3d4e5f6a7b8c9d0","eventType":"` + eventType + `","status":"` + status + `","amount":"12.34","currency":"USDC","network":"base","metadata":{"trade_no":"` + tradeNo + `","payment_uuid":"` + uuid + `"},"transactionHash":"0xabc"}`
}

func notify(t *testing.T, cfg map[string]string, body, header string) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/guest/payment/notify/coinbase/"+payUUID, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if header != "" {
		r.Header.Set("X-Hook0-Signature", header)
	}
	return (&Gateway{Now: func() time.Time { return vectorTime }}).Notify(context.Background(), cfg, r)
}

func TestNotify(t *testing.T) {
	cfg := map[string]string{"webhook_secret": secret}
	body := ev("checkout.payment.success", "COMPLETED", payUUID)
	res, resp, err := notify(t, cfg, body, sign(t, body, vectorTime))
	if err != nil || res == nil || resp.Body != "ok" {
		t.Fatalf("paid = %+v %+v %v", res, resp, err)
	}
	if res.TradeNo != tradeNo || res.CallbackNo != "6750a1b2c3d4e5f6a7b8c9d0" || res.Amount != 1234 {
		t.Fatalf("res = %+v", res)
	}
	// A non-1 rate still reports the provider USDC amount.
	res, _, _ = notify(t, map[string]string{"webhook_secret": secret, "exchange_rate": "0.14"}, body, sign(t, body, vectorTime))
	if res == nil || res.Amount != 1234 || res.Currency != "USDC" {
		t.Fatalf("fx res = %+v", res)
	}
	for name, b := range map[string]string{
		"failed":  ev("checkout.payment.failed", "FAILED", payUUID),
		"expired": ev("checkout.payment.expired", "EXPIRED", payUUID),
		"refund":  ev("checkout.refund.success", "REFUNDED", payUUID),
		"foreign": ev("checkout.payment.success", "COMPLETED", "ffffffffffffffffffffffffffffffff"),
	} {
		res, resp, err := notify(t, cfg, b, sign(t, b, vectorTime))
		if err != nil || res != nil || resp == nil || resp.Status != 200 {
			t.Fatalf("%s = %+v %+v %v", name, res, resp, err)
		}
	}
	if _, _, err := notify(t, cfg, body, ""); err == nil {
		t.Fatal("missing header must fail")
	}
	if _, _, err := notify(t, cfg, body, sign(t, body, vectorTime.Add(-10*time.Minute))); err == nil {
		t.Fatal("stale must fail")
	}
}

func ecKeyPEM(t *testing.T) (*ecdsa.PrivateKey, string, string) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sec1, _ := x509.MarshalECPrivateKey(k)
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(k)
	return k, string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1})), string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
}

func TestPay(t *testing.T) {
	key, sec1PEM, pkcs8PEM := ecKeyPEM(t)
	var (
		gotBody map[string]any
		gotAuth string
		gotIdem string
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/checkouts" {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		gotAuth, gotIdem = r.Header.Get("Authorization"), r.Header.Get("X-Idempotency-Key")
		if gotBody["description"] == "fail" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errorType":"unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"6750a1b2c3d4e5f6a7b8c9d0","url":"https://payments.coinbase.com/checkout/abc","status":"ACTIVE"}`))
	}))
	defer srv.Close()
	now := time.Now()
	gw := &Gateway{Client: srv.Client(), BaseURL: srv.URL, Now: func() time.Time { return now }}
	// Literal "\n" sequences are accepted (keys pasted as one line).
	cfg := map[string]string{"api_key_name": keyName, "api_key_secret": strings.ReplaceAll(sec1PEM, "\n", `\n`), "webhook_secret": secret, "exchange_rate": "0.14"}
	req := payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1234, Currency: "CNY", Subject: "GG Bond - Pro", NotifyURL: notifyURL,
		ReturnURL: "https://panel.example.com/r", ExpiresAt: time.Date(2026, 10, 21, 12, 30, 0, 0, time.UTC)}
	res, err := gw.Pay(context.Background(), cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != payment.PayTypeURL || res.Data != "https://payments.coinbase.com/checkout/abc" {
		t.Fatalf("res = %+v", res)
	}
	if gotBody["amount"] != "1.73" || gotBody["currency"] != "USDC" || gotBody["expiresAt"] != "2026-10-21T12:30:00Z" {
		t.Fatalf("body = %v", gotBody)
	}
	meta, _ := gotBody["metadata"].(map[string]any)
	if meta["trade_no"] != tradeNo || meta["payment_uuid"] != payUUID {
		t.Fatalf("metadata = %v", meta)
	}
	tok, err := jwt.Parse(strings.TrimPrefix(gotAuth, "Bearer "), func(*jwt.Token) (any, error) { return &key.PublicKey, nil }, jwt.WithValidMethods([]string{"ES256"}))
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	claims := tok.Claims.(jwt.MapClaims)
	host := strings.TrimPrefix(srv.URL, "https://")
	if claims["iss"] != "cdp" || claims["sub"] != keyName || claims["uri"] != "POST "+host+"/api/v1/checkouts" || tok.Header["kid"] != keyName || len(tok.Header["nonce"].(string)) != 32 {
		t.Fatalf("jwt = %v %v", tok.Header, claims)
	}
	first := gotIdem
	if _, err := gw.Pay(context.Background(), cfg, req); err != nil || gotIdem != first {
		t.Fatalf("idempotency key must be deterministic: %s vs %s (%v)", first, gotIdem, err)
	}
	cfg["api_key_secret"] = pkcs8PEM
	req.Subject = "fail"
	if _, err := gw.Pay(context.Background(), cfg, req); err == nil {
		t.Fatal("401 must fail")
	}
}

func TestValidateConfig(t *testing.T) {
	_, sec1PEM, _ := ecKeyPEM(t)
	gw := &Gateway{}
	if err := gw.ValidateConfig(map[string]string{"api_key_secret": sec1PEM}, "USD"); err != nil {
		t.Fatal(err)
	}
	if err := gw.ValidateConfig(map[string]string{"api_key_secret": sec1PEM}, "CNY"); err == nil {
		t.Fatal("non-USD site needs a rate")
	}
	if err := gw.ValidateConfig(map[string]string{"api_key_secret": "nope"}, "USD"); err == nil {
		t.Fatal("bad key")
	}
	if gw.Name() != Name || gw.Label() == "" || len(gw.Fields()) != 4 {
		t.Fatal("metadata")
	}
}
