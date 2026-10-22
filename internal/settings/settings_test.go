package settings

import (
	"errors"
	"path/filepath"
	"regexp"
	"testing"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/db"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	d, err := db.Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "s.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(d) })
	if err := db.Migrate(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDefaultsAndServerToken(t *testing.T) {
	d := newDB(t)
	s, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	if s.String("app_name") != "GG Bond" || s.String("currency") != "CNY" || s.Int("register_limit_count") != 3 ||
		!s.Bool("password_limit_enable") || s.Int("email_port") != 465 || s.String("captcha_provider") != "none" {
		t.Fatal("unexpected defaults")
	}
	if m := s.Strings("commission_withdraw_method"); len(m) != 2 || m[0] != "USDT" {
		t.Fatalf("withdraw methods = %v", m)
	}
	tok := s.String("server_token")
	if !regexp.MustCompile(`^[A-Za-z0-9]{32}$`).MatchString(tok) {
		t.Fatalf("server_token = %q", tok)
	}
	s2, err := New(d)
	if err != nil || s2.String("server_token") != tok {
		t.Fatal("server_token must be generated only once")
	}
	for _, k := range []string{"app_name", "show_info_to_user", "email_whitelist_suffix", "captcha_secret", "subscribe_url",
		"allow_user_rule_profile", "order_timeout_minutes", "commission_distribution_l3", "server_white_list",
		"device_limit_mode", "remind_mail_enable", "frontend_footer_html", "ios_version", "rulesets_custom"} {
		if _, ok := Defaults[k]; !ok {
			t.Errorf("missing default for %s", k)
		}
	}
}

func TestSetCoercionValidationAndReset(t *testing.T) {
	s, err := New(newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	err = s.Set(map[string]any{
		"app_name":                   "Example",
		"register_limit_count":       float64(7),
		"stop_register":              "true",
		"email_whitelist_suffix":     []any{"example.com", " ", "example.org"},
		"subscribe_url":              "https://a.example.com,\nhttps://b.example.com",
		"rulesets_custom":            map[string]any{"sources": map[string]any{}},
		"commission_withdraw_method": []string{"PayPal"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.String("app_name") != "Example" || s.Int("register_limit_count") != 7 || !s.Bool("stop_register") {
		t.Fatal("values not applied")
	}
	if v := s.Strings("email_whitelist_suffix"); len(v) != 2 || v[1] != "example.org" {
		t.Fatalf("suffix = %v", v)
	}
	if v := s.Strings("subscribe_url"); len(v) != 2 {
		t.Fatalf("subscribe_url = %v", v)
	}
	var custom map[string]any
	if err := s.JSON("rulesets_custom", &custom); err != nil || custom["sources"] == nil {
		t.Fatalf("json setting = %v %v", custom, err)
	}

	// Values survive a reload from the database.
	if err := s.Reload(t.Context()); err != nil || s.Int("register_limit_count") != 7 {
		t.Fatal("reload lost values")
	}

	err = s.Set(map[string]any{
		"unknown_key":          1,
		"register_limit_count": 1.5,
		"captcha_provider":     "mystery",
		"reset_traffic_method": 9,
		"stop_register":        "maybe",
		"app_name":             42,
		"server_white_list":    []any{1},
	})
	var fe FieldErrors
	if !errors.As(err, &fe) {
		t.Fatalf("expected FieldErrors, got %v", err)
	}
	for _, k := range []string{"unknown_key", "register_limit_count", "captcha_provider", "reset_traffic_method", "stop_register", "app_name", "server_white_list"} {
		if len(fe[k]) == 0 {
			t.Errorf("missing error for %s", k)
		}
	}
	if s.String("app_name") != "Example" {
		t.Fatal("invalid batch must not write anything")
	}

	if err := s.Set(map[string]any{"app_name": nil}); err != nil || s.String("app_name") != "GG Bond" {
		t.Fatal("nil must reset to default")
	}
	if s.Duration("server_push_interval", 1e9).Seconds() != 60 {
		t.Fatal("Duration")
	}
}

func TestGroupedAndSecrets(t *testing.T) {
	s, err := New(newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	g := s.AllGrouped()
	if g["site"]["app_name"] != "GG Bond" || g["server"]["server_token"] == "" || g["email"]["email_password"] != "" {
		t.Fatalf("grouped = %v", g["server"])
	}
	if _, hidden := g["subscribe"]["rulesets_custom"]; hidden {
		t.Fatal("hidden keys must not be grouped")
	}
	for _, k := range []string{"captcha_secret", "email_password", "server_token"} {
		if !IsSecret(k) {
			t.Errorf("%s must be secret", k)
		}
	}
	if IsSecret("app_name") {
		t.Fatal("app_name is not secret")
	}
	groups := Groups()
	if len(groups) != 10 || groups[0].Name != "site" || groups[8].Name != "app" || groups[9].Name != "security" {
		t.Fatalf("groups = %d", len(groups))
	}
	total := 0
	for _, grp := range groups {
		total += len(grp.Keys)
	}
	if total != len(SortedKeys())-1 { // rulesets_custom is hidden
		t.Fatalf("group keys = %d, all = %d", total, len(SortedKeys()))
	}
	if d, ok := Lookup("reset_traffic_method"); !ok || *d.Max != 4 {
		t.Fatal("Lookup")
	}
}
