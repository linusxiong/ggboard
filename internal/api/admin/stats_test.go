package admin_test

import (
	"net/http"
	"testing"
	"time"
	_ "time/tzdata" // LoadLocation on hosts without a zone database (the binary embeds it as well)

	"github.com/linusxiong/ggboard/internal/model"
	statsvc "github.com/linusxiong/ggboard/internal/service/stat"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestAdminStatsEndpoints(t *testing.T) {
	e := newOpsEnv(t)
	now := time.Now()
	paid := now
	if err := e.a.DB.Create(&model.Order{
		TradeNo: "S1", UserID: e.user.ID, PlanID: 1, Type: model.OrderTypeNew, Period: model.PeriodMonthly,
		Status: model.OrderStatusCompleted, TotalAmount: 1500, PaidAt: &paid,
	}).Error; err != nil {
		t.Fatal(err)
	}
	n := &model.Node{Type: model.NodeTrojan, Name: "Edge", Host: "edge.example.com", Port: "443", ServerPort: 443, Rate: 1}
	if err := e.a.DB.Create(n).Error; err != nil {
		t.Fatal(err)
	}
	e.a.Online.TouchPush(n.ID)
	today := model.Day(now)
	if err := e.a.DB.Create(&model.StatNode{NodeID: n.ID, U: 10, D: 20, RecordAt: today}).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.a.DB.Create(&model.StatUser{UserID: e.user.ID, NodeID: n.ID, Rate: 1, U: 3, D: 4, RecordAt: today}).Error; err != nil {
		t.Fatal(err)
	}

	rec := testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/stats/overview", nil, e.adminTok)
	ov := testutil.Decode[statsvc.Overview](t, rec)
	if ov.Today.Date != statsvc.DateKey(now) || ov.TimeZone != statsvc.ZoneOf(now) {
		t.Fatalf("overview day = %q in %+v, want %s", ov.Today.Date, ov.TimeZone, statsvc.DateKey(now))
	}
	if ov.Today.Income != 1500 || ov.Today.PaidOrders != 1 || ov.Today.Registers != 3 || ov.Totals.Users != 3 || ov.Nodes != (statsvc.NodeTotals{Total: 1, Online: 1}) {
		t.Fatalf("overview = %+v", ov)
	}

	rec = testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/stats/series?metric=income&range=7d", nil, e.adminTok)
	pts := testutil.Decode[[]statsvc.Point](t, rec)
	if len(pts) != 7 || pts[6].Date != statsvc.DateKey(now) || pts[6].Value != 1500 {
		t.Fatalf("series = %+v", pts)
	}
	rec = testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/stats/series?metric=traffic", nil, e.adminTok)
	if pts := testutil.Decode[[]statsvc.Point](t, rec); len(pts) != 30 || pts[29].Value != 30 {
		t.Fatalf("default range series = %d points, last %+v", len(pts), pts[len(pts)-1])
	}
	for _, q := range []string{"", "?metric=profit", "?metric=income&range=1y"} {
		e.expect(t, http.MethodGet, "/api/v1/admin/stats/series"+q, nil, e.adminTok, http.StatusBadRequest)
	}

	rec = testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/stats/nodes/ranking", nil, e.adminTok)
	if nodes := testutil.Decode[[]statsvc.NodeRank](t, rec); len(nodes) != 1 || nodes[0].Name != "Edge" || nodes[0].Total != 30 {
		t.Fatalf("node ranking = %+v", nodes)
	}
	rec = testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/stats/nodes/ranking?date="+statsvc.DateKey(now.AddDate(0, 0, -1)), nil, e.adminTok)
	if nodes := testutil.Decode[[]statsvc.NodeRank](t, rec); len(nodes) != 0 {
		t.Fatalf("yesterday ranking = %+v", nodes)
	}
	rec = testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/stats/users/ranking?limit=5&date="+statsvc.DateKey(now), nil, e.adminTok)
	if users := testutil.Decode[[]statsvc.UserRank](t, rec); len(users) != 1 || users[0].Email != "member@example.com" || users[0].Total != 7 {
		t.Fatalf("user ranking = %+v", users)
	}
	for _, q := range []string{"/nodes/ranking?date=22-10-2026", "/users/ranking?date=yesterday", "/users/ranking?limit=501"} {
		e.expect(t, http.MethodGet, "/api/v1/admin/stats"+q, nil, e.adminTok, http.StatusBadRequest)
	}
}

// withProcessZone runs the rest of the test with loc as the process time zone (what TZ sets). Call it before
// newOpsEnv: cleanups run last-in first-out, so the app is closed before the zone is restored. Tests of this
// package do not run in parallel.
func withProcessZone(t *testing.T, loc *time.Location) {
	t.Helper()
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })
}

