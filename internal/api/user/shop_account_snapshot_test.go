package user

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestShopOrderReadsRejectReplacedAccountAfterMiddleware(t *testing.T) {
	const tradeNo = "20261028000000999998"
	for _, tc := range []struct {
		name, route, path string
		handler           func(*app.App) gin.HandlerFunc
	}{
		{"list", "/orders", "/orders", shopListOrders},
		{"detail", "/orders/:trade_no", "/orders/" + tradeNo, shopGetOrder},
		{"status", "/orders/:trade_no/status", "/orders/" + tradeNo + "/status", shopOrderStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			u := testutil.CreateUser(t, a, "old@example.com")
			token := testutil.Login(t, a, u)
			plan := &model.Plan{Name: "New account plan"}
			if err := a.DB.Create(plan).Error; err != nil {
				t.Fatal(err)
			}
			r := gin.New()
			r.GET(tc.route, auth.RequireUser(a), func(c *gin.Context) {
				if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("uuid", "replacement-account-uuid").Error; err != nil {
					t.Error(err)
				}
				if err := a.DB.Create(&model.Order{TradeNo: tradeNo, UserID: u.ID, PlanID: plan.ID,
					Type: model.OrderTypeNew, Period: model.PeriodMonthly, Status: model.OrderStatusPending}).Error; err != nil {
					t.Error(err)
				}
				tc.handler(a)(c)
			})
			rec := testutil.Do(t, r, http.MethodGet, tc.path, nil, token)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("old account read replacement order: status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestShopCouponCheckRejectsReplacedAccountAfterMiddleware(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "old@example.com")
	token := testutil.Login(t, a, u)
	plan := &model.Plan{Name: "Pro", Prices: model.JSONObject[int64]{model.PeriodMonthly: 1000}}
	if err := a.DB.Create(plan).Error; err != nil {
		t.Fatal(err)
	}
	coupon := &model.Coupon{Code: "SAVE", Name: "Private", Type: model.CouponTypeAmount, Value: 100,
		Show: true, StartedAt: time.Now().Add(-time.Hour), EndedAt: time.Now().Add(time.Hour)}
	if err := a.DB.Create(coupon).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.POST("/check", auth.RequireUser(a), func(c *gin.Context) {
		if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("uuid", "replacement-account-uuid").Error; err != nil {
			t.Error(err)
		}
		shopCheckCoupon(a)(c)
	})
	rec := testutil.Do(t, r, http.MethodPost, "/check", map[string]any{
		"code": coupon.Code, "plan_id": plan.ID, "period": model.PeriodMonthly,
	}, token)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("stale session learned coupon eligibility: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
