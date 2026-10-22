package xboardimport

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// clock is the import start used by most tests: 2026-10-21 12:00 +08:00.
var clock = time.Date(2026, 10, 21, 12, 0, 0, 0, shanghai())

func shanghai() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return loc
}

// execScript runs a multi-statement SQL script against a SQLite file.
func execScript(t testing.TB, path string, scripts ...string) {
	t.Helper()
	db, err := sql.Open(sqlite.DriverName, path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer db.Close()
	for _, s := range scripts {
		if _, err := db.ExecContext(context.Background(), s); err != nil {
			t.Fatalf("exec script on %s: %v", path, err)
		}
	}
}

func readTestdata(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return string(b)
}

// newSource builds a temporary SQLite Xboard database from testdata/<flavor>_schema.sql and _seed.sql (plus
// optional extra SQL) and returns its sqlite:// DSN.
func newSource(t testing.TB, flavor Flavor, extra ...string) string {
	t.Helper()
	path := newSourceFile(t, flavor, extra...)
	return "sqlite://" + path
}

func newSourceFile(t testing.TB, flavor Flavor, extra ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), string(flavor)+".sqlite")
	scripts := []string{readTestdata(t, string(flavor)+"_schema.sql"), readTestdata(t, string(flavor)+"_seed.sql")}
	execScript(t, path, append(scripts, extra...)...)
	return path
}

// importInto runs the importer with the fixed clock.
func importInto(t testing.TB, a *app.App, dsn string, mod ...func(*Options)) (*Report, error) {
	t.Helper()
	opts := Options{DSN: dsn, SourceTZ: shanghai(), Now: func() time.Time { return clock }}
	for _, m := range mod {
		m(&opts)
	}
	return Run(context.Background(), a, opts)
}

// mustImport imports flavor's fixture into a fresh target app.
func mustImport(t testing.TB, flavor Flavor, mod ...func(*Options)) (*app.App, *Report) {
	t.Helper()
	a := testutil.NewTestApp(t)
	rep, err := importInto(t, a, newSource(t, flavor), mod...)
	if err != nil {
		t.Fatalf("import %s: %v", flavor, err)
	}
	return a, rep
}

func hasWarning(rep *Report, scope, contains string) bool {
	for _, w := range rep.Warnings {
		if (scope == "" || w.Scope == scope) && (contains == "" || containsFold(w.Message, contains)) {
			return true
		}
	}
	return false
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || indexFold(s, sub) >= 0
}

func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFoldASCII(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
