package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Review 2026-10-22: rate-limit keys used the full IPv6 address, so any host with a /64 could rotate past every
// per-IP limit (passport, send-code, register-by-IP, subscription misses, node token failures).
func TestReviewIPKey(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.7":          "203.0.113.7",
		"2001:db8:1:2::1":      "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::9": "2001:db8:1:2::/64",
		"::ffff:203.0.113.7":   "::ffff:203.0.113.7",
		"fe80::1%eth0":         "fe80::/64",
		"not-an-ip":            "not-an-ip",
	} {
		if got := IPKey(in); got != want {
			t.Errorf("IPKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// Review 2026-10-22: X-Forwarded-Proto was trusted from any client, so links built from the request (notify,
// return, Surge managed-config and passkey URLs when app_url is empty) followed whatever a client claimed.
func TestReviewRequestSchemeTrustsOnlyProxies(t *testing.T) {
	for _, tc := range []struct {
		trusted []string
		remote  string
		want    string
	}{
		{nil, "192.0.2.1:1234", "http"},
		{[]string{"10.0.0.0/8"}, "192.0.2.1:1234", "http"},
		{[]string{"192.0.2.1"}, "192.0.2.1:1234", "https"},
		{[]string{"::1"}, "[::1]:443", "https"},
	} {
		e := gin.New()
		e.Use(trustedPeer(parsePrefixes(tc.trusted)))
		e.GET("/", func(c *gin.Context) { c.String(200, RequestScheme(c)) })
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tc.remote
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Body.String() != tc.want {
			t.Errorf("trusted %v from %s: %s, want %s", tc.trusted, tc.remote, rec.Body.String(), tc.want)
		}
	}
}
