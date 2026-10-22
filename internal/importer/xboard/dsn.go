package xboardimport

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// Source drivers.
const (
	DriverMySQL    = "mysql"
	DriverPostgres = "postgres"
	DriverSQLite   = "sqlite"
)

// ErrUsage marks invalid command-line input (bad --dsn / --driver / --source-tz).
var ErrUsage = errors.New("usage")

func usageErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUsage, fmt.Sprintf(format, args...))
}

// SourceDSN is a parsed --dsn: the driver, the DSN handed to the Go driver and a display string that never
// contains the password.
type SourceDSN struct {
	Driver  string
	DSN     string
	Display string
}

// ParseDSN parses --dsn (docs/IMPORT_XBOARD.md §1.1). Accepted forms:
//
//	mysql://user:pass@host:3306/db?params          URL form
//	mysql://user:pass@tcp(host:3306)/db?params     go-sql-driver form
//	postgres://… / postgresql://…                  passed to pgx unchanged
//	sqlite:///abs/path/database.sqlite             (sqlite://relative/path is relative to the working dir)
//
// With driver set (--driver mysql|postgres|sqlite) dsn may be a raw driver DSN without a scheme.
func ParseDSN(dsn, driver string) (SourceDSN, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return SourceDSN{}, usageErr("--dsn is required")
	}
	scheme, rest, hasScheme := strings.Cut(dsn, "://")
	scheme = strings.ToLower(scheme)
	if !hasScheme || !isKnownScheme(scheme) {
		scheme, rest = "", dsn
	}
	driver = strings.ToLower(strings.TrimSpace(driver))
	switch driver {
	case "":
		if scheme == "" {
			return SourceDSN{}, usageErr("--dsn must start with mysql://, postgres:// or sqlite:// (or pass --driver)")
		}
		driver = schemeDriver(scheme)
	case "postgresql":
		driver = DriverPostgres
	case DriverMySQL, DriverPostgres, DriverSQLite:
		if scheme != "" && schemeDriver(scheme) != driver {
			return SourceDSN{}, usageErr("--driver %s does not match the %s:// DSN", driver, scheme)
		}
	default:
		return SourceDSN{}, usageErr("unsupported --driver %q (mysql, postgres or sqlite)", driver)
	}
	switch driver {
	case DriverMySQL:
		return mysqlDSN(rest)
	case DriverPostgres:
		if scheme == "" {
			return SourceDSN{Driver: DriverPostgres, DSN: rest, Display: "postgres"}, nil
		}
		return postgresDSN(dsn)
	default:
		return sqliteDSN(rest)
	}
}

func isKnownScheme(s string) bool {
	switch s {
	case "mysql", "postgres", "postgresql", "sqlite", "sqlite3":
		return true
	}
	return false
}

func schemeDriver(scheme string) string {
	switch scheme {
	case "mysql":
		return DriverMySQL
	case "postgres", "postgresql":
		return DriverPostgres
	}
	return DriverSQLite
}

// mysqlDSN accepts the go-sql-driver form or a URL form and forces parseTime=false, charset utf8mb4,
// interpolateParams=true and loc=UTC (the session time zone is never touched, §1.2).
func mysqlDSN(rest string) (SourceDSN, error) {
	var cfg *mysqldriver.Config
	if strings.Contains(rest, "(") || strings.HasPrefix(rest, "/") {
		c, err := mysqldriver.ParseDSN(rest)
		if err != nil {
			return SourceDSN{}, usageErr("invalid mysql DSN: %v", err)
		}
		cfg = c
	} else {
		u, err := url.Parse("mysql://" + rest)
		if err != nil {
			return SourceDSN{}, usageErr("invalid mysql DSN: %v", redactURLError(err))
		}
		cfg = mysqldriver.NewConfig()
		cfg.Net = "tcp"
		cfg.Addr = u.Host
		if u.Port() == "" && u.Host != "" {
			cfg.Addr = net.JoinHostPort(u.Hostname(), "3306")
		}
		if u.User != nil {
			cfg.User = u.User.Username()
			cfg.Passwd, _ = u.User.Password()
		}
		cfg.DBName = strings.TrimPrefix(u.Path, "/")
		q := u.Query()
		if len(q) > 0 {
			// Re-parse the query through the driver so every documented parameter is honoured.
			base, sep := cfg.FormatDSN(), "?"
			if strings.Contains(base, "?") {
				sep = "&"
			}
			c, err := mysqldriver.ParseDSN(base + sep + q.Encode())
			if err != nil {
				return SourceDSN{}, usageErr("invalid mysql DSN parameters: %v", err)
			}
			cfg = c
		}
	}
	if cfg.DBName == "" {
		return SourceDSN{}, usageErr("mysql DSN has no database name")
	}
	cfg.ParseTime = false
	cfg.InterpolateParams = true
	cfg.Loc = time.UTC
	if err := cfg.Apply(mysqldriver.Charset("utf8mb4", "")); err != nil {
		return SourceDSN{}, usageErr("invalid mysql DSN: %v", err)
	}
	if cfg.Params != nil {
		delete(cfg.Params, "time_zone")
		delete(cfg.Params, "loc")
	}
	host := cfg.Addr
	if cfg.Net == "unix" {
		host = "unix:" + cfg.Addr
	}
	return SourceDSN{Driver: DriverMySQL, DSN: cfg.FormatDSN(), Display: fmt.Sprintf("mysql %s/%s", host, cfg.DBName)}, nil
}

func postgresDSN(raw string) (SourceDSN, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return SourceDSN{}, usageErr("invalid postgres DSN: %v", redactURLError(err))
	}
	db := strings.TrimPrefix(u.Path, "/")
	return SourceDSN{Driver: DriverPostgres, DSN: raw, Display: fmt.Sprintf("postgres %s/%s", u.Host, db)}, nil
}

// sqliteDSN opens the file read-only: file:<path>?mode=ro&_pragma=busy_timeout(5000).
func sqliteDSN(path string) (SourceDSN, error) {
	path = strings.TrimPrefix(path, "file:")
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if path == "" {
		return SourceDSN{}, usageErr("sqlite DSN has no file path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return SourceDSN{}, usageErr("sqlite path: %v", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	dsn := "file:" + u.EscapedPath() + "?mode=ro&_pragma=busy_timeout(5000)"
	return SourceDSN{Driver: DriverSQLite, DSN: dsn, Display: "sqlite " + abs}, nil
}

// redactURLError drops the URL (which may contain a password) from a url.Parse error.
func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
