package epay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
)

const (
	tradeNo   = "202610211234567a8b9c0d1e2f3a4b5c"
	notifyURL = "https://panel.example.com/api/v1/guest/payment/notify/epay/0123456789abcdef0123456789abcdef"
	returnURL = "https://panel.example.com/orders/202610211234567a8b9c0d1e2f3a4b5c"
	key       = "epay_test_key_123"
)

var cfg = map[string]string{"url": "https://pay.example.com/", "pid": "1001", "key": key, "type": "alipay"}

func TestPayVector(t *testing.T) {
	res, err := (&Gateway{}).Pay(context.Background(), cfg, payment.PayRequest{
		TradeNo: tradeNo, TotalAmount: 1234, Subject: tradeNo, NotifyURL: notifyURL, ReturnURL: returnURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != payment.PayTypeURL || !strings.HasPrefix(res.Data, "https://pay.example.com/submit.php?") {
		t.Fatalf("result = %+v", res)
	}
	u, _ := url.Parse(res.Data)
	q := u.Query()
	if q.Get("sign") != "6964057e967af1328f0ec76ebbcb50e5" || q.Get("sign_type") != "MD5" || q.Get("money") != "12.34" {
		t.Fatalf("query = %v", q)
	}
	want := "money=12.34&name=" + tradeNo + "&notify_url=" + notifyURL + "&out_trade_no=" + tradeNo + "&pid=1001&return_url=" + returnURL + "&type=alipay"
	params := map[string]string{}
	for k := range q {
		params[k] = q.Get(k)
	}
	if got := gwutil.SignString(params, "sign", "sign_type"); got != want {
		t.Fatalf("sign string\n got %s\nwant %s", got, want)
	}
	if _, err := (&Gateway{}).Pay(context.Background(), cfg, payment.PayRequest{TradeNo: tradeNo}); err == nil {
		t.Fatal("zero amount must fail")
	}
	if _, err := (&Gateway{}).Pay(context.Background(), map[string]string{"url": "ftp://x", "pid": "1", "key": "k"}, payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1}); err == nil {
		t.Fatal("bad url must fail")
	}
}

func TestUpstreamVector(t *testing.T) {
	p := map[string]string{"money": "100", "name": "VIP", "notify_url": "https://merchant.example/notify", "out_trade_no": "ORD202605230001", "pid": "1000", "return_url": "https://merchant.example/return", "type": "alipay"}
	if got := Sign(p, "epusdt_secret_key"); got != "b865b0acbb2b01554c35a1bd33351452" {
		t.Fatalf("sign = %s", got)
	}
}

func notifyParams() url.Values {
	v := url.Values{}
	for k, val := range map[string]string{"pid": "1001", "trade_no": "2026102112000012345", "out_trade_no": tradeNo, "type": "alipay", "name": tradeNo, "money": "12.34", "trade_status": "TRADE_SUCCESS", "sign_type": "MD5"} {
		v.Set(k, val)
	}
	v.Set("sign", "33c9590808c14643afc49d9b1d12ed8a")
	return v
}

func TestNotify(t *testing.T) {
	gw := &Gateway{}
	// GET with the vector.
	r := httptest.NewRequest(http.MethodGet, "/notify?"+notifyParams().Encode(), nil)
	res, resp, err := gw.Notify(context.Background(), cfg, r)
	if err != nil || res == nil || resp.Body != "success" || resp.Status != 200 {
		t.Fatalf("notify = %+v %+v %v", res, resp, err)
	}
	if res.TradeNo != tradeNo || res.CallbackNo != "2026102112000012345" || res.Amount != 1234 {
		t.Fatalf("result = %+v", res)
	}
	// POST form, uppercase hex accepted.
	v := notifyParams()
	v.Set("sign", strings.ToUpper(v.Get("sign")))
	r = httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(v.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if res, _, err := gw.Notify(context.Background(), cfg, r); err != nil || res == nil {
		t.Fatalf("post notify: %v", err)
	}
	// Flipped byte, wrong key, missing sign, foreign pid.
	bad := notifyParams()
	bad.Set("money", "12.35")
	for name, tc := range map[string]struct {
		v   url.Values
		cfg map[string]string
	}{
		"tampered":  {bad, cfg},
		"wrong key": {notifyParams(), map[string]string{"pid": "1001", "key": "other"}},
		"no sign":   {func() url.Values { x := notifyParams(); x.Del("sign"); return x }(), cfg},
	} {
		r := httptest.NewRequest(http.MethodGet, "/notify?"+tc.v.Encode(), nil)
		if res, resp, err := gw.Notify(context.Background(), tc.cfg, r); err == nil || res != nil || resp != nil {
			t.Fatalf("%s: expected verification failure", name)
		}
	}
	foreign := map[string]string{"pid": "2002", "key": key}
	r = httptest.NewRequest(http.MethodGet, "/notify?"+notifyParams().Encode(), nil)
	if _, _, err := gw.Notify(context.Background(), foreign, r); err == nil {
		t.Fatal("pid mismatch must fail")
	}
	// Non-success status: signed ack without a result.
	p := map[string]string{"pid": "1001", "out_trade_no": tradeNo, "money": "10", "trade_status": "WAIT_BUYER_PAY"}
	q := url.Values{}
	for k, val := range p {
		q.Set(k, val)
	}
	q.Set("sign", Sign(p, key))
	r = httptest.NewRequest(http.MethodGet, "/notify?"+q.Encode(), nil)
	res, resp, err = gw.Notify(context.Background(), cfg, r)
	if err != nil || res != nil || resp == nil || resp.Body != "success" {
		t.Fatalf("pending = %+v %+v %v", res, resp, err)
	}
	// GM Pay style amount with 4 decimals.
	p = map[string]string{"pid": "1001", "out_trade_no": tradeNo, "trade_no": "x", "money": "100.0000", "trade_status": "TRADE_SUCCESS"}
	q = url.Values{}
	for k, val := range p {
		q.Set(k, val)
	}
	q.Set("sign", Sign(p, key))
	r = httptest.NewRequest(http.MethodGet, "/notify?"+q.Encode(), nil)
	if res, _, err := gw.Notify(context.Background(), cfg, r); err != nil || res.Amount != 10000 {
		t.Fatalf("gm pay amount = %+v %v", res, err)
	}
}

func TestMetadata(t *testing.T) {
	gw := &Gateway{}
	if gw.Name() != "epay" || gw.Label() == "" || len(gw.Fields()) != 4 {
		t.Fatal("metadata")
	}
	if err := gw.ValidateConfig(map[string]string{"url": "http://pay.example.com"}, "CNY"); err != nil {
		t.Fatalf("http allowed with warn: %v", err)
	}
	if err := gw.ValidateConfig(map[string]string{"url": "pay"}, "CNY"); err == nil {
		t.Fatal("invalid url")
	}
}
