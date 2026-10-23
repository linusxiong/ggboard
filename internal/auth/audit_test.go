package auth_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func auditRouter(t *testing.T, a *app.App) *gin.Engine {
	t.Helper()
	return testutil.NewRouterWith(t, a, func(r gin.IRouter, a *app.App) {
		me := func(c *gin.Context) { httpx.OK(c, auth.CurrentUser(c).Email) }
		r.GET("/user", auth.RequireUser(a), me)
		r.GET("/admin", auth.RequireAdmin(a), me)
	})
}

// Security audit 2026-10-22 §4: the JWT bound only sub (a reusable primary key) and password_version (0 for every
// new or imported row), so a token of a deleted account authenticated as the next account given the same id
// (import xboard --force, a restored backup, an id-reusing MySQL). The email claim now has to match as well.
func TestAuditTokenOfDeletedAccountDoesNotAuthenticateIDReuser(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := auditRouter(t, a)
	old := testutil.CreateUser(t, a, "a@example.com")
	oldTok := testutil.Login(t, a, old)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/user", nil, oldTok), http.StatusOK)

	if err := a.DB.Delete(&model.User{}, old.ID).Error; err != nil {
		t.Fatal(err)
	}
	reuser := model.User{ID: old.ID, Email: "b@example.com", PasswordHash: "!", IsAdmin: true,
		UUID: "30000000-0000-4000-8000-0000000000b1", Token: "c00000000000000000000000000000b1"}
	if err := a.DB.Create(&reuser).Error; err != nil {
		t.Fatal(err)
	}
	if reuser.PasswordVersion != old.PasswordVersion {
		t.Fatalf("precondition: password_version %d vs %d", reuser.PasswordVersion, old.PasswordVersion)
	}
	auth.InvalidateUser(a, old.ID)

	for _, path := range []string{"/user", "/admin"} {
		if e := testutil.DecodeError(t, testutil.Do(t, r, http.MethodGet, path, nil, oldTok)); e.Status != http.StatusUnauthorized {
			t.Fatalf("old token on %s = %+v, want 401", path, e)
		}
	}
	// The account that now owns the id signs in normally.
	if got := testutil.Decode[string](t, testutil.Do(t, r, http.MethodGet, "/admin", nil, testutil.Login(t, a, &reuser))); got != "b@example.com" {
		t.Fatalf("new owner = %q", got)
	}
}

// Changing an account's email (admin edit) ends the sessions issued for the old address; the comparison ignores
// case and surrounding spaces, and a stale cached copy does not reject a token issued after the change.
func TestAuditEmailChangeEndsOldSessions(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := auditRouter(t, a)
	u := testutil.CreateUser(t, a, "old@example.com")
	oldTok := testutil.Login(t, a, u)
	mixed := *u
	mixed.Email = " Old@Example.COM "
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/user", nil, testutil.Login(t, a, &mixed)), http.StatusOK)

	stale := *u
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("email", "new@example.com").Error; err != nil {
		t.Fatal(err)
	}
	renamed := *u
	renamed.Email = "new@example.com"
	newTok := testutil.Login(t, a, &renamed)
	a.Cache.Set("auth:user:"+strconv.FormatUint(u.ID, 10), stale, auth.UserCacheTTL) // a racing stale load
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/user", nil, newTok), http.StatusOK)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/user", nil, oldTok), http.StatusUnauthorized)
}

// Tokens without an email claim (or users without an email) are refused at both ends.
func TestAuditTokenRequiresEmailClaim(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := auditRouter(t, a)
	u := testutil.CreateUser(t, a, "claim@example.com")
	now := time.Now()
	noEmail, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": strconv.FormatUint(u.ID, 10), "ver": u.PasswordVersion, "iss": auth.Issuer,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}).SignedString([]byte(testutil.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.ParseWith([]byte(testutil.JWTSecret), noEmail); err == nil {
		t.Fatal("a token without an email claim must not parse")
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/user", nil, noEmail), http.StatusUnauthorized)
	if _, err := auth.IssueWith([]byte(testutil.JWTSecret), time.Hour, &model.User{ID: u.ID, Email: "  "}, now); err == nil {
		t.Fatal("issuing a token without an email must fail")
	}
}
