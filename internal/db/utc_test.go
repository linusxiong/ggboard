package db_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	ordersvc "github.com/linusxiong/ggboard/internal/service/order"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// inShanghai runs the test with the process time zone of the production container (TZ=Asia/Shanghai; no DST, so a
// fixed zone is exact). Tests of this package do not run in parallel.
func inShanghai(t *testing.T) {
	t.Helper()
	old := time.Local
	time.Local = time.FixedZone("CST", 8*3600)
	t.Cleanup(func() { time.Local = old })
}

// checkUTCTimes asserts what every driver must do whatever the process time zone: hand back times in UTC (so the API
// prints "…Z"), and renew a plan by adding the month in UTC. 20:00 UTC on Jan 31 is already Feb 1 in Shanghai: one
// month later is Mar 3 20:00 UTC, but Feb 28 20:00 UTC when the month is added in local time.
func checkUTCTimes(t *testing.T, a *app.App) {
	t.Helper()
	var now time.Time
	if err := a.DB.Raw("SELECT CURRENT_TIMESTAMP").Row().Scan(&now); err == nil && now.Location() != time.UTC {
		t.Errorf("%s CURRENT_TIMESTAMP = %v (%v), want UTC", a.DBDriver(), now, now.Location())
	}
	plan := model.Plan{GroupID: 1, Name: "Monthly", Prices: model.JSONObject[int64]{model.PeriodMonthly: 1000}, Show: true, Sell: true, Renew: true}
	if err := a.DB.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	group := uint64(1)
	exp := time.Date(2027, 1, 31, 20, 0, 0, 0, time.UTC)
	u := testutil.CreateUser(t, a, "renew@example.com", usersvc.CreateOptions{PlanID: &plan.ID, GroupID: &group, ExpiredAt: &exp})
	var loaded model.User
	if err := a.DB.First(&loaded, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if loaded.ExpiredAt.Location() != time.UTC || loaded.CreatedAt.Location() != time.UTC {
		t.Fatalf("%s: loaded expired_at %v, created_at %v: want UTC", a.DBDriver(), loaded.ExpiredAt, loaded.CreatedAt)
	}
	if b, _ := json.Marshal(loaded.ExpiredAt); string(b) != `"2027-01-31T20:00:00Z"` {
		t.Fatalf("%s: API form of expired_at = %s", a.DBDriver(), b)
	}

	orders := ordersvc.New(a)
	o, err := orders.Create(bg, u.ID, ordersvc.CreateInput{PlanID: plan.ID, Period: model.PeriodMonthly})
	if err != nil {
		t.Fatal(err)
	}
	if o.Type != model.OrderTypeRenew {
		t.Fatalf("order type %d, want renew", o.Type)
	}
	if _, err := orders.MarkPaid(bg, o.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.First(&loaded, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2027, 3, 3, 20, 0, 0, 0, time.UTC); !loaded.ExpiredAt.Equal(want) {
		t.Fatalf("%s: renewed expired_at %v, want %v (one month added in UTC)", a.DBDriver(), loaded.ExpiredAt.UTC(), want)
	}

	day := model.Day(time.Date(2027, 1, 31, 23, 0, 0, 0, time.UTC))
	if err := a.DB.Create(&model.StatNode{NodeID: 1, U: 1, D: 2, RecordAt: day}).Error; err != nil {
		t.Fatal(err)
	}
	var st model.StatNode
	if err := a.DB.Where("record_at = ?", day).First(&st).Error; err != nil || !st.RecordAt.Equal(day) || st.RecordAt.Location() != time.UTC {
		t.Fatalf("%s: date column = %v %v", a.DBDriver(), st.RecordAt, err)
	}
}

// The reference behaviour: SQLite (utcConnector) under TZ=Asia/Shanghai. The PostgreSQL and MySQL counterparts are in
// utc_integration_test.go.
func TestSQLiteTimesStayUTCInAnotherZone(t *testing.T) {
	inShanghai(t)
	checkUTCTimes(t, testutil.NewTestApp(t))
}
