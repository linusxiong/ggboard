package user_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/account"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const base = "/api/v1/user"

func setup(t *testing.T, opts ...testutil.Option) (*app.App, *gin.Engine) {
	t.Helper()
	a := testutil.NewTestApp(t, opts...)
	return a, testutil.NewRouterWith(t, a, user.Register)
}

func mustCreate(t *testing.T, a *app.App, v any) {
	t.Helper()
	if err := a.DB.Create(v).Error; err != nil {
		t.Fatal(err)
	}
}

func createPlan(t *testing.T, a *app.App, name string) *model.Plan {
	t.Helper()
	p := &model.Plan{GroupID: 1, Name: name, TransferEnable: 100}
	mustCreate(t, a, p)
	return p
}

func reload(t *testing.T, a *app.App, id uint64) *model.User {
	t.Helper()
	u, err := usersvc.New(a).FindByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestMe(t *testing.T) {
	a, r := setup(t)
	plan := createPlan(t, a, "Pro")
	exp := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	u := testutil.CreateUser(t, a, "Me@Example.com", usersvc.CreateOptions{
		PlanID: &plan.ID, GroupID: testutil.Ptr(uint64(1)), TransferEnable: 5 << 30, ExpiredAt: &exp,
		SpeedLimit: testutil.Ptr(50), Remarks: "vip",
	})
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{
		"u": 10, "d": 20, "balance": 300, "last_login_ip": "192.0.2.10", "telegram_id": 42,
	}).Error; err != nil {
		t.Fatal(err)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, base+"/me", nil, ""), http.StatusUnauthorized)

	rec := testutil.Do(t, r, http.MethodGet, base+"/me", nil, testutil.Login(t, a, u))
	me := testutil.Decode[account.UserView](t, rec)
	sum := md5.Sum([]byte("me@example.com"))
	if me.ID != u.ID || me.Email != "me@example.com" || me.UUID != u.UUID || me.Plan == nil || me.Plan.Name != "Pro" ||
		me.U != 10 || me.D != 20 || me.Balance != 300 || me.TransferEnable != 5<<30 || *me.SpeedLimit != 50 ||
		me.DeviceLimit != nil || me.ConnLimit != nil || !me.ExpiredAt.Equal(exp) ||
		me.AvatarURL != "https://www.gravatar.com/avatar/"+hex.EncodeToString(sum[:])+"?s=200&d=mp" {
		t.Fatalf("me = %+v", me)
	}
	raw := testutil.Decode[map[string]json.RawMessage](t, rec)
	for _, k := range []string{"token", "password_hash", "password_version", "remarks", "last_login_ip", "telegram_id", "invite_user_id"} {
		if _, ok := raw[k]; ok {
			t.Fatalf("self view exposes %q: %s", k, rec.Body.String())
		}
	}
	if string(raw["plan"]) != fmt.Sprintf(`{"id":%d,"name":"Pro"}`, plan.ID) || string(raw["next_reset_at"]) != "null" || string(raw["conn_limit"]) != "null" {
		t.Fatalf("plan = %s next_reset_at = %s conn_limit = %s", raw["plan"], raw["next_reset_at"], raw["conn_limit"])
	}

	// A deleted plan renders as null while plan_id stays.
	if err := a.DB.Delete(&model.Plan{}, plan.ID).Error; err != nil {
		t.Fatal(err)
	}
	raw = testutil.Decode[map[string]json.RawMessage](t, testutil.Do(t, r, http.MethodGet, base+"/me", nil, testutil.Login(t, a, u)))
	if string(raw["plan"]) != "null" || string(raw["plan_id"]) != fmt.Sprint(plan.ID) {
		t.Fatalf("plan = %s plan_id = %s", raw["plan"], raw["plan_id"])
	}
}

