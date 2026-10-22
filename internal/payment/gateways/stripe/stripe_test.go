package stripe

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/webhook"

	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
)

const (
	tradeNo   = "202610211234567a8b9c0d1e2f3a4b5c"
	uuid      = "0123456789abcdef0123456789abcdef"
	notifyURL = "https://panel.example.com/api/v1/guest/payment/notify/stripe/" + uuid
	returnURL = "https://panel.example.com/api/v1/guest/payment/return/" + tradeNo
	secret    = "whsec_test_secret"
)

func TestWebhookVector(t *testing.T) {
	payload := `{"id":"evt_test_1","object":"event","type":"checkout.session.completed"}`
	sig := sdk.ComputeSignature(time.Unix(1792584000, 0), []byte(payload), secret)
	if got := hex.EncodeToString(sig); got != "ff6756256066e28f04e7e24189356b8c4c5d7068404ca18564d9bbb9ab59bc1d" {
		t.Fatalf("signature = %s", got)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/guest/payment/notify/stripe/"+uuid, strings.NewReader(payload))
	r.Header.Set("Stripe-Signature", "t=1792584000,v1=ff6756256066e28f04e7e24189356b8c4c5d7068404ca18564d9bbb9ab59bc1d")
	res, resp, err := (&Gateway{Tolerance: -1}).Notify(context.Background(), map[string]string{"webhook_secret": secret}, r)
	if err != nil || res != nil || resp == nil || resp.Status != 200 || resp.Body != `{"received":true}` {
		t.Fatalf("vector = %+v %+v %v", res, resp, err)
	}
	// The same fixed timestamp is stale under the default tolerance.
	r = httptest.NewRequest(http.MethodPost, "/x/"+uuid, strings.NewReader(payload))
	r.Header.Set("Stripe-Signature", "t=1792584000,v1=ff6756256066e28f04e7e24189356b8c4c5d7068404ca18564d9bbb9ab59bc1d")
	if _, _, err := (&Gateway{}).Notify(context.Background(), map[string]string{"webhook_secret": secret}, r); err == nil {
		t.Fatal("stale timestamp must fail")
	}
}

func signed(t *testing.T, payload string) *http.Request {
	t.Helper()
	sp := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: []byte(payload), Secret: secret, Timestamp: time.Now()})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/guest/payment/notify/stripe/"+uuid, strings.NewReader(payload))
	r.Header.Set("Stripe-Signature", sp.Header)
	return r
}

func event(typ, status, metaUUID, currency, pi string) string {
	return `{"id":"evt_1","object":"event","api_version":"2020-08-27","type":"` + typ + `","data":{"object":{"id":"cs_test_1","object":"checkout.session","client_reference_id":"` + tradeNo +
		`","payment_status":"` + status + `","amount_total":1234,"currency":"` + currency + `","metadata":{"trade_no":"` + tradeNo + `","payment_uuid":"` + metaUUID + `"},"payment_intent":` + pi + `}}}`
}

