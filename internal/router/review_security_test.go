package router_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/httpx"
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
