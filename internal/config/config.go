// Package config loads the process configuration from (in increasing precedence) built-in defaults,
// an optional YAML file, GGBOARD_* environment variables and command-line flags.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// EnvPrefix is the prefix of every environment variable read by Load (dots become underscores:
// server.addr -> GGBOARD_SERVER_ADDR).
const EnvPrefix = "GGBOARD"

// Supported database drivers.
const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
	DriverMySQL    = "mysql"
)

// Config is the static process configuration. Runtime-editable options live in the settings table instead.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Log      LogConfig      `mapstructure:"log"`
	Frontend FrontendConfig `mapstructure:"frontend"`
	// DataDir holds the SQLite database, the generated JWT secret and other local state.
	DataDir string `mapstructure:"data_dir"`
}

// ServerConfig configures the HTTP listener.
type ServerConfig struct {
	Addr string `mapstructure:"addr"`
	// TrustedProxies are CIDRs/IPs whose X-Forwarded-For / X-Real-IP headers are trusted. Empty = trust none.
	TrustedProxies []string `mapstructure:"trusted_proxies"`
	// CORSOrigins widens CORS beyond same-origin ("*" allows any origin). Empty = same-origin only.
	CORSOrigins []string `mapstructure:"cors_origins"`
}

// DatabaseConfig selects the database driver and DSN.
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"` // sqlite | postgres | mysql
	DSN    string `mapstructure:"dsn"`
}

// JWTConfig configures user session tokens.
type JWTConfig struct {
	Secret string        `mapstructure:"secret"`
	TTL    time.Duration `mapstructure:"ttl"`
}

// LogConfig configures the slog logger.
type LogConfig struct {
	Level  string `mapstructure:"level"`  // debug | info | warn | error
	Format string `mapstructure:"format"` // text | json
}

// FrontendConfig optionally overrides the embedded SPA with a directory on disk.
type FrontendConfig struct {
	Dir string `mapstructure:"dir"`
}

// Options controls where Load reads configuration from.
type Options struct {
	// File is an explicit config file path; it must exist. When empty, GGBOARD_CONFIG is consulted and then
	// ./config.yaml is read if present.
	File string
	// Flags, when non-nil, are bound to config keys by name (see FlagKeys). Only flags that are set override.
	Flags *pflag.FlagSet
	// Logger receives warnings (e.g. a generated JWT secret). Defaults to slog.Default().
	Logger *slog.Logger
}

// FlagKeys maps command-line flag names to config keys.
var FlagKeys = map[string]string{
	"addr":       "server.addr",
	"data-dir":   "data_dir",
	"log-level":  "log.level",
	"log-format": "log.format",
	"db-driver":  "database.driver",
	"db-dsn":     "database.dsn",
}

func defaults(v *viper.Viper) {
	v.SetDefault("server.addr", ":8080")
	v.SetDefault("server.trusted_proxies", []string{})
	v.SetDefault("server.cors_origins", []string{})
	v.SetDefault("database.driver", DriverSQLite)
	v.SetDefault("database.dsn", "")
	v.SetDefault("jwt.secret", "")
	v.SetDefault("jwt.ttl", "168h")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "text")
	v.SetDefault("frontend.dir", "")
	v.SetDefault("data_dir", "./data")
}

// Load builds the configuration, creates DataDir, fills derived defaults (SQLite DSN) and makes sure a JWT
// secret exists (generated once and persisted to <DataDir>/jwt.secret when not configured).
func Load(opts Options) (*Config, error) {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	v := viper.New()
	defaults(v)
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	file := opts.File
	if file == "" {
		file = os.Getenv(EnvPrefix + "_CONFIG")
	}
	switch {
	case file != "":
		v.SetConfigFile(file)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("config: read %s: %w", file, err)
		}
	default:
		if _, err := os.Stat("config.yaml"); err == nil {
			v.SetConfigFile("config.yaml")
			if err := v.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("config: read config.yaml: %w", err)
			}
		}
	}

	if opts.Flags != nil {
		for name, key := range FlagKeys {
			if f := opts.Flags.Lookup(name); f != nil && f.Changed {
				if err := v.BindPFlag(key, f); err != nil {
					return nil, fmt.Errorf("config: bind flag %s: %w", name, err)
				}
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: decode: %w", err)
	}
	if err := cfg.finalize(log); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) finalize(log *slog.Logger) error {
	c.Database.Driver = strings.ToLower(strings.TrimSpace(c.Database.Driver))
	c.Log.Level = strings.ToLower(strings.TrimSpace(c.Log.Level))
	c.Log.Format = strings.ToLower(strings.TrimSpace(c.Log.Format))
	c.Server.TrustedProxies = cleanList(c.Server.TrustedProxies)
	c.Server.CORSOrigins = cleanList(c.Server.CORSOrigins)
	if c.DataDir == "" {
		c.DataDir = "./data"
	}
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return fmt.Errorf("config: create data_dir: %w", err)
	}
	if c.Database.Driver == DriverSQLite && c.Database.DSN == "" {
		c.Database.DSN = filepath.Join(c.DataDir, "ggboard.db")
	}
	if err := c.Validate(); err != nil {
		return err
	}
	return c.ensureJWTSecret(log)
}

// Validate checks enumerations and required values.
func (c *Config) Validate() error {
	var errs []error
	switch c.Database.Driver {
	case DriverSQLite, DriverPostgres, DriverMySQL:
	default:
		errs = append(errs, fmt.Errorf("database.driver %q must be one of sqlite, postgres, mysql", c.Database.Driver))
	}
	if c.Database.DSN == "" {
		errs = append(errs, errors.New("database.dsn is required for postgres/mysql"))
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "warning", "error":
	default:
		errs = append(errs, fmt.Errorf("log.level %q must be one of debug, info, warn, error", c.Log.Level))
	}
	switch c.Log.Format {
	case "text", "json":
	default:
		errs = append(errs, fmt.Errorf("log.format %q must be text or json", c.Log.Format))
	}
	if c.JWT.TTL <= 0 {
		errs = append(errs, errors.New("jwt.ttl must be positive"))
	}
	if c.Server.Addr == "" {
		errs = append(errs, errors.New("server.addr is required"))
	}
	for _, p := range c.Server.TrustedProxies {
		if net.ParseIP(p) == nil {
			if _, _, err := net.ParseCIDR(p); err != nil {
				errs = append(errs, fmt.Errorf("server.trusted_proxies: %q is not an IP or CIDR", p))
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}

// JWTSecretFile is the path of the persisted, generated JWT secret.
func (c *Config) JWTSecretFile() string { return filepath.Join(c.DataDir, "jwt.secret") }

func (c *Config) ensureJWTSecret(log *slog.Logger) error {
	if c.JWT.Secret != "" {
		if len(c.JWT.Secret) < 32 {
			log.Warn("jwt.secret is shorter than 32 characters; use a long random value")
		}
		return nil
	}
	path := c.JWTSecretFile()
	if b, err := os.ReadFile(path); err == nil { //nolint:gosec // path is <data_dir>/jwt.secret from trusted config
		if s := strings.TrimSpace(string(b)); s != "" {
			c.JWT.Secret = s
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config: read %s: %w", path, err)
	}
	buf := make([]byte, 48)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("config: generate jwt secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(buf)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return fmt.Errorf("config: persist jwt secret: %w", err)
	}
	c.JWT.Secret = secret
	log.Warn("jwt.secret not configured; generated a random secret", "file", path)
	return nil
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		for _, part := range strings.Split(s, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
