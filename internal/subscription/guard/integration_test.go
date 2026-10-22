//go:build integration

// Portability of the leak-protection and multi-profile queries on real PostgreSQL / MySQL servers (same
// environment variables as internal/importer/xboard/integration_test.go):
//
//	GGBOARD_IT_POSTGRES_DSN='postgres://postgres:test@127.0.0.1:55491/postgres?sslmode=disable' \
//	GGBOARD_IT_MYSQL_DSN='root:test@tcp(127.0.0.1:55492)/' \
//	go test -tags integration -run Integration ./internal/subscription/guard/
package guard_test

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func itApp(t *testing.T, driver string, settings map[string]any) *app.App {
	t.Helper()
	env := map[string]string{config.DriverPostgres: "GGBOARD_IT_POSTGRES_DSN", config.DriverMySQL: "GGBOARD_IT_MYSQL_DSN"}[driver]
	admin := os.Getenv(env)
	if admin == "" {
		t.Skip(env + " not set")
	}
	conn, err := sql.Open(map[string]string{config.DriverPostgres: "pgx", config.DriverMySQL: "mysql"}[driver], admin)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := "ggb_it_" + hex.EncodeToString(b)
	create, drop := "CREATE DATABASE "+name, "DROP DATABASE IF EXISTS "+name
	if driver == config.DriverMySQL {
		create += " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
	} else {
		drop += " WITH (FORCE)"
	}
	if _, err := conn.Exec(create); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(drop); _ = conn.Close() })
	dsn := ""
	if driver == config.DriverPostgres {
		u, _ := url.Parse(admin)
		u.Path = "/" + name
		dsn = u.String()
	} else {
		cfg, _ := mysqldriver.ParseDSN(admin)
		cfg.DBName = name
		dsn = cfg.FormatDSN()
	}
	return testutil.NewTestApp(t, testutil.WithSettings(settings), testutil.WithConfig(func(c *config.Config) {
		c.Database = config.DatabaseConfig{Driver: driver, DSN: dsn}
	}))
}

func TestIntegrationSubguard(t *testing.T) {
	for _, driver := range []string{config.DriverPostgres, config.DriverMySQL} {
		t.Run(driver, func(t *testing.T) {
			calls := hookCalls(t)
			a := itApp(t, driver, map[string]any{"subscribe_leak_max_ips": 2})
			ctx := t.Context()
			u := testutil.CreateUser(t, a, "alice@example.com")

			// Keys (row lock + update).
			if _, _, err := guard.GenerateKey(ctx, a, u.ID, false, nil); err != nil {
				t.Fatal(err)
			}
			k, _, err := guard.GenerateKey(ctx, a, u.ID, true, testutil.Ptr(true))
			if err != nil || !k.Enabled {
				t.Fatalf("regenerate = %+v %v", k, err)
			}

			// Access log: batched insert, distinct count, ordering, prune.
			now := time.Now()
			for i := range 120 {
				record(a, u.ID, u.Token, "203.0.113.1", now.Add(time.Duration(i-120)*time.Second))
			}
			record(a, u.ID, u.Token, "198.51.100.1", now)
			record(a, u.ID, u.Token, "2001:db8::1", now)
			record(a, u.ID, u.Token, "192.0.2.1", now.Add(-40*24*time.Hour))
			if err := guard.FlushAccess(ctx, a); err != nil {
				t.Fatal(err)
			}
			if len(*calls) != 1 || (*calls)[0].distinct != 3 {
				t.Fatalf("leak calls = %+v", *calls)
			}
			list, err := guard.ListAccess(ctx, a, u.ID, guard.UserLogLimit, true)
			if err != nil || len(list) != guard.UserLogLimit || list[0].IP != "2001:db8:0:0:*" {
				t.Fatalf("list = %v %+v", err, list[:1])
			}
			if err := guard.PruneAccess(ctx, a, now); err != nil {
				t.Fatal(err)
			}
			var n int64
			a.DB.Model(&model.SubscriptionAccess{}).Count(&n)
			if n != guard.KeepPerUser {
				t.Fatalf("rows after prune = %d", n)
			}

			// Profiles: create, default switch, slug selection.
			if _, err := engine.CreateProfile(ctx, a, u.ID, "Work", "", ""); err != nil {
				t.Fatal(err)
			}
			if err := engine.SaveProfileSlug(ctx, a, u.ID, "work", []byte(`{"template":"full"}`)); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.SetDefaultProfile(ctx, a, u.ID, "work"); err != nil {
				t.Fatal(err)
			}
			p, err := engine.EffectiveProfile(ctx, a, rulesets.Current(a.Settings), u, rulesets.Overrides{})
			if err != nil || p.Template != "full" {
				t.Fatalf("effective = %s %v", p.Template, err)
			}
			if err := engine.DeleteProfile(ctx, a, u.ID, "default"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