func TestUpdateMe(t *testing.T) {
	a, r := setup(t)
	u := testutil.CreateUser(t, a, "patch@example.com")
	tok := testutil.Login(t, a, u)
	if !u.RemindExpire || !u.RemindTraffic {
		t.Fatalf("defaults = %+v", u)
	}
	me := testutil.Decode[account.UserView](t, testutil.Do(t, r, http.MethodPatch, base+"/me", map[string]any{"remind_expire": false}, tok))
	if me.RemindExpire || !me.RemindTraffic {
		t.Fatalf("after patch = %+v", me)
	}
	me = testutil.Decode[account.UserView](t, testutil.Do(t, r, http.MethodPatch, base+"/me", map[string]any{"remind_traffic": false, "remind_expire": true}, tok))
	if !me.RemindExpire || me.RemindTraffic {
		t.Fatalf("after patch 2 = %+v", me)
	}
	me = testutil.Decode[account.UserView](t, testutil.Do(t, r, http.MethodPatch, base+"/me", map[string]any{}, tok))
	if !me.RemindExpire || me.RemindTraffic {
		t.Fatalf("empty patch changed = %+v", me)
	}
	if got := reload(t, a, u.ID); !got.RemindExpire || got.RemindTraffic {
		t.Fatalf("stored = %+v", got)
	}
	e := testutil.DecodeError(t, testutil.Do(t, r, http.MethodPatch, base+"/me", `{"remind_expire":"yes"}`, tok))
	if e.Status != http.StatusBadRequest || len(e.Errors["remind_expire"]) == 0 {
		t.Fatalf("bad type = %+v", e)
	}
}

func TestChangePassword(t *testing.T) {
	a, r := setup(t, testutil.WithSettings(map[string]any{"password_limit_count": 2}))
	u := testutil.CreateUser(t, a, "pw@example.com")
	tok := testutil.Login(t, a, u)

	e := testutil.DecodeError(t, testutil.Do(t, r, http.MethodPost, base+"/password", map[string]any{"old_password": testutil.Password, "new_password": "short"}, tok))
	if e.Status != http.StatusBadRequest || len(e.Errors["new_password"]) == 0 {
		t.Fatalf("short = %+v", e)
	}
	e = testutil.DecodeError(t, testutil.Do(t, r, http.MethodPost, base+"/password", map[string]any{"old_password": "wrong-pass", "new_password": "new-password-1"}, tok))
	if e.Status != http.StatusBadRequest || len(e.Errors["old_password"]) == 0 {
		t.Fatalf("wrong old = %+v", e)
	}
	if ok := testutil.Decode[bool](t, testutil.Do(t, r, http.MethodPost, base+"/password",
		map[string]any{"old_password": testutil.Password, "new_password": "new-password-1"}, tok)); !ok {
		t.Fatal("password = false")
	}
	got := reload(t, a, u.ID)
	if got.PasswordVersion != u.PasswordVersion+1 || !auth.VerifyPassword(got.PasswordHash, "new-password-1") {
		t.Fatal("password not changed")
	}
	// The old session is gone; a new one works.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, base+"/me", nil, tok), http.StatusUnauthorized)
	tok = testutil.Login(t, a, got)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, base+"/me", nil, tok), http.StatusOK)

	// Wrong old passwords count towards the password lockout (the success above cleared earlier failures).
	for range 2 {
		testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/password", map[string]any{"old_password": "wrong-pass", "new_password": "new-password-2"}, tok), http.StatusBadRequest)
	}
	rec := testutil.Do(t, r, http.MethodPost, base+"/password", map[string]any{"old_password": "new-password-1", "new_password": "new-password-2"}, tok)
	if e := testutil.DecodeError(t, rec); e.Status != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("locked = %+v", e)
	}
}

func order(userID uint64, inviter *uint64, status model.OrderStatus, cs model.CommissionStatus, commission int64) *model.Order {
	return &model.Order{
		TradeNo: fmt.Sprintf("T%d%d%d%d%d", userID, status, cs, commission, time.Now().UnixNano()), UserID: userID, PlanID: 1,
		InviteUserID: inviter, Type: model.OrderTypeNew, Period: model.PeriodMonthly, Status: status,
		CommissionStatus: cs, CommissionBalance: commission,
	}
}

