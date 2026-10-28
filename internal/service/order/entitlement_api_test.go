package ordersvc_test

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/client"
	"github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription/fixture"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestUserCannotGrantUnpaidEntitlements(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"show_info_to_server_enable": false}))
	if _, err := fixture.SeedNodes(a.DB); err != nil {
		t.Fatal(err)
	}
	plan := model.Plan{GroupID: fixture.GroupID, Name: "Pro", TransferEnable: 100, Show: true, Sell: true, Renew: true,
		Prices: model.JSONObject[int64]{model.PeriodMonthly: 1000}}
	if err := a.DB.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	u := testutil.CreateUser(t, a, "buyer@example.com")
	token := testutil.Login(t, a, u)
	r := testutil.NewRouterWith(t, a, user.Register, client.Register)

	// Profile writes only accept notification preferences; client-supplied entitlements are ignored.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, "/api/v1/user/me", map[string]any{
		"remind_expire": true, "plan_id": plan.ID, "group_id": plan.GroupID, "transfer_enable": 1 << 40,
		"balance": 100000, "expired_at": "2099-01-01T00:00:00Z", "is_admin": true,
	}, token), http.StatusOK)
	// Forged money/status fields cannot turn an ordinary order into a free or completed order.
	created := testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders", map[string]any{
		"plan_id": plan.ID, "period": model.PeriodMonthly, "total_amount": 0, "balance_amount": 1000, "status": 3,
	}, token)
	testutil.ExpectStatus(t, created, http.StatusCreated)
	o := testutil.Decode[struct {
		TradeNo string            `json:"trade_no"`
		Status  model.OrderStatus `json:"status"`
	}](t, created)
	if o.Status != model.OrderStatusPending {
		t.Fatalf("unpaid order status = %d", o.Status)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/user/orders/"+o.TradeNo+"/checkout",
		map[string]any{"total_amount": 0, "status": 3}, token), http.StatusBadRequest)
	var stored model.User
	if err := a.DB.First(&stored, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PlanID != nil || stored.GroupID != nil || stored.TransferEnable != 0 || stored.Balance != 0 || stored.ExpiredAt != nil || stored.IsAdmin {
		t.Fatal("ordinary user granted entitlement or balance")
	}
	nodes := testutil.Decode[[]any](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/nodes", nil, token))
	if len(nodes) != 0 {
		t.Fatal("unpaid user received nodes")
	}
	// Both public subscription entry points must remain empty before payment.
	for _, path := range []string{"/api/v1/client/subscribe?flag=v2ray&token=" + u.Token, "/s/" + u.Token + "?flag=v2ray"} {
		resp := testutil.Do(t, r, http.MethodGet, path, nil, "")
		testutil.ExpectStatus(t, resp, http.StatusOK)
		links, err := base64.StdEncoding.DecodeString(resp.Body.String())
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(links)) != "" {
			t.Fatal("unpaid subscription contains node links")
		}
	}
}
