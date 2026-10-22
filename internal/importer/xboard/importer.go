// Package xboardimport implements `ggboard import xboard`: a one-shot migration from an Xboard database
// (the legacy per-protocol server tables of the linusxiong/Xboard fork, or upstream cedar2025/Xboard's unified
// v2_server table) into the ggboard schema. docs/IMPORT_XBOARD.md is the specification; section numbers in
// comments refer to it.
//
// Guarantees: the source is read in one read-only snapshot; every target write happens in one transaction
// (any fatal error rolls everything back); primary keys are kept except for colliding legacy nodes; orders are
// never imported; every lossy conversion produces a warning.
package xboardimport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8" // --source-tz must resolve on hosts without a zoneinfo database

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/model"
)

// DefaultSourceTZ is Laravel's app.timezone in both Xboard versions.
const DefaultSourceTZ = "Asia/Shanghai"

// DefaultStatsDays is the --stats-days default.
const DefaultStatsDays = 90

// batchSize is the maximum number of rows per INSERT (docs/BACKEND.md §9).
const batchSize = 500

// Options configures Run.
type Options struct {
	DSN       string         // --dsn
	Driver    string         // --driver, only needed for a DSN without a scheme
	DryRun    bool           // read and write everything, then roll back
	Force     bool           // clear the target tables first (same transaction)
	WithStats bool           // import v2_stat / v2_stat_server / v2_stat_user (last StatsDays days)
	StatsDays int            // default DefaultStatsDays
	SourceTZ  *time.Location // default Asia/Shanghai
	Now       func() time.Time
}

// Fatal errors. Run wraps them with details; test with errors.Is.
var (
	ErrNotXboard      = errors.New("not an Xboard database")
	ErrHalfMigrated   = errors.New("half-applied 2025_01_05 server migration, finish it in Xboard first")
	ErrTargetNotEmpty = errors.New("the target database is not empty")
	errDryRunRollback = errors.New("dry run: rolled back")
)

// Flavor is the detected Xboard schema.
type Flavor string

// Schema flavors (§1.3).
const (
	FlavorLegacy   Flavor = "legacy"
	FlavorUpstream Flavor = "upstream"
)

// legacyServerTables in Xboard's own merge order (§3.2).
var legacyServerTables = []struct{ table, typ string }{
	{"v2_server_shadowsocks", model.NodeShadowsocks},
	{"v2_server_vmess", model.NodeVmess},
	{"v2_server_trojan", model.NodeTrojan},
	{"v2_server_hysteria", model.NodeHysteria},
	{"v2_server_vless", model.NodeVless},
}

// detectFlavor applies the table probes of §1.3.
func detectFlavor(tables map[string]bool) (Flavor, error) {
	if !tables["v2_user"] || !tables["v2_settings"] {
		return "", fmt.Errorf("%w: v2_user or v2_settings missing", ErrNotXboard)
	}
	legacy := false
	for _, t := range legacyServerTables {
		legacy = legacy || tables[t.table]
	}
	unified := tables["v2_server"]
	switch {
	case unified && legacy:
		return "", ErrHalfMigrated
	case unified:
		return FlavorUpstream, nil
	case legacy:
		return FlavorLegacy, nil
	}
	return "", fmt.Errorf("%w: neither v2_server nor any v2_server_<protocol> table exists", ErrNotXboard)
}

// targetTables are checked for emptiness and cleared by --force, children first (§3.4).
var targetTables = []struct {
	name  string
	model any
}{
	{"ticket_messages", &model.TicketMessage{}},
	{"tickets", &model.Ticket{}},
	{"rule_profiles", &model.RuleProfile{}},
	// Second factors are keyed by user id: --force re-creates users with Xboard's ids, so a leftover passkey
	// would sign in as the imported user that receives the same id.
	{"passkeys", &model.Passkey{}},
	{"user_mfa", &model.UserMFA{}},
	{"stat_users", &model.StatUser{}},
	{"stat_nodes", &model.StatNode{}},
	{"stats", &model.Stat{}},
	{"commission_logs", &model.CommissionLog{}},
	{"invite_codes", &model.InviteCode{}},
	{"orders", &model.Order{}},
	{"coupons", &model.Coupon{}},
	{"payments", &model.Payment{}},
	{"notices", &model.Notice{}},
	{"knowledge", &model.Knowledge{}},
	{"nodes", &model.Node{}},
	{"node_routes", &model.NodeRoute{}},
	{"users", &model.User{}},
	{"plans", &model.Plan{}},
	{"server_groups", &model.ServerGroup{}},
}

