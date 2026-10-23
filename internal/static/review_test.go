package static_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/static"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Review 2026-10-22: unknown /.well-known/* paths got the SPA page with 200, which clients (WebAuthn related
// origins, universal links, ACME) parse as the resource. Existing files there are still served.
func TestReviewWellKnownIsNotSPA(t *testing.T) {
	fs := dist()
	fs[".well-known/security.txt"] = fs["robots.txt"]
	e := router(t, fs)
	for _, p := range []string{"/.well-known/webauthn", "/.well-known/apple-app-site-association", "/.well-known/acme-challenge/x"} {
		if r := get(t, e, p, ""); r.code != 404 || strings.Contains(r.body, "<title>") {
			t.Fatalf("%s = %d %q", p, r.code, r.body)
		}
	}
	if r := get(t, e, "/.well-known/security.txt", ""); r.code != 200 {
		t.Fatalf("existing well-known file = %d", r.code)
	}
}

// Security audit 2026-10-22 §8: trailing-slash redirects are off, so "/s/<token>/" no longer redirects to the short
// link. It must be a 404 like the other API paths, not the SPA shell (a 200 a client would parse as a subscription).
func TestReviewSubscriptionShortLinkIsNotSPA(t *testing.T) {
	e := router(t, dist())
	for _, p := range []string{"/s/abc/", "/s/", "/s/abc/x"} {
		if r := get(t, e, p, ""); r.code != 404 || strings.Contains(r.body, "<title>") {
			t.Fatalf("%s = %d %q", p, r.code, r.body)
		}
	}
}

// Review 2026-10-22: the SPA's CSP (script-src 'self') blocked the captcha widget the login page loads, so with a
// provider enabled nobody could sign in. The policy now allows exactly the enabled provider.
func TestReviewCSPAllowsEnabledCaptcha(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>GG</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := testutil.NewTestApp(t)
	a.Cfg.Frontend.Dir = dir
	e := testutil.NewRouterWith(t, a, func(gin.IRouter, *app.App) {})
	if err := static.Mount(e, a); err != nil {
		t.Fatal(err)
	}
	csp := func() string { return get(t, e, "/login", "").h.Get("Content-Security-Policy") }
	if got := csp(); got != static.CSP {
		t.Fatalf("no captcha: %s", got)
	}
	for provider, want := range map[string]string{
		"turnstile": "script-src 'self' https://challenges.cloudflare.com",
		"recaptcha": "https://www.recaptcha.net/recaptcha/",
		"hcaptcha":  "connect-src 'self' https://hcaptcha.com https://*.hcaptcha.com",
	} {
		if err := a.Settings.Set(map[string]any{"captcha_provider": provider, "captcha_site_key": "k", "captcha_secret": "s"}); err != nil {
			t.Fatal(err)
		}
		got := csp()
		if !strings.Contains(got, want) || !strings.Contains(got, "frame-src 'self' https://") || !strings.Contains(got, "frame-ancestors 'none'") {
			t.Fatalf("%s: %s", provider, got)
		}
		for other := range map[string]bool{"challenges.cloudflare.com": true, "hcaptcha.com": true, "recaptcha": true} {
			if !strings.Contains(want, other) && strings.Contains(got, other) {
				t.Fatalf("%s policy also allows %s: %s", provider, other, got)
			}
		}
	}
}
