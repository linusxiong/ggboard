package auth_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestJWTIssueParse(t *testing.T) {
	secret := []byte(testutil.JWTSecret)
	u := &model.User{ID: 42, Email: "a@example.com", PasswordVersion: 3}
	now := time.Now()
	tok, err := auth.IssueWith(secret, time.Hour, u, now)
	if err != nil {
		t.Fatal(err)
	}
	c, err := auth.ParseWith(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := c.UserID(); id != 42 || c.Email != "a@example.com" || c.Ver != 3 || c.Issuer != auth.Issuer {
		t.Fatalf("claims = %+v", c)
	}
	if _, err := auth.ParseWith([]byte("other-secret"), tok); err == nil {
		t.Fatal("wrong secret must fail")
	}
	expired, _ := auth.IssueWith(secret, time.Minute, u, now.Add(-2*time.Hour))
	if _, err := auth.ParseWith(secret, expired); err == nil {
		t.Fatal("expired token must fail")
	}
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "42", "iss": auth.Issuer, "exp": now.Add(time.Hour).Unix()}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := auth.ParseWith(secret, none); err == nil {
		t.Fatal("alg=none must be rejected")
	}
	hs512, _ := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{"sub": "42", "iss": auth.Issuer, "exp": now.Add(time.Hour).Unix()}).SignedString(secret)
	if _, err := auth.ParseWith(secret, hs512); err == nil {
		t.Fatal("only HS256 is accepted")
	}
	badSub, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "x", "iss": auth.Issuer, "exp": now.Add(time.Hour).Unix()}).SignedString(secret)
	if _, err := auth.ParseWith(secret, badSub); err == nil {
		t.Fatal("non-numeric subject must fail")
	}
	if _, err := auth.IssueWith(nil, time.Hour, u, now); err == nil {
		t.Fatal("empty secret must fail")
	}
}

func routes(r gin.IRouter, a *app.App) {
	me := func(c *gin.Context) { httpx.OK(c, auth.CurrentUser(c).Email) }
	r.GET("/user", auth.RequireUser(a), me)
	r.GET("/admin", auth.RequireAdmin(a), me)
	r.GET("/staff", auth.RequireStaff(a), me)
	r.GET("/optional", auth.OptionalUser(a), func(c *gin.Context) {
		if u := auth.CurrentUser(c); u != nil {
			httpx.OK(c, u.Email)
			return
		}
		httpx.OK(c, "anonymous")
	})
}

func TestMiddlewares(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := testutil.NewRouterWith(t, a, routes)
	user := testutil.CreateUser(t, a, "user@example.com")
	admin := testutil.CreateAdmin(t, a, "admin@example.com")
	staff := testutil.CreateStaff(t, a, "staff@example.com")
	ut, at, st := testutil.Login(t, a, user), testutil.Login(t, a, admin), testutil.Login(t, a, staff)

	expect := func(path, token string, status int) {
		t.Helper()
		testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, path, nil, token), status)
	}
	expect("/user", "", 401)
	expect("/user", "garbage", 401)
	expect("/user", ut, 200)
	expect("/admin", ut, 403)
	expect("/admin", st, 403)
	expect("/admin", at, 200)
	expect("/staff", ut, 403)
	expect("/staff", st, 200)
	expect("/staff", at, 200)
	if got := testutil.Decode[string](t, testutil.Do(t, r, http.MethodGet, "/user", nil, ut)); got != "user@example.com" {
		t.Fatalf("current user = %s", got)
	}
	if got := testutil.Decode[string](t, testutil.Do(t, r, http.MethodGet, "/optional", nil, "")); got != "anonymous" {
		t.Fatal("optional without token")
	}
	if got := testutil.Decode[string](t, testutil.Do(t, r, http.MethodGet, "/optional", nil, "bad")); got != "anonymous" {
		t.Fatal("optional with bad token")
	}
	if got := testutil.Decode[string](t, testutil.Do(t, r, http.MethodGet, "/optional", nil, ut)); got != "user@example.com" {
		t.Fatal("optional with token")
	}

	// Raw "Authorization: <token>" without the Bearer scheme is rejected.
	rec := testutil.DoWithHeaders(t, r, http.MethodGet, "/user", nil, "", map[string]string{"Authorization": ut})
	testutil.ExpectStatus(t, rec, 401)

	// Banning takes effect once the cached copy is invalidated.
	a.DB.Model(&model.User{}).Where("id = ?", user.ID).Update("banned", true)
	auth.InvalidateUser(a, user.ID)
	if e := testutil.DecodeError(t, testutil.Do(t, r, http.MethodGet, "/user", nil, ut)); e.Status != 403 || !strings.Contains(e.Message, "banned") {
		t.Fatalf("banned = %+v", e)
	}

	// Password change bumps password_version: old tokens die, new ones work.
	svc := usersvc.New(a)
	if err := svc.SetPassword(context.Background(), staff, "new-password-1"); err != nil {
		t.Fatal(err)
	}
	expect("/staff", st, 401)
	expect("/staff", testutil.Login(t, a, staff), 200)

	// Deleted users are unauthorized.
	a.DB.Delete(&model.User{}, admin.ID)
	auth.InvalidateUser(a, admin.ID)
	expect("/admin", at, 401)
}

func TestCurrentUserIsACopy(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "copy@example.com")
	first, err := auth.LoadUser(a, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	first.Balance = 999
	second, _ := auth.LoadUser(a, u.ID)
	if second.Balance != 0 {
		t.Fatal("cached user must not be shared between callers")
	}
}

func TestLoginTokens(t *testing.T) {
	a := testutil.NewTestApp(t)
	tok := auth.IssueLoginToken(a, 7)
	if len(tok) != 48 {
		t.Fatalf("token = %q", tok)
	}
	if id, ok := auth.ConsumeLoginToken(a, tok); !ok || id != 7 {
		t.Fatal("consume")
	}
	if _, ok := auth.ConsumeLoginToken(a, tok); ok {
		t.Fatal("tokens are single-use")
	}
	if _, ok := auth.ConsumeLoginToken(a, ""); ok {
		t.Fatal("empty token")
	}
	if d, ok := a.KV.TTL("auth:login_token:" + auth.IssueLoginToken(a, 8)); !ok || d > auth.LoginTokenTTL {
		t.Fatalf("ttl = %v", d)
	}
}