func TestNotify(t *testing.T) {
	gw := &Gateway{}
	cfg := map[string]string{"secret_key": "sk_test_x", "webhook_secret": secret}
	res, resp, err := gw.Notify(context.Background(), cfg, signed(t, event("checkout.session.completed", "paid", uuid, "cny", `"pi_123"`)))
	if err != nil || res == nil || resp.Status != 200 {
		t.Fatalf("paid = %+v %+v %v", res, resp, err)
	}
	if res.TradeNo != tradeNo || res.CallbackNo != "pi_123" || res.Amount != 1234 {
		t.Fatalf("res = %+v", res)
	}
	// Expanded payment intent, async success.
	res, _, err = gw.Notify(context.Background(), cfg, signed(t, event("checkout.session.async_payment_succeeded", "paid", uuid, "cny", `{"id":"pi_456"}`)))
	if err != nil || res == nil || res.CallbackNo != "pi_456" {
		t.Fatalf("async = %+v %v", res, err)
	}
	// No payment intent: session id fallback; FX rate → amount not reported.
	fx := map[string]string{"webhook_secret": secret, "currency": "usd", "exchange_rate": "0.14"}
	res, _, err = gw.Notify(context.Background(), fx, signed(t, event("checkout.session.completed", "paid", uuid, "usd", `null`)))
	if err != nil || res == nil || res.CallbackNo != "cs_test_1" || res.Amount != 0 {
		t.Fatalf("fx = %+v %v", res, err)
	}
	// Acks without a result.
	for name, payload := range map[string]string{
		"unpaid":       event("checkout.session.completed", "unpaid", uuid, "cny", `null`),
		"expired":      event("checkout.session.expired", "unpaid", uuid, "cny", `null`),
		"failed":       event("checkout.session.async_payment_failed", "unpaid", uuid, "cny", `null`),
		"foreign uuid": event("checkout.session.completed", "paid", "ffffffffffffffffffffffffffffffff", "cny", `"pi_1"`),
		"other event":  `{"id":"evt_2","object":"event","type":"customer.created","data":{"object":{"id":"cus_1"}}}`,
	} {
		res, resp, err := gw.Notify(context.Background(), cfg, signed(t, payload))
		if err != nil || res != nil || resp == nil || resp.Status != 200 {
			t.Fatalf("%s = %+v %+v %v", name, res, resp, err)
		}
	}
	// Bad signature / wrong secret / missing header.
	r := signed(t, event("checkout.session.completed", "paid", uuid, "cny", `"pi_1"`))
	if _, _, err := gw.Notify(context.Background(), map[string]string{"webhook_secret": "whsec_other"}, r); err == nil {
		t.Fatal("wrong secret must fail")
	}
	r = httptest.NewRequest(http.MethodPost, "/x/"+uuid, strings.NewReader(`{}`))
	if _, _, err := gw.Notify(context.Background(), cfg, r); err == nil {
		t.Fatal("missing signature must fail")
	}
}

type fakeStripe struct {
	mu    sync.Mutex
	forms []url.Values
	mode  string
}

func (f *fakeStripe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/checkout/sessions" {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()
	f.mu.Lock()
	f.forms = append(f.forms, r.PostForm)
	n := len(f.forms)
	mode := f.mode
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case mode == "small":
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"amount_too_small","message":"Amount must be at least $0.50 usd","type":"invalid_request_error"}}`))
		return
	case mode == "wechat" && n == 1:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"The payment method type wechat_pay is not activated","param":"payment_method_options[wechat_pay]","type":"invalid_request_error"}}`))
		return
	}
	_, _ = w.Write([]byte(`{"id":"cs_test_1","object":"checkout.session","url":"https://checkout.stripe.com/c/pay/cs_test_1"}`))
}

