package settings

import (
	"errors"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
)

// Review 2026-10-22: server_token authenticates every node backend and node endpoints are not rate limited, so
// an admin-chosen short token could be brute-forced online. Xboard requires 16 characters as well.
func TestReviewServerTokenMinLength(t *testing.T) {
	s, err := New(newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(map[string]any{"server_token": "short-token"}); err == nil {
		t.Fatal("an 11-character server_token was accepted")
	}
	for _, ok := range []string{"0123456789abcdef", ""} {
		if err := s.Set(map[string]any{"server_token": ok}); err != nil {
			t.Fatalf("%q rejected: %v", ok, err)
		}
	}
}

// Review 2026-10-22: PUT /admin/settings {"server_token": null} reset it to "" until the next boot, rejecting every
// node. A reset now generates a new token at once.
func TestReviewServerTokenResetRegenerates(t *testing.T) {
	s, err := New(newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	old := s.String("server_token")
	if err := s.Set(map[string]any{"server_token": nil}); err != nil {
		t.Fatal(err)
	}
	if got := s.String("server_token"); len(got) != 32 || got == old {
		t.Fatalf("server_token after reset = %q (old %q)", got, old)
	}
}

// Security audit 2026-11-25 §5: the link settings rendered as href in the SPA (tos_url on the landing and register
// pages, telegram_discuss_link, the client download links) and used as bases (app_url, the mail logo) took any
// string. They are http(s) URLs now; a value without a scheme means https, and a stored value that does not pass is
// ignored on load (the default "" applies).
func TestReviewLinkSettingsAreHTTPURLs(t *testing.T) {
	d := newDB(t)
	s, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{"app_url", "logo", "tos_url", "telegram_discuss_link",
		"windows_download_url", "macos_download_url", "android_download_url", "ios_download_url"}
	for _, k := range keys {
		for _, bad := range []string{"javascript:alert(1)", "JavaScript://%0aalert(1)", "data:text/html,<b>x</b>",
			"vbscript:msgbox(1)", "/logo.svg", "//cdn.example.com/logo.svg", "tg://resolve?domain=example",
			"mailto:admin@example.com", "https://user:pass@example.com/", "https://example.com/a b"} {
			var fe FieldErrors
			if err := s.Set(map[string]any{k: bad}); !errors.As(err, &fe) || len(fe[k]) == 0 {
				t.Errorf("%s = %q accepted (err %v)", k, bad, err)
			}
		}
		for in, want := range map[string]string{
			"https://example.com/x?y=1": "https://example.com/x?y=1",
			" http://example.com:8080 ": "http://example.com:8080",
			"t.me/example_group":        "https://t.me/example_group",
			"":                          "",
		} {
			if err := s.Set(map[string]any{k: in}); err != nil || s.String(k) != want {
				t.Errorf("%s = %q: %v, saved %q, want %q", k, in, err, s.String(k), want)
			}
		}
	}

	// Rows written before the check: javascript: and relative values are ignored, a host without a scheme loads
	// as https.
	for k, v := range map[string]string{"tos_url": `"javascript:alert(1)"`, "logo": `"/logo.svg"`, "app_url": `"panel.example.com"`} {
		if err := d.Save(&model.Setting{Key: k, Value: v}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if s.String("tos_url") != "" || s.String("logo") != "" || s.String("app_url") != "https://panel.example.com" {
		t.Fatalf("legacy rows: tos_url %q logo %q app_url %q", s.String("tos_url"), s.String("logo"), s.String("app_url"))
	}
}
