package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/model"
)

// Copy (`ggboard db copy`, docs/DEPLOY.md §5) moves every row of a ggboard database into another one, e.g. from
// SQLite to PostgreSQL and back. Rows are read and written as plain column values (no GORM hooks, defaults,
// autoCreateTime/autoUpdateTime or serializers), so JSON text, NULLs, zero values and primary keys arrive exactly as
// stored. The only conversions are the ones the target column type forces: timestamps are truncated to the target's
// precision (PostgreSQL microseconds, MySQL milliseconds) and date columns keep their calendar day.

// Copy errors; test with errors.Is.
var (
	ErrSameDatabase   = errors.New("source and target are the same database")
	ErrTargetNotEmpty = errors.New("the target database is not empty")
	ErrUnknownTable   = errors.New("unknown table")
	ErrUnknownColumn  = errors.New("unknown column")
	ErrVerify         = errors.New("verification failed")
	errCopyDryRun     = errors.New("db copy: dry run, rolled back")
)

// DefaultCopyBatch is the number of rows per SELECT and INSERT of Copy (docs/BACKEND.md §9).
const DefaultCopyBatch = 500

// maxCopyParams caps the bind parameters of one INSERT, well below SQLite's 32766 and the 65535 of PostgreSQL and MySQL.
const maxCopyParams = 30000

// maxCopyProblems is the number of differences reported per table when verification fails.
const maxCopyProblems = 5

// CopyOptions configures Copy.
type CopyOptions struct {
	// Overwrite deletes every row of the target tables first, in the transaction of the copy. Without it a target
	// holding any row is refused with ErrTargetNotEmpty.
	Overwrite bool
	// DryRun copies and verifies everything, then rolls the target transaction back. PostgreSQL sequences are left
	// alone (setval is not transactional); the schema created by Migrate stays.
	DryRun bool
	// BatchSize is the number of rows per SELECT and INSERT (0 = DefaultCopyBatch; capped so one INSERT stays below
	// every driver's bind-parameter limit).
	BatchSize int
	// Log receives one INFO record per finished copy (nil = discard).
	Log *slog.Logger
}

// CopyReport is the outcome of a successful Copy.
type CopyReport struct {
	Source, Target string // dialect names
	Overwrite      bool
	DryRun         bool
	Tables         []TableCopy // in copy order (model.All())
	// Sequences is the number of PostgreSQL serial/identity sequences moved past the copied ids (always 0 for other
	// targets, whose counters follow explicit ids, and in dry runs).
	Sequences int
	// TimePrecision is the coarsest timestamp precision of source and target: finer digits were dropped and the
	// verification compares timestamps at this precision (1ns when both sides keep nanoseconds, i.e. SQLite).
	TimePrecision time.Duration
	Took          time.Duration
}

// TableCopy is one table of a CopyReport.
type TableCopy struct {
	Name    string
	Rows    int64 // rows copied; verification proved source and target hold exactly these rows
	Cleared int64 // target rows deleted first (Overwrite)
	// Digest is the hex SHA-256 over the canonical form of every row in primary-key order. It was computed on both
	// sides from what each database returned, and they matched.
	Digest string
}

// TotalRows is the sum of the copied rows.
func (r *CopyReport) TotalRows() int64 {
	var n int64
	for _, t := range r.Tables {
		n += t.Rows
	}
	return n
}