// keptIDTables are inserted with explicit ids; PostgreSQL sequences are reset for them (§3.3).
var keptIDTables = []string{
	"server_groups", "plans", "node_routes", "nodes", "users", "invite_codes", "commission_logs", "coupons",
	"payments", "tickets", "ticket_messages", "notices", "knowledge",
}

type nodeKey struct {
	typ string
	id  uint64
}

type planInfo struct {
	reset    *model.ResetMethod
	capacity *int
	active   int
}

// importer holds the state of one run.
type importer struct {
	ctx    context.Context
	a      *app.App
	opts   Options
	src    *source
	tx     *gorm.DB
	flavor Flavor
	rep    *Report
	log    *slog.Logger

	start time.Time      // import start (instant)
	loc   *time.Location // --source-tz

	groups        map[uint64]bool
	plans         map[uint64]*planInfo
	routes        map[uint64]bool // imported routes
	skippedRoutes map[uint64]bool
	users         map[uint64]bool
	nodeMap       map[nodeKey]uint64 // L: (type, legacy id) → node id; U: (type, v2_server.id) → id
	nodeCodes     map[nodeKey]uint64 // U: (type, code) → id
	tickets       map[uint64]bool

	resetMethod model.ResetMethod // effective setting reset_traffic_method after import
	currency    string            // effective setting currency after import
	appURL      string
}

// Run imports the Xboard database described by opts into a's database. The returned Report is non-nil
// whenever the source could be opened; it describes a dry run exactly like a real one.
func Run(ctx context.Context, a *app.App, opts Options) (*Report, error) {
	if opts.StatsDays <= 0 {
		opts.StatsDays = DefaultStatsDays
	}
	if opts.SourceTZ == nil {
		loc, err := time.LoadLocation(DefaultSourceTZ)
		if err != nil {
			return nil, err
		}
		opts.SourceTZ = loc
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	dsn, err := ParseDSN(opts.DSN, opts.Driver)
	if err != nil {
		return nil, err
	}
	src, err := openSource(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer src.close()

	im := &importer{
		ctx: ctx, a: a, opts: opts, src: src, log: a.Log,
		start:         opts.Now().UTC().Truncate(time.Second),
		loc:           opts.SourceTZ,
		groups:        map[uint64]bool{},
		plans:         map[uint64]*planInfo{},
		routes:        map[uint64]bool{},
		skippedRoutes: map[uint64]bool{},
		users:         map[uint64]bool{},
		nodeMap:       map[nodeKey]uint64{},
		nodeCodes:     map[nodeKey]uint64{},
		tickets:       map[uint64]bool{},
	}
	if im.log == nil {
		im.log = slog.New(slog.DiscardHandler)
	}
	im.rep = &Report{
		Source: dsn.Display, DryRun: opts.DryRun, Force: opts.Force, WithStats: opts.WithStats,
		StatsDays: opts.StatsDays, SourceTZ: opts.SourceTZ.String(), Target: a.DBDriver(),
	}

	if err := src.loadTables(ctx); err != nil {
		return im.rep, err
	}
	if im.flavor, err = detectFlavor(src.tables); err != nil {
		return im.rep, err
	}
	im.rep.Flavor = im.flavor
	if err := src.begin(ctx); err != nil {
		return im.rep, err
	}
	if err := src.loadColumns(ctx); err != nil {
		return im.rep, err
	}
	if im.rep.Migrations, im.rep.LastMigration, err = src.lastMigration(ctx); err != nil {
		return im.rep, err
	}
	im.probeTables()

	err = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		im.tx = tx
		if err := im.prepareTarget(); err != nil {
			return err
		}
		if err := im.importAll(); err != nil {
			return err
		}
		if opts.DryRun {
			return errDryRunRollback
		}
		return im.resetSequences()
	})
	if errors.Is(err, errDryRunRollback) {
		err = nil
	}
	if err != nil {
		return im.rep, err
	}
	im.rep.finish(im)
	if !opts.DryRun {
		if err := a.Settings.Reload(ctx); err != nil {
			return im.rep, fmt.Errorf("reload settings: %w", err)
		}
		if a.Cache != nil {
			a.Cache.Clear()
		}
	}
	return im.rep, nil
}

