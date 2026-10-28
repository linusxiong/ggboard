package epay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/payment"
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