// Copy copies every table of model.All() from src to dst and proves the copy (row counts plus a per-row hash of every
// column, read back from both sides). src must already be migrated (the App migrates it on init); dst is migrated
// here with Migrate, so its schema is exactly what `ggboard serve` creates.
//
// Refused before anything is written: the same database on both sides (ErrSameDatabase); a table or column on either
// side that the ggboard schema does not have (ErrUnknownTable / ErrUnknownColumn: its rows would be left behind or the
// database belongs to something else); a target holding rows without opts.Overwrite (ErrTargetNotEmpty).
//
// The source is read in one read-only transaction (a consistent snapshot) and the target is written in one
// transaction: emptying (Overwrite), the inserts in batches, the PostgreSQL sequence reset and the verification. Any
// error, including a verification mismatch (ErrVerify), rolls the target back. The schema has no foreign-key
// constraints (Open disables them), so table order does not matter; model.All() is dependency-friendly anyway. The
// server must be stopped: rows written to the source during the copy are not copied, and traffic is buffered in
// memory until a graceful shutdown.
func Copy(ctx context.Context, src, dst *gorm.DB, opts CopyOptions) (*CopyReport, error) {
	start := time.Now()
	if opts.BatchSize <= 0 {
		opts.BatchSize = DefaultCopyBatch
	}
	log := opts.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	src, dst = src.WithContext(ctx), dst.WithContext(ctx)
	sd, dd := src.Dialector.Name(), dst.Dialector.Name()
	for _, d := range []string{sd, dd} {
		if d != config.DriverSQLite && d != config.DriverPostgres && d != config.DriverMySQL {
			return nil, fmt.Errorf("db copy: unsupported driver %q", d)
		}
	}
	same, err := sameDatabase(ctx, src, dst)
	if err != nil {
		return nil, err
	}
	if same {
		return nil, fmt.Errorf("db copy: %w; point the target at another database", ErrSameDatabase)
	}
	tables, err := copyPlan(src, sd, dd)
	if err != nil {
		return nil, err
	}

	// Both sides must hold nothing but ggboard tables. The target is checked before Migrate creates anything in it.
	if err := checkTables(src, "source", tables, true); err != nil {
		return nil, err
	}
	if err := checkTables(dst, "target", tables, false); err != nil {
		return nil, err
	}
	for _, t := range tables {
		if err := checkColumns(ctx, src, "source", t); err != nil {
			return nil, err
		}
	}
	if err := Migrate(dst); err != nil {
		return nil, fmt.Errorf("db copy: migrate the target: %w", err)
	}
	for _, t := range tables {
		if err := checkColumns(ctx, dst, "target", t); err != nil {
			return nil, err
		}
	}

	rep := &CopyReport{Source: sd, Target: dd, Overwrite: opts.Overwrite, DryRun: opts.DryRun, TimePrecision: time.Nanosecond}
	for _, t := range tables {
		for _, c := range t.cols {
			if c.kind == kindTime {
				rep.TimePrecision = max(rep.TimePrecision, c.cmp)
			}
		}
	}
	err = dst.Transaction(func(dtx *gorm.DB) error {
		return src.Transaction(func(stx *gorm.DB) error {
			c := &copier{ctx: ctx, src: stx, dst: dtx, sd: sd, dd: dd, tables: tables, opts: opts, rep: rep}
			if err := c.run(); err != nil {
				return err
			}
			if opts.DryRun {
				return errCopyDryRun
			}
			return nil
		}, sourceTxOptions(sd))
	})
	if err != nil && !errors.Is(err, errCopyDryRun) {
		return nil, err
	}
	rep.Took = time.Since(start)
	log.Info("db copy", "source", sd, "target", dd, "tables", len(rep.Tables), "rows", rep.TotalRows(),
		"sequences", rep.Sequences, "overwrite", opts.Overwrite, "dry_run", opts.DryRun, "took", rep.Took.String())
	return rep, nil
}

// sourceTxOptions is a read-only snapshot. SQLite ignores the options: its BEGIN IMMEDIATE (Open's _txlock) takes
// the write lock, so a server that is still running cannot change the source until the copy ends.
func sourceTxOptions(dialect string) *sql.TxOptions {
	if dialect == config.DriverSQLite {
		return nil
	}
	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
}

// colKind is how a column is scanned, written, compared and hashed on every driver.
type colKind uint8

const (
	kindBool colKind = iota + 1
	kindInt
	kindFloat
	kindString
	kindBytes
	kindTime
	kindDate
)

type copyColumn struct {
	name string
	kind colKind
	// size is the character limit of a varchar column (0 = unlimited), checked before writing to PostgreSQL / MySQL,
	// which reject longer values (SQLite does not enforce it).
	size int
	// dstPrec is the precision the target stores timestamps with; cmp is the coarser of source and target, the
	// precision verification compares at.
	dstPrec, cmp time.Duration
}

type copyTable struct {
	name string
	cols []copyColumn
	pk   int // index of the primary-key column in cols
}

func (t *copyTable) pkCol() copyColumn { return t.cols[t.pk] }

// copyPlan describes every table of model.All() from its GORM schema.
func copyPlan(db *gorm.DB, sd, dd string) ([]*copyTable, error) {
	var out []*copyTable
	for _, m := range model.All() {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(m); err != nil {
			return nil, fmt.Errorf("db copy: parse %T: %w", m, err)
		}
		s := stmt.Schema
		if len(s.PrimaryFields) != 1 {
			return nil, fmt.Errorf("db copy: table %s has %d primary-key columns; copy supports exactly one", s.Table, len(s.PrimaryFields))
		}
		t := &copyTable{name: s.Table}
		for _, name := range s.DBNames {
			f := s.FieldsByDBName[name]
			k, err := columnKind(f)
			if err != nil {
				return nil, fmt.Errorf("db copy: %s.%s: %w", s.Table, name, err)
			}
			c := copyColumn{name: name, kind: k}
			if f.DataType == schema.String && f.Size > 0 {
				c.size = f.Size
			}
			if k == kindTime {
				c.dstPrec = timePrecision(dd, f)
				c.cmp = max(timePrecision(sd, f), c.dstPrec)
			}
			if f == s.PrimaryFields[0] {
				if k != kindInt && k != kindString {
					return nil, fmt.Errorf("db copy: primary key %s.%s must be an integer or a string", s.Table, name)
				}
				t.pk = len(t.cols)
			}
			t.cols = append(t.cols, c)
		}
		out = append(out, t)
	}
	return out, nil
}

