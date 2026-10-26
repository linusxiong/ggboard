package statsvc_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	statsvc "github.com/linusxiong/ggboard/internal/service/stat"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

var tradeSeq atomic.Int64

func at(day, hour int) time.Time { return time.Date(2026, 10, day, hour, 0, 0, 0, time.Local) }

func mustCreate(t *testing.T, a *app.App, v any) {
	t.Helper()
	if err := a.DB.Create(v).Error; err != nil {
		t.Fatalf("create %T: %v", v, err)
	}
}

type orderOpt struct {
	status     model.OrderStatus
	total      int64
	commission int64
	paidAt     *time.Time
	createdAt  time.Time
	inviter    *uint64
}

func order(t *testing.T, a *app.App, userID uint64, o orderOpt) *model.Order {
	t.Helper()
	ord := &model.Order{
		TradeNo: fmt.Sprintf("T%020d", tradeSeq.Add(1)), UserID: userID, PlanID: 1, Type: model.OrderTypeNew,
		Period: model.PeriodMonthly, Status: o.status, TotalAmount: o.total, CommissionBalance: o.commission,
		PaidAt: o.paidAt, CreatedAt: o.createdAt, InviteUserID: o.inviter,
	}
	mustCreate(t, a, ord)
	return ord
}

func setCreated(t *testing.T, a *app.App, u *model.User, ts time.Time) {
	t.Helper()
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).UpdateColumn("created_at", ts).Error; err != nil {
		t.Fatal(err)
	}
}

func node(t *testing.T, a *app.App, name string, parent *uint64) *model.Node {
	t.Helper()
	n := &model.Node{Type: model.NodeVmess, Name: name, Host: "node.example.com", Port: "443", ServerPort: 443, Rate: 1, ParentID: parent}
	mustCreate(t, a, n)
	return n
}

