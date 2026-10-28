package user

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestRuleProfilePutRejectsSessionChangedAfterMiddleware(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "alice@example.com")
	r := gin.New()
	r.PUT("/profile", func(c *gin.Context) {
		c.Set(auth.CtxUser, u)
		if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("password_version", u.PasswordVersion+1).Error; err != nil {
			t.Error(err)
		}
		c.Next()
	}, putRuleProfile(a, ""))
	rec := testutil.Do(t, r, http.MethodPut, "/profile", map[string]any{"template": "minimal"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("stale profile write: %d %s", rec.Code, rec.Body.String())
	}
	var n int64
	if err := a.DB.Model(&model.RuleProfile{}).Where("user_id = ?", u.ID).Count(&n).Error; err != nil || n != 0 {
		t.Fatalf("profiles changed: %d, %v", n, err)
	}
}

func TestRuleProfileReadsRejectReusedAccountID(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "alice@example.com")
	if err := engine.EnsureDefault(t.Context(), a, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).
		Update("uuid", "00000000-0000-0000-0000-000000000002").Error; err != nil {
		t.Fatal(err)
	}
	if err := engine.SaveProfile(t.Context(), a, u.ID, []byte(`{"template":"full"}`)); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(auth.CtxUser, u); c.Next() })
	r.GET("/profile", getRuleProfile(a, ""))
	r.GET("/profiles", listRuleProfiles(a))
	for _, path := range []string{"/profile", "/profiles"} {
		rec := testutil.Do(t, r, http.MethodGet, path, nil, "")
		if rec.Code != http.StatusUnauthorized || string(rec.Body.Bytes()) == "" {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
}
