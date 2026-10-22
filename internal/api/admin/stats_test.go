package admin_test

import (
	"net/http"
	"testing"
	"time"

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
