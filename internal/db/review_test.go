package db

import (
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// Review 2026-10-22: a DSN with loc=<zone> kept that zone, shifting model.Day dates (midnight UTC).
func TestReviewMySQLDSNForcesUTC(t *testing.T) {
	got, err := MySQLDSN("user:pass@tcp(db.example.com:3306)/ggboard?loc=Asia%2FShanghai")
	if err != nil {
		t.Fatal(err)
	}
	c, err := mysqldriver.ParseDSN(got)
	if err != nil {
		t.Fatal(err)
	}
	if c.Loc != time.UTC {
		t.Fatalf("loc = %v, want UTC (%s)", c.Loc, got)
	}
}
