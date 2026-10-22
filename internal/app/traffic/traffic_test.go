package traffic_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app/traffic"
	"github.com/linusxiong/ggboard/internal/db"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestFlushUpdatesUsersAndStats(t *testing.T) {
	a := testutil.NewTestApp(t)
	u1 := testutil.CreateUser(t, a, "u1@example.com")
	u2 := testutil.CreateUser(t, a, "u2@example.com")
	ctx := context.Background()

	acc := traffic.New()
	day := time.Date(2026, 10, 21, 23, 0, 0, 0, time.Local)
	acc.SetClock(func() time.Time { return day })
	acc.Add(1, u1.ID, 2, 100, 1000)
	acc.Add(1, u1.ID, 2, 50, 0)
	acc.Add(2, u1.ID, 0.5, 10, 10)
	acc.Add(2, u2.ID, 0.5, 0, 0) // ignored
	acc.Add(2, u2.ID, 1, -5, 7)  // negative ignored
	if acc.Pending() != 3 {
		t.Fatalf("pending = %d", acc.Pending())
	}
	if err := acc.Flush(ctx, a.DB); err != nil {
		t.Fatal(err)
	}
	if acc.Pending() != 0 {
		t.Fatal("flush must clear the buffer")
	}
	var got model.User
	a.DB.First(&got, u1.ID)
	if got.U != 150*2+5 || got.D != 1000*2+5 {
		t.Fatalf("u1 u/d = %d/%d", got.U, got.D)
	}
	var got2 model.User // never reuse a struct whose primary key is set: GORM adds it as a condition
	a.DB.First(&got2, u2.ID)
	if got2.U != 0 || got2.D != 7 {
		t.Fatalf("u2 u/d = %d/%d", got2.U, got2.D)
	}

	// Same day again: upserts accumulate.
	acc.Add(1, u1.ID, 2, 1, 1)
	if err := acc.Flush(ctx, a.DB); err != nil {
		t.Fatal(err)
	}
	var su []model.StatUser
	a.DB.Where("user_id = ?", u1.ID).Order("node_id").Find(&su)
	if len(su) != 2 || su[0].U != 302 || su[0].D != 2002 || su[0].Rate != 2 || su[1].U != 5 {
		t.Fatalf("stat_users = %+v", su)
	}
	if !su[0].RecordAt.Equal(model.Day(day)) {
		t.Fatalf("record_at = %v", su[0].RecordAt)
	}
	var sn []model.StatNode
	a.DB.Order("node_id").Find(&sn)
	if len(sn) != 2 || sn[0].U != 151 || sn[0].D != 1001 || sn[1].U != 10 || sn[1].D != 17 {
		t.Fatalf("stat_nodes = %+v", sn)
	}

	// Next day creates new rows.
	acc.SetClock(func() time.Time { return day.Add(2 * time.Hour) })
	acc.Add(1, u1.ID, 1, 1, 1)
	if err := acc.Flush(ctx, a.DB); err != nil {
		t.Fatal(err)
	}
	var n int64
	a.DB.Model(&model.StatNode{}).Where("node_id = ?", 1).Count(&n)
	if n != 2 {
		t.Fatalf("stat_nodes rows for node 1 = %d", n)
	}
	if err := acc.Flush(ctx, a.DB); err != nil {
		t.Fatal("empty flush must be a no-op")
	}
}

func TestFlushFailureRestores(t *testing.T) {
	a := testutil.NewTestApp(t)
	acc := traffic.New()
	acc.Add(1, 1, 1, 10, 10)
	sqlDB, _ := a.DB.DB()
	broken, err := db.Open(a.Cfg.Database, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close(broken)
	if err := acc.Flush(context.Background(), broken); err == nil {
		t.Fatal("flush on a closed database must fail")
	}
	if acc.Pending() != 1 {
		t.Fatal("failed flush must restore buffered data")
	}
	acc.Add(1, 1, 1, 5, 5)
	if err := sqlDB.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := acc.Flush(context.Background(), a.DB); err != nil {
		t.Fatal(err)
	}
	var sn model.StatNode
	a.DB.First(&sn)
	if sn.U != 15 || sn.D != 15 {
		t.Fatalf("restored data lost: %+v", sn)
	}
}

// Review 2026-10-22: counters saturate instead of wrapping (a wrapped counter lowered users.u), the rate is
// applied with floor (docs/NODE_PROTOCOLS.md §2.3) and unknown or unbindable uids are ignored at flush.
func TestReviewFlushSaturatesAndIgnoresUnknown(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "sat@example.com")
	v := testutil.CreateUser(t, a, "floor@example.com")
	acc := traffic.New()
	acc.Add(1, u.ID, 1, math.MaxInt64, 0)
	acc.Add(1, u.ID, 1, math.MaxInt64, 0)
	acc.Add(1, v.ID, 0.5, 1, 3)
	acc.Add(1, 1<<63, 1, 10, 10)
	acc.Add(1, 999999, 1, 10, 10)
	if err := acc.Flush(context.Background(), a.DB); err != nil {
		t.Fatal(err)
	}
	if acc.Pending() != 0 {
		t.Fatal("flush must not restore anything")
	}
	var got model.User
	a.DB.First(&got, u.ID)
	if got.U != math.MaxInt64 {
		t.Fatalf("u = %d, want saturated MaxInt64", got.U)
	}
	var got2 model.User
	a.DB.First(&got2, v.ID)
	if got2.U != 0 || got2.D != 1 {
		t.Fatalf("floor(rate): u/d = %d/%d, want 0/1", got2.U, got2.D)
	}
	var n int64
	a.DB.Model(&model.StatUser{}).Count(&n)
	if n != 2 {
		t.Fatalf("stat_users rows = %d, want 2", n)
	}
}
