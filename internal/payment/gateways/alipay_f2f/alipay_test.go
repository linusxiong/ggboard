package alipayf2f

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/payment"
)

const (
	tradeNo   = "202610211234567a8b9c0d1e2f3a4b5c"
	notifyURL = "https://panel.example.com/api/v1/guest/payment/notify/alipay_f2f/0123456789abcdef0123456789abcdef"
	appID     = "2021000000000000"
)

var (
	keysOnce          sync.Once
	appKey, alipayKey *rsa.PrivateKey
)

func keys(t *testing.T) (*rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	keysOnce.Do(func() {
		var err error
		if appKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			t.Fatal(err)
		}
		if alipayKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			t.Fatal(err)
		}
	})
	return appKey, alipayKey
}

func pemOf(typ string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}))
}

func cfgFor(t *testing.T) map[string]string {
	app, ali := keys(t)
	pub, _ := x509.MarshalPKIXPublicKey(&ali.PublicKey)
	return map[string]string{
		"app_id":            appID,
		"private_key":       base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(app)),
		"alipay_public_key": base64.StdEncoding.EncodeToString(pub),
	}
}

func sha(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func TestRequestVector(t *testing.T) {
	params := map[string]string{
		"app_id": appID, "method": "alipay.trade.precreate", "format": "JSON", "charset": "utf-8", "sign_type": "RSA2",
		"version": "1.0", "timestamp": "2026-10-21 20:00:00", "notify_url": notifyURL,
		"biz_content": `{"out_trade_no":"202610211234567a8b9c0d1e2f3a4b5c","total_amount":"12.34","subject":"GG Bond 202610211234567a8b9c0d1e2f3a4b5c"}`,
	}
	content := SignContent(params)
	want := `app_id=2021000000000000&biz_content={"out_trade_no":"202610211234567a8b9c0d1e2f3a4b5c","total_amount":"12.34","subject":"GG Bond 202610211234567a8b9c0d1e2f3a4b5c"}&charset=utf-8&format=JSON&method=alipay.trade.precreate&notify_url=https://panel.example.com/api/v1/guest/payment/notify/alipay_f2f/0123456789abcdef0123456789abcdef&sign_type=RSA2&timestamp=2026-10-21 20:00:00&version=1.0`
	if content != want {
		t.Fatalf("content\n got %s\nwant %s", content, want)
	}
	if got := sha(content); got != "2ac5f7bd2bb0fc82971f6507637b67dc494441d3e15d74b50e767469342ac3ab" {
		t.Fatalf("sha256 = %s", got)
	}
	biz, _ := marshalNoEscape(bizContent{OutTradeNo: tradeNo, TotalAmount: "12.34", Subject: "GG Bond " + tradeNo})
	if biz != params["biz_content"] {
		t.Fatalf("biz = %s", biz)
	}
	app, _ := keys(t)
	sig, err := SignRSA2(app, content)
	if err != nil {
		t.Fatal(err)
	}
	sig2, _ := SignRSA2(app, content)
	if sig != sig2 {
		t.Fatal("PKCS#1 v1.5 must be deterministic")
	}
	if err := VerifyRSA2(&app.PublicKey, content, sig); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRSA2(&app.PublicKey, content+"x", sig); err == nil {
		t.Fatal("tampered content must fail")
	}
}

func notifyParams() map[string]string {
	return map[string]string{
		"app_id": appID, "auth_app_id": appID, "buyer_id": "2088000000000000", "buyer_pay_amount": "12.34", "charset": "utf-8",
		"fund_bill_list": `[{"amount":"12.34","fundChannel":"ALIPAYACCOUNT"}]`, "gmt_create": "2026-10-21 20:00:05", "gmt_payment": "2026-10-21 20:00:10",
		"invoice_amount": "12.34", "notify_id": "2026102100222200012345678901234567", "notify_time": "2026-10-21 20:00:11", "notify_type": "trade_status_sync",
		"out_trade_no": tradeNo, "point_amount": "0.00", "receipt_amount": "12.34", "seller_email": "", "seller_id": "2088000000000001",
		"subject": "GG Bond " + tradeNo, "total_amount": "12.34", "trade_no": "2026102122001400001234567890", "trade_status": "TRADE_SUCCESS", "version": "1.0",
		"sign_type": "RSA2",
	}
}

func postNotify(t *testing.T, params map[string]string, cfg map[string]string) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	t.Helper()
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	r := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	return (&Gateway{}).Notify(context.Background(), cfg, r)
}

