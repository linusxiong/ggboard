package auth_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Review 2026-10-22: a user-cache load that read the row before a password change committed could be stored after
// InvalidateUser, and the fresh token was then rejected for up to 30 s. A version mismatch now re-reads the row.
func TestReviewStaleUserCacheDoesNotRejectNewToken(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "fresh@example.com")
	r := testutil.NewRouterWith(t, a, func(r gin.IRouter, a *app.App) {
		r.GET("/me", auth.RequireUser(a), func(c *gin.Context) { httpx.OK(c, true) })
	})
	stale := *u // password_version before the change
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("password_version", u.PasswordVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	var cur model.User
	if err := a.DB.First(&cur, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	newTok := testutil.Login(t, a, &cur)
	oldTok := testutil.Login(t, a, &stale)
	a.Cache.Set("auth:user:"+strconv.FormatUint(u.ID, 10), stale, auth.UserCacheTTL) // the racing stale load
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/me", nil, newTok), http.StatusOK)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/me", nil, oldTok), http.StatusUnauthorized)
}
