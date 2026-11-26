package client_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Surge and Surfboard refresh from the #!MANAGED-CONFIG URL, which carries the subscription token. Behind a TLS proxy
// that is not in server.trusted_proxies (the default is none) every request looks like plain http, and the URL used
// to keep http:// on subscribe_url hosts, so the client refreshed, and sent the token, over plain HTTP.
func TestManagedConfigURLUsesHTTPSForConfiguredHosts(t *testing.T) {
	e := setup(t, map[string]any{"subscribe_url": []string{"https://sub.example.com", "http://plain.example.com", "edge.example.com"}})
	managed := func(target string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil) // plain http from 192.0.2.1, an untrusted peer
		req.Header.Set("User-Agent", "Surge iOS/3000")
		rec := httptest.NewRecorder()
		e.r.ServeHTTP(rec, req)
		line, _, _ := strings.Cut(rec.Body.String(), "\n")
		if rec.Code != http.StatusOK || !strings.HasPrefix(line, "#!MANAGED-CONFIG ") {
			t.Fatalf("%s: %d %.200s", target, rec.Code, rec.Body.String())
		}
		url, _, _ := strings.Cut(strings.TrimPrefix(line, "#!MANAGED-CONFIG "), " ")
		return url
	}
	for _, path := range []string{"/s/" + e.u.Token, "/api/v1/client/subscribe?token=" + e.u.Token} {
		for host, scheme := range map[string]string{
			"sub.example.com":   "https", // an https subscribe_url entry
			"SUB.example.com":   "https", // host names are case-insensitive
			"panel.example.com": "https", // app_url
			"plain.example.com": "http",  // configured as http
			"edge.example.com":  "http",  // no scheme: not known to be https
			"other.example.com": "http",  // not configured
		} {
			if got, want := managed("http://"+host+path), scheme+"://"+host+path; got != want {
				t.Errorf("managed URL %s, want %s", got, want)
			}
		}
	}
}