func TestStats(t *testing.T) {
	a, r := setup(t)
	me := testutil.CreateUser(t, a, "me@example.com")
	other := testutil.CreateUser(t, a, "other@example.com")
	testutil.CreateUser(t, a, "invitee1@example.com", usersvc.CreateOptions{InviteUserID: &me.ID})
	invitee := testutil.CreateUser(t, a, "invitee2@example.com", usersvc.CreateOptions{InviteUserID: &me.ID})
	testutil.CreateUser(t, a, "stranger@example.com", usersvc.CreateOptions{InviteUserID: &other.ID})

	for _, o := range []*model.Order{
		order(me.ID, nil, model.OrderStatusPending, 0, 0),
		order(me.ID, nil, model.OrderStatusPending, 0, 0),
		order(me.ID, nil, model.OrderStatusCompleted, 0, 0),
		order(other.ID, nil, model.OrderStatusPending, 0, 0),
		// Commission of orders invited by me.
		order(invitee.ID, &me.ID, model.OrderStatusCompleted, model.CommissionStatusPending, 100),
		order(invitee.ID, &me.ID, model.OrderStatusProcessing, model.CommissionStatusProcessing, 50),
		order(invitee.ID, &me.ID, model.OrderStatusCompleted, model.CommissionStatusValid, 999),
		order(invitee.ID, &me.ID, model.OrderStatusCompleted, model.CommissionStatusInvalid, 888),
		order(invitee.ID, &me.ID, model.OrderStatusCancelled, model.CommissionStatusPending, 555),
		order(invitee.ID, &me.ID, model.OrderStatusPending, model.CommissionStatusPending, 777),
		order(invitee.ID, &other.ID, model.OrderStatusCompleted, model.CommissionStatusPending, 444),
	} {
		mustCreate(t, a, o)
	}
	for _, tk := range []*model.Ticket{
		{UserID: me.ID, Subject: "open", Status: model.TicketStatusOpen},
		{UserID: me.ID, Subject: "closed", Status: model.TicketStatusClosed},
		{UserID: other.ID, Subject: "other", Status: model.TicketStatusOpen},
	} {
		mustCreate(t, a, tk)
	}
	for _, n := range []*model.Notice{{Title: "a", Show: true}, {Title: "b", Show: true}, {Title: "hidden"}} {
		mustCreate(t, a, n)
	}
	st := testutil.Decode[account.Stats](t, testutil.Do(t, r, http.MethodGet, base+"/stats", nil, testutil.Login(t, a, me)))
	want := account.Stats{PendingOrders: 2, OpenTickets: 1, NoticesCount: 2, InviteCount: 2, CommissionPending: 150}
	if st != want {
		t.Fatalf("stats = %+v, want %+v", st, want)
	}
}

func TestSubscription(t *testing.T) {
	a, r := setup(t, testutil.WithSettings(map[string]any{"subscribe_url": []string{"https://sub.example.com/"}}))
	plan := createPlan(t, a, "Basic")
	exp := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	next := time.Now().Add(36 * time.Hour).UTC().Truncate(time.Second)
	u := testutil.CreateUser(t, a, "sub@example.com", usersvc.CreateOptions{PlanID: &plan.ID, TransferEnable: 1 << 30, ExpiredAt: &exp, DeviceLimit: testutil.Ptr(3),
		ConnLimit: testutil.Ptr(64)})
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{"next_reset_at": next, "u": 5, "d": 7}).Error; err != nil {
		t.Fatal(err)
	}
	sub := testutil.Decode[user.Subscription](t, testutil.Do(t, r, http.MethodGet, base+"/subscription", nil, testutil.Login(t, a, u)))
	if sub.SubscribeURL != "https://sub.example.com"+usersvc.SubscribePath+u.Token || sub.Token != u.Token {
		t.Fatalf("url = %q token = %q", sub.SubscribeURL, sub.Token)
	}
	if sub.Plan == nil || sub.Plan.ID != plan.ID || sub.Plan.Name != "Basic" || sub.TransferEnable != 1<<30 || sub.U != 5 || sub.D != 7 ||
		!sub.ExpiredAt.Equal(exp) || !sub.NextResetAt.Equal(next) || sub.ResetDay == nil || *sub.ResetDay != 2 ||
		sub.DeviceLimit == nil || *sub.DeviceLimit != 3 || sub.ConnLimit == nil || *sub.ConnLimit != 64 || sub.SpeedLimit != nil {
		t.Fatalf("sub = %+v", sub)
	}

	// No plan, no reset date.
	v := testutil.CreateUser(t, a, "free@example.com")
	raw := testutil.Decode[map[string]json.RawMessage](t, testutil.Do(t, r, http.MethodGet, base+"/subscription", nil, testutil.Login(t, a, v)))
	for _, k := range []string{"plan", "expired_at", "next_reset_at", "reset_day", "device_limit", "conn_limit", "speed_limit"} {
		if string(raw[k]) != "null" {
			t.Fatalf("%s = %s", k, raw[k])
		}
	}
}

func TestSubscriptionReset(t *testing.T) {
	a, r := setup(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	u := testutil.CreateUser(t, a, "reset@example.com")
	tok := testutil.Login(t, a, u)
	res := testutil.Decode[user.SubscriptionReset](t, testutil.Do(t, r, http.MethodPost, base+"/subscription/reset", nil, tok))
	got := reload(t, a, u.ID)
	if res.Token == u.Token || res.Token != got.Token || got.UUID == u.UUID || got.PasswordVersion != u.PasswordVersion+1 {
		t.Fatalf("reset = %+v stored = %+v", res, got)
	}
	if res.SubscribeURL != "https://panel.example.com"+usersvc.SubscribePath+got.Token {
		t.Fatalf("subscribe_url = %q", res.SubscribeURL)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, base+"/me", nil, tok), http.StatusUnauthorized)
	me := testutil.Decode[account.UserView](t, testutil.Do(t, r, http.MethodGet, base+"/me", nil, res.AuthToken))
	if me.UUID != got.UUID {
		t.Fatalf("uuid = %s, want %s", me.UUID, got.UUID)
	}
}