func columnKind(f *schema.Field) (colKind, error) {
	switch f.GORMDataType {
	case schema.Bool:
		return kindBool, nil
	case schema.Int, schema.Uint:
		return kindInt, nil
	case schema.Float:
		return kindFloat, nil
	case schema.String, "text": // "text": the JSON column types of package model (GormDataType)
		return kindString, nil
	case schema.Bytes:
		return kindBytes, nil
	case schema.Time:
		if strings.EqualFold(string(f.DataType), "date") {
			return kindDate, nil
		}
		return kindTime, nil
	}
	return 0, fmt.Errorf("unsupported column type %q (%s); teach db copy how to copy it", f.GORMDataType, f.FieldType)
}

// timePrecision is the timestamp precision of a column on a dialect: SQLite stores the text of the full value,
// PostgreSQL timestamptz keeps microseconds, GORM's MySQL datetime(3) milliseconds.
func timePrecision(dialect string, f *schema.Field) time.Duration {
	p := f.Precision
	switch dialect {
	case config.DriverPostgres:
		if p <= 0 || p > 6 {
			p = 6
		}
	case config.DriverMySQL:
		if p <= 0 {
			p = 3
		}
		p = min(p, 6)
	default:
		return time.Nanosecond
	}
	d := time.Second
	for range p {
		d /= 10
	}
	return d
}

// listTables returns the tables of db's current database/schema without SQLite's internal ones (sqlite_sequence).
func listTables(db *gorm.DB) ([]string, error) {
	names, err := db.Migrator().GetTables()
	if err != nil {
		return nil, fmt.Errorf("db copy: list tables: %w", err)
	}
	if db.Dialector.Name() == config.DriverSQLite {
		names = slices.DeleteFunc(names, func(n string) bool { return strings.HasPrefix(n, "sqlite_") })
	}
	sort.Strings(names)
	return names, nil
}

// checkTables refuses tables that the ggboard schema does not have; with needAll, every planned table must exist.
func checkTables(db *gorm.DB, side string, tables []*copyTable, needAll bool) error {
	names, err := listTables(db)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(tables))
	for _, t := range tables {
		known[t.name] = true
	}
	var unknown []string
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
		if !known[n] {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("db copy: the %s database has tables that ggboard does not create (%s); their rows would not be copied. "+
			"Point the copy at a database that holds only ggboard data: %w", side, strings.Join(unknown, ", "), ErrUnknownTable)
	}
	if needAll {
		for _, t := range tables {
			if !have[t.name] {
				return fmt.Errorf("db copy: the %s database has no table %s; run `ggboard migrate` on it first", side, t.name)
			}
		}
	}
	return nil
}

