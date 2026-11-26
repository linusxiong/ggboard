package db

import (
	"path/filepath"
	"testing"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/model"
)

// tickets.kind marks withdrawal requests (an open one blocks commission transfers). The upgrade that adds the column
// marks the requests filed before it by their subject (ggboard zh-CN/en, Xboard en); later tickets are never
// re-classified by subject.
func TestUpgradeAddsTicketKind(t *testing.T) {
	d, err := Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(d) })
	// A database from before the column existed.
	if err := d.AutoMigrate(model.All()...); err != nil {
		t.Fatal(err)
	}
	if err := d.Migrator().DropColumn(&model.Ticket{}, "kind"); err != nil {
		t.Fatal(err)
	}
	subjects := append([]string{"Cannot connect", "[提现申请]本工单由系统发出 (typed by a user)"}, model.WithdrawalTicketSubjects...)
	for i, s := range subjects {
		tk := model.Ticket{UserID: uint64(i + 1), Subject: s, Status: model.TicketStatusOpen}
		if err := d.Omit("Kind").Create(&tk).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	var got []model.Ticket
	if err := d.Order("id").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != len(subjects) {
		t.Fatalf("tickets = %d", len(got))
	}
	for i, tk := range got {
		want := model.TicketKindSupport
		if i >= 2 {
			want = model.TicketKindWithdraw
		}
		if tk.Kind != want {
			t.Errorf("%q kind = %q, want %q", tk.Subject, tk.Kind, want)
		}
	}

	// After the upgrade the subject means nothing: a user's ticket titled like a withdrawal stays a support ticket.
	later := model.Ticket{UserID: 9, Subject: model.WithdrawalTicketSubjects[0], Status: model.TicketStatusOpen}
	if err := d.Create(&later).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	var again model.Ticket
	if err := d.First(&again, later.ID).Error; err != nil || again.Kind != model.TicketKindSupport {
		t.Fatalf("support ticket after a second migrate = %q %v", again.Kind, err)
	}

	// A fresh database gets the column from AutoMigrate.
	d2, err := Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "t2.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(d2) })
	if err := Migrate(d2); err != nil || !d2.Migrator().HasColumn(&model.Ticket{}, "kind") {
		t.Fatalf("fresh migrate: %v", err)
	}
}