func TestTrafficLogs(t *testing.T) {
	a, r := setup(t)
	u := testutil.CreateUser(t, a, "traffic@example.com")
	other := testutil.CreateUser(t, a, "other@example.com")
	node := &model.Node{Type: model.NodeVless, Name: "Tokyo 1", Host: "node.example.com", Port: "443", ServerPort: 443, Rate: 1.5}
	mustCreate(t, a, node)
	day := func(n int) time.Time { return model.Day(time.Date(2026, 10, n, 12, 0, 0, 0, time.UTC)) }
	for _, s := range []*model.StatUser{
		{UserID: u.ID, NodeID: node.ID, Rate: 1.5, U: 1, D: 2, RecordAt: day(1)},
		{UserID: u.ID, NodeID: node.ID, Rate: 1.5, U: 3, D: 4, RecordAt: day(2)},
		{UserID: u.ID, NodeID: 0, Rate: 1, U: 5, D: 6, RecordAt: day(2)},
		{UserID: u.ID, NodeID: 999, Rate: 2, U: 7, D: 8, RecordAt: day(3)},
		{UserID: other.ID, NodeID: node.ID, Rate: 1.5, U: 9, D: 9, RecordAt: day(3)},
	} {
		mustCreate(t, a, s)
	}
	tok := testutil.Login(t, a, u)
	rec := testutil.Do(t, r, http.MethodGet, base+"/traffic-logs", nil, tok)
	logs, total := testutil.DecodeList[user.TrafficLog](t, rec)
	if total != 4 || len(logs) != 4 {
		t.Fatalf("total = %d len = %d", total, len(logs))
	}
	if logs[0].RecordAt != "2026-10-03" || logs[0].NodeID != 999 || logs[0].NodeName != nil || logs[0].Rate != 2 || logs[0].U != 7 {
		t.Fatalf("logs[0] = %+v", logs[0])
	}
	if logs[1].RecordAt != "2026-10-02" || logs[1].NodeID != 0 || logs[1].NodeName != nil {
		t.Fatalf("logs[1] = %+v", logs[1])
	}
	if logs[2].RecordAt != "2026-10-02" || logs[2].NodeName == nil || *logs[2].NodeName != "Tokyo 1" || logs[2].Rate != 1.5 || logs[2].D != 4 {
		t.Fatalf("logs[2] = %+v", logs[2])
	}
	if logs[3].RecordAt != "2026-10-01" {
		t.Fatalf("logs[3] = %+v", logs[3])
	}
	if !strings.Contains(rec.Body.String(), `"node_name":null`) {
		t.Fatalf("node_name must be null: %s", rec.Body.String())
	}

	page, total := testutil.DecodeList[user.TrafficLog](t, testutil.Do(t, r, http.MethodGet, base+"/traffic-logs?page=2&page_size=3", nil, tok))
	if total != 4 || len(page) != 1 || page[0].RecordAt != "2026-10-01" {
		t.Fatalf("page 2 = %+v (%d)", page, total)
	}
	page, total = testutil.DecodeList[user.TrafficLog](t, testutil.Do(t, r, http.MethodGet, base+"/traffic-logs?page=9", nil, tok))
	if total != 4 || len(page) != 0 {
		t.Fatalf("page 9 = %+v (%d)", page, total)
	}
	empty, total := testutil.DecodeList[user.TrafficLog](t, testutil.Do(t, r, http.MethodGet, base+"/traffic-logs", nil,
		testutil.Login(t, a, testutil.CreateUser(t, a, "nolog@example.com"))))
	if total != 0 || empty == nil || len(empty) != 0 {
		t.Fatalf("empty = %+v (%d)", empty, total)
	}
}

func TestLogout(t *testing.T) {
	a, r := setup(t)
	u := testutil.CreateUser(t, a, "bye@example.com")
	if ok := testutil.Decode[bool](t, testutil.Do(t, r, http.MethodPost, base+"/logout", nil, testutil.Login(t, a, u))); !ok {
		t.Fatal("logout = false")
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/logout", nil, ""), http.StatusUnauthorized)
}
