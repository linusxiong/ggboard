package admin

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestSettingsRejectsDemotionAfterInitialAuthentication(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			u := testutil.CreateAdmin(t, a, "admin@example.com")
			tok := testutil.Login(t, a, u)
			r := gin.New()
			r.Use(auth.RequireUser(a), func(c *gin.Context) {
				if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("is_admin", false).Error; err != nil {
					t.Error(err)
				}
				c.Next()
			})
			Register(r, a)
			body := any(nil)
			if method == http.MethodPut {
				body = map[string]any{"app_name": "Taken Over"}
			}
			rec := testutil.Do(t, r, method, "/api/v1/admin/settings", body, tok)
			if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), a.Settings.String("server_token")) {
				t.Fatalf("revoked admin: %d %s", rec.Code, rec.Body.String())
			}
			if a.Settings.String("app_name") == "Taken Over" {
				t.Fatal("revoked admin changed settings")
			}
		})
	}
}

func TestSettingsWriteRechecksRoleInsideTransaction(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateAdmin(t, a, "admin@example.com")
	tok := testutil.Login(t, a, u)
	claims, err := auth.Parse(a, tok)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.PUT("/settings", func(c *gin.Context) {
		c.Set(auth.CtxClaims, claims)
		c.Set(auth.CtxUser, u)
		if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("is_admin", false).Error; err != nil {
			t.Error(err)
		}
		c.Next()
	}, settingsPutHandler(a))
	rec := testutil.Do(t, r, http.MethodPut, "/settings", map[string]any{"app_name": "Taken Over"}, "")
	if rec.Code != http.StatusForbidden || a.Settings.String("app_name") == "Taken Over" {
		t.Fatalf("revoked admin write: %d %s", rec.Code, rec.Body.String())
	}
}
