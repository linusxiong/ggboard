package btcpay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/payment"
)

const (
	tradeNo = "202610211234567a8b9c0d1e2f3a4b5c"
	secret  = "btcpay_test_secret"
	vector  = `{"deliveryId":"Dlv1","webhookId":"Wh1","originalDeliveryId":"Dlv1","isRedelivery":false,"type":"InvoiceSettled","timestamp":1792584000,"storeId":"Store1","invoiceId":"Inv1","metadata":{"orderId":"202610211234567a8b9c0d1e2f3a4b5c"},"manuallyMarked":false,"overPaid":false}`
)

func TestSignatureVector(t *testing.T) {
	if got := Signature(secret, []byte(vector)); got != "sha256=f1a2d817b438106a73e851ffee748de61af0666fb1038809ca63cbdc4d7fea17" {
		t.Fatalf("sig = %s", got)
	}
}

type fakeServer struct {
	status string
	amount string
	posted map[string]any
	auth   string
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.auth = r.Header.Get("Authorization")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/stores/Store1/invoices":
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &f.posted)
		_, _ = w.Write([]byte(`{"id":"Inv1","storeId":"Store1","amount":"12.34","currency":"CNY","status":"New","checkoutLink":"https://btcpay.example.com/i/Inv1"}`))
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/stores/Denied/invoices":
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"missing-permission","message":"Insufficient API Permissions"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stores/Store1/invoices/Inv1":
		_, _ = w.Write([]byte(`{"id":"Inv1","storeId":"Store1","amount":"` + f.amount + `","currency":"CNY","status":"` + f.status + `","additionalStatus":"None","metadata":{"orderId":"` + tradeNo + `"}}`))
	default:
		http.NotFound(w, r)
	}
}

func setup(t *testing.T) (*fakeServer, *Gateway, map[string]string) {
	t.Helper()
	fs := &fakeServer{status: "Settled", amount: "12.34"}
	srv := httptest.NewTLSServer(fs)
	t.Cleanup(srv.Close)
	cfg := map[string]string{"server_url": srv.URL + "/", "store_id": "Store1", "api_key": "btcpay_api_key", "webhook_secret": secret}
	return fs, &Gateway{Client: srv.Client()}, cfg
}

func TestPay(t *testing.T) {
	fs, gw, cfg := setup(t)
	now := time.Now()
	gw.Now = func() time.Time { return now }
	res, err := gw.Pay(context.Background(), cfg, payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1234, Currency: "cny", Subject: "GG Bond - Pro",
		ReturnURL: "https://panel.example.com/r", ExpiresAt: now.Add(30 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != payment.PayTypeURL || res.Data != "https://btcpay.example.com/i/Inv1" {
		t.Fatalf("res = %+v", res)
	}
	meta, _ := fs.posted["metadata"].(map[string]any)
	co, _ := fs.posted["checkout"].(map[string]any)
	if fs.posted["amount"] != "12.34" || fs.posted["currency"] != "CNY" || meta["orderId"] != tradeNo || meta["itemDesc"] != "GG Bond - Pro" ||
		co["expirationMinutes"] != float64(30) || co["redirectAutomatically"] != true || fs.auth != "token btcpay_api_key" {
		t.Fatalf("posted = %v auth=%s", fs.posted, fs.auth)
	}
	cfg["store_id"] = "Denied"
	if _, err := gw.Pay(context.Background(), cfg, payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1}); err == nil {
		t.Fatal("403 must fail")
	}
	cfg["server_url"] = "http://btcpay.example.com"
	if _, err := gw.Pay(context.Background(), cfg, payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1}); err == nil {
		t.Fatal("plain http must be rejected")
	}
}

func post(t *testing.T, gw *Gateway, cfg map[string]string, body, sig string) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/guest/payment/notify/btcpay/0123456789abcdef0123456789abcdef", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if sig != "" {
		r.Header.Set("Btcpay-Sig", sig)
	}
	return gw.Notify(context.Background(), cfg, r)
}

func TestNotify(t *testing.T) {
	fs, gw, cfg := setup(t)
	res, resp, err := post(t, gw, cfg, vector, Signature(secret, []byte(vector)))
	if err != nil || res == nil || resp.Body != "ok" {
		t.Fatalf("settled = %+v %+v %v", res, resp, err)
	}
	if res.TradeNo != tradeNo || res.CallbackNo != "Inv1" || res.Amount != 1234 {
		t.Fatalf("res = %+v", res)
	}
	// Uppercase hex accepted.
	if res, _, err := post(t, gw, cfg, vector, "sha256="+strings.ToUpper(strings.TrimPrefix(Signature(secret, []byte(vector)), "sha256="))); err != nil || res == nil {
		t.Fatalf("uppercase: %v", err)
	}
	// Partial payment event while the invoice is not settled → ack only.
	fs.status = "Processing"
	partial := strings.Replace(vector, "InvoiceSettled", "InvoicePaymentSettled", 1)
	if res, resp, err := post(t, gw, cfg, partial, Signature(secret, []byte(partial))); err != nil || res != nil || resp == nil {
		t.Fatalf("partial = %+v %+v %v", res, resp, err)
	}
	// ...but the same event with a settled invoice pays.
	fs.status = "Settled"
	if res, _, err := post(t, gw, cfg, partial, Signature(secret, []byte(partial))); err != nil || res == nil {
		t.Fatalf("payment settled = %+v %v", res, err)
	}
	for name, body := range map[string]string{
		"created": strings.Replace(vector, "InvoiceSettled", "InvoiceCreated", 1),
		"expired": strings.Replace(strings.Replace(vector, "InvoiceSettled", "InvoiceExpired", 1), `"overPaid":false`, `"partiallyPaid":true`, 1),
		"store":   strings.Replace(vector, "Store1", "Store2", 1),
	} {
		res, resp, err := post(t, gw, cfg, body, Signature(secret, []byte(body)))
		if err != nil || res != nil || resp == nil || resp.Status != 200 {
			t.Fatalf("%s = %+v %+v %v", name, res, resp, err)
		}
	}
	for name, sig := range map[string]string{
		"missing": "",
		"flipped": strings.Replace(Signature(secret, []byte(vector)), "f1a2", "f1a3", 1),
		"key":     Signature("other", []byte(vector)),
		"scheme":  strings.Replace(Signature(secret, []byte(vector)), "sha256=", "sha1=", 1),
	} {
		if _, _, err := post(t, gw, cfg, vector, sig); err == nil {
			t.Fatalf("%s: expected failure", name)
		}
	}
	// Untrimmed body: the signature covers the raw bytes.
	if _, _, err := post(t, gw, cfg, vector+"\n", Signature(secret, []byte(vector))); err == nil {
		t.Fatal("signature must cover raw bytes")
	}
	if gw.Name() != Name || gw.Label() == "" || len(gw.Fields()) != 4 || gw.ValidateConfig(cfg, "CNY") != nil {
		t.Fatal("metadata")
	}
}
