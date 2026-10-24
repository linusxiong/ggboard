package db

import (
	"path/filepath"
	"testing"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/model"
)

// users.conn_limit and plans.conn_limit (docs/DATA_MODEL.md) are plain nullable columns: AutoMigrate adds them to a
// database from before they existed, and the rows already there stay unlimited (NULL).
func TestUpgradeAddsConnLimit(t *testing.T) {
	d, err := Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(d) })
	if err := d.AutoMigrate(model.All()...); err != nil {
		t.Fatal(err)
	}
	m := d.Migrator()
	if err := m.DropColumn(&model.User{}, "conn_limit"); err != nil {
		t.Fatal(err)
	}
	if err := m.DropColumn(&model.Plan{}, "conn_limit"); err != nil {
		t.Fatal(err)
	}
	if m.HasColumn(&model.User{}, "conn_limit") || m.HasColumn(&model.Plan{}, "conn_limit") {
		t.Fatal("columns still present after the drop")
	}
	old := model.User{Email: "old@example.com", PasswordHash: "!", UUID: "u1", Token: "t1", DeviceLimit: new(3)}
	if err := d.Omit("ConnLimit").Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	plan := model.Plan{Name: "Old", GroupID: 1, DeviceLimit: new(3)}
	if err := d.Omit("ConnLimit").Create(&plan).Error; err != nil {
		t.Fatal(err)
	}

	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	if !m.HasColumn(&model.User{}, "conn_limit") || !m.HasColumn(&model.Plan{}, "conn_limit") {
		t.Fatal("Migrate did not add conn_limit")
	}
	var u model.User
	var p model.Plan
	if err := d.First(&u, old.ID).Error; err != nil || u.ConnLimit != nil || u.DeviceLimit == nil || *u.DeviceLimit != 3 {
		t.Fatalf("existing user after upgrade: conn_limit %v device_limit %v (%v)", u.ConnLimit, u.DeviceLimit, err)
	}
	if err := d.First(&p, plan.ID).Error; err != nil || p.ConnLimit != nil {
		t.Fatalf("existing plan after upgrade: conn_limit %v (%v)", p.ConnLimit, err)
	}
	// The new column round-trips and a second migrate keeps it.
	if err := d.Model(&model.User{}).Where("id = ?", old.ID).Update("conn_limit", 64).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	if err := d.First(&u, old.ID).Error; err != nil || u.ConnLimit == nil || *u.ConnLimit != 64 {
		t.Fatalf("conn_limit after a second migrate = %v (%v)", u.ConnLimit, err)
	}
}
