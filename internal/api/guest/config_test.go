package guest_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/guest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestConfigShape(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := testutil.NewRouterWith(t, a, guest.Register)
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/guest/config", nil, "")
	raw := testutil.Decode[map[string]json.RawMessage](t, rec)
	want := []string{"app_name", "app_description", "app_url", "logo", "tos_url", "currency", "currency_symbol",
		"register_enabled", "email_verify", "email_whitelist_suffix", "invite_force", "captcha", "downloads", "locales"}
	var got []string
	for k := range raw {
		got = append(got, k)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	cfg := testutil.Decode[guest.Config](t, rec)
	if cfg.AppName != "GG Bond" || cfg.Currency != "CNY" || cfg.CurrencySymbol != "¥" || !cfg.RegisterEnabled ||
		cfg.EmailVerify || cfg.InviteForce || cfg.Captcha.Provider != "none" || cfg.Captcha.SiteKey != "" ||
		cfg.EmailWhitelistSuffix == nil || len(cfg.EmailWhitelistSuffix) != 0 || !slices.Equal(cfg.Locales, []string{"zh-CN", "en"}) {
		t.Fatalf("defaults = %+v", cfg)
	}
	var dl map[string]map[string]string
	if err := json.Unmarshal(raw["downloads"], &dl); err != nil || len(dl) != 4 || dl["ios"] == nil || len(dl["windows"]) != 2 {
		t.Fatalf("downloads = %s (%v)", raw["downloads"], err)
	}
}

func TestConfigReflectsSettingsWithoutSecrets(t *testing.T) {
	const secret = "captcha-secret-must-not-leak"
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{
		"app_name": "Example", "app_url": "https://panel.example.com", "tos_url": "https://example.com/tos",
		"currency": "USD", "currency_symbol": "$", "stop_register": true, "email_verify": true, "invite_force": true,
		"email_whitelist_suffix": []string{"example.com"}, "captcha_provider": "turnstile",
		"captcha_site_key": "site-key", "captcha_secret": secret, "macos_download_url": "https://example.com/mac.dmg",
		"macos_version": "1.2.3", "email_password": "smtp-secret", "server_token": "node-secret",
	}))
	r := testutil.NewRouterWith(t, a, guest.Register)
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/guest/config", nil, "")
	body := rec.Body.String()
	for _, s := range []string{secret, "smtp-secret", "node-secret"} {
		if strings.Contains(body, s) {
			t.Fatalf("secret %q leaked: %s", s, body)
		}
	}
	cfg := testutil.Decode[guest.Config](t, rec)
	if cfg.AppName != "Example" || cfg.Currency != "USD" || cfg.RegisterEnabled || !cfg.EmailVerify || !cfg.InviteForce ||
		cfg.Captcha.Provider != "turnstile" || cfg.Captcha.SiteKey != "site-key" || cfg.TosURL != "https://example.com/tos" {
		t.Fatalf("cfg = %+v", cfg)
	}
	// The whitelist is only published when it is enabled.
	if len(cfg.EmailWhitelistSuffix) != 0 {
		t.Fatalf("whitelist published while disabled: %v", cfg.EmailWhitelistSuffix)
	}
	if cfg.Downloads.MacOS.URL != "https://example.com/mac.dmg" || cfg.Downloads.MacOS.Version != "1.2.3" || cfg.Downloads.Windows.URL != "" {
		t.Fatalf("downloads = %+v", cfg.Downloads)
	}
	if err := a.Settings.Set(map[string]any{"email_whitelist_enable": true}); err != nil {
		t.Fatal(err)
	}
	cfg = testutil.Decode[guest.Config](t, testutil.Do(t, r, http.MethodGet, "/api/v1/guest/config", nil, ""))
	if !slices.Equal(cfg.EmailWhitelistSuffix, []string{"example.com"}) {
		t.Fatalf("whitelist = %v", cfg.EmailWhitelistSuffix)
	}
}
