package epusdt

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
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
)

const (
	tradeNo   = "202610211234567a8b9c0d1e2f3a4b5c"
	notifyURL = "https://panel.example.com/api/v1/guest/payment/notify/epusdt/0123456789abcdef0123456789abcdef"
	returnURL = "https://panel.example.com/orders/202610211234567a8b9c0d1e2f3a4b5c"
	token     = "epusdt_test_token"
)

func TestSignVectors(t *testing.T) {
	p := map[string]string{"amount": "12.34", "notify_url": notifyURL, "order_id": tradeNo, "redirect_url": returnURL, "trade_type": "usdt.trc20"}
	want := "amount=12.34&notify_url=" + notifyURL + "&order_id=" + tradeNo + "&redirect_url=" + returnURL + "&trade_type=usdt.trc20"
	if got := gwutil.SignString(p, "signature"); got != want {
		t.Fatalf("sign string = %s", got)
	}
	if got := Sign(p, token); got != "965b836ba79aeace6e2863b8447de372" {
		t.Fatalf("sign = %s", got)
	}
	up := map[string]string{"amount": "42", "notify_url": "http://example.com/notify", "order_id": "20220201030210321", "redirect_url": "http://example.com/redirect"}
	if got := Sign(up, "epusdt_password_xasddawqe"); got != "1cd4b52df5587cfb1968b0c0c6e156cd" {
		t.Fatalf("upstream sign = %s", got)
	}
}

func TestPay(t *testing.T) {
	var got map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/order/create-transaction" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.UseNumber()
		_ = dec.Decode(&got)
		if !strings.Contains(string(b), `"amount":12.3,`) {
			t.Errorf("amount must be a trimmed JSON number: %s", b)
		}
		_, _ = w.Write([]byte(`{"status_code":200,"message":"success","data":{"trade_id":12345,"order_id":"x","amount":"12.30","payment_url":"https://usdt.example.com/pay/checkout-counter/abc"},"request_id":""}`))
	}))
	defer srv.Close()
	gw := &Gateway{Client: srv.Client()}
	cfg := map[string]string{"api_url": srv.URL + "/", "api_token": token}
	res, err := gw.Pay(context.Background(), cfg, payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1230, Currency: "USD", NotifyURL: notifyURL, ReturnURL: returnURL})
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != payment.PayTypeURL || res.Data != "https://usdt.example.com/pay/checkout-counter/abc" {
		t.Fatalf("res = %+v", res)
	}
	params := map[string]string{}
	for k, v := range got {
		if s, ok, _ := canonical(v, false); ok && k != "signature" {
			params[k] = s
		}
	}
	if params["fiat"] != "USD" || params["trade_type"] != DefaultTradeType || got["signature"] != Sign(params, token) {
		t.Fatalf("request = %v", got)
	}
	// Provider error with HTTP 200.
	errSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status_code":400,"message":"签名认证错误","data":null}`))
	}))
	defer errSrv.Close()
	if _, err := (&Gateway{Client: errSrv.Client()}).Pay(context.Background(), map[string]string{"api_url": errSrv.URL, "api_token": token},
		payment.PayRequest{TradeNo: tradeNo, TotalAmount: 100}); err == nil || !strings.Contains(err.Error(), "签名认证错误") {
		t.Fatalf("expected provider error, got %v", err)
	}
	// Context timeout.
	slow := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(300 * time.Millisecond):
		}
	}))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := (&Gateway{Client: slow.Client()}).Pay(ctx, map[string]string{"api_url": slow.URL, "api_token": token}, payment.PayRequest{TradeNo: tradeNo, TotalAmount: 100}); err == nil {
		t.Fatal("expected timeout")
	}
	if _, err := gw.Pay(context.Background(), cfg, payment.PayRequest{TradeNo: tradeNo, TotalAmount: 100_000_000}); err == nil {
		t.Fatal("expected amount limit")
	}
}

const notifyBody = `{"trade_id":"202610211200001234","order_id":"202610211234567a8b9c0d1e2f3a4b5c","amount":12.34,"actual_amount":"1.7","token":"TTestTronAddress0000000000000000001","block_transaction_id":"9f2c4e1a","signature":"b1b1ae9d8ce1af4ad5a94812d665299f","status":2}`

func notify(t *testing.T, body string) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return (&Gateway{}).Notify(context.Background(), map[string]string{"api_url": "https://usdt.example.com", "api_token": token}, r)
}

func TestNotify(t *testing.T) {
	res, resp, err := notify(t, notifyBody)
	if err != nil || res == nil || resp.Body != "ok" {
		t.Fatalf("notify = %+v %+v %v", res, resp, err)
	}
	if res.TradeNo != tradeNo || res.CallbackNo != "202610211200001234" || res.Amount != 1234 {
		t.Fatalf("res = %+v", res)
	}
	// Uppercase hex accepted.
	if res, _, err := notify(t, strings.Replace(notifyBody, "b1b1ae9d8ce1af4ad5a94812d665299f", "B1B1AE9D8CE1AF4AD5A94812D665299F", 1)); err != nil || res == nil {
		t.Fatalf("uppercase: %v", err)
	}
	for name, body := range map[string]string{
		"tampered": strings.Replace(notifyBody, `"amount":12.34`, `"amount":12.35`, 1),
		"no sig":   strings.Replace(notifyBody, `"signature":"b1b1ae9d8ce1af4ad5a94812d665299f",`, "", 1),
		"nested":   strings.Replace(notifyBody, `"status":2`, `"status":2,"x":{"a":1}`, 1),
		"garbage":  `not json`,
	} {
		if res, resp, err := notify(t, body); err == nil || res != nil || resp != nil {
			t.Fatalf("%s: expected failure", name)
		}
	}
	// Waiting/expired events are signed acks without a result.
	p := map[string]string{"trade_id": "1", "order_id": tradeNo, "amount": "12.34", "status": "1"}
	body := `{"trade_id":"1","order_id":"` + tradeNo + `","amount":12.34,"status":1,"signature":"` + Sign(p, token) + `"}`
	if res, resp, err := notify(t, body); err != nil || res != nil || resp.Body != "ok" {
		t.Fatalf("waiting = %+v %+v %v", res, resp, err)
	}
	// BEpusdt %v float formatting fallback: 1e+06 style numbers.
	p = map[string]string{"trade_id": "2", "order_id": tradeNo, "amount": "1e+06", "status": "2"}
	body = `{"trade_id":"2","order_id":"` + tradeNo + `","amount":1000000,"status":2,"signature":"` + Sign(p, token) + `"}`
	if res, _, err := notify(t, body); err != nil || res == nil || res.Amount != 100_000_000 {
		t.Fatalf("fallback = %+v %v", res, err)
	}
}

func TestMetadata(t *testing.T) {
	gw := &Gateway{}
	if gw.Name() != Name || gw.Label() == "" || len(gw.Fields()) != 3 {
		t.Fatal("metadata")
	}
	if gw.ValidateConfig(map[string]string{"api_url": "https://usdt.example.com"}, "CNY") != nil || gw.ValidateConfig(map[string]string{}, "CNY") == nil {
		t.Fatal("validate")
	}
}

func TestNotifyRequiresPositiveAmount(t *testing.T) {
	for _, amount := range []string{"", "0", "-1"} {
		p := map[string]string{"trade_id": "fixture-transaction", "order_id": tradeNo, "status": "2"}
		if amount != "" {
			p["amount"] = amount
		}
		p["signature"] = Sign(p, token)
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if res, _, err := notify(t, string(raw)); err == nil || res != nil {
			t.Fatalf("paid amount %q accepted", amount)
		}
	}
}
