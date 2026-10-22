package db

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/model"
)

// legacyV1 creates the one-profile-per-user rule_profiles table of the first release with two rows.
func legacyV1(t *testing.T, d *gorm.DB) {
	t.Helper()
	if err := d.Table("rule_profiles").Migrator().CreateTable(&legacyRuleProfile{}); err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	rows := []legacyRuleProfile{
		{UserID: 7, Data: model.RawJSON(`{"template":"minimal"}`), UpdatedAt: ts},
		{UserID: 9, Data: model.RawJSON(`null`), UpdatedAt: ts},
	}
	if err := d.Table("rule_profiles").Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
}

// checkUpgraded asserts the v1 rows became default profiles and the v1 table is gone.
func checkUpgraded(t *testing.T, d *gorm.DB) {
	t.Helper()
	if d.Migrator().HasTable(LegacyRuleProfilesTable) {
		t.Fatal("v1 table still present")
	}
	var got []model.RuleProfile
	if err := d.Order("user_id").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].UserID != 7 || got[0].Slug != model.RuleProfileDefaultSlug || got[0].Name != model.RuleProfileDefaultName ||
		!got[0].IsDefault || string(got[0].Data) != `{"template":"minimal"}` || got[0].ID == 0 || got[1].UserID != 9 ||
		!got[0].UpdatedAt.Equal(time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("upgraded rows = %+v", got)
	}
	// The new table works: another profile for the same user, and (user_id, slug) is unique.
	extra := model.RuleProfile{UserID: 7, Slug: "work", Name: "Work", Data: model.RawJSON(`null`)}
	if err := d.Create(&extra).Error; err != nil || extra.ID <= got[1].ID {
		t.Fatalf("insert after upgrade: %v (id %d)", err, extra.ID)
	}
	dup := model.RuleProfile{UserID: 7, Slug: "work", Name: "Again", Data: model.RawJSON(`null`)}
	if err := d.Create(&dup).Error; err == nil {
		t.Fatal("duplicate (user_id, slug) accepted")
	}
	if err := Migrate(d); err != nil { // idempotent
		t.Fatal(err)
	}
	var n int64
	d.Model(&model.RuleProfile{}).Count(&n)
	if n != 3 {
		t.Fatalf("rows after second migrate = %d", n)
	}
}

func TestUpgradeRuleProfilesV1(t *testing.T) {
	d, err := Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(d) })
	legacyV1(t, d)
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	checkUpgraded(t, d)
}

// An upgrade interrupted after AutoMigrate (v1 rows still in rule_profiles_v1) finishes on the next start.
func TestUpgradeRuleProfilesResumes(t *testing.T) {
	d, err := Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(d) })
	legacyV1(t, d)
	if err := beforeAutoMigrate(d); err != nil {
		t.Fatal(err)
	}
	if err := d.AutoMigrate(model.All()...); err != nil {
		t.Fatal(err)
	}
	// One row was already copied before the crash.
	if err := d.Create(&model.RuleProfile{UserID: 7, Slug: model.RuleProfileDefaultSlug, Name: model.RuleProfileDefaultName, IsDefault: true,
		Data: model.RawJSON(`{"template":"minimal"}`), UpdatedAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	checkUpgraded(t, d)
}

func TestUpgradeRefusesTwoCopies(t *testing.T) {
	d, err := Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(d) })
	legacyV1(t, d)
	if err := d.Table(LegacyRuleProfilesTable).Migrator().CreateTable(&legacyRuleProfile{}); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err == nil {
		t.Fatal("expected an error when both v1 tables exist")
	}
}
