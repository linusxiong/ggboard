package xboardimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/cli"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// dump serializes every table the importer writes (rows ordered by primary key) plus the settings rows.
func dump(t *testing.T, a *app.App, mask func(table string, r map[string]any)) string {
	t.Helper()
	var buf bytes.Buffer
	for i := len(targetTables) - 1; i >= 0; i-- {
		name := targetTables[i].name
		order := "id"
		if name == "rule_profiles" {
			order = "user_id"
		}
		var rows []map[string]any
		if err := a.DB.Table(name).Order(order).Find(&rows).Error; err != nil {
			t.Fatalf("dump %s: %v", name, err)
		}
		for _, r := range rows {
			if mask != nil {
				mask(name, r)
			}
		}
		b, _ := json.Marshal(rows)
		buf.WriteString(name + " " + string(b) + "\n")
	}
	var settingsRows []map[string]any
	a.DB.Table("settings").Order("key").Find(&settingsRows)
	for _, r := range settingsRows {
		if mask != nil {
			mask("settings", r)
		}
	}
	b, _ := json.Marshal(settingsRows)
	buf.WriteString("settings " + string(b) + "\n")
	return buf.String()
}

// maskRegenerated blanks values generated randomly per run: the uuid/token regenerated for duplicates
// (users 9 and 11) and ggboard's own server_token (kept when Xboard has none).
func maskRegenerated(table string, r map[string]any) {
	if table == "settings" && r["key"] == "server_token" {
		r["value"], r["updated_at"] = "generated", nil
	}
	if table != "users" {
		return
	}
	switch id, _ := flexInt(r["id"]); id {
	case 9:
		r["token"] = "regenerated"
	case 11:
		r["uuid"] = "regenerated"
	}
}

func targetEmpty(t *testing.T, a *app.App) {
	t.Helper()
	for _, tt := range targetTables {
		var n int64
		a.DB.Model(tt.model).Count(&n)
		if n != 0 {
			t.Errorf("%s has %d rows, want 0", tt.name, n)
		}
	}
}

// §9 assertion 13: a second run fails without writing; --force replaces everything identically.
func TestIdempotency(t *testing.T) {
	a := testutil.NewTestApp(t)
	src := newSource(t, FlavorLegacy)
	if _, err := importInto(t, a, src); err != nil {
		t.Fatal(err)
	}
	before := dump(t, a, nil)
	if _, err := importInto(t, a, src); !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("second run: %v, want ErrTargetNotEmpty", err)
	}
	if after := dump(t, a, nil); after != before {
		t.Error("a refused run must not write anything")
	}

	// extra rows that --force must remove
	extra := []any{
		&model.Order{TradeNo: "T1", UserID: 1, PlanID: 1, Type: model.OrderTypeNew, Period: model.PeriodMonthly},
		&model.RuleProfile{UserID: 1, Data: model.RawJSON(`{}`)},
		&model.User{ID: 100, Email: "extra@example.com", PasswordHash: "!", UUID: "30000000-0000-4000-8000-000000000001", Token: "c0000000000000000000000000000001"},
	}
	for _, e := range extra {
		if err := a.DB.Create(e).Error; err != nil {
			t.Fatal(err)
		}
	}
	rep, err := importInto(t, a, src, func(o *Options) { o.Force = true })
	if err != nil {
		t.Fatalf("--force: %v", err)
	}
	if !rep.Force {
		t.Error("report must record --force")
	}
	if got, want := dump(t, a, maskRegenerated), maskDump(t, before); got != want {
		t.Errorf("--force dump differs from the first import:\n%s\n---\n%s", got, want)
	}
}

// maskDump applies maskRegenerated to a dump produced without a mask.
func maskDump(t *testing.T, d string) string {
	t.Helper()
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(d, "\n"), "\n") {
		name, body, _ := strings.Cut(line, " ")
		var rows []map[string]any
		if err := json.Unmarshal([]byte(body), &rows); err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			maskRegenerated(name, r)
		}
		b, _ := json.Marshal(rows)
		out.WriteString(name + " " + string(b) + "\n")
	}
	return out.String()
}

