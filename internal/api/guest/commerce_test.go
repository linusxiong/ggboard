package guest_test

import (
	"net/http"
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
		{Name: "A", Show: true, Sell: true, Sort: 1, Prices: model.JSONObject[int64]{"monthly": 100}},
		{Name: "Hidden", Show: false, Sell: true},
		{Name: "Legacy", Show: true, Sell: false},
	} {
		if err := a.DB.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	}
	type plan struct {
		Name   string           `json:"name"`
		Prices map[string]int64 `json:"prices"`
	}
	plans, total := testutil.DecodeList[plan](t, testutil.Do(t, r, http.MethodGet, "/api/v1/guest/plans", nil, ""))
	if total != 2 || plans[0].Name != "A" || plans[1].Name != "B" || plans[1].Prices["monthly"] != 200 {
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
