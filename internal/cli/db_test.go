package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/db"
	"github.com/linusxiong/ggboard/internal/model"
)

func TestDBCopyCommand(t *testing.T) {
	auth.SetParams(auth.Params{Memory: 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16})
	t.Cleanup(func() { auth.SetParams(auth.DefaultParams) })
	t.Chdir(t.TempDir())
	t.Setenv("GGBOARD_DATA_DIR", t.TempDir())
	t.Setenv("GGBOARD_LOG_LEVEL", "error")
	if out, code := run(t, "admin", "create", "--email", "root@example.com", "--password", "password123"); code != 0 {
		t.Fatalf("create = %d %q", code, out)
	}
	target := filepath.Join(t.TempDir(), "copy.db")

	out, code := run(t, "db", "copy", "--to-driver", "SQLite", "--to-dsn", target)
	if code != 0 {
		t.Fatalf("copy = %d %q", code, out)
	}
	for _, want := range []string{
		"copied and verified 25 tables", "from sqlite to sqlite", "target    sqlite · " + target, "mode      committed",
		"same tables, row counts and SHA-256 of every row", "users", "settings", "next: set database.driver",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("plain output must not contain escapes: %q", out)
	}
	d, err := db.Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: target}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var u model.User
	if err := d.Where("email = ?", "root@example.com").First(&u).Error; err != nil || !u.IsAdmin {
		t.Fatalf("copied admin = %+v %v", u, err)
	}
	_ = db.Close(d)

	if out, code := run(t, "db", "copy", "--to-driver", "sqlite", "--to-dsn", target); code == 0 ||
		!strings.Contains(out, "target database is not empty") || !strings.Contains(out, "--overwrite") {
		t.Fatalf("second copy = %d %q", code, out)
	}
	out, code = run(t, "db", "copy", "--to-driver", "sqlite", "--to-dsn", target, "--overwrite", "--dry-run")
	if code != 0 || !strings.Contains(out, "dry run: copied and verified") || !strings.Contains(out, "deleted") ||
		strings.Contains(out, "next:") {
		t.Fatalf("overwrite dry run = %d %q", code, out)
	}
	if out, code := run(t, "db", "copy", "--to-driver", "sqlite", "--to-dsn", target, "--overwrite"); code != 0 ||
		!strings.Contains(out, "--overwrite (target rows deleted first)") {
		t.Fatalf("overwrite = %d %q", code, out)
	}

	for _, args := range [][]string{
		{"db", "copy", "--to-driver", "oracle", "--to-dsn", target},
		{"db", "copy", "--to-driver", "postgres", "--to-dsn", " "},
		{"db", "copy", "--to-dsn", target},
	} {
		if out, code := run(t, args...); code == 0 || !strings.Contains(out, "to-d") {
			t.Errorf("%v = %d %q", args, code, out)
		}
	}
	if out, code := run(t, "db", "copy", "--to-driver", "sqlite", "--to-dsn", filepath.Join(".", "data", "ggboard.db")); code == 0 {
		t.Errorf("copy onto the source = %q", out)
	}
}