// probeTables warns about unexpected v2_server_* tables and records the skipped tables of §4.15.
func (im *importer) probeTables() {
	known := map[string]bool{"v2_server_group": true, "v2_server_route": true, "v2_server_log": true, "v2_server_stat": true}
	for _, t := range legacyServerTables {
		known[t.table] = true
	}
	names := make([]string, 0, len(im.src.tables))
	for t := range im.src.tables {
		names = append(names, t)
	}
	sort.Strings(names)
	for _, t := range names {
		if strings.HasPrefix(t, "v2_server_") && !known[t] && !strings.HasPrefix(t, "v2_server_machine") {
			im.warn("source", "table %s is not an Xboard table ggboard knows; ignored", t)
		}
	}
	for _, s := range skippedTables {
		if !im.src.has(s.table) {
			continue
		}
		n, err := im.src.countRows(im.ctx, s.table)
		if err != nil {
			continue
		}
		im.rep.SkippedTables = append(im.rep.SkippedTables, SkippedTable{Table: s.table, Rows: n, Reason: s.reason})
	}
}

// skippedTables are never imported (§4.15). v2_settings/v2_plugins are read but not imported as tables.
var skippedTables = []struct{ table, reason string }{
	{"v2_order", "historical orders are never imported"},
	{"v2_log", "operational log"},
	{"v2_admin_audit_log", "operational log"},
	{"v2_mail_log", "operational log"},
	{"v2_server_log", "operational log"},
	{"v2_traffic_reset_logs", "operational log"},
	{"failed_jobs", "framework internals"},
	{"v2_tutorial", "replaced by knowledge"},
	{"v2_plugins", "PHP plugins (only read for payment enable state)"},
	{"v2_gift_card_template", "no gift cards in ggboard"},
	{"v2_gift_card_code", "no gift cards in ggboard"},
	{"v2_gift_card_usage", "no gift cards in ggboard"},
	{"v2_subscribe_templates", "subscription templates differ"},
	{"v2_mail_templates", "mail templates differ"},
	{"v2_server_machine", "no machine agent in ggboard"},
	{"v2_server_machine_load_history", "no machine agent in ggboard"},
	{"personal_access_tokens", "framework internals"},
	{"v2_server_stat", "legacy internals"},
}

// prepareTarget refuses a non-empty target, or clears it with --force (§3.4).
func (im *importer) prepareTarget() error {
	if im.opts.Force {
		for _, t := range targetTables {
			if err := im.tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(t.model).Error; err != nil {
				return fmt.Errorf("clear %s: %w", t.name, err)
			}
		}
		return nil
	}
	var nonEmpty []string
	for _, t := range targetTables {
		var n int64
		if err := im.tx.Model(t.model).Count(&n).Error; err != nil {
			return fmt.Errorf("check %s: %w", t.name, err)
		}
		if n > 0 {
			nonEmpty = append(nonEmpty, fmt.Sprintf("%s (%d)", t.name, n))
		}
	}
	if len(nonEmpty) > 0 {
		return fmt.Errorf("%w: %s; run the import right after `ggboard migrate`, or pass --force to replace the data",
			ErrTargetNotEmpty, strings.Join(nonEmpty, ", "))
	}
	return nil
}