// The overview reports the server's statistics day and zone; the rankings default to that same day. A browser
// whose calendar day differs (the admin in New York, the server in Shanghai) uses today.date, not its own date.
func TestAdminStatsDayFollowsServerZone(t *testing.T) {
	type wire struct { // decoded by JSON name, so the test pins the wire format
		Today struct {
			Date string `json:"date"`
		} `json:"today"`
		TimeZone struct {
			Name   string `json:"name"`
			Offset int    `json:"offset"`
		} `json:"timezone"`
	}
	check := func(t *testing.T, loc *time.Location, wantName string) string {
		withProcessZone(t, loc)
		e := newOpsEnv(t)
		before := time.Now().In(loc)
		ov := testutil.Decode[wire](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/stats/overview", nil, e.adminTok))
		after := time.Now().In(loc)
		if d := ov.Today.Date; d != before.Format(time.DateOnly) && d != after.Format(time.DateOnly) {
			t.Fatalf("today.date = %q, want the %s day %s", d, loc, before.Format(time.DateOnly))
		}
		if _, off := before.Zone(); ov.TimeZone.Name != wantName || ov.TimeZone.Offset != off {
			t.Fatalf("timezone = %+v, want {%s %d}", ov.TimeZone, wantName, off)
		}

		// Traffic of that day is what the rankings show by default and for ?date=<today.date>.
		day, err := time.Parse(time.DateOnly, ov.Today.Date)
		if err != nil {
			t.Fatal(err)
		}
		n := &model.Node{Type: model.NodeTrojan, Name: "Edge", Host: "edge.example.com", Port: "443", ServerPort: 443, Rate: 1}
		if err := e.a.DB.Create(n).Error; err != nil {
			t.Fatal(err)
		}
		if err := e.a.DB.Create(&model.StatNode{NodeID: n.ID, U: 1, D: 2, RecordAt: model.Day(day)}).Error; err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{"", "?date=" + ov.Today.Date} {
			rec := testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/stats/nodes/ranking"+q, nil, e.adminTok)
			if nodes := testutil.Decode[[]statsvc.NodeRank](t, rec); len(nodes) != 1 || nodes[0].Total != 3 {
				t.Fatalf("ranking%s = %+v, want the traffic of %s", q, nodes, ov.Today.Date)
			}
		}
		return ov.Today.Date
	}

	// UTC+14 and UTC-11 are 25 hours apart, so their calendar days always differ: a date right for one zone
	// cannot also be the UTC (or test runner's) day by accident in both runs.
	days := map[string]string{}
	for _, name := range []string{"Pacific/Kiritimati", "Pacific/Pago_Pago"} {
		t.Run(name, func(t *testing.T) {
			loc, err := time.LoadLocation(name)
			if err != nil {
				t.Fatal(err)
			}
			days[name] = check(t, loc, name)
		})
	}
	if a, b := days["Pacific/Kiritimati"], days["Pacific/Pago_Pago"]; a == "" || a == b {
		t.Errorf("today.date = %q in UTC+14 and %q in UTC-11, want two different days", a, b)
	}
	// No TZ: Go names the zone read from /etc/localtime "Local"; the overview shows the offset instead.
	t.Run("Local", func(t *testing.T) {
		check(t, time.FixedZone("Local", 8*3600), "UTC+08:00")
	})
}
