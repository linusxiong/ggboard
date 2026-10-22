package db

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/model"
)

func openTemp(t *testing.T) *gorm.DB {
	t.Helper()
	d, err := Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(d) })
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestOpenSQLitePragmasAndMigrate(t *testing.T) {
	d := openTemp(t)
	var mode string
	if err := d.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil || strings.ToLower(mode) != "wal" {
		t.Fatalf("journal_mode = %q %v", mode, err)
	}
	var fk, busy int
	d.Raw("PRAGMA foreign_keys").Scan(&fk)
	d.Raw("PRAGMA busy_timeout").Scan(&busy)
	if fk != 1 || busy != 5000 {
		t.Fatalf("foreign_keys=%d busy_timeout=%d", fk, busy)
	}
	for _, m := range model.All() {
		if !d.Migrator().HasTable(m) {
			t.Errorf("missing table for %T", m)
		}
	}
	if !d.Migrator().HasTable("knowledge") || !d.Migrator().HasIndex(&model.StatUser{}, "uniq_stat_users_day") {
		t.Fatal("expected knowledge table and stat_users unique index")
	}
	// Migrate is idempotent.
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
}

func TestRoundTripJSONAndUTC(t *testing.T) {
	d := openTemp(t)
	loc := time.FixedZone("UTC+8", 8*3600)
	exp := time.Date(2027, 1, 1, 8, 0, 0, 0, loc) // == 2027-01-01T00:00:00Z
	n := model.Node{Type: model.NodeVless, Name: "n1", Host: "a.example.com", Port: "443", ServerPort: 443, Rate: 1.5,
		GroupIDs: model.JSONSlice[uint64]{1, 2}, Tags: model.JSONSlice[string]{"HK"},
		ProtocolSettings: model.JSONMap{"tls": 2, "flow": "xtls-rprx-vision"}}
	if err := d.Create(&n).Error; err != nil {
		t.Fatal(err)
	}
	u := model.User{Email: "a@example.com", PasswordHash: "x", UUID: "u-1", Token: "t-1", ExpiredAt: &exp, TransferEnable: 10}
	if err := d.Create(&u).Error; err != nil {
		t.Fatal(err)
	}

	var got model.Node
	if err := d.First(&got, n.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(got.GroupIDs) != 2 || got.GroupIDs[1] != 2 || got.Tags[0] != "HK" || got.ProtocolSettings.Int("tls") != 2 {
		t.Fatalf("node = %+v", got)
	}

	// Comparisons must work no matter which zone the parameter is in.
	var count int64
	d.Model(&model.User{}).Where("expired_at > ?", time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC)).Count(&count)
	if count != 1 {
		t.Fatalf("UTC comparison count = %d", count)
	}
	d.Model(&model.User{}).Where("expired_at > ?", time.Date(2027, 1, 1, 7, 30, 0, 0, loc)).Count(&count)
	if count != 1 {
		t.Fatalf("local comparison count = %d", count)
	}
	d.Model(&model.User{}).Where("expired_at > ?", time.Date(2027, 1, 1, 8, 30, 0, 0, loc)).Count(&count)
	if count != 0 {
		t.Fatalf("after-expiry count = %d", count)
	}
	var back model.User
	d.First(&back, u.ID)
	if !back.ExpiredAt.Equal(exp) {
		t.Fatalf("expired_at = %v, want %v", back.ExpiredAt, exp)
	}

	dup := model.User{Email: "a@example.com", PasswordHash: "x", UUID: "u-2", Token: "t-2"}
	if err := d.Create(&dup).Error; !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate email err = %v", err)
	}
}

func TestMemoryDSN(t *testing.T) {
	dsn, mem := SQLiteDSN("file:x?mode=memory&cache=shared")
	if !mem || strings.Contains(dsn, "journal_mode") || !strings.Contains(dsn, "_txlock=immediate") {
		t.Fatalf("memory dsn = %s", dsn)
	}
	dsn, mem = SQLiteDSN("/data/ggboard.db")
	if mem || !strings.Contains(dsn, "journal_mode%28WAL%29") {
		t.Fatalf("file dsn = %s", dsn)
	}
	custom, _ := SQLiteDSN("/data/x.db?_pragma=busy_timeout(1)")
	if strings.Contains(custom, "WAL") {
		t.Fatalf("custom pragmas must be kept as-is: %s", custom)
	}
	d, err := Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: ":memory:"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(d)
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
}

func TestMySQLDSN(t *testing.T) {
	got, err := MySQLDSN("user:pass@tcp(db.example.com:3306)/ggboard")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "parseTime=true") || !strings.Contains(got, "charset=utf8mb4") {
		t.Fatalf("dsn = %s", got)
	}
	if _, err := MySQLDSN("::bad"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Open(config.DatabaseConfig{Driver: "oracle"}, nil); err == nil {
		t.Fatal("expected unsupported driver error")
	}
}
