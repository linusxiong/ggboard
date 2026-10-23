package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/linusxiong/ggboard/internal/logx"
)

// testSecret is an obviously fake but acceptable jwt.secret (32+ bytes, varied characters).
const testSecret = "test-jwt-secret-0123456789abcdef-example"

func load(t *testing.T, opts Options) *Config {
	t.Helper()
	opts.Logger = logx.Discard()
	cfg, err := Load(opts)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestLoadDefaultsAndGeneratedSecret(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join(t.TempDir(), "data")
	t.Setenv("GGBOARD_DATA_DIR", dir)

	cfg := load(t, Options{})
	if cfg.Server.Addr != ":8080" || cfg.Database.Driver != DriverSQLite || cfg.JWT.TTL != 168*time.Hour {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.Database.DSN != filepath.Join(dir, "ggboard.db") {
		t.Fatalf("DSN = %q", cfg.Database.DSN)
	}
	if cfg.Log.Level != "info" || cfg.Log.Format != "text" {
		t.Fatalf("log = %+v", cfg.Log)
	}
	if len(cfg.JWT.Secret) < 32 {
		t.Fatalf("secret not generated: %q", cfg.JWT.Secret)
	}
	st, err := os.Stat(cfg.JWTSecretFile())
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("secret file mode = %v", st.Mode().Perm())
	}
	again := load(t, Options{})
	if again.JWT.Secret != cfg.JWT.Secret {
		t.Fatal("secret must be stable across loads")
	}
}

func TestEnvOverride(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GGBOARD_DATA_DIR", t.TempDir())
	t.Setenv("GGBOARD_SERVER_ADDR", "127.0.0.1:9999")
	t.Setenv("GGBOARD_SERVER_TRUSTED_PROXIES", "127.0.0.1, 10.0.0.0/8")
	t.Setenv("GGBOARD_SERVER_CORS_ORIGINS", "https://a.example.com")
	t.Setenv("GGBOARD_JWT_SECRET", testSecret)
	t.Setenv("GGBOARD_JWT_TTL", "2h")
	t.Setenv("GGBOARD_LOG_LEVEL", "DEBUG")
	t.Setenv("GGBOARD_LOG_FORMAT", "json")
	t.Setenv("GGBOARD_DATABASE_DRIVER", "postgres")
	t.Setenv("GGBOARD_DATABASE_DSN", "postgres://u:p@db.example.com/ggboard")

	cfg := load(t, Options{})
	if cfg.Server.Addr != "127.0.0.1:9999" {
		t.Errorf("addr = %q", cfg.Server.Addr)
	}
	if got := strings.Join(cfg.Server.TrustedProxies, "|"); got != "127.0.0.1|10.0.0.0/8" {
		t.Errorf("trusted proxies = %q", got)
	}
	if len(cfg.Server.CORSOrigins) != 1 || cfg.Server.CORSOrigins[0] != "https://a.example.com" {
		t.Errorf("cors = %v", cfg.Server.CORSOrigins)
	}
	if cfg.JWT.TTL != 2*time.Hour || cfg.JWT.Secret != testSecret {
		t.Errorf("jwt = %+v", cfg.JWT)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Format != "json" {
		t.Errorf("log = %+v", cfg.Log)
	}
	if cfg.Database.Driver != DriverPostgres || cfg.Database.DSN != "postgres://u:p@db.example.com/ggboard" {
		t.Errorf("db = %+v", cfg.Database)
	}
	if _, err := os.Stat(cfg.JWTSecretFile()); !os.IsNotExist(err) {
		t.Error("configured secret must not be persisted")
	}
}

func TestPrecedenceFileEnvFlag(t *testing.T) {
	t.Chdir(t.TempDir())
	dataDir := t.TempDir()
	file := filepath.Join(t.TempDir(), "ggboard.yaml")
	yaml := "server:\n  addr: \":7000\"\nlog:\n  level: warn\ndata_dir: " + dataDir + "\njwt:\n  secret: " + testSecret + "\n"
	if err := os.WriteFile(file, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := load(t, Options{File: file})
	if cfg.Server.Addr != ":7000" || cfg.Log.Level != "warn" || cfg.DataDir != dataDir {
		t.Fatalf("file values not applied: %+v", cfg)
	}

	t.Setenv("GGBOARD_SERVER_ADDR", ":7001")
	if cfg = load(t, Options{File: file}); cfg.Server.Addr != ":7001" {
		t.Fatalf("env must override file, got %q", cfg.Server.Addr)
	}

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.String("addr", "", "")
	fs.String("log-level", "", "")
	if err := fs.Parse([]string{"--addr", ":7002"}); err != nil {
		t.Fatal(err)
	}
	cfg = load(t, Options{File: file, Flags: fs})
	if cfg.Server.Addr != ":7002" {
		t.Fatalf("flag must override env, got %q", cfg.Server.Addr)
	}
	if cfg.Log.Level != "warn" {
		t.Fatalf("unset flag must not override, got %q", cfg.Log.Level)
	}
}

func TestExplicitFileMustExist(t *testing.T) {
	if _, err := Load(Options{File: filepath.Join(t.TempDir(), "missing.yaml"), Logger: logx.Discard()}); err == nil {
		t.Fatal("expected error for missing explicit config file")
	}
}

func TestValidate(t *testing.T) {
	base := func() Config {
		return Config{
			Server:   ServerConfig{Addr: ":8080"},
			Database: DatabaseConfig{Driver: DriverSQLite, DSN: "x.db"},
			JWT:      JWTConfig{TTL: time.Hour},
			Log:      LogConfig{Level: "info", Format: "text"},
		}
	}
	cases := map[string]func(*Config){
		"driver":  func(c *Config) { c.Database.Driver = "oracle" },
		"dsn":     func(c *Config) { c.Database.Driver, c.Database.DSN = DriverMySQL, "" },
		"level":   func(c *Config) { c.Log.Level = "loud" },
		"format":  func(c *Config) { c.Log.Format = "xml" },
		"ttl":     func(c *Config) { c.JWT.TTL = 0 },
		"addr":    func(c *Config) { c.Server.Addr = "" },
		"proxies": func(c *Config) { c.Server.TrustedProxies = []string{"not-an-ip"} },
	}
	ok := base()
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := base()
			mutate(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// Security audit 2026-10-22 §5: the public .env.example placeholder (33 characters) was accepted as the HS256 key,
// so anyone who read the repository could mint an admin token. Placeholders are now refused, case-insensitively
// and ignoring surrounding spaces, and the error says how to fix it.
func TestJWTSecretPlaceholderRejected(t *testing.T) {
	for _, secret := range []string{
		"change-me-to-a-long-random-string",
		"  CHANGE-ME-to-a-long-random-string\n",
		"change-me-to-a-long-random-string-0123456789",
		"changeme", "Secret", "password",
	} {
		t.Run(strings.TrimSpace(secret), func(t *testing.T) {
			t.Chdir(t.TempDir())
			dir := t.TempDir()
			t.Setenv("GGBOARD_DATA_DIR", dir)
			t.Setenv("GGBOARD_JWT_SECRET", secret)
			_, err := Load(Options{Logger: logx.Discard()})
			if err == nil {
				t.Fatal("placeholder accepted as jwt.secret")
			}
			for _, hint := range []string{"openssl rand -hex 32", filepath.Join(dir, "jwt.secret")} {
				if !strings.Contains(err.Error(), hint) {
					t.Errorf("error %q does not mention %q", err, hint)
				}
			}
		})
	}
}

// Secrets shorter than 32 bytes, or with fewer than 8 distinct characters, are refused (configured or persisted);
// 32 varied bytes are enough.
func TestJWTSecretTooWeakRejected(t *testing.T) {
	for name, secret := range map[string]string{
		"short":        "0123456789abcdef0123456789abcde", // 31 bytes
		"short-padded": "   0123456789abcdef   ",
		"repeated":     strings.Repeat("s", 40),
		"low-variety":  strings.Repeat("ab12", 10),
	} {
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("GGBOARD_DATA_DIR", t.TempDir())
			t.Setenv("GGBOARD_JWT_SECRET", secret)
			if _, err := Load(Options{Logger: logx.Discard()}); err == nil {
				t.Fatalf("weak jwt.secret %q accepted", secret)
			}
		})
	}

	t.Run("persisted file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		dir := t.TempDir()
		t.Setenv("GGBOARD_DATA_DIR", dir)
		if err := os.WriteFile(filepath.Join(dir, "jwt.secret"), []byte("change-me-to-a-long-random-string\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(Options{Logger: logx.Discard()}); err == nil {
			t.Fatal("placeholder in <data_dir>/jwt.secret accepted")
		}
	})

	t.Run("minimum accepted", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv("GGBOARD_DATA_DIR", t.TempDir())
		t.Setenv("GGBOARD_JWT_SECRET", "0123456789abcdef0123456789abcdef")
		if cfg := load(t, Options{}); len(cfg.JWT.Secret) != MinJWTSecretLen {
			t.Fatalf("secret = %q", cfg.JWT.Secret)
		}
	})
}
