package router_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/router"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Security audit 2026-11-25 §1: every anonymous /passport/* route decoded YAML (and XML, TOML, msgpack, BSON,
// protobuf) when the client said so, and goccy/go-yaml expands aliases without limit: a ~700-byte alias bomb with
// Content-Type application/x-yaml to /passport/login OOM-killed the panel. The body here is a single
// self-referencing alias (old code: decoded, 400), never a bomb; the fix answers 415 before reading it.
func TestReviewPassportRefusesNonJSONBodies(t *testing.T) {
	a := testutil.NewTestApp(t)
	e, err := router.New(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/passport/login", "/api/v1/passport/register", "/api/v1/passport/login/token", "/api/v1/passport/forgot",
		"/api/v1/passport/email/send-code", "/api/v1/passport/login/mfa", "/api/v1/passport/passkey/login/begin",
		"/api/v1/passport/passkey/login/finish",
	} {
		for _, ct := range []string{"application/x-yaml", "application/yaml"} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("email: &a [*a]"))
			req.Header.Set("Content-Type", ct)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if got := testutil.DecodeError(t, rec); got.Status != http.StatusUnsupportedMediaType || got.Code != httpx.CodeUnsupportedMediaType {
				t.Errorf("POST %s (%s) = %d %s, want 415", path, ct, rec.Code, rec.Body.String())
			}
		}
	}
	// The same route still decodes JSON: wrong credentials are a 401, not a 415.
	rec := testutil.Do(t, e, http.MethodPost, "/api/v1/passport/login", map[string]any{"email": "nobody@example.com", "password": "wrong-password"}, "")
	if got := testutil.DecodeError(t, rec); got.Status != http.StatusUnauthorized {
		t.Fatalf("JSON login = %d %s, want 401", rec.Code, rec.Body.String())
	}
}

// Security audit 2026-11-25 §4: API responses (the login JWT, admin settings with secrets) carried no
// Cache-Control. They default to no-store now; handlers with their own caching keep it, and the node endpoints'
// ETag/304 still works.
func TestReviewAPIResponsesAreNotCacheable(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"server_token": "router-cache-token-0123456789abcdef"}))
	e, err := router.New(a)
	if err != nil {
		t.Fatal(err)
	}
	cc := func(rec *httptest.ResponseRecorder) string { return rec.Header().Get("Cache-Control") }

	u := testutil.CreateUser(t, a, "cache@example.com")
	rec := testutil.Do(t, e, http.MethodPost, "/api/v1/passport/login", map[string]any{"email": u.Email, "password": testutil.Password}, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token"`) || cc(rec) != "no-store" {
		t.Fatalf("login = %d Cache-Control %q", rec.Code, cc(rec))
	}
	admin := testutil.Login(t, a, testutil.CreateAdmin(t, a, "cache-admin@example.com"))
	for _, p := range []string{"/api/v1/admin/settings", "/api/v1/user/subscription", "/healthz"} {
		if rec := testutil.Do(t, e, http.MethodGet, p, nil, admin); rec.Code != http.StatusOK || cc(rec) != "no-store" {
			t.Errorf("GET %s = %d Cache-Control %q, want no-store", p, rec.Code, cc(rec))
		}
	}
	// Own values are kept: the public rule-set catalog, the passkey well-known document and static files (HTML and
	// assets: internal/static tests). Without app_url the well-known URLs come from the request Host, so that
	// document is no-store until app_url is set.
	for p, want := range map[string]string{
		"/api/v1/guest/rulesets/catalog":  "public, max-age=300",
		"/.well-known/passkey-endpoints":  "no-store",
		"/assets/does-not-exist-12345.js": "no-cache",
	} {
		if rec := testutil.Do(t, e, http.MethodGet, p, nil, ""); cc(rec) != want {
			t.Errorf("GET %s Cache-Control = %q, want %q", p, cc(rec), want)
		}
	}
	if err := a.Settings.Set(map[string]any{"app_url": "https://panel.example.com"}); err != nil {
		t.Fatal(err)
	}
	if rec := testutil.Do(t, e, http.MethodGet, "/.well-known/passkey-endpoints", nil, ""); cc(rec) != "public, max-age=3600" {
		t.Errorf("GET /.well-known/passkey-endpoints with app_url Cache-Control = %q", cc(rec))
	}
	// A node's user list: no-store, and a revalidation with its ETag is still a 304.
	users := "/api/v1/server/UniProxy/user?node_id=1&node_type=vless&token=router-cache-token-0123456789abcdef"
	if err := a.DB.Create(&model.Node{ID: 1, Type: "vless", Name: "n", Host: "n.example.com", Port: "443", ServerPort: 443}).Error; err != nil {
		t.Fatal(err)
	}
	rec = testutil.Do(t, e, http.MethodGet, users, nil, "")
	tag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || tag == "" || cc(rec) != "no-store" {
		t.Fatalf("node users = %d ETag %q Cache-Control %q", rec.Code, tag, cc(rec))
	}
	rec = testutil.DoWithHeaders(t, e, http.MethodGet, users, nil, "", map[string]string{"If-None-Match": tag})
	if rec.Code != http.StatusNotModified || rec.Header().Get("ETag") != tag {
		t.Fatalf("node users revalidation = %d ETag %q", rec.Code, rec.Header().Get("ETag"))
	}
}

// Security audit 2026-11-25 §2: gin routes on the slash-cleaned path, but the SPA fallback judged the raw one, so
// "//s/<token>/" and "//api/v1/<unknown>" were answered with the HTML shell and 200 instead of the JSON 404.
func TestReviewDoubleSlashAPIPathsAreNotSPA(t *testing.T) {
	a := testutil.NewTestApp(t)
	e, err := router.New(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"//s/tok/", "///s/tok/", "//api/v1/nothing", "//soga/api/v1/nothing"} {
		rec := testutil.Do(t, e, http.MethodGet, p, nil, "")
		if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") ||
			!strings.Contains(rec.Body.String(), `"code":"`+httpx.CodeNotFound+`"`) {
			t.Errorf("GET %s = %d %s, want the JSON 404", p, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}
