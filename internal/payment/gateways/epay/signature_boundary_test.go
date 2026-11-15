package epay

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil/gwutiltest"
)

func TestPayRejectsCallbackHostInjection(t *testing.T) {
	for _, host := range []string{
		"panel.example.com&trade_no=forged&trade_status=TRADE_SUCCESS&type=",
		"buyer@panel.example.com",
		"panel.example.com%26trade_no=forged",
		"panel.example.com/path&trade_no=forged&trade_status=TRADE_SUCCESS&type=",
		"panel.example.com?trade_status=TRADE_SUCCESS",
		"panel.example.com#trade_status=TRADE_SUCCESS",
	} {
		for _, field := range []string{"notify", "return"} {
			t.Run(field+"/"+host, func(t *testing.T) {
				req := payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1234, NotifyURL: notifyURL, ReturnURL: returnURL}
				if field == "notify" {
					req.NotifyURL = "https://" + host + "/notify"
				} else {
					req.ReturnURL = "https://" + host + "/return"
				}
				if _, err := (&Gateway{}).Pay(context.Background(), cfg, req); err == nil {
					t.Fatal("signed a callback URL with an unsafe authority")
				}
			})
		}
	}
	for _, host := range []string{"panel.example.com", "panel.example.com:8443", "127.0.0.1:8080", "[2001:db8::1]:8080"} {
		if _, err := (&Gateway{}).Pay(context.Background(), cfg, payment.PayRequest{
			TradeNo: tradeNo, TotalAmount: 1234, NotifyURL: "https://" + host + "/notify", ReturnURL: "https://" + host + "/return",
		}); err != nil {
			t.Fatalf("valid host %s: %v", host, err)
		}
	}
}

func TestNotifyRejectsCheckoutSignature(t *testing.T) {
	result, err := (&Gateway{}).Pay(t.Context(), cfg, payment.PayRequest{
		TradeNo: tradeNo, TotalAmount: 1234, NotifyURL: notifyURL, ReturnURL: returnURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			query := u.Query().Encode()
			r := httptest.NewRequest(method, "/notify?"+query, nil)
			if method == http.MethodPost {
				r = httptest.NewRequest(method, "/notify", strings.NewReader(query))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			if res, _, err := (&Gateway{}).Notify(t.Context(), cfg, r); err == nil || res != nil {
				t.Fatal("checkout signature accepted as a callback")
			}
		})
	}
}

func TestNotifyRejectsMerchantRequestFields(t *testing.T) {
	for _, field := range []string{"notify_url", "return_url", "return_url=x&trade_no", ""} {
		t.Run(field, func(t *testing.T) {
			params := map[string]string{
				"pid": "1001", "out_trade_no": tradeNo, "trade_no": "fixture-transaction",
				"money": "12.34", "trade_status": "TRADE_SUCCESS", field: "fixture",
			}
			q := url.Values{}
			for k, v := range params {
				q.Set(k, v)
			}
			q.Set("sign", Sign(params, key))
			r := httptest.NewRequest(http.MethodGet, "/notify?"+q.Encode(), nil)
			if res, _, err := (&Gateway{}).Notify(context.Background(), cfg, r); err == nil || res != nil {
				t.Fatal("accepted merchant-only field or ambiguous parameter name")
			}
		})
	}
}

// hostileName carries a whole successful callback after a raw '&', as an administrator-controlled app_name or
// plan name could (MISTAKES #302).
const hostileName = "GG Bond - Pro&out_trade_no=" + tradeNo + "&pid=1001&trade_no=forged&trade_status=TRADE_SUCCESS&type=x"

// requireNoReparseVerifies sends every parse of a merchant-request sign string to Notify with its signature and
// returns how many of them looked like a successful payment.
func requireNoReparseVerifies(t *testing.T, signed, sign string) (payments int) {
	t.Helper()
	parses := gwutiltest.Reparses(signed, "sign", "sign_type")
	if len(parses) == 0 {
		t.Fatalf("no parse of %s", signed)
	}
	for _, p := range parses {
		if p["trade_status"] == "TRADE_SUCCESS" && p["pid"] == "1001" && p["trade_no"] != "" && p["out_trade_no"] == tradeNo {
			payments++
		}
		q := url.Values{}
		for k, v := range p {
			q.Set(k, v)
		}
		q.Set("sign", sign)
		q.Set("sign_type", "MD5")
		r := httptest.NewRequest(http.MethodGet, "/notify?"+q.Encode(), nil)
		if res, resp, err := (&Gateway{}).Notify(t.Context(), cfg, r); err == nil {
			t.Fatalf("merchant-request signature verified as a callback: %v -> %+v %+v", p, res, resp)
		}
	}
	return payments
}

