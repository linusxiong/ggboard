package db

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// asiaShanghai stands in for the production container's TZ=Asia/Shanghai (no DST, so a fixed zone is exact).
var asiaShanghai = time.FixedZone("CST", 8*3600)

// withLocal sets the process time zone for one test (tests in this package do not run in parallel).
func withLocal(t *testing.T, loc *time.Location) {
	t.Helper()
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })
}

// Without the codec pgx returns timestamptz in the local zone, and calendar arithmetic follows that zone; with it,
// values come back in UTC like SQLite and MySQL.
func TestScanTimestamptzInUTC(t *testing.T) {
	withLocal(t, asiaShanghai)
	at := time.Date(2027, 1, 31, 20, 0, 0, 123456000, time.UTC)
	scan := func(m *pgtype.Map, format int16) time.Time {
		t.Helper()
		buf, err := m.Encode(pgtype.TimestamptzOID, format, at, nil)
		if err != nil {
			t.Fatal(err)
		}
		var got pgtype.Timestamptz
		if err := m.Scan(pgtype.TimestamptzOID, format, buf, &got); err != nil {
			t.Fatal(err)
		}
		return got.Time
	}
	if got := scan(pgtype.NewMap(), pgtype.BinaryFormatCode); got.Location() == time.UTC || got.AddDate(0, 1, 0).Equal(at.AddDate(0, 1, 0)) {
		t.Fatalf("default pgx map scanned %v; this test no longer shows the local-zone behaviour it guards against", got)
	}
	m := pgtype.NewMap()
	scanTimestamptzInUTC(m)
	for _, format := range []int16{pgtype.BinaryFormatCode, pgtype.TextFormatCode} {
		got := scan(m, format)
		if got.Location() != time.UTC || !got.Equal(at) {
			t.Fatalf("format %d: scanned %v (%v), want %v in UTC", format, got, got.Location(), at)
		}
		if want := time.Date(2027, 3, 3, 20, 0, 0, 123456000, time.UTC); !got.AddDate(0, 1, 0).Equal(want) {
			t.Fatalf("format %d: one month after %v is %v, want %v", format, got, got.AddDate(0, 1, 0), want)
		}
	}
}

func TestOpenPostgresRejectsABadDSNWithoutTheSecret(t *testing.T) {
	_, err := openPostgres("postgres://ggboard:s3cret-value@db.example.com:5432/ggboard?sslmode=bogus")
	if err == nil {
		t.Fatal("bogus sslmode accepted")
	}
	if got := err.Error(); strings.Contains(got, "s3cret-value") {
		t.Fatalf("error %q", got)
	}
}
