package btcpay

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/payment"
)

const (
	tradeNo   = "202610211234567a8b9c0d1e2f3a4b5c"
	payUUID   = "0123456789abcdef0123456789abcdef"
	notifyURL = "https://panel.example.com/api/v1/guest/payment/notify/btcpay/" + payUUID
	secret    = "btcpay_test_secret"
	vector    = `{"deliveryId":"Dlv1","webhookId":"Wh1","originalDeliveryId":"Dlv1","isRedelivery":false,"type":"InvoiceSettled","timestamp":1792584000,"storeId":"Store1","invoiceId":"Inv1","metadata":{"orderId":"202610211234567a8b9c0d1e2f3a4b5c"},"manuallyMarked":false,"overPaid":false}`
)

func TestSignatureVector(t *testing.T) {
	if got := Signature(secret, []byte(vector)); got != "sha256=f1a2d817b438106a73e851ffee748de61af0666fb1038809ca63cbdc4d7fea17" {
		t.Fatalf("sig = %s", got)
	}
}

// fakeServer is a BTCPay Greenfield store "Store1": POST creates invoice Inv1 from the request, GET returns the
// stored invoice (tests rewrite it to play invoices created elsewhere on the store).
type fakeServer struct {
	mu      sync.Mutex
	posted  map[string]any
	auth    string
	invoice map[string]any // nil = 404
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = r.Header.Get("Authorization")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/stores/Store1/invoices":
		b, _ := io.ReadAll(r.Body)
		f.posted = nil
		_ = json.Unmarshal(b, &f.posted)
		f.invoice = map[string]any{"id": "Inv1", "storeId": "Store1", "amount": f.posted["amount"], "currency": f.posted["currency"],
			"status": "New", "additionalStatus": "None", "checkoutLink": "https://btcpay.example.com/i/Inv1", "metadata": f.posted["metadata"]}
		_ = json.NewEncoder(w).Encode(f.invoice)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/stores/Denied/invoices":
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"missing-permission","message":"Insufficient API Permissions"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stores/Store1/invoices/Inv1" && f.invoice != nil:
		_ = json.NewEncoder(w).Encode(f.invoice)
	default:
		http.NotFound(w, r)
	}
}

// edit changes the stored invoice under the lock.
func (f *fakeServer) edit(fn func(inv map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.invoice)
}

func (f *fakeServer) request() (posted map[string]any, auth string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posted, f.auth
}

func setup(t *testing.T) (*fakeServer, *Gateway, map[string]string) {
	t.Helper()
	fs := &fakeServer{}
	srv := httptest.NewTLSServer(fs)
	t.Cleanup(srv.Close)
	cfg := map[string]string{"server_url": srv.URL + "/", "store_id": "Store1", "api_key": "btcpay_api_key", "webhook_secret": secret}
	return fs, &Gateway{Client: srv.Client()}, cfg
}

func payRequest() payment.PayRequest {
	return payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1234, Currency: "cny", Subject: "GG Bond - Pro",
		NotifyURL: notifyURL, ReturnURL: "https://panel.example.com/r"}
}

// settledInvoice creates the order's invoice through Pay and marks it Settled, as BTCPay does once it is paid.
func settledInvoice(t *testing.T, fs *fakeServer, gw *Gateway, cfg map[string]string) {
	t.Helper()
	if _, err := gw.Pay(context.Background(), cfg, payRequest()); err != nil {
		t.Fatal(err)
	}
	fs.edit(func(inv map[string]any) { inv["status"] = "Settled" })
}

func TestPay(t *testing.T) {
	fs, gw, cfg := setup(t)
	now := time.Now()
	gw.Now = func() time.Time { return now }
	req := payRequest()
	req.ExpiresAt = now.Add(30 * time.Minute)
	res, err := gw.Pay(context.Background(), cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != payment.PayTypeURL || res.Data != "https://btcpay.example.com/i/Inv1" {
		t.Fatalf("res = %+v", res)
	}
	posted, auth := fs.request()
	meta, _ := posted["metadata"].(map[string]any)
	co, _ := posted["checkout"].(map[string]any)
	if posted["amount"] != "12.34" || posted["currency"] != "CNY" || meta["orderId"] != tradeNo || meta["itemDesc"] != "GG Bond - Pro" ||
		co["expirationMinutes"] != float64(30) || co["redirectAutomatically"] != true || auth != "token btcpay_api_key" {
		t.Fatalf("posted = %v auth=%s", posted, auth)
	}
	// The invoice is bound to this payment method, order, amount and currency.
	if meta[metaPayment] != payUUID || meta[metaBinding] != binding(secret, payUUID, tradeNo, 1234, "CNY") {
		t.Fatalf("binding metadata = %v", meta)
	}

	for name, mutate := range map[string]func(map[string]string, *payment.PayRequest){
		"403":            func(c map[string]string, _ *payment.PayRequest) { c["store_id"] = "Denied" },
		"plain http":     func(c map[string]string, _ *payment.PayRequest) { c["server_url"] = "http://btcpay.example.com" },
		"no secret":      func(c map[string]string, _ *payment.PayRequest) { c["webhook_secret"] = " " },
		"no currency":    func(_ map[string]string, r *payment.PayRequest) { r.Currency = "" },
		"no notify uuid": func(_ map[string]string, r *payment.PayRequest) { r.NotifyURL = "" },
		"zero amount":    func(_ map[string]string, r *payment.PayRequest) { r.TotalAmount = 0 },
	} {
		c, r := maps.Clone(cfg), payRequest()
		mutate(c, &r)
		if _, err := gw.Pay(context.Background(), c, r); err == nil {
			t.Fatalf("%s: expected failure", name)
		}
	}
}

func postTo(t *testing.T, gw *Gateway, cfg map[string]string, uuid, body, sig string) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/guest/payment/notify/btcpay/"+uuid, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if sig != "" {
		r.Header.Set("Btcpay-Sig", sig)
	}
	return gw.Notify(context.Background(), cfg, r)
}