func TestPay(t *testing.T) {
	fs := &fakeStripe{}
	srv := httptest.NewTLSServer(fs)
	defer srv.Close()
	gw := &Gateway{Client: srv.Client(), BackendURL: srv.URL}
	cfg := map[string]string{"secret_key": "sk_test_x", "webhook_secret": secret, "payment_methods": "card"}
	req := payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1234, Currency: "CNY", Subject: "GG Bond - Pro", NotifyURL: notifyURL, ReturnURL: returnURL, ExpiresAt: time.Now().Add(time.Hour)}
	res, err := gw.Pay(context.Background(), cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != payment.PayTypeURL || res.Data != "https://checkout.stripe.com/c/pay/cs_test_1" {
		t.Fatalf("res = %+v", res)
	}
	f := fs.forms[0]
	for k, want := range map[string]string{
		"mode": "payment", "client_reference_id": tradeNo, "success_url": returnURL, "metadata[payment_uuid]": uuid, "metadata[trade_no]": tradeNo,
		"line_items[0][price_data][unit_amount]": "1234", "line_items[0][price_data][currency]": "cny", "line_items[0][quantity]": "1",
		"line_items[0][price_data][product_data][name]": "GG Bond - Pro", "allowed_payment_method_types[0]": "card", "payment_intent_data[metadata][trade_no]": tradeNo,
	} {
		if f.Get(k) != want {
			t.Fatalf("%s = %q, want %q (form %v)", k, f.Get(k), want, f)
		}
	}
	if f.Get("payment_method_options[wechat_pay][client]") != "" || f.Get("payment_method_types[0]") != "" {
		t.Fatalf("unexpected params: %v", f)
	}
	// FX + zero-decimal currency.
	fs.forms = nil
	jpy := map[string]string{"secret_key": "sk_test_x", "webhook_secret": secret, "currency": "JPY", "exchange_rate": "20.5", "payment_methods": "wechat_pay"}
	if _, err := gw.Pay(context.Background(), jpy, req); err != nil {
		t.Fatal(err)
	}
	if got := fs.forms[0].Get("line_items[0][price_data][unit_amount]"); got != "253" { // 12.34 CNY × 20.5 = 252.97 JPY
		t.Fatalf("jpy unit_amount = %s", got)
	}
	if fs.forms[0].Get("payment_method_options[wechat_pay][client]") != "web" || fs.forms[0].Get("allowed_payment_method_types[0]") != "wechat_pay" {
		t.Fatalf("wechat form = %v", fs.forms[0])
	}
	// auto: retry without the WeChat option when Stripe rejects it.
	fs.forms, fs.mode = nil, "wechat"
	if _, err := gw.Pay(context.Background(), map[string]string{"secret_key": "sk_test_x", "webhook_secret": secret}, req); err != nil {
		t.Fatal(err)
	}
	if len(fs.forms) != 2 || fs.forms[0].Get("payment_method_options[wechat_pay][client]") != "web" || fs.forms[1].Get("payment_method_options[wechat_pay][client]") != "" {
		t.Fatalf("retry forms = %v", fs.forms)
	}
	// amount_too_small → user-facing error.
	fs.forms, fs.mode = nil, "small"
	_, err = gw.Pay(context.Background(), cfg, req)
	var ue *gwutil.UserError
	if !errors.As(err, &ue) {
		t.Fatalf("expected UserError, got %v", err)
	}
	// Config errors.
	if _, err := gw.Pay(context.Background(), map[string]string{"secret_key": "x", "webhook_secret": "y", "currency": "usd"}, req); err == nil {
		t.Fatal("missing exchange rate must fail")
	}
	if _, err := gw.Pay(context.Background(), map[string]string{"secret_key": "x", "webhook_secret": "y", "currency": "isk", "exchange_rate": "17.33"}, req); err == nil {
		t.Fatal("isk fractional amount must fail")
	}
}

func TestValidateConfig(t *testing.T) {
	gw := &Gateway{}
	cases := []struct {
		cfg  map[string]string
		site string
		ok   bool
	}{
		{map[string]string{}, "CNY", true},
		{map[string]string{"currency": "usd", "exchange_rate": "0.14"}, "CNY", true},
		{map[string]string{"currency": "usd"}, "CNY", false},
		{map[string]string{"currency": "kwd", "exchange_rate": "1"}, "CNY", false},
		{map[string]string{"currency": "usd", "exchange_rate": "-1"}, "CNY", false},
		{map[string]string{"payment_methods": "paypal"}, "CNY", false},
		{map[string]string{"currency": "USD"}, "usd", true},
	}
	for i, c := range cases {
		if err := gw.ValidateConfig(c.cfg, c.site); (err == nil) != c.ok {
			t.Fatalf("case %d: %v", i, err)
		}
	}
	if gw.Name() != Name || gw.Label() == "" || len(gw.Fields()) != 5 {
		t.Fatal("metadata")
	}
}
