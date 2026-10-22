// Package db opens the GORM database for the configured driver and runs migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/model"
)

// SlowQueryThreshold is the duration above which queries are logged at WARN.
const SlowQueryThreshold = 300 * time.Millisecond

// Open connects to the database described by cfg. SQLite runs with WAL, busy_timeout=5000, foreign_keys=1
// and immediate transactions; every time.Time parameter is normalized to UTC so text comparisons stay
// correct. MySQL DSNs get parseTime=true (and loc=UTC unless set). log may be nil.
func Open(cfg config.DatabaseConfig, log *slog.Logger) (*gorm.DB, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	gcfg := &gorm.Config{
		Logger: logger.NewSlogLogger(log.With("component", "gorm"), logger.Config{
			SlowThreshold:             SlowQueryThreshold,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
		DisableForeignKeyConstraintWhenMigrating: true,
		TranslateError:                           true,
	}

	var (
		dialector gorm.Dialector
		sqlite3   *sql.DB
		memory    bool
	)
	switch cfg.Driver {
	case config.DriverSQLite:
		dsn, mem := SQLiteDSN(cfg.DSN)
		memory = mem
		conn, err := openSQLiteUTC(dsn)
		if err != nil {
			return nil, err
		}
		sqlite3 = conn
		dialector = &sqlite.Dialector{DriverName: sqlite.DriverName, DSN: dsn, Conn: conn}
	case config.DriverPostgres:
		dialector = postgres.Open(cfg.DSN)
	case config.DriverMySQL:
		dsn, err := MySQLDSN(cfg.DSN)
		if err != nil {
			return nil, err
		}
		dialector = mysql.Open(dsn)
	default:
		return nil, fmt.Errorf("db: unsupported driver %q", cfg.Driver)
	}

	gdb, err := gorm.Open(dialector, gcfg)
	if err != nil {
		if sqlite3 != nil {
			_ = sqlite3.Close()
		}
		return nil, fmt.Errorf("db: open %s: %w", cfg.Driver, err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	switch {
	case cfg.Driver == config.DriverSQLite && memory:
		// A private in-memory database lives only as long as its single connection.
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(0)
		sqlDB.SetConnMaxIdleTime(0)
	case cfg.Driver == config.DriverSQLite:
		sqlDB.SetMaxOpenConns(8)
		sqlDB.SetMaxIdleConns(8)
	default:
		sqlDB.SetMaxOpenConns(32)
		sqlDB.SetMaxIdleConns(8)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: ping %s: %w", cfg.Driver, err)
	}
	return gdb, nil
}

// SQLiteDSN adds the default pragmas to a SQLite path/URI unless the caller already set _pragma/_txlock.
// It reports whether the DSN is an in-memory database.
func SQLiteDSN(dsn string) (string, bool) {
	memory := dsn == ":memory:" || strings.Contains(dsn, "mode=memory") || strings.HasPrefix(dsn, "file::memory:")
	path, rawQuery, _ := strings.Cut(dsn, "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		q = url.Values{}
	}
	if len(q["_pragma"]) == 0 {
		q.Add("_pragma", "busy_timeout(5000)")
		q.Add("_pragma", "foreign_keys(1)")
		if !memory {
			q.Add("_pragma", "journal_mode(WAL)")
			q.Add("_pragma", "synchronous(NORMAL)")
		}
	}
	if q.Get("_txlock") == "" {
		q.Set("_txlock", "immediate")
	}
	if path == "" {
		path = ":memory:"
	}
	if path == ":memory:" {
		memory = true
	}
	return path + "?" + q.Encode(), memory
}

// MySQLDSN forces parseTime=true and loc=UTC and defaults charset to utf8mb4. loc is forced (not defaulted):
// date columns hold model.Day values (midnight UTC), and any other loc shifts them to another day or turns
// record_at = ? lookups into mismatches (the driver always sets a Loc, so a nil check never applied).
func MySQLDSN(dsn string) (string, error) {
	c, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("db: invalid mysql dsn: %w", err)
	}
	c.ParseTime = true
	c.Loc = time.UTC
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	if _, ok := c.Params["charset"]; !ok {
		c.Params["charset"] = "utf8mb4"
	}
	return c.FormatDSN(), nil
}

// Migrate auto-migrates every model in model.All(). It is idempotent and safe to run on every boot.
func Migrate(db *gorm.DB) error {
	tx := db
	if db.Dialector.Name() == config.DriverMySQL {
		tx = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci")
	}
	if err := tx.AutoMigrate(model.All()...); err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}
	return nil
}

// Close closes the underlying *sql.DB.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