func TestNotify(t *testing.T) {
	_, ali := keys(t)
	cfg := cfgFor(t)
	p := notifyParams()
	content := VerifyContent(p)
	want := `app_id=2021000000000000&auth_app_id=2021000000000000&buyer_id=2088000000000000&buyer_pay_amount=12.34&charset=utf-8&fund_bill_list=[{"amount":"12.34","fundChannel":"ALIPAYACCOUNT"}]&gmt_create=2026-10-21 20:00:05&gmt_payment=2026-10-21 20:00:10&invoice_amount=12.34&notify_id=2026102100222200012345678901234567&notify_time=2026-10-21 20:00:11&notify_type=trade_status_sync&out_trade_no=202610211234567a8b9c0d1e2f3a4b5c&point_amount=0.00&receipt_amount=12.34&seller_id=2088000000000001&subject=GG Bond 202610211234567a8b9c0d1e2f3a4b5c&total_amount=12.34&trade_no=2026102122001400001234567890&trade_status=TRADE_SUCCESS&version=1.0`
	if content != want {
		t.Fatalf("content\n got %s\nwant %s", content, want)
	}
	if got := sha(content); got != "c4f73092a9a24c5300711a639602e1b66c0b925e015ddf44d30399473baa78ef" {
		t.Fatalf("sha256 = %s", got)
	}
	p["sign"], _ = SignRSA2(ali, content)
	res, resp, err := postNotify(t, p, cfg)
	if err != nil || res == nil || resp.Body != "success" {
		t.Fatalf("notify = %+v %+v %v", res, resp, err)
	}
	if res.TradeNo != tradeNo || res.CallbackNo != "2026102122001400001234567890" || res.Amount != 1234 {
		t.Fatalf("res = %+v", res)
	}
	// Tampered amount, missing sign, foreign app_id.
	bad := notifyParams()
	bad["sign"] = p["sign"]
	bad["total_amount"] = "0.01"
	if _, _, err := postNotify(t, bad, cfg); err == nil {
		t.Fatal("tampered must fail")
	}
	nosign := notifyParams()
	if _, _, err := postNotify(t, nosign, cfg); err == nil {
		t.Fatal("missing sign must fail")
	}
	other := notifyParams()
	other["app_id"] = "2021999999999999"
	other["sign"], _ = SignRSA2(ali, VerifyContent(other))
	if _, _, err := postNotify(t, other, cfg); err == nil {
		t.Fatal("app_id mismatch must fail")
	}
	// WAIT_BUYER_PAY is a no-op ack.
	wait := notifyParams()
	wait["trade_status"] = "WAIT_BUYER_PAY"
	wait["sign"], _ = SignRSA2(ali, VerifyContent(wait))
	if res, resp, err := postNotify(t, wait, cfg); err != nil || res != nil || resp.Body != "success" {
		t.Fatalf("wait = %+v %+v %v", res, resp, err)
	}
}

func TestNotifyRequiresPositiveAmount(t *testing.T) {
	_, ali := keys(t)
	cfg := cfgFor(t)
	for _, amount := range []string{"", "0", "-1"} {
		p := notifyParams()
		if amount == "" {
			delete(p, "total_amount")
		} else {
			p["total_amount"] = amount
		}
		var err error
		p["sign"], err = SignRSA2(ali, VerifyContent(p))
		if err != nil {
			t.Fatal(err)
		}
		if res, _, err := postNotify(t, p, cfg); err == nil || res != nil {
			t.Fatalf("paid amount %q accepted", amount)
		}
	}
}