func TestOverview(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	now := at(22, 12)
	future := now.Add(30 * 24 * time.Hour)
	past := now.Add(-time.Hour)

	alice := testutil.CreateUser(t, a, "alice@example.com", usersvc.CreateOptions{TransferEnable: 10 << 30, ExpiredAt: &future})
	bob := testutil.CreateUser(t, a, "bob@example.com", usersvc.CreateOptions{TransferEnable: 10 << 30})
	carol := testutil.CreateUser(t, a, "carol@example.com", usersvc.CreateOptions{TransferEnable: 10 << 30, ExpiredAt: &past})
	dave := testutil.CreateUser(t, a, "dave@example.com") // no traffic
	if err := a.DB.Model(bob).UpdateColumn("banned", true).Error; err != nil {
		t.Fatal(err)
	}
	setCreated(t, a, alice, at(22, 8))
	setCreated(t, a, bob, at(21, 10))
	setCreated(t, a, carol, time.Date(2026, 9, 15, 10, 0, 0, 0, time.Local))
	setCreated(t, a, dave, at(22, 1))

	p := func(t time.Time) *time.Time { return &t }
	inviter := alice.ID
	order(t, a, bob.ID, orderOpt{status: model.OrderStatusCompleted, total: 1000, commission: 100, paidAt: p(at(22, 9)), createdAt: at(22, 9), inviter: &inviter})
	order(t, a, bob.ID, orderOpt{status: model.OrderStatusProcessing, total: 500, paidAt: p(at(22, 10)), createdAt: at(22, 10)})
	order(t, a, bob.ID, orderOpt{status: model.OrderStatusDiscounted, total: 2000, paidAt: p(at(5, 10)), createdAt: at(5, 10)})
	order(t, a, bob.ID, orderOpt{status: model.OrderStatusCancelled, total: 700, paidAt: p(at(22, 11)), createdAt: at(22, 11)})
	order(t, a, bob.ID, orderOpt{status: model.OrderStatusPending, total: 300, createdAt: at(22, 11)})
	order(t, a, bob.ID, orderOpt{status: model.OrderStatusCompleted, total: 9999, paidAt: p(time.Date(2026, 9, 30, 23, 0, 0, 0, time.Local)), createdAt: at(1, 1)})

	mustCreate(t, a, &model.Ticket{UserID: bob.ID, Subject: "awaiting", Status: model.TicketStatusOpen, ReplyStatus: model.TicketAwaitingStaff})
	mustCreate(t, a, &model.Ticket{UserID: bob.ID, Subject: "replied", Status: model.TicketStatusOpen, ReplyStatus: model.TicketReplied})
	mustCreate(t, a, &model.Ticket{UserID: bob.ID, Subject: "closed", Status: model.TicketStatusClosed, ReplyStatus: model.TicketAwaitingStaff})

	n1 := node(t, a, "pushing", nil)
	node(t, a, "child", &n1.ID)
	n3 := node(t, a, "idle", nil)
	node(t, a, "never", nil)
	n5 := node(t, a, "stale", nil)
	a.Online.SetClock(func() time.Time { return now.Add(-time.Hour) })
	a.Online.Touch(n5.ID)
	a.Online.SetClock(func() time.Time { return now })
	a.Online.TouchPush(n1.ID)
	a.Online.Touch(n3.ID)
	a.Online.SetAliveIPs(n1.ID, alice.ID, []string{"192.0.2.1"})

	got, err := statsvc.New(a).Overview(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	want := statsvc.Overview{
		Today:    statsvc.TodayStats{Date: "2026-10-22", Income: 1500, PaidOrders: 2, Registers: 2, Commission: 100},
		Month:    statsvc.PeriodStats{Income: 3500, PaidOrders: 3, Registers: 3},
		Totals:   statsvc.Totals{Users: 4, ActiveUsers: 1, OnlineUsers: 1, PendingTickets: 1, PendingOrders: 1, PendingCommission: 1},
		Nodes:    statsvc.NodeTotals{Total: 5, Online: 3},
		TimeZone: statsvc.ZoneOf(now),
	}
	if *got != want {
		t.Fatalf("overview =\n%+v\nwant\n%+v", *got, want)
	}
}

func TestSeries(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	now := at(22, 12)
	u := testutil.CreateUser(t, a, "buyer@example.com")
	setCreated(t, a, u, at(21, 9))
	for _, ts := range []time.Time{at(21, 20), at(22, 3)} {
		v := testutil.CreateUser(t, a, fmt.Sprintf("r%d@example.com", ts.Unix()))
		setCreated(t, a, v, ts)
	}
	p := func(t time.Time) *time.Time { return &t }
	order(t, a, u.ID, orderOpt{status: model.OrderStatusCompleted, total: 5, paidAt: p(at(20, 10)), createdAt: at(20, 10)}) // has a stats row
	order(t, a, u.ID, orderOpt{status: model.OrderStatusCompleted, total: 300, paidAt: p(at(21, 0)), createdAt: at(21, 0)})
	order(t, a, u.ID, orderOpt{status: model.OrderStatusProcessing, total: 200, paidAt: p(at(21, 23)), createdAt: at(21, 23)})
	order(t, a, u.ID, orderOpt{status: model.OrderStatusCompleted, total: 100, paidAt: p(at(22, 1)), createdAt: at(22, 1)})
	order(t, a, u.ID, orderOpt{status: model.OrderStatusCompleted, total: 1, paidAt: p(at(15, 1)), createdAt: at(15, 1)}) // out of range
	mustCreate(t, a, &model.Stat{RecordAt: model.Day(at(20, 0)), PaidTotal: 777, PaidCount: 7, RegisterCount: 3})
	mustCreate(t, a, &model.Stat{RecordAt: model.Day(at(22, 0)), PaidTotal: 99999, PaidCount: 99, RegisterCount: 99}) // today is always live
	for _, sn := range []model.StatNode{
		{NodeID: 1, U: 100, D: 200, RecordAt: model.Day(at(21, 0))},
		{NodeID: 2, U: 50, D: 50, RecordAt: model.Day(at(21, 0))},
		{NodeID: 1, U: 1, D: 2, RecordAt: model.Day(at(22, 0))},
		{NodeID: 1, U: 9, D: 9, RecordAt: model.Day(at(10, 0))}, // out of range
	} {
		mustCreate(t, a, &sn)
	}

	dates := []string{"2026-10-16", "2026-10-17", "2026-10-18", "2026-10-19", "2026-10-20", "2026-10-21", "2026-10-22"}
	tests := []struct {
		metric string
		want   []int64
	}{
		{statsvc.MetricIncome, []int64{0, 0, 0, 0, 777, 500, 100}},
		{statsvc.MetricOrders, []int64{0, 0, 0, 0, 7, 2, 1}},
		{statsvc.MetricRegisters, []int64{0, 0, 0, 0, 3, 2, 1}},
		{statsvc.MetricTraffic, []int64{0, 0, 0, 0, 0, 400, 3}},
	}
	for _, tc := range tests {
		t.Run(tc.metric, func(t *testing.T) {
			pts, err := statsvc.New(a).Series(ctx, tc.metric, 7, now)
			if err != nil {
				t.Fatal(err)
			}
			if len(pts) != len(dates) {
				t.Fatalf("len = %d", len(pts))
			}
			for i, p := range pts {
				if p.Date != dates[i] || p.Value != tc.want[i] {
					t.Errorf("point %d = %+v, want {%s %d}", i, p, dates[i], tc.want[i])
				}
			}
		})
	}
	if _, err := statsvc.New(a).Series(ctx, "bogus", 7, now); err == nil {
		t.Fatal("unknown metric accepted")
	}
	pts, err := statsvc.New(a).Series(ctx, statsvc.MetricIncome, 90, now)
	if err != nil || len(pts) != 90 || pts[89].Date != "2026-10-22" || pts[0].Date != "2026-07-25" {
		t.Fatalf("90d series: %v %d %+v", err, len(pts), pts[0])
	}
}

func TestRankings(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	day := at(21, 0)
	n1 := node(t, a, "Tokyo", nil)
	n2 := node(t, a, "Osaka", nil)
	u1 := testutil.CreateUser(t, a, "heavy@example.com")
	u2 := testutil.CreateUser(t, a, "light@example.com")
	for _, sn := range []model.StatNode{
		{NodeID: n1.ID, U: 10, D: 20, RecordAt: model.Day(day)},
		{NodeID: n2.ID, U: 100, D: 200, RecordAt: model.Day(day)},
		{NodeID: 999, U: 1, D: 1, RecordAt: model.Day(day)},
		{NodeID: n1.ID, U: 1000, D: 1000, RecordAt: model.Day(at(22, 0))},
	} {
		mustCreate(t, a, &sn)
	}
	for _, su := range []model.StatUser{
		{UserID: u1.ID, NodeID: n1.ID, Rate: 1, U: 100, D: 100, RecordAt: model.Day(day)},
		{UserID: u1.ID, NodeID: n2.ID, Rate: 1, U: 50, D: 50, RecordAt: model.Day(day)},
		{UserID: u2.ID, NodeID: n1.ID, Rate: 1, U: 1, D: 2, RecordAt: model.Day(day)},
		{UserID: 4242, NodeID: n1.ID, Rate: 1, U: 5, D: 5, RecordAt: model.Day(day)},
		{UserID: u2.ID, NodeID: n1.ID, Rate: 1, U: 9000, D: 9000, RecordAt: model.Day(at(20, 0))},
	} {
		mustCreate(t, a, &su)
	}

	nodes, err := statsvc.New(a).NodeRanking(ctx, day)
	if err != nil {
		t.Fatal(err)
	}
	wantNodes := []statsvc.NodeRank{
		{NodeID: n2.ID, Name: "Osaka", U: 100, D: 200, Total: 300},
		{NodeID: n1.ID, Name: "Tokyo", U: 10, D: 20, Total: 30},
		{NodeID: 999, Name: "", U: 1, D: 1, Total: 2},
	}
	if fmt.Sprint(nodes) != fmt.Sprint(wantNodes) {
		t.Fatalf("nodes = %+v, want %+v", nodes, wantNodes)
	}

	users, err := statsvc.New(a).UserRanking(ctx, day, 2)
	if err != nil {
		t.Fatal(err)
	}
	wantUsers := []statsvc.UserRank{
		{UserID: u1.ID, Email: "heavy@example.com", U: 150, D: 150, Total: 300},
		{UserID: 4242, Email: "", U: 5, D: 5, Total: 10},
	}
	if fmt.Sprint(users) != fmt.Sprint(wantUsers) {
		t.Fatalf("users = %+v, want %+v", users, wantUsers)
	}
	empty, err := statsvc.New(a).NodeRanking(ctx, at(1, 0))
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty ranking = %#v, %v", empty, err)
	}
}

