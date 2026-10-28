package admin

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

func TestCatalogEndpointsRejectDemotedAdminAfterMiddleware(t *testing.T) {
	type route struct {
		name, method, path string
		register           func(*gin.RouterGroup, *app.App)
		body               func(uint64) any
		table              any
	}
	start, end := time.Now().Add(-time.Hour).Format(time.RFC3339), time.Now().Add(time.Hour).Format(time.RFC3339)
	for _, tc := range []route{
		{"plans read", http.MethodGet, "/plans", registerPlans, nil, nil},
		{"plans write", http.MethodPost, "/plans", registerPlans, func(id uint64) any {
			return map[string]any{"group_id": id, "name": "Pro", "prices": map[string]int64{"monthly": 1000}}
		}, &model.Plan{}},
		{"groups read", http.MethodGet, "/groups", registerGroups, nil, nil},
		{"groups write", http.MethodPost, "/groups", registerGroups, func(uint64) any { return map[string]any{"name": "New group"} }, &model.ServerGroup{}},
		{"routes read", http.MethodGet, "/routes", registerRoutes, nil, nil},
		{"routes write", http.MethodPost, "/routes", registerRoutes, func(uint64) any {
			return map[string]any{"match": []string{"example.com"}, "action": "block"}
		}, &model.NodeRoute{}},
		{"coupons read", http.MethodGet, "/coupons", registerCoupons, nil, nil},
		{"coupons write", http.MethodPost, "/coupons", registerCoupons, func(uint64) any {
			return map[string]any{"name": "New coupon", "code": "NEWCODE", "type": 1, "value": 100, "started_at": start, "ended_at": end}
		}, &model.Coupon{}},
		{"rulesets read", http.MethodGet, "/rulesets", registerRulesets, nil, nil},
		{"rulesets write", http.MethodPut, "/rulesets", registerRulesets, func(uint64) any { return map[string]any{"custom": nil} }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			admin := testutil.CreateAdmin(t, a, "admin@example.com")
			token := testutil.Login(t, a, admin)
			group := &model.ServerGroup{Name: "Existing"}
			if err := a.DB.Create(group).Error; err != nil {
				t.Fatal(err)
			}
			var before int64
			if tc.table != nil {
				if err := a.DB.Model(tc.table).Count(&before).Error; err != nil {
					t.Fatal(err)
				}
			}
			r := gin.New()
			g := r.Group("/api/v1/admin", auth.RequireAdmin(a), func(c *gin.Context) {
				if err := a.DB.Model(&model.User{}).Where("id = ?", admin.ID).Update("is_admin", false).Error; err != nil {
					t.Error(err)
				}
				c.Next()
			})
			tc.register(g, a)
			var body any
			if tc.body != nil {
				body = tc.body(group.ID)
			}
			rec := testutil.Do(t, r, tc.method, "/api/v1/admin"+tc.path, body, token)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("demoted admin got %d: %s", rec.Code, rec.Body.String())
			}
			if tc.table != nil {
				var after int64
				if err := a.DB.Model(tc.table).Count(&after).Error; err != nil || before != after {
					t.Fatalf("demoted admin changed %s: before=%d after=%d err=%v", tc.name, before, after, err)
				}
			}
		})
	}
}

func TestCatalogReadRejectsReplacedAdminAfterMiddleware(t *testing.T) {
	a := testutil.NewTestApp(t)
	admin := testutil.CreateAdmin(t, a, "admin@example.com")
	token := testutil.Login(t, a, admin)
	r := gin.New()
	g := r.Group("/api/v1/admin", auth.RequireAdmin(a), func(c *gin.Context) {
		if err := a.DB.Model(&model.User{}).Where("id = ?", admin.ID).Update("uuid", "replacement-account-uuid").Error; err != nil {
			t.Error(err)
		}
		c.Next()
	})
	registerCoupons(g, a)
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/admin/coupons", nil, token)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old admin read replacement account coupons: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