func post(t *testing.T, gw *Gateway, cfg map[string]string, body, sig string) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	t.Helper()
	return postTo(t, gw, cfg, payUUID, body, sig)
}

func TestNotify(t *testing.T) {
	fs, gw, cfg := setup(t)
	settledInvoice(t, fs, gw, cfg)
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
	fs.edit(func(inv map[string]any) { inv["status"] = "Processing" })
	partial := strings.Replace(vector, "InvoiceSettled", "InvoicePaymentSettled", 1)
	if res, resp, err := post(t, gw, cfg, partial, Signature(secret, []byte(partial))); err != nil || res != nil || resp == nil {
		t.Fatalf("partial = %+v %+v %v", res, resp, err)
	}
	// ...but the same event with a settled invoice pays.
	fs.edit(func(inv map[string]any) { inv["status"] = "Settled" })
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
	// The fetched invoice must be the webhook's invoice.
	fs.edit(func(inv map[string]any) { inv["id"] = "Inv2" })
	if res, _, err := post(t, gw, cfg, vector, Signature(secret, []byte(vector))); err == nil || res != nil {
		t.Fatalf("mismatched invoice id = %+v %v", res, err)
	}
	if gw.Name() != Name || gw.Label() == "" || len(gw.Fields()) != 4 || gw.ValidateConfig(cfg, "CNY") != nil {
		t.Fatal("metadata")
	}
}

// A store's public invoice surfaces (Point of Sale custom amounts, Pay Button, crowdfunds, top-ups) let anyone
// create and pay an invoice with a chosen orderId, amount and currency, and BTCPay signs its InvoiceSettled
// webhook like ours. Only the invoice Pay created for exactly this order, amount, currency and payment method pays.
func TestNotifyRequiresBoundInvoice(t *testing.T) {
	setMeta := func(k string, v any) func(map[string]any) {
		return func(inv map[string]any) {
			meta := maps.Clone(inv["metadata"].(map[string]any))
			meta[k] = v
			inv["metadata"] = meta
		}
	}
	cases := []struct {
		name   string
		edit   func(inv map[string]any)
		uuid   string
		paid   bool
		amount int64
	}{
		{name: "valid", paid: true, amount: 1234},
		{name: "manually marked settled", edit: func(inv map[string]any) { inv["additionalStatus"] = "Marked" }, paid: true, amount: 1234},
		{name: "amount mismatch", edit: func(inv map[string]any) { inv["amount"] = "0.01" }},
		{name: "zero amount top-up", edit: func(inv map[string]any) { inv["amount"] = "0" }},
		{name: "currency mismatch", edit: func(inv map[string]any) { inv["currency"] = "JPY" }},
		{name: "orderId mismatch", edit: setMeta("orderId", "20261021ffffffffffffffffffffffff")},
		{name: "not settled", edit: func(inv map[string]any) { inv["status"] = "Processing" }},
		{name: "expired paid late", edit: func(inv map[string]any) { inv["status"] = "Expired"; inv["additionalStatus"] = "PaidLate" }},
		{name: "unbound point of sale invoice", edit: func(inv map[string]any) { inv["metadata"] = map[string]any{"orderId": tradeNo} }},
		{name: "binding under another secret", edit: setMeta(metaBinding, binding("other", payUUID, tradeNo, 1234, "CNY"))},
		{name: "another payment method on the store", uuid: "fedcba9876543210fedcba9876543210"},
		{name: "orderId only in the webhook body", edit: func(inv map[string]any) { inv["metadata"] = map[string]any{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs, gw, cfg := setup(t)
			settledInvoice(t, fs, gw, cfg)
			if tc.edit != nil {
				fs.edit(tc.edit)
			}
			uuid := tc.uuid
			if uuid == "" {
				uuid = payUUID
			}
			res, resp, err := postTo(t, gw, cfg, uuid, vector, Signature(secret, []byte(vector)))
			if err != nil || resp == nil || resp.Status != http.StatusOK || resp.Body != "ok" {
				t.Fatalf("verified webhooks are acknowledged: %+v %v", resp, err)
			}
			if !tc.paid {
				if res != nil {
					t.Fatalf("must not be a payment: %+v", res)
				}
				return
			}
			if res == nil || res.TradeNo != tradeNo || res.CallbackNo != "Inv1" || res.Amount != tc.amount {
				t.Fatalf("res = %+v", res)
			}
		})
	}
}
