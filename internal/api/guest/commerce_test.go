package guest_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/guest"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/payment/gateways/epay"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestGuestPlansAndPaymentRoutes(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := testutil.NewRouterWith(t, a, guest.Register)
	for _, p := range []model.Plan{
		{Name: "B", Show: true, Sell: true, Sort: 2, Prices: model.JSONObject[int64]{"monthly": 200}},
		{Name: "A", Show: true, Sell: true, Sort: 1, Prices: model.JSONObject[int64]{"monthly": 100}, ConnLimit: testutil.Ptr(64)},
		{Name: "Hidden", Show: false, Sell: true},
		{Name: "Legacy", Show: true, Sell: false},
	} {
		if err := a.DB.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	}
	type plan struct {
		Name      string           `json:"name"`
		Prices    map[string]int64 `json:"prices"`
		ConnLimit *int             `json:"conn_limit"`
	}
	plans, total := testutil.DecodeList[plan](t, testutil.Do(t, r, http.MethodGet, "/api/v1/guest/plans", nil, ""))
	if total != 2 || plans[0].Name != "A" || plans[1].Name != "B" || plans[1].Prices["monthly"] != 200 ||
		plans[0].ConnLimit == nil || *plans[0].ConnLimit != 64 || plans[1].ConnLimit != nil {
		t.Fatalf("plans = %+v", plans)
	}

	// Without app_url the return endpoint redirects relative to the panel.
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/guest/payment/return/2026102212000012345?a=1", nil, "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/orders/2026102212000012345" {
		t.Fatalf("return = %d %s", rec.Code, rec.Header().Get("Location"))
	}

	// Unknown payment, disabled payment and bad signatures answer 400 "fail".
	pay := &model.Payment{UUID: strings.Repeat("d", 32), Gateway: epay.Name, Name: "EPay", Enabled: false,
		Config: model.JSONObject[string]{"url": "https://pay.example.com", "pid": "1", "key": "k"}}
	if err := a.DB.Create(pay).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/guest/payment/notify/epay/" + strings.Repeat("0", 32),
		"/api/v1/guest/payment/notify/epay/" + pay.UUID,
	} {
		rec := testutil.Do(t, r, http.MethodPost, path, url.Values{"pid": {"1"}}, "")
		if rec.Code != http.StatusBadRequest || rec.Body.String() != "fail" {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body.String())
		}
	}
	a.DB.Model(pay).Update("enabled", true)
	// A signed non-payment event is acknowledged.
	params := map[string]string{"pid": "1", "out_trade_no": "x", "trade_status": "WAIT_BUYER_PAY"}
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	q.Set("sign", epay.Sign(params, "k"))
	rec = testutil.Do(t, r, http.MethodPost, "/api/v1/guest/payment/notify/EPay/"+pay.UUID, q, "")
	if rec.Code != http.StatusOK || rec.Body.String() != "success" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("ack = %d %s", rec.Code, rec.Body.String())
	}
	q.Set("sign", "00000000000000000000000000000000")
	if rec = testutil.Do(t, r, http.MethodPost, "/api/v1/guest/payment/notify/epay/"+pay.UUID, q, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad sign = %d", rec.Code)
	}
}

// Security review 2026-10-29: the notify endpoint was the only unauthenticated route without a limit, and it hits the
// database before verifying. Unverified notifications are charged to a per-IP miss bucket (429 before any lookup once
// it is empty); verified callbacks are never charged, so a gateway's retries keep working.
func TestGuestPaymentNotifyMissesLimited(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := testutil.NewRouterWith(t, a, guest.Register)
	pay := &model.Payment{UUID: strings.Repeat("e", 32), Gateway: epay.Name, Name: "EPay", Enabled: true,
		Config: model.JSONObject[string]{"url": "https://pay.example.com", "pid": "1", "key": "k"}}
	if err := a.DB.Create(pay).Error; err != nil {
		t.Fatal(err)
	}
	params := map[string]string{"pid": "1", "out_trade_no": "x", "trade_status": "WAIT_BUYER_PAY"}
	signed := url.Values{}
	for k, v := range params {
		signed.Set(k, v)
	}
	signed.Set("sign", epay.Sign(params, "k"))
	path := "/api/v1/guest/payment/notify/epay/" + pay.UUID

	// Verified callbacks: never charged, however many.
	for i := range guest.NotifyMissBurst + 10 {
		if rec := testutil.Do(t, r, http.MethodPost, path, signed, ""); rec.Code != http.StatusOK {
			t.Fatalf("verified callback %d = %d", i+1, rec.Code)
		}
	}
	// Misses: the burst answers 400 "fail", then 429 with Retry-After.
	codes := map[int]int{}
	var limited *httptest.ResponseRecorder
	for range guest.NotifyMissBurst + 5 {
		rec := testutil.Do(t, r, http.MethodPost, "/api/v1/guest/payment/notify/epay/"+strings.Repeat("0", 32), url.Values{"pid": {"1"}}, "")
		codes[rec.Code]++
		if rec.Code == http.StatusTooManyRequests {
			limited = rec
		}
	}
	if codes[http.StatusBadRequest] != guest.NotifyMissBurst || codes[http.StatusTooManyRequests] != 5 || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("miss codes = %v", codes)
	}
	// An exhausted address is refused before the lookup, verified or not; another address is unaffected.
	if rec := testutil.Do(t, r, http.MethodPost, path, signed, ""); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("exhausted address = %d, want 429", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(signed.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "198.51.100.20:443"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("other address = %d %s", rec.Code, rec.Body.String())
	}
}
