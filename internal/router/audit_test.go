package router_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/router"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Security audit 2026-10-22 §8: with RemoveExtraSlash on, gin routed "//evil.example/../healthz/" as "/healthz/",
// found the trailing-slash match and redirected to the raw path: "301 Location: //evil.example/../healthz", which
// browsers treat as protocol-relative (an open redirect on the panel's own domain). Redirects are now off.
func TestAuditNoOpenRedirectThroughTrailingSlash(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"server_token": "router-audit-token-0123456789abcdef"}))
	e, err := router.New(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "//evil.example%2f..%2fhealthz/"},
		{http.MethodGet, "//evil.example/../healthz/"},
		{http.MethodGet, "//evil.example%2f..%2fs%2fabc/?x=1"},
		{http.MethodGet, "//evil.example%2f..%2f.well-known%2fchange-password/"},
		{http.MethodPost, "//evil.example%2f..%2fapi%2fv1%2fpassport%2flogin/"},
		{http.MethodGet, "/healthz/"},
	} {
		for _, prefix := range []string{"", "//evil.example"} {
			req := httptest.NewRequest(tc.method, tc.target, strings.NewReader("{}"))
			if prefix != "" {
				req.Header.Set("X-Forwarded-Prefix", prefix)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code >= 300 && rec.Code < 400 || rec.Header().Get("Location") != "" {
				t.Errorf("%s %s (prefix %q) = %d Location %q, want no redirect",
					tc.method, tc.target, prefix, rec.Code, rec.Header().Get("Location"))
			}
		}
	}

	// Extra slashes are still tolerated where clients need them (soga's naive webapi_url join, XrayR's ApiHost):
	// the request reaches the node routes, which reject the missing key instead of answering 404.
	for _, target := range []string{"/soga//api/v1/users", "//api/v1/server/UniProxy/config?node_id=1&node_type=vmess"} {
		rec := testutil.Do(t, e, http.MethodGet, target, nil, "")
		if rec.Code != http.StatusForbidden {
			t.Errorf("GET %s = %d %s, want 403 from the node routes", target, rec.Code, rec.Body.String())
		}
	}
	testutil.ExpectStatus(t, testutil.Do(t, e, http.MethodGet, "//healthz", nil, ""), http.StatusOK)
}
