package epay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// Review 2026-10-22: Amount 0 means "not reported" to ordersvc and skips the amount check, so a signed
// "money=0.00" success must not be returned as a payment.
func TestReviewZeroAmountIsNotAPayment(t *testing.T) {
	gw := &Gateway{}
	for _, money := range []string{"0", "0.00", "-1"} {
		p := map[string]string{"pid": "1001", "out_trade_no": tradeNo, "trade_no": "x", "money": money, "trade_status": "TRADE_SUCCESS"}
		q := url.Values{}
		for k, v := range p {
			q.Set(k, v)
		}
		q.Set("sign", Sign(p, key))
		r := httptest.NewRequest(http.MethodGet, "/notify?"+q.Encode(), nil)
		if res, _, err := gw.Notify(context.Background(), cfg, r); err == nil || res != nil {
			t.Fatalf("money=%s accepted: %+v", money, res)
		}
	}
}
