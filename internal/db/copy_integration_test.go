//go:build integration

// db copy against real PostgreSQL / MySQL servers (same environment variables as upgrade_integration_test.go):
//
//	GGBOARD_IT_POSTGRES_DSN='postgres://postgres:test@127.0.0.1:55491/postgres?sslmode=disable' \
//	GGBOARD_IT_MYSQL_DSN='root:test@tcp(127.0.0.1:55492)/' \
//	go test -tags integration -run Integration ./internal/db/
package db_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/db"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func openIT(t *testing.T, driver, dsn string) *gorm.DB {
	t.Helper()
	d, err := db.Open(config.DatabaseConfig{Driver: driver, DSN: dsn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(d) })
	return d
}

// serialTables counts the models whose primary key is an auto-increment id (settings, user_mfa and
// subscription_keys are keyed by name or user id).
func serialTables(t *testing.T, d *gorm.DB) int {
	t.Helper()
	n := 0
	for _, m := range model.All() {
		stmt := &gorm.Statement{DB: d}
		if err := stmt.Parse(m); err != nil {
			t.Fatal(err)
		}
		if stmt.Schema.PrioritizedPrimaryField.AutoIncrement {
			n++
		}
	}
	return n
}

var itPrecision = map[string]time.Duration{config.DriverPostgres: time.Microsecond, config.DriverMySQL: time.Millisecond}

// SQLite → server → SQLite: every row and column survives (timestamps at the server's precision), the server's id
// counters continue after the copied ids, and the copy back is the way to roll back.
func TestIntegrationCopyRoundTrip(t *testing.T) {
	for _, driver := range []string{config.DriverPostgres, config.DriverMySQL} {
		t.Run(driver, func(t *testing.T) {
			dsn := db.ITDatabase(t, driver)
			prec := itPrecision[driver]
			a := seedApp(t)
			server := openIT(t, driver, dsn)

			rep, err := db.Copy(bg, a.DB, server, db.CopyOptions{BatchSize: 3})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Target != driver || rep.TimePrecision != prec || !reflect.DeepEqual(reportCounts(rep), counts(t, a.DB)) {
				t.Fatalf("report = %+v", rep)
			}
			if want := serialTables(t, server); driver == config.DriverPostgres && rep.Sequences != want {
				t.Fatalf("sequences reset = %d, want %d", rep.Sequences, want)
			}
			compareModels(t, a.DB, server, prec)
			var raw string
			if err := server.Model(&model.Node{}).Where("protocol_settings LIKE ?", "%big%").Pluck("protocol_settings", &raw).Error; err != nil || raw != rawNodeSettings {
				t.Fatalf("raw JSON on %s = %q %v", driver, raw, err)
			}

			// The same database through another pool is refused, a second copy needs --overwrite.
			if _, err := db.Copy(bg, server, openIT(t, driver, dsn), db.CopyOptions{Overwrite: true}); !errors.Is(err, db.ErrSameDatabase) {
				t.Fatalf("same %s database: %v", driver, err)
			}
			if _, err := db.Copy(bg, a.DB, server, db.CopyOptions{}); !errors.Is(err, db.ErrTargetNotEmpty) {
				t.Fatalf("non-empty %s target: %v", driver, err)
			}
			if _, err := db.Copy(bg, a.DB, server, db.CopyOptions{Overwrite: true, BatchSize: 2}); err != nil {
				t.Fatal(err)
			}
			compareModels(t, a.DB, server, prec)

			// Back to SQLite (the rollback path).
			back := openSQLite(t, filepath.Join(t.TempDir(), "back.db"))
			rep2, err := db.Copy(bg, server, back, db.CopyOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if rep2.Source != driver || rep2.TimePrecision != prec || rep2.TotalRows() != rep.TotalRows() {
				t.Fatalf("copy back report = %+v", rep2)
			}
			for i := range rep.Tables {
				if driver == config.DriverPostgres && rep.Tables[i].Digest != rep2.Tables[i].Digest {
					t.Errorf("%s digest %s there, %s back", rep.Tables[i].Name, rep.Tables[i].Digest, rep2.Tables[i].Digest)
				}
			}
			compareModels(t, server, back, prec)
			compareModels(t, a.DB, back, prec)
			compareSQLiteRaw(t, a.DB, back, true) // every column but the datetimes is byte-identical after the round trip

			// New rows on the server continue after the copied ids.
			if driver == config.DriverPostgres {
				for _, m := range model.All() {
					stmt := &gorm.Statement{DB: server}
					if err := stmt.Parse(m); err != nil {
						t.Fatal(err)
					}
					pk := stmt.Schema.PrioritizedPrimaryField
					if !pk.AutoIncrement {
						continue
					}
					var top, next int64
					server.Raw("SELECT COALESCE(MAX(id), 0) FROM " + stmt.Schema.Table).Scan(&top)
					if err := server.Raw("SELECT nextval(pg_get_serial_sequence(?, 'id'))", stmt.Schema.Table).Scan(&next).Error; err != nil || next != top+1 {
						t.Errorf("%s: nextval %d after max(id) %d: %v", stmt.Schema.Table, next, top, err)
					}
				}
			}
			var maxID uint64
			server.Model(&model.Node{}).Select("MAX(id)").Scan(&maxID)
			n := model.Node{Type: model.NodeVless, Name: "new", Host: "x.example.com", Port: "443", ServerPort: 443, Rate: 1}
			if err := server.Create(&n).Error; err != nil || n.ID <= maxID {
				t.Fatalf("new node on %s: id %d after %d: %v", driver, n.ID, maxID, err)
			}
		})
	}
}

// Values SQLite stores but the server rejects fail before the write, naming table, row and column, and leave the
// target empty.
func TestIntegrationCopyRefusesValuesTheTargetCannotStore(t *testing.T) {
	for _, driver := range []string{config.DriverPostgres, config.DriverMySQL} {
		t.Run(driver, func(t *testing.T) {
			server := openIT(t, driver, db.ITDatabase(t, driver))
			a := testutil.NewTestApp(t)
			u := testutil.CreateUser(t, a, "long@example.com")
			cases := []struct {
				column, value, want string
				drivers             []string
			}{
				{"email", strings.Repeat("é", 200) + "@example.com", "column email has 212 characters; the " + driver + " column holds 191", nil},
				{"remarks", "a\x00b", "column remarks contains a NUL byte", []string{config.DriverPostgres}},
				{"remarks", "bad \xff utf-8", "column remarks is not valid UTF-8", nil},
			}
			for _, c := range cases {
				if c.drivers != nil && !strings.Contains(strings.Join(c.drivers, ","), driver) {
					continue
				}
				if err := a.DB.Exec("UPDATE users SET email = ?, remarks = ? WHERE id = ?", u.Email, "", u.ID).Error; err != nil {
					t.Fatal(err)
				}
				if err := a.DB.Exec("UPDATE users SET "+c.column+" = ? WHERE id = ?", c.value, u.ID).Error; err != nil {
					t.Fatal(err)
				}
				_, err := db.Copy(bg, a.DB, server, db.CopyOptions{})
				if err == nil || !strings.Contains(err.Error(), "users id 1: "+c.want) {
					t.Fatalf("%s: %v", c.want, err)
				}
				for table, n := range counts(t, server) {
					if n != 0 {
						t.Fatalf("refused copy left %d rows in %s", n, table)
					}
				}
			}
		})
	}
}