// §9 assertion 13: --dry-run writes nothing and reports exactly what the real run reports.
func TestDryRun(t *testing.T) {
	a := testutil.NewTestApp(t)
	src := newSource(t, FlavorLegacy)
	settingsBefore := dump(t, a, nil)
	dry, err := importInto(t, a, src, func(o *Options) { o.DryRun = true; o.WithStats = true; o.StatsDays = 1000 })
	if err != nil {
		t.Fatal(err)
	}
	targetEmpty(t, a)
	if dump(t, a, nil) != settingsBefore || a.Settings.String("app_name") != "GG Bond" {
		t.Error("a dry run must not change settings")
	}
	real, err := importInto(t, a, src, func(o *Options) { o.WithStats = true; o.StatsDays = 1000 })
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(dry.Tables, real.Tables) || !reflect.DeepEqual(dry.Warnings, real.Warnings) {
		t.Errorf("dry-run report differs from the real run:\n%+v\n%+v", dry.Tables, real.Tables)
	}
	var out bytes.Buffer
	dry.WriteSummary(&out)
	if !strings.Contains(out.String(), "DRY-RUN (rolled back)") || !strings.Contains(out.String(), "nothing was written") {
		t.Errorf("dry-run summary:\n%s", out.String())
	}
}

// §9 assertion 14: malformed JSON produces warnings, old forks may lack columns, and the source is unchanged.
func TestRobustnessAndReadOnlySource(t *testing.T) {
	path := newSourceFile(t, FlavorLegacy,
		`UPDATE "v2_server_vmess" SET "networkSettings" = '{"path":' WHERE "id" = 1;`,
		`UPDATE "v2_plan" SET "content" = NULL WHERE "id" = 1;`,
		`UPDATE "v2_coupon" SET "limit_plan_ids" = '[1,' WHERE "id" = 1;`,
		`ALTER TABLE "v2_server_hysteria" DROP COLUMN "version";`,
		`ALTER TABLE "v2_server_hysteria" DROP COLUMN "is_obfs";`,
		`ALTER TABLE "v2_server_hysteria" DROP COLUMN "alpn";`,
		`ALTER TABLE "v2_server_trojan" DROP COLUMN "network";`,
		`ALTER TABLE "v2_server_trojan" DROP COLUMN "networkSettings";`,
	)
	sum := func() [32]byte {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return sha256.Sum256(b)
	}
	before := sum()
	entries, _ := os.ReadDir(filepath.Dir(path))
	a := testutil.NewTestApp(t)
	rep, err := importInto(t, a, "sqlite://"+path, func(o *Options) { o.WithStats = true })
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if sum() != before {
		t.Error("the source database file changed")
	}
	if after, _ := os.ReadDir(filepath.Dir(path)); len(after) != len(entries) {
		t.Errorf("the importer created files next to the source: %v", after)
	}
	if !hasWarning(rep, "nodes#11", "networkSettings") || !hasWarning(rep, "coupons#1", "limit_plan_ids") {
		t.Error("malformed JSON must warn")
	}
	hy := loadNode(t, a, 7)
	if hy.ProtocolSettings.Int("version") != 1 || hy.ProtocolSettings.String("obfs") != "salamander" || hy.ProtocolSettings.String("alpn") != "hysteria" {
		t.Errorf("old-fork hysteria defaults: %v", hy.ProtocolSettings)
	}
	tj := loadNode(t, a, 6)
	if tj.ProtocolSettings.String("network") != "tcp" {
		t.Errorf("old-fork trojan network: %v", tj.ProtocolSettings)
	}
	var p model.Plan
	a.DB.First(&p, 1)
	if p.Content != "" {
		t.Errorf("NULL content = %q", p.Content)
	}
}