// importAll runs every table mapping in reference order (§3.1).
func (im *importer) importAll() error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"server_groups", im.importGroups},
		{"plans", im.importPlans},
		{"settings", im.importSettings},
		{"node_routes", im.importRoutes},
		{"nodes", im.importNodes},
		{"users", im.importUsers},
		{"invite_codes", im.importInviteCodes},
		{"commission_logs", im.importCommissionLogs},
		{"coupons", im.importCoupons},
		{"payments", im.importPayments},
		{"tickets", im.importTickets},
		{"notices", im.importNotices},
		{"knowledge", im.importKnowledge},
	}
	if im.opts.WithStats {
		steps = append(steps, struct {
			name string
			fn   func() error
		}{"stats", im.importStats})
	}
	for _, s := range steps {
		if err := im.ctx.Err(); err != nil {
			return err
		}
		if err := s.fn(); err != nil {
			return fmt.Errorf("import %s: %w", s.name, err)
		}
	}
	return nil
}

// resetSequences moves PostgreSQL sequences past the imported ids (§3.3). MySQL and SQLite advance their
// counters on explicit ids. Skipped in dry runs: setval is not transactional.
func (im *importer) resetSequences() error {
	if im.tx.Dialector.Name() != config.DriverPostgres {
		return nil
	}
	for _, t := range keptIDTables {
		q := "SELECT setval(pg_get_serial_sequence('" + t + "', 'id'), COALESCE((SELECT MAX(id) FROM " + t + "), 0) + 1, false)"
		if err := im.tx.Exec(q).Error; err != nil {
			return fmt.Errorf("reset sequence of %s: %w", t, err)
		}
	}
	return nil
}

// warn records a W line and mirrors it to slog.
func (im *importer) warn(scope, format string, args ...any) {
	w := Warning{Scope: scope, Message: fmt.Sprintf(format, args...)}
	im.rep.Warnings = append(im.rep.Warnings, w)
	im.log.Warn("xboard import", "scope", w.Scope, "warning", w.Message)
}

func scopeID(table string, id uint64) string { return fmt.Sprintf("%s#%d", table, id) }

// fit truncates s to the target column size (n characters) with a warning. Xboard's varchar(255) columns
// can hold more than some ggboard columns, and PostgreSQL / strict MySQL reject over-long values.
func (im *importer) fit(scope, col, s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	im.warn(scope, "%s is longer than %d characters — truncated", col, n)
	return string([]rune(s)[:n])
}

// fitURL drops a URL that does not fit the target column (a truncated URL is useless).
func (im *importer) fitURL(scope, col, s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	im.warn(scope, "%s is longer than %d characters — dropped", col, n)
	return ""
}

// ts converts a unix column; NULL, 0 and negative values become the import start (created_at semantics).
func (im *importer) ts(scope string, v any) time.Time {
	t, ms, ok := unixTime(v)
	if !ok {
		return im.start
	}
	if ms {
		im.warn(scope, "timestamp %v looks like milliseconds; divided by 1000", v)
	}
	return t
}

// tsPtr converts a nullable unix column; NULL and 0 become nil.
func (im *importer) tsPtr(scope string, v any) *time.Time {
	t, ms, ok := unixTime(v)
	if !ok {
		return nil
	}
	if ms {
		im.warn(scope, "timestamp %v looks like milliseconds; divided by 1000", v)
	}
	return &t
}

// batcher buffers rows and inserts them with CreateInBatches (≤ batchSize per statement).
type batcher[T any] struct {
	tx  *gorm.DB
	buf []T
	n   int
}

func newBatcher[T any](tx *gorm.DB) *batcher[T] { return &batcher[T]{tx: tx} }

func (b *batcher[T]) add(v T) error {
	b.buf = append(b.buf, v)
	if len(b.buf) >= batchSize {
		return b.flush()
	}
	return nil
}

func (b *batcher[T]) flush() error {
	if len(b.buf) == 0 {
		return nil
	}
	if err := b.tx.Omit(clause.Associations).CreateInBatches(&b.buf, batchSize).Error; err != nil {
		return err
	}
	b.n += len(b.buf)
	b.buf = b.buf[:0]
	return nil
}