// Notify alone must refuse every merchant-request signature, also one over a value with a raw '&' (a request
// signed by an older version, or a field that Pay forgot to clean). Checking only the parameter names misses a
// field hidden inside another value: type=x&notify_url=… still reproduces the signed string.
func TestNotifyRejectsEveryReparseOfMerchantSignature(t *testing.T) {
	for _, name := range []string{tradeNo, hostileName} {
		p := map[string]string{
			"pid": "1001", "type": "alipay", "out_trade_no": tradeNo, "notify_url": notifyURL, "return_url": returnURL,
			"name": name, "money": "12.34",
		}
		payments := requireNoReparseVerifies(t, gwutil.SignString(p, "sign", "sign_type"), Sign(p, key))
		if name == hostileName && payments == 0 {
			t.Fatal("the hostile fixture has no parse that looks like a payment, so it proves nothing")
		}
	}
}

// Pay must never sign a value with the raw '&' separator, so the fields it signs are exactly the fields of the
// string, and every other parse of it still carries notify_url and return_url.
func TestPayNeverSignsFieldSeparator(t *testing.T) {
	res, err := (&Gateway{}).Pay(t.Context(), cfg, payment.PayRequest{
		TradeNo: tradeNo, TotalAmount: 1234, Subject: hostileName, NotifyURL: notifyURL, ReturnURL: returnURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(res.Data)
	if err != nil {
		t.Fatal(err)
	}
	params := map[string]string{}
	for k := range u.Query() {
		params[k] = u.Query().Get(k)
	}
	if want := strings.ReplaceAll(hostileName, "&", "＆"); params["name"] != want {
		t.Fatalf("name = %q, want %q", params["name"], want)
	}
	for k, v := range params {
		if strings.Contains(v, "&") {
			t.Fatalf("signed %s = %q", k, v)
		}
	}
	requireNoReparseVerifies(t, gwutil.SignString(params, "sign", "sign_type"), params["sign"])

	// Configuration is not rewritten: Pay refuses to sign a separator in it.
	for _, k := range []string{"pid", "type"} {
		bad := maps.Clone(cfg)
		bad[k] += "&trade_status=TRADE_SUCCESS"
		if _, err := (&Gateway{}).Pay(t.Context(), bad, payment.PayRequest{
			TradeNo: tradeNo, TotalAmount: 1234, NotifyURL: notifyURL, ReturnURL: returnURL,
		}); err == nil {
			t.Fatalf("signed a %s with a raw '&'", k)
		}
	}
}

// Real callbacks keep working: providers echo name as sent (an order signed before the upgrade can still carry
// '&'), and some clones add fields of their own, which is why Notify has no allowlist of field names.
func TestNotifyAcceptsGenuineCallbackShapes(t *testing.T) {
	p := map[string]string{
		"pid": "1001", "trade_no": "2026102112000012345", "out_trade_no": tradeNo, "type": "alipay",
		"name": "GG Bond - Pro & Family", "money": "12.34", "trade_status": "TRADE_SUCCESS", "param": "fixture",
		"buyer": "fixture-buyer",
	}
	q := url.Values{}
	for k, v := range p {
		q.Set(k, v)
	}
	q.Set("sign", Sign(p, key))
	q.Set("sign_type", "MD5")
	r := httptest.NewRequest(http.MethodGet, "/notify?"+q.Encode(), nil)
	res, _, err := (&Gateway{}).Notify(t.Context(), cfg, r)
	if err != nil || res == nil || res.TradeNo != tradeNo || res.CallbackNo != p["trade_no"] || res.Amount != 1234 {
		t.Fatalf("genuine callback: %+v %v", res, err)
	}
}
