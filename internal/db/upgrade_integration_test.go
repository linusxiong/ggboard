//go:build integration

// The rule_profiles upgrade against real PostgreSQL / MySQL servers (same environment variables as the
// importer suite in internal/importer/xboard/integration_test.go):
//
//	GGBOARD_IT_POSTGRES_DSN='postgres://postgres:test@127.0.0.1:55491/postgres?sslmode=disable' \
//	GGBOARD_IT_MYSQL_DSN='root:test@tcp(127.0.0.1:55492)/' \
//	go test -tags integration -run Integration ./internal/db/
package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/linusxiong/ggboard/internal/config"
)

func itDatabase(t *testing.T, driver string) string {
	t.Helper()
	env := map[string]string{config.DriverPostgres: "GGBOARD_IT_POSTGRES_DSN", config.DriverMySQL: "GGBOARD_IT_MYSQL_DSN"}[driver]
	admin := os.Getenv(env)
	if admin == "" {
		t.Skip(env + " not set")
	}
	sqlDriver := map[string]string{config.DriverPostgres: "pgx", config.DriverMySQL: "mysql"}[driver]
	conn, err := sql.Open(sqlDriver, admin)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	name := "ggb_it_" + hex.EncodeToString(b)
	create := "CREATE DATABASE " + name
	if driver == config.DriverMySQL {
		create += " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
	}
	if _, err := conn.Exec(create); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		drop := "DROP DATABASE IF EXISTS " + name
		if driver == config.DriverPostgres {
			drop += " WITH (FORCE)"
		}
		_, _ = conn.Exec(drop)
		_ = conn.Close()
	})
	if driver == config.DriverPostgres {
		u, err := url.Parse(admin)
		if err != nil {
			t.Fatal(err)
		}
		u.Path = "/" + name
		return u.String()
	}
	cfg, err := mysqldriver.ParseDSN(admin)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	return cfg.FormatDSN()
}

func TestIntegrationUpgradeRuleProfiles(t *testing.T) {
	for _, driver := range []string{config.DriverPostgres, config.DriverMySQL} {
		t.Run(driver, func(t *testing.T) {
			d, err := Open(config.DatabaseConfig{Driver: driver, DSN: itDatabase(t, driver)}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = Close(d) })
			legacyV1(t, d)
			if err := Migrate(d); err != nil {
				t.Fatal(err)
			}
			checkUpgraded(t, d)
		})
	}
}
