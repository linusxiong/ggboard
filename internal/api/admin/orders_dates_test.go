package admin_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// The order list's date filter: RFC 3339 bounds are instants (date_to exclusive), which is what the web app
// sends for the admin's local days; a bare YYYY-MM-DD is a calendar day of the server zone (date_to inclusive).
// Server in Shanghai, admin in New York: an order created 2026-10-27T02:30Z shows as Oct 26 22:30 in the browser.
func TestAdminOrdersDateBounds(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	withProcessZone(t, shanghai)
	e := newCommerceEnv(t)
	plan := &model.Plan{Name: "Pro", Show: true, Sell: true, Renew: true, TransferEnable: 10, Prices: model.JSONObject[int64]{"monthly": 1000}}
	if err := e.a.DB.Create(plan).Error; err != nil {
		t.Fatal(err)
	}
	u := testutil.CreateUser(t, e.a, "cust@example.com")
	created := time.Date(2026, 10, 27, 2, 30, 0, 0, time.UTC)
	if err := e.a.DB.Create(&model.Order{
		TradeNo: "D1", UserID: u.ID, PlanID: plan.ID, Type: model.OrderTypeNew, Period: model.PeriodMonthly,
		Status: model.OrderStatusPending, TotalAmount: 1000, CreatedAt: created,
	}).Error; err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name     string
		from, to string
		want     int64
	}{
		{"New York Oct 26 (what the web app sends)", "2026-10-26T04:00:00Z", "2026-10-27T04:00:00Z", 1},
		{"New York Oct 27", "2026-10-27T04:00:00Z", "2026-10-28T04:00:00Z", 0},
		{"date_to is exclusive", "", "2026-10-27T02:30:00Z", 0},
		{"date_from is inclusive", "2026-10-27T02:30:00Z", "", 1},
		{"offset form", "2026-10-26T00:00:00-04:00", "2026-10-27T00:00:00-04:00", 1},
		{"server day Oct 26 (date only)", "2026-10-26", "2026-10-26", 0},
		{"server day Oct 27 (date only, inclusive)", "2026-10-27", "2026-10-27", 1},
	} {
		q := url.Values{}
		if c.from != "" {
			q.Set("date_from", c.from)
		}
		if c.to != "" {
			q.Set("date_to", c.to)
		}
		_, total := testutil.DecodeList[commerceOrder](t, e.do(t, http.MethodGet, "/orders?"+q.Encode(), nil))
		if total != c.want {
			t.Errorf("%s (%s): total %d, want %d", c.name, q.Encode(), total, c.want)
		}
	}
}