// §9 assertion 1 (fatal cases) and the all-or-nothing guarantee.
func TestFatalErrorsWriteNothing(t *testing.T) {
	cases := map[string]struct {
		dsn  func(t *testing.T) string
		want error
	}{
		"both flavors": {func(t *testing.T) string {
			return newSource(t, FlavorLegacy, `CREATE TABLE "v2_server" ("id" integer primary key autoincrement not null);`)
		}, ErrHalfMigrated},
		"neither flavor": {func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "x.sqlite")
			execScript(t, p, `CREATE TABLE v2_user (id integer primary key); CREATE TABLE v2_settings (id integer primary key);`)
			return "sqlite://" + p
		}, ErrNotXboard},
		"not xboard": {func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "x.sqlite")
			execScript(t, p, `CREATE TABLE users (id integer primary key);`)
			return "sqlite://" + p
		}, ErrNotXboard},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			_, err := importInto(t, a, c.dsn(t))
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			targetEmpty(t, a)
		})
	}
	t.Run("failure mid-import rolls back", func(t *testing.T) {
		a := testutil.NewTestApp(t)
		src := newSource(t, FlavorLegacy, `DROP TABLE "v2_ticket_message";`)
		if _, err := importInto(t, a, src); err == nil || !strings.Contains(err.Error(), "v2_ticket_message") {
			t.Fatalf("err = %v", err)
		}
		targetEmpty(t, a)
		if a.Settings.String("app_name") != "GG Bond" {
			t.Error("settings written before the failure must be rolled back")
		}
	})
	t.Run("missing file", func(t *testing.T) {
		a := testutil.NewTestApp(t)
		if _, err := importInto(t, a, "sqlite://"+filepath.Join(t.TempDir(), "nope.sqlite")); err == nil {
			t.Fatal("a missing source file must fail")
		}
	})
}

// The cobra command end to end: config, migration, summary, exit codes.
func TestCLI(t *testing.T) {
	testutil.FastHashing()
	dir := t.TempDir()
	target := filepath.Join(dir, "ggboard.db")
	src := newSource(t, FlavorUpstream)
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		base := []string{"--data-dir", dir, "--db-driver", "sqlite", "--db-dsn", target, "--log-level", "error"}
		code := cli.Run(context.Background(), "test", append(base, args...), &out, &errb)
		return code, out.String(), errb.String()
	}
	if code, out, errOut := run("import", "xboard", "--dsn", src, "--dry-run", "--with-stats"); code != 0 ||
		!strings.Contains(out, "flavor upstream") || !strings.Contains(out, "DRY-RUN (rolled back)") || !strings.Contains(out, "post-import checklist") {
		t.Fatalf("dry run: exit %d\n%s\n%s", code, out, errOut)
	}
	if code, out, errOut := run("import", "xboard", "--dsn", src, "--source-tz", "Asia/Shanghai"); code != 0 || !strings.Contains(out, "COMMITTED") {
		t.Fatalf("import: exit %d\n%s\n%s", code, out, errOut)
	}
	if code, _, errOut := run("import", "xboard", "--dsn", src); code != 1 || !strings.Contains(errOut, "not empty") {
		t.Fatalf("second import: exit %d, stderr %q", code, errOut)
	}
	if code, out, errOut := run("import", "xboard", "--dsn", src, "--force"); code != 0 || !strings.Contains(out, "--force") {
		t.Fatalf("--force: exit %d\n%s\n%s", code, out, errOut)
	}
	for _, bad := range [][]string{
		{"import", "xboard"},
		{"import", "xboard", "--dsn", "xboard.sqlite"},
		{"import", "xboard", "--dsn", src, "--source-tz", "Mars/Olympus"},
	} {
		if code, _, _ := run(bad...); code != 1 {
			t.Errorf("%v: exit %d, want 1", bad, code)
		}
	}

	db, err := gorm.Open(sqlite.Open(target), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	}()
	var users, nodes int64
	db.Model(&model.User{}).Count(&users)
	db.Model(&model.Node{}).Count(&nodes)
	if users != 2 || nodes != 12 {
		t.Errorf("target after CLI import: %d users, %d nodes", users, nodes)
	}
	var st model.Setting
	db.Where(map[string]any{"key": "app_name"}).First(&st)
	if st.Value != `"Upstream Board"` {
		t.Errorf("app_name row = %q", st.Value)
	}
}