func TestSaveDayIsIdempotent(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	day := at(21, 0)
	inviter := testutil.CreateUser(t, a, "inviter@example.com")
	invited := testutil.CreateUser(t, a, "invited@example.com", usersvc.CreateOptions{InviteUserID: &inviter.ID})
	plain := testutil.CreateUser(t, a, "plain@example.com")
	setCreated(t, a, invited, at(21, 10))
	setCreated(t, a, plain, at(21, 23))
	setCreated(t, a, inviter, at(20, 23))
	p := func(t time.Time) *time.Time { return &t }
	order(t, a, plain.ID, orderOpt{status: model.OrderStatusPending, total: 50, createdAt: at(21, 5)})
	order(t, a, plain.ID, orderOpt{status: model.OrderStatusCompleted, total: 400, paidAt: p(at(21, 6)), createdAt: at(21, 6)})
	order(t, a, plain.ID, orderOpt{status: model.OrderStatusCompleted, total: 900, paidAt: p(at(21, 1)), createdAt: at(20, 22)})
	mustCreate(t, a, &model.CommissionLog{InviteUserID: inviter.ID, UserID: invited.ID, TradeNo: "C1", OrderAmount: 400, GetAmount: 40, CreatedAt: at(21, 7)})
	mustCreate(t, a, &model.CommissionLog{InviteUserID: inviter.ID, UserID: invited.ID, TradeNo: "C2", OrderAmount: 400, GetAmount: 99, CreatedAt: at(22, 7)})
	mustCreate(t, a, &model.StatNode{NodeID: 1, U: 10, D: 5, RecordAt: model.Day(day)})
	mustCreate(t, a, &model.StatNode{NodeID: 2, U: 1, D: 1, RecordAt: model.Day(day)})

	svc := statsvc.New(a)
	want := model.Stat{
		RecordAt: model.Day(day), OrderCount: 2, OrderTotal: 450, PaidCount: 2, PaidTotal: 1300,
		CommissionCount: 1, CommissionTotal: 40, RegisterCount: 2, InviteCount: 1, TransferUsedTotal: 17,
	}
	check := func(want model.Stat) {
		t.Helper()
		var rows []model.Stat
		if err := a.DB.Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("rows = %d", len(rows))
		}
		got := rows[0]
		got.ID, want.ID = 0, 0
		got.RecordAt, want.RecordAt = got.RecordAt.UTC(), want.RecordAt.UTC()
		if got != want {
			t.Fatalf("stat =\n%+v\nwant\n%+v", got, want)
		}
	}
	for range 2 {
		if _, err := svc.SaveDay(ctx, at(21, 15)); err != nil {
			t.Fatal(err)
		}
		check(want)
	}
	order(t, a, plain.ID, orderOpt{status: model.OrderStatusCompleted, total: 100, paidAt: p(at(21, 22)), createdAt: at(21, 22)})
	if _, err := svc.SaveDay(ctx, day); err != nil {
		t.Fatal(err)
	}
	want.OrderCount, want.OrderTotal, want.PaidCount, want.PaidTotal = 3, 550, 3, 1400
	check(want)
}
