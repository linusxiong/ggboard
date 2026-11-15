package epusdt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil/gwutiltest"
)

// Without app_url the callback URLs come from the request Host, which Go accepts with '&' and '='. Pay must refuse
// them before anything is signed or sent.
func TestPayRejectsCallbackHostInjection(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"status_code":200,"message":"success","data":{"trade_id":1,"payment_url":"https://usdt.example.com/pay/x"}}`))
	}))
	defer srv.Close()
	gw := &Gateway{Client: srv.Client()}
	cfg := map[string]string{"api_url": srv.URL, "api_token": token}
	for _, host := range []string{
		"panel.example.com&status=2&su=",
		"buyer@panel.example.com",
		"panel.example.com?status=2",
		"panel.example.com#status=2",
	} {
		for _, field := range []string{"notify", "return"} {
			req := payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1234, NotifyURL: notifyURL, ReturnURL: returnURL}
			if field == "notify" {
				req.NotifyURL = "https://" + host + "/notify"
			} else {
				req.ReturnURL = "https://" + host + "/return"
			}
			if _, err := gw.Pay(t.Context(), cfg, req); err == nil {
				t.Fatalf("%s URL with host %q accepted", field, host)
			}
		}
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("sent %d create requests with unsafe callback URLs", n)
	}
}

// The create-transaction signature only travels to the Epusdt server, yet it must still never verify as a
// callback, whatever its URLs contain: before Pay checked them, a request Host with '&status=2&su=' produced a
// signature that a re-split JSON body passed off as a paid callback.
func TestNotifyRejectsEveryReparseOfMerchantSignature(t *testing.T) {
	const hostile = "https://panel.example.com&status=2&su="
	for _, base := range []string{"https://panel.example.com", hostile} {
		p := map[string]string{
			"amount": "12.34", "order_id": tradeNo, "trade_type": DefaultTradeType,
			"notify_url":   base + "/api/v1/guest/payment/notify/epusdt/0123456789abcdef0123456789abcdef",
			"redirect_url": base + "/orders/" + tradeNo,
		}
		signed := gwutil.SignString(p, "signature")
		parses := gwutiltest.Reparses(signed, "signature")
		payments := 0
		for _, rp := range parses {
			if rp["status"] == "2" && rp["order_id"] == tradeNo {
				payments++
			}
			body := map[string]any{"signature": Sign(p, token)}
			for k, v := range rp {
				body[k] = v
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			if res, resp, err := notify(t, string(raw)); err == nil {
				t.Fatalf("merchant-request signature verified as a callback: %s -> %+v %+v", raw, res, resp)
			}
		}
		if len(parses) == 0 || (base == hostile && payments == 0) {
			t.Fatalf("%s: the fixture proves nothing (%d parses, %d payments)", base, len(parses), payments)
		}
	}
}

// Pay must never sign a value with the raw '&' separator, and a replay of the request must fail.
func TestPayNeverSignsFieldSeparator(t *testing.T) {
	var body []byte
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = gwutil.ReadBody(r.Body)
		_, _ = w.Write([]byte(`{"status_code":200,"message":"success","data":{"trade_id":1,"payment_url":"https://usdt.example.com/pay/x"}}`))
	}))
	defer srv.Close()
	gw := &Gateway{Client: srv.Client()}
	cfg := map[string]string{"api_url": srv.URL, "api_token": token}
	if _, err := gw.Pay(t.Context(), cfg, payment.PayRequest{
		TradeNo: tradeNo, TotalAmount: 1234, Currency: "USD", NotifyURL: notifyURL, ReturnURL: returnURL,
	}); err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	if err := dec.Decode(&sent); err != nil {
		t.Fatal(err)
	}
	params := map[string]string{}
	for k, v := range sent {
		if s, ok, _ := canonical(v, false); ok && k != "signature" {
			params[k] = s
		}
	}
	for k, v := range params {
		if strings.Contains(v, "&") {
			t.Fatalf("signed %s = %q", k, v)
		}
	}
	if res, resp, err := notify(t, string(body)); err == nil {
		t.Fatalf("replayed create request verified as a callback: %+v %+v", res, resp)
	}

	// Configuration is not rewritten: Pay refuses to sign a separator in it.
	cfg["currency_type"] = "usdt.trc20&status=2"
	if _, err := gw.Pay(t.Context(), cfg, payment.PayRequest{
		TradeNo: tradeNo, TotalAmount: 1234, NotifyURL: notifyURL, ReturnURL: returnURL,
	}); err == nil {
		t.Fatal("signed a trade_type with a raw '&'")
	}
}