// checkColumns requires the table's columns to be exactly the schema's.
func checkColumns(ctx context.Context, db *gorm.DB, side string, t *copyTable) error {
	rows, err := db.Statement.ConnPool.QueryContext(ctx, "SELECT * FROM "+quote(db, t.name)+" WHERE 1 = 0")
	if err != nil {
		return fmt.Errorf("db copy: read the columns of %s %s: %w", side, t.name, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("db copy: read the columns of %s %s: %w", side, t.name, err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("db copy: read the columns of %s %s: %w", side, t.name, err)
	}
	have := make(map[string]bool, len(cols))
	for _, c := range cols {
		have[strings.ToLower(c)] = true
	}
	want := make(map[string]bool, len(t.cols))
	for _, c := range t.cols {
		want[c.name] = true
		if !have[c.name] {
			return fmt.Errorf("db copy: %s table %s has no column %s; run `ggboard migrate` on it first", side, t.name, c.name)
		}
	}
	var extra []string
	for _, c := range cols {
		if !want[strings.ToLower(c)] {
			extra = append(extra, c)
		}
	}
	if len(extra) > 0 {
		return fmt.Errorf("db copy: %s table %s has columns that the ggboard schema does not have (%s); they would not be copied: %w",
			side, t.name, strings.Join(extra, ", "), ErrUnknownColumn)
	}
	return nil
}

// sameDatabase reports whether a and b are one database: the same SQLite file, or the same PostgreSQL / MySQL
// server, database and schema.
func sameDatabase(ctx context.Context, a, b *gorm.DB) (bool, error) {
	if a.Dialector.Name() != b.Dialector.Name() {
		return false, nil
	}
	if pa, pb := a.Statement.ConnPool, b.Statement.ConnPool; pa == pb {
		return true, nil
	}
	ia, err := databaseIdentity(ctx, a)
	if err != nil {
		return false, err
	}
	ib, err := databaseIdentity(ctx, b)
	if err != nil {
		return false, err
	}
	if a.Dialector.Name() != config.DriverSQLite {
		return ia == ib, nil
	}
	if ia == "" || ib == "" { // in-memory databases are private to their connection pool
		return false, nil
	}
	sa, err := os.Stat(ia)
	if err != nil {
		return false, fmt.Errorf("db copy: %w", err)
	}
	sb, err := os.Stat(ib)
	if err != nil {
		return false, fmt.Errorf("db copy: %w", err)
	}
	return os.SameFile(sa, sb), nil
}

// databaseIdentity is the SQLite main file ("" in memory), or a server/database/schema fingerprint.
func databaseIdentity(ctx context.Context, db *gorm.DB) (string, error) {
	pool := db.Statement.ConnPool
	var id string
	switch db.Dialector.Name() {
	case config.DriverSQLite:
		rows, err := pool.QueryContext(ctx, "PRAGMA database_list")
		if err != nil {
			return "", fmt.Errorf("db copy: identify the sqlite database: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var (
				seq        int
				name, file string
			)
			if err := rows.Scan(&seq, &name, &file); err != nil {
				return "", fmt.Errorf("db copy: identify the sqlite database: %w", err)
			}
			if name == "main" {
				id = file
			}
		}
		if err := rows.Err(); err != nil {
			return "", fmt.Errorf("db copy: identify the sqlite database: %w", err)
		}
		return id, nil
	case config.DriverPostgres:
		err := pool.QueryRowContext(ctx, `SELECT current_database() || '/' || COALESCE(current_schema(), '') || '@' ||
			pg_postmaster_start_time()::text || '#' || (SELECT oid FROM pg_database WHERE datname = current_database())::text`).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("db copy: identify the postgres database: %w", err)
		}
		return id, nil
	default:
		if err := pool.QueryRowContext(ctx, "SELECT CONCAT(@@server_uuid, '/', DATABASE())").Scan(&id); err == nil {
			return id, nil
		}
		// MariaDB has no server_uuid.
		if err := pool.QueryRowContext(ctx, "SELECT CONCAT(@@hostname, ':', @@port, '/', DATABASE())").Scan(&id); err != nil {
			return "", fmt.Errorf("db copy: identify the mysql database: %w", err)
		}
		return id, nil
	}
}

// copier runs inside the source (read) and target (write) transactions.
type copier struct {
	ctx      context.Context
	src, dst *gorm.DB
	sd, dd   string
	tables   []*copyTable
	opts     CopyOptions
	rep      *CopyReport
}

func (c *copier) run() error {
	cleared, err := c.prepareTarget()
	if err != nil {
		return err
	}
	copied := make(map[string]int64, len(c.tables))
	for _, t := range c.tables {
		n, err := c.copyTable(t)
		if err != nil {
			return err
		}
		copied[t.name] = n
	}
	if err := c.verify(copied, cleared); err != nil {
		return err
	}
	if c.dd == config.DriverPostgres && !c.opts.DryRun {
		n, err := c.resetSequences()
		if err != nil {
			return err
		}
		c.rep.Sequences = n
	}
	return nil
}

// prepareTarget refuses a target holding rows unless Overwrite, which deletes them (children first).
func (c *copier) prepareTarget() (map[string]int64, error) {
	counts := map[string]int64{}
	var nonEmpty []string
	for _, t := range c.tables {
		n, err := countRows(c.ctx, c.dst, t)
		if err != nil {
			return nil, err
		}
		counts[t.name] = n
		if n > 0 {
			nonEmpty = append(nonEmpty, fmt.Sprintf("%s %d", t.name, n))
		}
	}
	if len(nonEmpty) == 0 {
		return counts, nil
	}
	if !c.opts.Overwrite {
		return nil, fmt.Errorf("db copy: %w (%s); copy into an empty database or pass --overwrite to delete these rows first",
			ErrTargetNotEmpty, strings.Join(nonEmpty, ", "))
	}
	for i := len(c.tables) - 1; i >= 0; i-- {
		t := c.tables[i]
		if counts[t.name] == 0 {
			continue
		}
		if _, err := c.dst.Statement.ConnPool.ExecContext(c.ctx, "DELETE FROM "+quote(c.dst, t.name)); err != nil {
			return nil, fmt.Errorf("db copy: empty target %s: %w", t.name, err)
		}
	}
	return counts, nil
}

// copyTable copies t in primary-key order, batch by batch, and returns the number of rows written.
func (c *copier) copyTable(t *copyTable) (int64, error) {
	batch := max(1, min(c.opts.BatchSize, maxCopyParams/len(t.cols)))
	var (
		last   any
		copied int64
	)
	for {
		rows, err := readRows(c.ctx, c.src, t, last, batch)
		if err != nil {
			return copied, fmt.Errorf("db copy: read source %s: %w", t.name, err)
		}
		if len(rows) == 0 {
			return copied, nil
		}
		for _, row := range rows {
			if err := c.prepareRow(t, row); err != nil {
				return copied, err
			}
		}
		if err := insertRows(c.ctx, c.dst, t, rows); err != nil {
			return copied, fmt.Errorf("db copy: write target %s (%s %v to %v): %w", t.name, t.pkCol().name, rows[0][t.pk], rows[len(rows)-1][t.pk], err)
		}
		copied += int64(len(rows))
		last = rows[len(rows)-1][t.pk]
		if len(rows) < batch {
			return copied, nil
		}
	}
}

// prepareRow converts a source row for the target in place (timestamp precision, calendar days) and refuses values
// the target cannot store, naming the table, row and column (never the value: it may be a secret).
func (c *copier) prepareRow(t *copyTable, row []any) error {
	for i, col := range t.cols {
		switch v := row[i].(type) {
		case time.Time:
			if col.kind == kindDate {
				row[i] = model.Day(v)
			} else {
				row[i] = v.Truncate(col.dstPrec)
			}
		case string:
			if c.dd == config.DriverSQLite {
				continue
			}
			var problem string
			switch {
			case c.dd == config.DriverPostgres && strings.IndexByte(v, 0) >= 0:
				problem = "contains a NUL byte, which PostgreSQL text cannot store"
			case !utf8.ValidString(v):
				problem = "is not valid UTF-8, which " + c.dd + " rejects"
			case col.size > 0 && utf8.RuneCountInString(v) > col.size:
				problem = fmt.Sprintf("has %d characters; the %s column holds %d", utf8.RuneCountInString(v), c.dd, col.size)
			}
			if problem != "" {
				return fmt.Errorf("db copy: %s %s %v: column %s %s; fix the row in the source and run the copy again",
					t.name, t.pkCol().name, row[t.pk], col.name, problem)
			}
		}
	}
	return nil
}

// verify proves that every table holds the same rows on both sides: the table lists match, COUNT(*) equals the rows
// copied, and the per-row hashes (every column, as each database returns it) match in primary-key order.
func (c *copier) verify(copied, cleared map[string]int64) error {
	srcNames, err := listTables(c.src)
	if err != nil {
		return err
	}
	dstNames, err := listTables(c.dst)
	if err != nil {
		return err
	}
	if !slices.Equal(srcNames, dstNames) {
		return fmt.Errorf("db copy: %w: the source has tables %s, the target %s", ErrVerify,
			strings.Join(srcNames, ", "), strings.Join(dstNames, ", "))
	}
	byName := make(map[string]*copyTable, len(c.tables))
	for _, t := range c.tables {
		byName[t.name] = t
	}
	var problems []string
	for _, name := range srcNames {
		t := byName[name]
		if t == nil {
			return fmt.Errorf("db copy: %w: table %s is not part of the copy", ErrVerify, name)
		}
		res, err := c.compareTable(t, copied[name])
		if err != nil {
			return err
		}
		problems = append(problems, res.problems...)
		if len(res.problems) == 0 {
			c.rep.Tables = append(c.rep.Tables, TableCopy{Name: name, Rows: res.rows, Cleared: cleared[name], Digest: res.digest})
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("db copy: %w, nothing was written:\n  %s", ErrVerify, strings.Join(problems, "\n  "))
	}
	// Report in copy order.
	order := make(map[string]int, len(c.tables))
	for i, t := range c.tables {
		order[t.name] = i
	}
	sort.Slice(c.rep.Tables, func(i, j int) bool { return order[c.rep.Tables[i].Name] < order[c.rep.Tables[j].Name] })
	return nil
}

type compareResult struct {
	rows     int64
	digest   string
	problems []string
}

func (c *copier) compareTable(t *copyTable, copied int64) (compareResult, error) {
	var res compareResult
	note := func(format string, args ...any) {
		if len(res.problems) < maxCopyProblems {
			res.problems = append(res.problems, t.name+": "+fmt.Sprintf(format, args...))
		}
	}
	srcCount, err := countRows(c.ctx, c.src, t)
	if err != nil {
		return res, err
	}
	dstCount, err := countRows(c.ctx, c.dst, t)
	if err != nil {
		return res, err
	}
	if srcCount != copied || dstCount != copied {
		note("%d rows in the source, %d copied, %d in the target", srcCount, copied, dstCount)
	}
	batch := max(1, min(c.opts.BatchSize, maxCopyParams/len(t.cols)))
	a := &hashStream{ctx: c.ctx, tx: c.src, t: t, batch: batch}
	b := &hashStream{ctx: c.ctx, tx: c.dst, t: t, batch: batch}
	sumA, sumB := sha256.New(), sha256.New()
	var ra, rb *hashedRow
	var na, nb int64
	for {
		if ra == nil {
			if ra, err = a.next(); err != nil {
				return res, fmt.Errorf("db copy: verify source %s: %w", t.name, err)
			}
		}
		if rb == nil {
			if rb, err = b.next(); err != nil {
				return res, fmt.Errorf("db copy: verify target %s: %w", t.name, err)
			}
		}
		if ra == nil && rb == nil {
			break
		}
		var cmp int
		switch {
		case ra == nil:
			cmp = 1
		case rb == nil:
			cmp = -1
		default:
			cmp = bytes.Compare(ra.key, rb.key)
		}
		if cmp <= 0 {
			sumA.Write(ra.key)
			sumA.Write(ra.sum[:])
			na++
		}
		if cmp >= 0 {
			sumB.Write(rb.key)
			sumB.Write(rb.sum[:])
			nb++
		}
		switch {
		case cmp < 0:
			note("%s %v is missing in the target", t.pkCol().name, ra.pk)
		case cmp > 0:
			note("%s %v exists only in the target", t.pkCol().name, rb.pk)
		case ra.sum != rb.sum:
			cols, err := c.diffColumns(t, ra.pk)
			if err != nil {
				return res, err
			}
			note("%s %v differs in %s", t.pkCol().name, ra.pk, strings.Join(cols, ", "))
		}
		if cmp <= 0 {
			ra = nil
		}
		if cmp >= 0 {
			rb = nil
		}
	}
	if na != srcCount || nb != dstCount {
		note("read %d source and %d target rows, COUNT(*) says %d and %d", na, nb, srcCount, dstCount)
	}
	sa, sb := hex.EncodeToString(sumA.Sum(nil)), hex.EncodeToString(sumB.Sum(nil))
	if len(res.problems) == 0 && sa != sb {
		note("table digests differ (%s, %s)", sa[:12], sb[:12])
	}
	res.rows, res.digest = na, sa
	return res, nil
}

// diffColumns names the columns of one row that differ between source and target.
func (c *copier) diffColumns(t *copyTable, pk any) ([]string, error) {
	a, err := readRow(c.ctx, c.src, t, pk)
	if err != nil {
		return nil, fmt.Errorf("db copy: verify source %s: %w", t.name, err)
	}
	b, err := readRow(c.ctx, c.dst, t, pk)
	if err != nil {
		return nil, fmt.Errorf("db copy: verify target %s: %w", t.name, err)
	}
	var out []string
	for i, col := range t.cols {
		if a == nil || b == nil || !bytes.Equal(encodeValue(nil, col, a[i]), encodeValue(nil, col, b[i])) {
			out = append(out, col.name)
		}
	}
	if len(out) == 0 {
		out = append(out, "(no column on a second read)")
	}
	return out, nil
}

// pgSequencesSQL lists the sequences owned by serial (deptype a) and identity (deptype i) columns of the current schema.
const pgSequencesSQL = `SELECT t.relname, a.attname, s.oid::regclass::text
FROM pg_depend d
JOIN pg_class s ON s.oid = d.objid AND s.relkind = 'S'
JOIN pg_class t ON t.oid = d.refobjid
JOIN pg_namespace n ON n.oid = t.relnamespace
JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = d.refobjsubid
WHERE d.classid = 'pg_class'::regclass AND d.refclassid = 'pg_class'::regclass
  AND d.deptype IN ('a', 'i') AND n.nspname = current_schema()
ORDER BY 1, 2`

type pgSequence struct{ table, column, name string }

func pgSequences(ctx context.Context, pool gorm.ConnPool) ([]pgSequence, error) {
	rows, err := pool.QueryContext(ctx, pgSequencesSQL)
	if err != nil {
		return nil, fmt.Errorf("db copy: list sequences: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []pgSequence
	for rows.Next() {
		var s pgSequence
		if err := rows.Scan(&s.table, &s.column, &s.name); err != nil {
			return nil, fmt.Errorf("db copy: list sequences: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db copy: list sequences: %w", err)
	}
	return out, nil
}

// resetSequences moves every serial/identity sequence of the copied tables to max(column)+1 and checks that each
// one's next value is above every copied id.
func (c *copier) resetSequences() (int, error) {
	pool := c.dst.Statement.ConnPool
	seqs, err := pgSequences(c.ctx, pool)
	if err != nil {
		return 0, err
	}
	known := map[string]bool{}
	for _, t := range c.tables {
		known[t.name] = true
	}
	n := 0
	for _, s := range seqs {
		if !known[s.table] {
			continue
		}
		maxSQL := "SELECT COALESCE(MAX(" + quote(c.dst, s.column) + "), 0) FROM " + quote(c.dst, s.table)
		var top int64
		if err := pool.QueryRowContext(c.ctx, maxSQL).Scan(&top); err != nil {
			return n, fmt.Errorf("db copy: max(%s.%s): %w", s.table, s.column, err)
		}
		if _, err := pool.ExecContext(c.ctx, "SELECT setval($1::text::regclass, $2, false)", s.name, top+1); err != nil {
			return n, fmt.Errorf("db copy: reset sequence %s: %w", s.name, err)
		}
		var (
			last   int64
			called bool
		)
		// s.name is the sequence's regclass text from the catalog (quoted and schema-qualified as needed).
		if err := pool.QueryRowContext(c.ctx, "SELECT last_value, is_called FROM "+s.name).Scan(&last, &called); err != nil {
			return n, fmt.Errorf("db copy: read sequence %s: %w", s.name, err)
		}
		next := last
		if called {
			next++
		}
		if next <= top {
			return n, fmt.Errorf("db copy: %w: sequence %s would hand out %d, but %s.%s already has %d", ErrVerify, s.name, next, s.table, s.column, top)
		}
		n++
	}
	return n, nil
}

// quote quotes an identifier for db's dialect.
func quote(db *gorm.DB, name string) string {
	var b strings.Builder
	db.Dialector.QuoteTo(&b, name)
	return b.String()
}

func bindVar(db *gorm.DB, i int) string {
	if db.Dialector.Name() == config.DriverPostgres {
		return "$" + strconv.Itoa(i)
	}
	return "?"
}

func countRows(ctx context.Context, db *gorm.DB, t *copyTable) (int64, error) {
	var n int64
	if err := db.Statement.ConnPool.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quote(db, t.name)).Scan(&n); err != nil {
		return 0, fmt.Errorf("db copy: count %s: %w", t.name, err)
	}
	return n, nil
}

func selectColumns(db *gorm.DB, t *copyTable) string {
	var b strings.Builder
	b.WriteString("SELECT ")
	for i, c := range t.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quote(db, c.name))
	}
	b.WriteString(" FROM ")
	b.WriteString(quote(db, t.name))
	return b.String()
}

// readRows reads up to limit rows with a primary key above after (nil = from the start), in primary-key order.
func readRows(ctx context.Context, db *gorm.DB, t *copyTable, after any, limit int) ([][]any, error) {
	q := selectColumns(db, t)
	pk := quote(db, t.pkCol().name)
	var args []any
	if after != nil {
		q += " WHERE " + pk + " > " + bindVar(db, 1)
		args = append(args, after)
	}
	q += " ORDER BY " + pk + " LIMIT " + strconv.Itoa(limit)
	return queryRows(ctx, db, t, q, args...)
}

// readRow reads the row with primary key pk (nil when there is none).
func readRow(ctx context.Context, db *gorm.DB, t *copyTable, pk any) ([]any, error) {
	q := selectColumns(db, t) + " WHERE " + quote(db, t.pkCol().name) + " = " + bindVar(db, 1)
	rows, err := queryRows(ctx, db, t, q, pk)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return rows[0], nil
}

func queryRows(ctx context.Context, db *gorm.DB, t *copyTable, q string, args ...any) ([][]any, error) {
	rows, err := db.Statement.ConnPool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out [][]any
	for rows.Next() {
		dest := make([]any, len(t.cols))
		for i, c := range t.cols {
			dest[i] = scanTarget(c.kind)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := make([]any, len(t.cols))
		for i := range dest {
			row[i] = scannedValue(dest[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func scanTarget(k colKind) any {
	switch k {
	case kindBool:
		return new(sql.NullBool)
	case kindInt:
		return new(sql.NullInt64)
	case kindFloat:
		return new(sql.NullFloat64)
	case kindString:
		return new(sql.NullString)
	case kindBytes:
		return new(sql.Null[[]byte])
	default: // kindTime, kindDate
		return new(sql.NullTime)
	}
}

// scannedValue unwraps a scan target into nil, bool, int64, float64, string, []byte or time.Time.
func scannedValue(dest any) any {
	switch v := dest.(type) {
	case *sql.NullBool:
		if v.Valid {
			return v.Bool
		}
	case *sql.NullInt64:
		if v.Valid {
			return v.Int64
		}
	case *sql.NullFloat64:
		if v.Valid {
			return v.Float64
		}
	case *sql.NullString:
		if v.Valid {
			return v.String
		}
	case *sql.Null[[]byte]:
		if v.Valid {
			if v.V == nil {
				return []byte{}
			}
			return v.V
		}
	case *sql.NullTime:
		if v.Valid {
			return v.Time
		}
	}
	return nil
}

func insertRows(ctx context.Context, db *gorm.DB, t *copyTable, rows [][]any) error {
	var b strings.Builder
	b.WriteString("INSERT INTO ")
	b.WriteString(quote(db, t.name))
	b.WriteString(" (")
	for i, c := range t.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quote(db, c.name))
	}
	b.WriteString(") VALUES ")
	args := make([]any, 0, len(rows)*len(t.cols))
	for r, row := range rows {
		if r > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('(')
		for i, v := range row {
			if i > 0 {
				b.WriteString(", ")
			}
			args = append(args, v)
			b.WriteString(bindVar(db, len(args)))
		}
		b.WriteByte(')')
	}
	_, err := db.Statement.ConnPool.ExecContext(ctx, b.String(), args...)
	return err
}

// hashedRow is one row reduced to its order-preserving primary-key encoding and the SHA-256 of all its columns.
type hashedRow struct {
	pk  any
	key []byte
	sum [sha256.Size]byte
}

// hashStream yields the hashed rows of a table in ascending key order. Integer keys are read batch by batch in the
// database's order (numeric everywhere); text keys are read whole and sorted bytewise, because PostgreSQL and MySQL
// may collate text differently from SQLite.
type hashStream struct {
	ctx   context.Context
	tx    *gorm.DB
	t     *copyTable
	batch int
	buf   []hashedRow
	i     int
	last  any
	done  bool
}

func (s *hashStream) next() (*hashedRow, error) {
	for s.i >= len(s.buf) {
		if s.done {
			return nil, nil
		}
		if err := s.fill(); err != nil {
			return nil, err
		}
	}
	r := &s.buf[s.i]
	s.i++
	return r, nil
}

func (s *hashStream) fill() error {
	s.buf, s.i = s.buf[:0], 0
	for {
		rows, err := readRows(s.ctx, s.tx, s.t, s.last, s.batch)
		if err != nil {
			return err
		}
		for _, row := range rows {
			s.buf = append(s.buf, hashRow(s.t, row))
		}
		if len(rows) < s.batch {
			s.done = true
		}
		if len(rows) > 0 {
			s.last = rows[len(rows)-1][s.t.pk]
		}
		if s.t.pkCol().kind == kindInt || s.done {
			break
		}
	}
	if s.t.pkCol().kind != kindInt {
		sort.Slice(s.buf, func(i, j int) bool { return bytes.Compare(s.buf[i].key, s.buf[j].key) < 0 })
	}
	return nil
}

func hashRow(t *copyTable, row []any) hashedRow {
	h := sha256.New()
	var buf []byte
	for i, c := range t.cols {
		buf = encodeValue(buf[:0], c, row[i])
		h.Write(buf)
	}
	r := hashedRow{pk: row[t.pk]}
	h.Sum(r.sum[:0])
	switch v := row[t.pk].(type) {
	case int64:
		r.key = binary.BigEndian.AppendUint64(nil, uint64(v)^(1<<63)) // sign flip keeps numeric order bytewise
	case string:
		r.key = []byte(v)
	}
	return r
}

// encodeValue appends the canonical, type-tagged, length-prefixed form of a value. Timestamps are compared as UTC
// instants at the column's comparison precision; date columns as calendar days.
func encodeValue(buf []byte, c copyColumn, v any) []byte {
	switch x := v.(type) {
	case nil:
		return append(buf, 0)
	case bool:
		if x {
			return append(buf, 'b', 1)
		}
		return append(buf, 'b', 0)
	case int64:
		return binary.BigEndian.AppendUint64(append(buf, 'i'), uint64(x))
	case float64:
		return binary.BigEndian.AppendUint64(append(buf, 'f'), math.Float64bits(x))
	case string:
		return append(binary.AppendUvarint(append(buf, 's'), uint64(len(x))), x...)
	case []byte:
		return append(binary.AppendUvarint(append(buf, 'x'), uint64(len(x))), x...)
	case time.Time:
		if c.kind == kindDate {
			y, m, d := x.Date()
			return binary.BigEndian.AppendUint32(append(buf, 'd', byte(m), byte(d)), uint32(int32(y)))
		}
		u := x.Truncate(max(c.cmp, time.Nanosecond)).UTC()
		buf = binary.BigEndian.AppendUint64(append(buf, 't'), uint64(u.Unix()))
		return binary.BigEndian.AppendUint32(buf, uint32(u.Nanosecond()))
	}
	// Unreachable: scannedValue returns only the types above. Still encode something stable rather than panic.
	return fmt.Appendf(append(buf, '?'), "%T:%v", v, v)
}
