package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/model"
)

// Security audit 2026-10-22 §3: users.email_verified_at is added with a one-time backfill (created_at) for the
// accounts that existed before; NULLs written afterwards ("never verified") must survive later migrations.
func TestUpgradeAddsEmailVerifiedAt(t *testing.T) {
	d, err := Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(d) })
	// A database from before the column existed.
	if err := d.AutoMigrate(model.All()...); err != nil {
		t.Fatal(err)
	}
	if err := d.Migrator().DropColumn(&model.User{}, "email_verified_at"); err != nil {
		t.Fatal(err)
	}
	created := []time.Time{time.Date(2025, 3, 1, 8, 0, 0, 0, time.UTC), time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)}
	for i, at := range created {
		u := model.User{Email: []string{"old1@example.com", "old2@example.com"}[i], PasswordHash: "!", UUID: []string{"u1", "u2"}[i],
			Token: []string{"t1", "t2"}[i], CreatedAt: at, UpdatedAt: at}
		if err := d.Omit("EmailVerifiedAt").Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	var got []model.User
	if err := d.Order("id").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("users = %d", len(got))
	}
	for i, u := range got {
		if u.EmailVerifiedAt == nil || !u.EmailVerifiedAt.Equal(created[i]) {
			t.Fatalf("%s email_verified_at = %v, want %v", u.Email, u.EmailVerifiedAt, created[i])
		}
	}

	// A self-registered, unverified account created after the upgrade stays unverified.
	fresh := model.User{Email: "new@example.com", PasswordHash: "!", UUID: "u3", Token: "t3"}
	if err := d.Create(&fresh).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	var again model.User
	if err := d.First(&again, fresh.ID).Error; err != nil || again.EmailVerifiedAt != nil {
		t.Fatalf("unverified user after a second migrate = %v %v", again.EmailVerifiedAt, err)
	}

	// A fresh database gets the column from AutoMigrate.
	d2, err := Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t2.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(d2) })
	if err := Migrate(d2); err != nil || !d2.Migrator().HasColumn(&model.User{}, "email_verified_at") {
		t.Fatalf("fresh migrate: %v", err)
	}
}
