package xboardimport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// source is the read-only Xboard database. Every read after begin runs inside one snapshot transaction
// (docs/IMPORT_XBOARD.md §1.2). A transaction owns a single connection, so callers must never start a second
// source query while iterating rows of another: every query is consumed completely before the next one.
type source struct {
	dsn     SourceDSN
	db      *gorm.DB
	tx      *gorm.DB
	tables  map[string]bool
	columns map[string]map[string]bool
}

func openSource(ctx context.Context, d SourceDSN) (*source, error) {
	var dial gorm.Dialector
	switch d.Driver {
	case DriverMySQL:
		dial = mysql.Open(d.DSN)
	case DriverPostgres:
		dial = postgres.Open(d.DSN)
	case DriverSQLite:
		dial = sqlite.Open(d.DSN)
	default:
		return nil, usageErr("unsupported driver %q", d.Driver)
	}
	db, err := gorm.Open(dial, &gorm.Config{Logger: logger.Discard, SkipDefaultTransaction: true})
	if err != nil {
		return nil, fmt.Errorf("open source %s: %w", d.Display, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("open source %s: %w", d.Display, err)
	}
	sqlDB.SetMaxOpenConns(2)
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("connect to source %s: %w", d.Display, err)
	}
	return &source{dsn: d, db: db, columns: map[string]map[string]bool{}}, nil
}

func (s *source) close() {
	if s.tx != nil {
		s.tx.Rollback()
		s.tx = nil
	}
	if sqlDB, err := s.db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// begin starts the read-only snapshot transaction (REPEATABLE READ on MySQL and PostgreSQL; a plain read
// transaction on SQLite, which serializes anyway).
func (s *source) begin(ctx context.Context) error {
	opts := &sql.TxOptions{ReadOnly: true}
	if s.dsn.Driver != DriverSQLite {
		opts.Isolation = sql.LevelRepeatableRead
	}
	tx := s.db.WithContext(ctx).Begin(opts)
	if tx.Error != nil {
		return fmt.Errorf("source snapshot: %w", tx.Error)
	}
	s.tx = tx
	return nil
}

func (s *source) conn() *gorm.DB {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// loadTables lists the tables of the current database / schema (§1.3).
func (s *source) loadTables(ctx context.Context) error {
	var q string
	switch s.dsn.Driver {
	case DriverMySQL:
		q = "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()"
	case DriverPostgres:
		q = "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()"
	default:
		q = "SELECT name FROM sqlite_master WHERE type = 'table'"
	}
	rows, err := s.conn().WithContext(ctx).Raw(q).Rows()
	if err != nil {
		return fmt.Errorf("list source tables: %w", err)
	}
	defer rows.Close()
	s.tables = map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("list source tables: %w", err)
		}
		s.tables[strings.ToLower(name)] = true
	}
	return rows.Err()
}

func (s *source) has(table string) bool { return s.tables[table] }

// loadColumns probes the column set of every v2_ table (and migrations) with a zero-row SELECT, which is
// portable across the three drivers. Run it before iterating any rows.
func (s *source) loadColumns(ctx context.Context) error {
	for t := range s.tables {
		if !strings.HasPrefix(t, "v2_") && t != "migrations" {
			continue
		}
		cols, err := s.probeColumns(ctx, t)
		if err != nil {
			return fmt.Errorf("probe columns of %s: %w", t, err)
		}
		set := make(map[string]bool, len(cols))
		for _, c := range cols {
			set[strings.ToLower(c)] = true
		}
		s.columns[t] = set
	}
	return nil
}

func (s *source) probeColumns(ctx context.Context, table string) ([]string, error) {
	// Names read from source metadata are identifiers, never SQL expressions.
	quote := `"`
	if s.dsn.Driver == DriverMySQL {
		quote = "`"
	}
	name := quote + strings.ReplaceAll(table, quote, quote+quote) + quote
	rows, err := s.conn().WithContext(ctx).Raw("SELECT * FROM " + name + " WHERE 1 = 0").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	return cols, rows.Err()
}

func (s *source) hasCol(table, col string) bool { return s.columns[table][col] }

// row is one source row keyed by lower-cased column name. Values are whatever the driver returned
// (int64, float64, bool, string, []byte, time.Time or nil): always read them through the flex helpers.
type row map[string]any

func (r row) has(c string) bool { _, ok := r[c]; return ok }

func (r row) isNull(c string) bool { return r[c] == nil }

func (r row) int(c string) (int64, bool) { return flexInt(r[c]) }

func (r row) i64(c string) int64 {
	n, _ := flexInt(r[c])
	return n
}

func (r row) str(c string) string { return str(r[c]) }

func (r row) bool(c string) bool { return flexBool(r[c]) }

func (r row) id() uint64 {
	n, _ := flexInt(r["id"])
	if n < 0 {
		return 0
	}
	return uint64(n)
}

// each streams "SELECT * FROM <table> [WHERE …] ORDER BY id" and calls fn per row. fn must not query the
// source.
func (s *source) each(ctx context.Context, table, where string, args []any, fn func(row) error) error {
	q := "SELECT * FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	q += " ORDER BY id"
	return s.query(ctx, q, args, fn)
}

// query streams an arbitrary SELECT (identifiers in q must not be reserved words: they are never quoted).
func (s *source) query(ctx context.Context, q string, args []any, fn func(row) error) error {
	rows, err := s.conn().WithContext(ctx).Raw(q, args...).Rows()
	if err != nil {
		return fmt.Errorf("source query %q: %w", q, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	for i := range cols {
		cols[i] = strings.ToLower(cols[i])
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		for i := range vals {
			vals[i] = nil
		}
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("source query %q: %w", q, err)
		}
		r := make(row, len(cols))
		for i, c := range cols {
			r[c] = vals[i]
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("source query %q: %w", q, err)
	}
	return nil
}

// countRows returns SELECT COUNT(*) of a table.
func (s *source) countRows(ctx context.Context, table string) (int64, error) {
	var n int64
	if err := s.conn().WithContext(ctx).Raw("SELECT COUNT(*) FROM " + table).Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return n, nil
}

// lastMigration returns the row count and last name of Laravel's migrations table ("" when absent).
func (s *source) lastMigration(ctx context.Context) (int64, string, error) {
	if !s.has("migrations") {
		return 0, "", nil
	}
	n, err := s.countRows(ctx, "migrations")
	if err != nil {
		return 0, "", err
	}
	var name sql.NullString
	err = s.conn().WithContext(ctx).Raw("SELECT migration FROM migrations ORDER BY id DESC LIMIT 1").Row().Scan(&name)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, "", fmt.Errorf("read migrations: %w", err)
	}
	return n, name.String, nil
}

// datetime converts a U datetime column (Laravel writes wall-clock strings in app.timezone) to an instant
// using loc. time.Time values from drivers that parse DATETIME (pgx, glebarez) carry the same wall clock.
func datetime(v any, loc *time.Location) (time.Time, bool) {
	switch x := v.(type) {
	case nil:
		return time.Time{}, false
	case time.Time:
		if x.IsZero() {
			return time.Time{}, false
		}
		return time.Date(x.Year(), x.Month(), x.Day(), x.Hour(), x.Minute(), x.Second(), 0, loc), true
	case int64, int, float64:
		t, _, ok := unixTime(x)
		return t, ok
	}
	s, _ := flexStr(v)
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "0000-00-00") {
		return time.Time{}, false
	}
	for _, layout := range []string{time.DateTime, "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05", time.DateOnly} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, true
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, _, ok := unixTime(s); ok {
		return t, true
	}
	return time.Time{}, false
}