func TestKeyParsing(t *testing.T) {
	app, _ := keys(t)
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(app)
	pkcs1 := x509.MarshalPKCS1PrivateKey(app)
	for name, raw := range map[string]string{
		"pkcs1 pem":  pemOf("RSA PRIVATE KEY", pkcs1),
		"pkcs8 pem":  strings.ReplaceAll(pemOf("PRIVATE KEY", pkcs8), "\n", "\r\n"),
		"pkcs1 bare": base64.StdEncoding.EncodeToString(pkcs1),
		"pkcs8 bare": " " + base64.StdEncoding.EncodeToString(pkcs8)[:40] + "\n" + base64.StdEncoding.EncodeToString(pkcs8)[40:] + " ",
	} {
		k, err := ParsePrivateKey(raw)
		if err != nil || k.N.Cmp(app.N) != 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
	spki, _ := x509.MarshalPKIXPublicKey(&app.PublicKey)
	for name, raw := range map[string]string{
		"spki pem":  pemOf("PUBLIC KEY", spki),
		"pkcs1 pem": pemOf("RSA PUBLIC KEY", x509.MarshalPKCS1PublicKey(&app.PublicKey)),
		"spki bare": base64.StdEncoding.EncodeToString(spki),
	} {
		k, err := ParsePublicKey(raw)
		if err != nil || k.N.Cmp(app.N) != 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
	small, _ := rsa.GenerateKey(rand.Reader, 1024)
	if _, err := ParsePrivateKey(pemOf("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(small))); err == nil {
		t.Fatal("1024-bit key must be rejected")
	}
	for _, bad := range []string{"", "not base64!", pemOf("CERTIFICATE", []byte{1, 2})} {
		if _, err := ParsePrivateKey(bad); err == nil {
			t.Fatalf("%q must fail", bad)
		}
		if _, err := ParsePublicKey(bad); err == nil {
			t.Fatalf("%q must fail", bad)
		}
	}
	gw := &Gateway{}
	if err := gw.ValidateConfig(cfgFor(t), "CNY"); err != nil {
		t.Fatal(err)
	}
	if err := gw.ValidateConfig(cfgFor(t), "USD"); err == nil {
		t.Fatal("non-CNY site must be rejected")
	}
}

func TestPay(t *testing.T) {
	app, ali := keys(t)
	var form url.Values
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		params := map[string]string{}
		for k := range form {
			params[k] = form.Get(k)
		}
		if err := VerifyRSA2(&app.PublicKey, SignContent(params), params["sign"]); err != nil {
			t.Errorf("request signature: %v", err)
		}
		var biz map[string]string
		_ = json.Unmarshal([]byte(params["biz_content"]), &biz)
		if biz["out_trade_no"] == "fail" {
			_, _ = w.Write([]byte(`{"alipay_trade_precreate_response":{"code":"40004","msg":"Business Failed","sub_code":"ACQ.TRADE_HAS_SUCCESS","sub_msg":"交易已被支付"},"sign":""}`))
			return
		}
		inner := `{"code":"10000","msg":"Success","out_trade_no":"` + biz["out_trade_no"] + `","qr_code":"https://qr.alipay.com/bax0test"}`
		sig, _ := SignRSA2(ali, inner)
		if biz["out_trade_no"] == "badsig" {
			sig, _ = SignRSA2(app, inner)
		}
		_, _ = w.Write([]byte(`{"alipay_trade_precreate_response":` + inner + `,"sign":"` + sig + `"}`))
	}))
	defer srv.Close()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	gw := &Gateway{Client: srv.Client(), Endpoint: srv.URL, Now: func() time.Time { return now }}
	cfg := cfgFor(t)
	req := payment.PayRequest{TradeNo: tradeNo, TotalAmount: 1234, Currency: "CNY", Subject: "GG Bond - Pro", NotifyURL: notifyURL, ExpiresAt: now.Add(30 * time.Minute)}
	res, err := gw.Pay(context.Background(), cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != payment.PayTypeQRCode || res.Data != "https://qr.alipay.com/bax0test" {
		t.Fatalf("res = %+v", res)
	}
	if form.Get("timestamp") != "2026-10-21 20:00:00" || form.Get("method") != "alipay.trade.precreate" || !strings.Contains(form.Get("biz_content"), `"timeout_express":"30m"`) {
		t.Fatalf("form = %v", form)
	}
	req.TradeNo = "fail"
	if _, err := gw.Pay(context.Background(), cfg, req); err == nil || !strings.Contains(err.Error(), "ACQ.TRADE_HAS_SUCCESS") {
		t.Fatalf("expected provider error, got %v", err)
	}
	req.TradeNo = "badsig"
	if _, err := gw.Pay(context.Background(), cfg, req); err == nil {
		t.Fatal("bad response signature must fail")
	}
	req.TradeNo, req.Currency = tradeNo, "USD"
	if _, err := gw.Pay(context.Background(), cfg, req); err == nil {
		t.Fatal("USD must fail")
	}
	if (&Gateway{}).endpoint(map[string]string{"sandbox": "1"}) != SandboxEndpoint || (&Gateway{}).endpoint(nil) != ProductionEndpoint {
		t.Fatal("endpoint")
	}
	if gw.Name() != Name || gw.Label() == "" || len(gw.Fields()) != 4 {
		t.Fatal("metadata")
	}
}
