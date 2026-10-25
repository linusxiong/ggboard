//go:build integration

package db_test

import (
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/db"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// PostgreSQL and MySQL must behave like SQLite (TestSQLiteTimesStayUTCInAnotherZone) when the process runs in another
// zone, as production does with TZ=Asia/Shanghai. On PostgreSQL a DSN with TimeZone= (a session setting sent to the
// server) must not change that.
func TestIntegrationTimesStayUTCInAnotherZone(t *testing.T) {
	cases := []struct{ name, driver, param string }{
		{"postgres", config.DriverPostgres, ""},
		{"postgres with TimeZone in the dsn", config.DriverPostgres, "TimeZone=Asia/Shanghai"},
		{"mysql", config.DriverMySQL, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dsn := db.ITDatabase(t, c.driver)
			if c.param != "" {
				sep := "?"
				if strings.Contains(dsn, "?") {
					sep = "&"
				}
				dsn += sep + c.param
			}
			inShanghai(t)
			a := testutil.NewTestApp(t, testutil.WithConfig(func(cfg *config.Config) {
				cfg.Database = config.DatabaseConfig{Driver: c.driver, DSN: dsn}
			}))
			if a.DBDriver() != c.driver {
				t.Fatalf("driver %s", a.DBDriver())
			}
			checkUTCTimes(t, a)
		})
	}
}
