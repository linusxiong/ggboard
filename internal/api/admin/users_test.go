package admin_test

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/adminuser"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func usersPath(id uint64, suffix string) string {
	return "/api/v1/admin/users/" + strconv.FormatUint(id, 10) + suffix
}

func opsGroupPlan(t *testing.T, e *opsEnv, name string, gb int64, method *model.ResetMethod) (*model.ServerGroup, *model.Plan) {
	t.Helper()
	g := &model.ServerGroup{Name: name}
	if err := e.a.DB.Create(g).Error; err != nil {
		t.Fatal(err)
	}
	p := &model.Plan{GroupID: g.ID, Name: name, TransferEnable: gb, SpeedLimit: testutil.Ptr(100), DeviceLimit: testutil.Ptr(3), ResetTrafficMethod: method}
	if err := e.a.DB.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	return g, p
}

func opsEmails(vs []adminuser.View) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Email
	}
	return out
}

func TestAdminUsersListFilters(t *testing.T) {
	e := newOpsEnv(t)
	g, p := opsGroupPlan(t, e, "Pro", 10, nil)
	future, past := time.Now().Add(48*time.Hour), time.Now().Add(-48*time.Hour)
	alice := testutil.CreateUser(t, e.a, "alice@example.com", usersvc.CreateOptions{
		PlanID: &p.ID, GroupID: &g.ID, ExpiredAt: &future, InviteUserID: &e.user.ID, TransferEnable: 10 << 30, Remarks: "VIP customer", Balance: 100,
	})
	if err := e.a.DB.Model(alice).UpdateColumn("u", 5).Error; err != nil {
		t.Fatal(err)
	}
	bob := testutil.CreateUser(t, e.a, "bob@example.com", usersvc.CreateOptions{ExpiredAt: &past})
	if err := e.a.DB.Model(bob).UpdateColumn("banned", true).Error; err != nil {
		t.Fatal(err)
	}
	testutil.CreateUser(t, e.a, "carol@example.org")

	list := func(query string) ([]adminuser.View, int64) {
		t.Helper()
		rec := testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/users?"+query, nil, e.adminTok)
		return testutil.DecodeList[adminuser.View](t, rec)
	}
	all := []string{"root@example.com", "staff@example.com", "member@example.com", "alice@example.com", "bob@example.com", "carol@example.org"}
	tests := []struct {
		query string
		want  []string
	}{
		{"sort=id", all},
		{"sort=id&email=ALICE", []string{"alice@example.com"}},
		{"sort=id&email=example.org", []string{"carol@example.org"}},
		{"sort=id&email=" + url.QueryEscape("_"), nil},
		{"sort=id&plan_id=" + strconv.FormatUint(p.ID, 10), []string{"alice@example.com"}},
		{"sort=id&plan_id=0", slices.DeleteFunc(slices.Clone(all), func(s string) bool { return s == "alice@example.com" })},
		{"sort=id&group_id=" + strconv.FormatUint(g.ID, 10), []string{"alice@example.com"}},
		{"sort=id&banned=true", []string{"bob@example.com"}},
		{"sort=id&is_admin=true", []string{"root@example.com"}},
		{"sort=id&is_staff=1", []string{"staff@example.com"}},
		{"sort=id&invite_user_id=" + strconv.FormatUint(e.user.ID, 10), []string{"alice@example.com"}},
		{"sort=id&expired=active", []string{"alice@example.com"}},
		{"sort=id&expired=expired", []string{"bob@example.com"}},
		{"sort=id&expired=never", []string{"root@example.com", "staff@example.com", "member@example.com", "carol@example.org"}},
		{"sort=id&id=" + strconv.FormatUint(alice.ID, 10), []string{"alice@example.com"}},
		{"sort=id&uuid=" + alice.UUID, []string{"alice@example.com"}},
		{"sort=id&token=" + bob.Token, []string{"bob@example.com"}},
		{"sort=id&remarks=vip", []string{"alice@example.com"}},
		{"sort=email", []string{"alice@example.com", "bob@example.com", "carol@example.org", "member@example.com", "root@example.com", "staff@example.com"}},
		{"sort=-u,id", []string{"alice@example.com", "root@example.com", "staff@example.com", "member@example.com", "bob@example.com", "carol@example.org"}},
		{"sort=-balance,-id&page_size=2", []string{"alice@example.com", "carol@example.org"}},
		{"sort=id&page=2&page_size=4", []string{"bob@example.com", "carol@example.org"}},
		{"sort=id&page=9", nil},
		{"", []string{"carol@example.org", "bob@example.com", "alice@example.com", "member@example.com", "staff@example.com", "root@example.com"}},
	}
	for _, tc := range tests {
		views, total := list(tc.query)
		if got := opsEmails(views); !slices.Equal(got, tc.want) && (len(got) != 0 || len(tc.want) != 0) {
			t.Errorf("%q = %v, want %v", tc.query, got, tc.want)
		}
		if strings.Contains(tc.query, "page") && total != int64(len(all)) {
			t.Errorf("%q total = %d", tc.query, total)
		}
	}
	e.expect(t, http.MethodGet, "/api/v1/admin/users?expired=soon", nil, e.adminTok, http.StatusBadRequest)

	views, _ := list("id=" + strconv.FormatUint(alice.ID, 10))
	v := views[0]
	if v.Plan == nil || v.Plan.Name != "Pro" || v.InviteUserEmail == nil || *v.InviteUserEmail != "member@example.com" ||
		v.Remarks != "VIP customer" || v.Token != alice.Token || v.UUID != alice.UUID || !strings.HasPrefix(v.AvatarURL, "https://www.gravatar.com/avatar/") {
		t.Fatalf("admin view = %+v", v)
	}
	raw := e.expect(t, http.MethodGet, "/api/v1/admin/users?id="+strconv.FormatUint(alice.ID, 10), nil, e.adminTok, http.StatusOK)
	if strings.Contains(string(raw), "password") {
		t.Fatalf("view leaks password fields: %s", raw)
	}

	// Staff read the same view without credentials.
	rec := testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/users?id="+strconv.FormatUint(alice.ID, 10), nil, e.staffTok)
	staffViews, _ := testutil.DecodeList[adminuser.View](t, rec)
	if len(staffViews) != 1 || staffViews[0].UUID != "" || staffViews[0].Token != "" || staffViews[0].Email != "alice@example.com" {
		t.Fatalf("staff view = %+v", staffViews)
	}
}

func TestAdminUsersCreate(t *testing.T) {
	e := newOpsEnv(t)
	g, p := opsGroupPlan(t, e, "Basic", 50, nil)
	exp := time.Now().Add(90 * 24 * time.Hour).UTC().Truncate(time.Second)

	rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users", map[string]any{
		"email": "New@Example.com", "password": "password123", "plan_id": p.ID, "expired_at": exp, "remarks": "from test",
		"invite_user_id": e.user.ID, "balance": 250,
	}, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	v := testutil.Decode[adminuser.View](t, rec)
	if v.Email != "new@example.com" || v.GroupID == nil || *v.GroupID != g.ID || v.TransferEnable != 50<<30 ||
		v.SpeedLimit == nil || *v.SpeedLimit != 100 || v.DeviceLimit == nil || *v.DeviceLimit != 3 || v.Balance != 250 ||
		v.NextResetAt == nil || v.ExpiredAt == nil || !v.ExpiredAt.Equal(exp) || v.Plan == nil || v.Plan.ID != p.ID ||
		v.InviteUserEmail == nil || *v.InviteUserEmail != "member@example.com" || v.Remarks != "from test" {
		t.Fatalf("created = %+v", v)
	}
	if u := e.reload(t, v.ID); !auth.VerifyPassword(u.PasswordHash, "password123") || u.NextResetAt == nil {
		t.Fatalf("stored user = %+v", u)
	}

	rec = testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users", map[string]any{
		"email": "gb@example.com", "password": "password123", "transfer_enable": 1.5, "speed_limit": 0, "is_staff": true,
	}, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	v = testutil.Decode[adminuser.View](t, rec)
	if v.TransferEnable != 3<<29 || v.PlanID != nil || v.GroupID != nil || v.NextResetAt != nil || !v.IsStaff || v.SpeedLimit == nil || *v.SpeedLimit != 0 {
		t.Fatalf("created without plan = %+v", v)
	}

	errs := []struct {
		body   map[string]any
		status int
		field  string
	}{
		{map[string]any{"email": "new@example.com", "password": "password123"}, http.StatusConflict, "email"},
		{map[string]any{"email": "x@example.com", "password": "short"}, http.StatusBadRequest, "password"},
		{map[string]any{"email": "not-an-email", "password": "password123"}, http.StatusBadRequest, "email"},
		{map[string]any{"email": "x@example.com", "password": "password123", "plan_id": 999}, http.StatusBadRequest, "plan_id"},
		{map[string]any{"email": "x@example.com", "password": "password123", "group_id": 999}, http.StatusBadRequest, "group_id"},
		{map[string]any{"email": "x@example.com", "password": "password123", "invite_user_id": 999}, http.StatusBadRequest, "invite_user_id"},
		{map[string]any{"email": "x@example.com", "password": "password123", "transfer_enable": -1}, http.StatusBadRequest, "transfer_enable"},
		{map[string]any{"email": "x@example.com", "password": "password123", "balance": -1}, http.StatusBadRequest, "balance"},
	}
	for _, tc := range errs {
		rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users", tc.body, e.adminTok)
		testutil.ExpectStatus(t, rec, tc.status)
		if he := testutil.DecodeError(t, rec); len(he.Errors[tc.field]) == 0 {
			t.Errorf("%v: errors = %v", tc.body, he.Errors)
		}
	}
}

func TestAdminUsersDetail(t *testing.T) {
	e := newOpsEnv(t)
	buyer := testutil.CreateUser(t, e.a, "buyer@example.com", usersvc.CreateOptions{InviteUserID: &e.user.ID})
	paid := time.Now()
	for i, st := range []model.OrderStatus{model.OrderStatusCompleted, model.OrderStatusDiscounted, model.OrderStatusPending, model.OrderStatusCancelled} {
		o := &model.Order{TradeNo: fmt.Sprintf("D%d", i), UserID: buyer.ID, PlanID: 1, Type: model.OrderTypeNew, Period: model.PeriodMonthly,
			Status: st, TotalAmount: int64(100 * (i + 1))}
		if st != model.OrderStatusPending {
			o.PaidAt = &paid
		}
		if err := e.a.DB.Create(o).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i, amt := range []int64{30, 12} {
		if err := e.a.DB.Create(&model.CommissionLog{InviteUserID: buyer.ID, UserID: e.user.ID, TradeNo: fmt.Sprintf("C%d", i), OrderAmount: 100, GetAmount: amt}).Error; err != nil {
			t.Fatal(err)
		}
	}
	rec := testutil.Do(t, e.r, http.MethodGet, usersPath(buyer.ID, ""), nil, e.adminTok)
	d := testutil.Decode[adminuser.Detail](t, rec)
	if d.ID != buyer.ID || d.Stats.OrdersTotal != 300 || d.Stats.CommissionTotal != 42 || d.InviteUserEmail == nil || d.Token == "" {
		t.Fatalf("detail = %+v", d)
	}
	rec = testutil.Do(t, e.r, http.MethodGet, usersPath(buyer.ID, ""), nil, e.staffTok)
	if d := testutil.Decode[adminuser.Detail](t, rec); d.Token != "" || d.UUID != "" || d.Stats.OrdersTotal != 300 {
		t.Fatalf("staff detail = %+v", d)
	}
	e.expect(t, http.MethodGet, usersPath(99999, ""), nil, e.adminTok, http.StatusNotFound)
	e.expect(t, http.MethodGet, "/api/v1/admin/users/abc", nil, e.adminTok, http.StatusNotFound)
}

func TestAdminUsersPatch(t *testing.T) {
	e := newOpsEnv(t)
	g1, p1 := opsGroupPlan(t, e, "One", 10, nil)
	g2, p2 := opsGroupPlan(t, e, "Two", 20, testutil.Ptr(model.ResetFirstDayOfYear))
	exp := time.Now().Add(400 * 24 * time.Hour).UTC().Truncate(time.Second)
	u := testutil.CreateUser(t, e.a, "target@example.com", usersvc.CreateOptions{
		PlanID: &p1.ID, GroupID: &g1.ID, ExpiredAt: &exp, SpeedLimit: testutil.Ptr(5), InviteUserID: &e.user.ID, Remarks: "old",
	})
	path := usersPath(u.ID, "")

	e.expect(t, http.MethodPatch, path, map[string]any{
		"u": 7, "d": 8, "transfer_enable": 123, "banned": true, "is_staff": true, "balance": 5, "commission_balance": 6,
		"commission_type": 2, "commission_rate": 20, "discount": 10, "remarks": "", "remind_expire": false, "remind_traffic": false,
		"telegram_id": 42, "speed_limit": nil, "device_limit": 2, "invite_user_id": nil, "email": "Renamed@Example.com",
	}, e.adminTok, http.StatusOK)
	got := e.reload(t, u.ID)
	if got.U != 7 || got.D != 8 || got.TransferEnable != 123 || !got.Banned || !got.IsStaff || got.Balance != 5 || got.CommissionBalance != 6 ||
		got.CommissionType != model.CommissionTypeOneTime || got.CommissionRate == nil || *got.CommissionRate != 20 || got.Discount == nil ||
		*got.Discount != 10 || got.Remarks != "" || got.RemindExpire || got.RemindTraffic || got.TelegramID == nil || *got.TelegramID != 42 ||
		got.SpeedLimit != nil || got.DeviceLimit == nil || *got.DeviceLimit != 2 || got.InviteUserID != nil || got.Email != "renamed@example.com" ||
		got.PlanID == nil || *got.PlanID != p1.ID || got.ExpiredAt == nil {
		t.Fatalf("after patch = %+v", got)
	}

	// Changing the plan moves the user to its group and recomputes next_reset_at (yearly on Jan 1).
	rec := testutil.Do(t, e.r, http.MethodPatch, path, map[string]any{"plan_id": p2.ID}, e.adminTok)
	v := testutil.Decode[adminuser.View](t, rec)
	if v.PlanID == nil || *v.PlanID != p2.ID || v.GroupID == nil || *v.GroupID != g2.ID || v.NextResetAt == nil ||
		v.NextResetAt.In(time.Local).Month() != time.January || v.NextResetAt.In(time.Local).Day() != 1 {
		t.Fatalf("plan change = %+v", v)
	}
	// Explicit group wins; clearing plan and expiry clears next_reset_at.
	e.expect(t, http.MethodPatch, path, map[string]any{"plan_id": p1.ID, "group_id": g2.ID}, e.adminTok, http.StatusOK)
	if got := e.reload(t, u.ID); *got.GroupID != g2.ID || *got.PlanID != p1.ID {
		t.Fatalf("explicit group = %+v", got)
	}
	e.expect(t, http.MethodPatch, path, map[string]any{"plan_id": nil, "expired_at": nil, "group_id": 0}, e.adminTok, http.StatusOK)
	if got := e.reload(t, u.ID); got.PlanID != nil || got.ExpiredAt != nil || got.NextResetAt != nil || got.GroupID != nil {
		t.Fatalf("cleared = %+v", got)
	}

	// Password: new hash, bumped version.
	before := e.reload(t, u.ID)
	e.expect(t, http.MethodPatch, path, map[string]any{"password": "newpassword1"}, e.adminTok, http.StatusOK)
	after := e.reload(t, u.ID)
	if !auth.VerifyPassword(after.PasswordHash, "newpassword1") || after.PasswordVersion != before.PasswordVersion+1 {
		t.Fatalf("password not changed: %+v", after)
	}

	errs := []struct {
		body   map[string]any
		status int
		field  string
	}{
		{map[string]any{"invite_user_id": u.ID}, http.StatusBadRequest, "invite_user_id"},
		{map[string]any{"invite_user_id": 99999}, http.StatusBadRequest, "invite_user_id"},
		{map[string]any{"plan_id": 99999}, http.StatusBadRequest, "plan_id"},
		{map[string]any{"group_id": 99999}, http.StatusBadRequest, "group_id"},
		{map[string]any{"commission_rate": 101}, http.StatusBadRequest, "commission_rate"},
		{map[string]any{"discount": -1}, http.StatusBadRequest, "discount"},
		{map[string]any{"commission_type": 5}, http.StatusBadRequest, "commission_type"},
		{map[string]any{"u": -1}, http.StatusBadRequest, "u"},
		{map[string]any{"password": "short"}, http.StatusBadRequest, "password"},
		{map[string]any{"email": "nope"}, http.StatusBadRequest, "email"},
		{map[string]any{"email": "member@example.com"}, http.StatusConflict, "email"},
	}
	for _, tc := range errs {
		rec := testutil.Do(t, e.r, http.MethodPatch, path, tc.body, e.adminTok)
		testutil.ExpectStatus(t, rec, tc.status)
		if he := testutil.DecodeError(t, rec); len(he.Errors[tc.field]) == 0 {
			t.Errorf("%v: errors = %v", tc.body, he.Errors)
		}
	}
	e.expect(t, http.MethodPatch, path, `{"plan_id": "abc"}`, e.adminTok, http.StatusBadRequest)
	e.expect(t, http.MethodPatch, usersPath(99999, ""), map[string]any{"remarks": "x"}, e.adminTok, http.StatusNotFound)

	// Self protection and demotion.
	self := usersPath(e.admin.ID, "")
	e.expect(t, http.MethodPatch, self, map[string]any{"banned": true}, e.adminTok, http.StatusConflict)
	e.expect(t, http.MethodPatch, self, map[string]any{"is_admin": false}, e.adminTok, http.StatusConflict)
	e.expect(t, http.MethodPatch, self, map[string]any{"remarks": "me"}, e.adminTok, http.StatusOK)
	other := testutil.CreateAdmin(t, e.a, "admin2@example.com")
	e.expect(t, http.MethodPatch, usersPath(other.ID, ""), map[string]any{"is_admin": false}, e.adminTok, http.StatusOK)
	if got := e.reload(t, other.ID); got.IsAdmin {
		t.Fatal("admin2 still admin")
	}
	// A banned or demoted user's cached session is dropped immediately.
	staffPath := usersPath(e.staff.ID, "")
	e.expect(t, http.MethodGet, "/api/v1/admin/users", nil, e.staffTok, http.StatusOK)
	e.expect(t, http.MethodPatch, staffPath, map[string]any{"is_staff": false}, e.adminTok, http.StatusOK)
	e.expect(t, http.MethodGet, "/api/v1/admin/users", nil, e.staffTok, http.StatusForbidden)
}

func TestAdminUsersDelete(t *testing.T) {
	e := newOpsEnv(t)
	victim := testutil.CreateUser(t, e.a, "victim@example.com")
	invitee := testutil.CreateUser(t, e.a, "invitee@example.com", usersvc.CreateOptions{InviteUserID: &victim.ID})
	keep := testutil.CreateUser(t, e.a, "keep@example.com")
	create := func(v any) {
		t.Helper()
		if err := e.a.DB.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	create(&model.Order{TradeNo: "V1", UserID: victim.ID, PlanID: 1, Type: 1, Period: model.PeriodMonthly})
	create(&model.Order{TradeNo: "K1", UserID: keep.ID, PlanID: 1, Type: 1, Period: model.PeriodMonthly, InviteUserID: &victim.ID})
	tk := &model.Ticket{UserID: victim.ID, Subject: "help"}
	create(tk)
	create(&model.TicketMessage{TicketID: tk.ID, UserID: victim.ID, Message: "hi"})
	ktk := &model.Ticket{UserID: keep.ID, Subject: "other"}
	create(ktk)
	create(&model.TicketMessage{TicketID: ktk.ID, UserID: keep.ID, Message: "keep me"})
	create(&model.StatUser{UserID: victim.ID, NodeID: 1, Rate: 1, U: 1, D: 1, RecordAt: model.Today()})
	create(&model.InviteCode{UserID: victim.ID, Code: "VICTIMCODE"})
	create(&model.CommissionLog{InviteUserID: victim.ID, UserID: invitee.ID, TradeNo: "X", OrderAmount: 1, GetAmount: 1})
	create(&model.RuleProfile{UserID: victim.ID, Data: model.RawJSON(`{}`)})

	e.expect(t, http.MethodDelete, usersPath(victim.ID, ""), nil, e.adminTok, http.StatusOK)
	counts := []struct {
		model any
		where string
		want  int64
	}{
		{&model.User{}, "id = " + strconv.FormatUint(victim.ID, 10), 0},
		{&model.Order{}, "user_id = " + strconv.FormatUint(victim.ID, 10), 0},
		{&model.Order{}, "invite_user_id IS NOT NULL", 0},
		{&model.Order{}, "user_id = " + strconv.FormatUint(keep.ID, 10), 1},
		{&model.Ticket{}, "1 = 1", 1},
		{&model.TicketMessage{}, "1 = 1", 1},
		{&model.StatUser{}, "1 = 1", 0},
		{&model.InviteCode{}, "1 = 1", 0},
		{&model.CommissionLog{}, "1 = 1", 0},
		{&model.RuleProfile{}, "1 = 1", 0},
		{&model.User{}, "invite_user_id IS NOT NULL", 0},
	}
	for _, c := range counts {
		var n int64
		if err := e.a.DB.Model(c.model).Where(c.where).Count(&n).Error; err != nil || n != c.want {
			t.Errorf("%T where %s = %d (%v), want %d", c.model, c.where, n, err, c.want)
		}
	}
	e.expect(t, http.MethodDelete, usersPath(victim.ID, ""), nil, e.adminTok, http.StatusNotFound)
	e.expect(t, http.MethodDelete, usersPath(e.admin.ID, ""), nil, e.adminTok, http.StatusConflict)
}

func TestAdminUsersSecretLoginTraffic(t *testing.T) {
	e := newOpsEnv(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com/"}))
	_, p := opsGroupPlan(t, e, "Monthly", 10, testutil.Ptr(model.ResetFirstDayOfMonth))
	exp := time.Now().Add(100 * 24 * time.Hour)
	u := testutil.CreateUser(t, e.a, "secret@example.com", usersvc.CreateOptions{PlanID: &p.ID, ExpiredAt: &exp})
	old := e.reload(t, u.ID)

	rec := testutil.Do(t, e.r, http.MethodPost, usersPath(u.ID, "/reset-secret"), nil, e.adminTok)
	v := testutil.Decode[adminuser.View](t, rec)
	cur := e.reload(t, u.ID)
	if v.UUID == old.UUID || v.Token == old.Token || cur.UUID != v.UUID || cur.PasswordVersion != old.PasswordVersion+1 {
		t.Fatalf("reset secret: %+v -> %+v", old, cur)
	}

	rec = testutil.Do(t, e.r, http.MethodPost, usersPath(u.ID, "/login-token"), nil, e.adminTok)
	out := testutil.Decode[struct {
		URL string `json:"url"`
	}](t, rec)
	const prefix = "https://panel.example.com/login?token="
	if !strings.HasPrefix(out.URL, prefix) {
		t.Fatalf("url = %q", out.URL)
	}
	if id, ok := auth.ConsumeLoginToken(e.a, strings.TrimPrefix(out.URL, prefix)); !ok || id != u.ID {
		t.Fatalf("token resolves to %d, %v", id, ok)
	}
	e.expect(t, http.MethodPost, usersPath(99999, "/login-token"), nil, e.adminTok, http.StatusNotFound)

	if err := e.a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(map[string]any{"u": 10, "d": 20, "next_reset_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	rec = testutil.Do(t, e.r, http.MethodPost, usersPath(u.ID, "/traffic-reset"), nil, e.adminTok)
	v = testutil.Decode[adminuser.View](t, rec)
	now := time.Now()
	wantNext := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.Local)
	if v.U != 0 || v.D != 0 || v.NextResetAt == nil || !v.NextResetAt.Equal(wantNext) {
		t.Fatalf("traffic reset = %+v (want next %v)", v, wantNext)
	}
	e.expect(t, http.MethodPost, usersPath(99999, "/traffic-reset"), nil, e.adminTok, http.StatusNotFound)
}

func opsCSV(t *testing.T, s string) [][]string {
	t.Helper()
	rows, err := csv.NewReader(strings.NewReader(s)).ReadAll()
	if err != nil {
		t.Fatalf("csv %q: %v", s, err)
	}
	return rows
}

func TestAdminUsersGenerate(t *testing.T) {
	e := newOpsEnv(t)
	g, p := opsGroupPlan(t, e, "Gift", 30, nil)
	exp := time.Now().Add(60 * 24 * time.Hour).UTC().Truncate(time.Second)
	rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users/generate", map[string]any{
		"email_suffix": "@Gift.Example.com", "count": 3, "plan_id": p.ID, "expired_at": exp,
	}, e.adminTok)
	res := testutil.Decode[adminuser.GenerateResult](t, rec)
	rows := opsCSV(t, res.CSV)
	if res.Count != 3 || len(rows) != 4 || !slices.Equal(rows[0], []string{"email", "password"}) {
		t.Fatalf("generate = %+v", res)
	}
	for _, row := range rows[1:] {
		local, domain, _ := strings.Cut(row[0], "@")
		if len(local) != 8 || domain != "gift.example.com" || len(row[1]) != 12 {
			t.Fatalf("row = %v", row)
		}
		var u model.User
		if err := e.a.DB.Where("email = ?", row[0]).First(&u).Error; err != nil {
			t.Fatal(err)
		}
		if !auth.VerifyPassword(u.PasswordHash, row[1]) || u.PlanID == nil || *u.PlanID != p.ID || u.GroupID == nil || *u.GroupID != g.ID ||
			u.TransferEnable != 30<<30 || u.ExpiredAt == nil || !u.ExpiredAt.Equal(exp) || u.NextResetAt == nil || u.UUID == "" || len(u.Token) != 32 {
			t.Fatalf("generated user = %+v", u)
		}
	}

	rec = testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users/generate", map[string]any{
		"email_prefix": "VIP", "email_suffix": "example.com", "count": 1, "password": "sharedpass1",
	}, e.adminTok)
	res = testutil.Decode[adminuser.GenerateResult](t, rec)
	if rows := opsCSV(t, res.CSV); res.Count != 1 || !slices.Equal(rows[1], []string{"vip@example.com", "sharedpass1"}) {
		t.Fatalf("prefixed = %+v", res)
	}
	rec = testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users/generate", map[string]any{
		"email_prefix": "vip", "email_suffix": "example.com", "count": 2, "password": "sharedpass1",
	}, e.adminTok)
	res = testutil.Decode[adminuser.GenerateResult](t, rec)
	for _, row := range opsCSV(t, res.CSV)[1:] {
		if !strings.HasPrefix(row[0], "vip") || len(row[0]) != len("vip")+8+len("@example.com") {
			t.Fatalf("prefixed batch row = %v", row)
		}
	}

	bad := []map[string]any{
		{"email_prefix": "vip", "email_suffix": "example.com", "count": 1},
		{"email_suffix": "example.com", "count": 0},
		{"email_suffix": "example.com", "count": 501},
		{"email_suffix": "not a host!", "count": 1},
		{"email_suffix": "example.com", "count": 1, "plan_id": 99999},
		{"email_suffix": "example.com", "count": 1, "password": "short"},
		{"count": 1},
	}
	for i, body := range bad {
		rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users/generate", body, e.adminTok)
		if rec.Code != http.StatusBadRequest && !(i == 0 && rec.Code == http.StatusConflict) {
			t.Errorf("%v: status %d %s", body, rec.Code, rec.Body)
		}
	}
}

func TestAdminUsersExport(t *testing.T) {
	e := newOpsEnv(t)
	_, p := opsGroupPlan(t, e, "Exported", 1, nil)
	exp := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
	u := testutil.CreateUser(t, e.a, "=cmd@example.com", usersvc.CreateOptions{PlanID: &p.ID, ExpiredAt: &exp, TransferEnable: 1 << 30, Balance: 42})
	if err := e.a.DB.Model(u).Updates(map[string]any{"u": 1, "d": 2, "banned": true}).Error; err != nil {
		t.Fatal(err)
	}
	rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users/export", map[string]any{"banned": true}, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("content-type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="users-`) {
		t.Fatalf("content-disposition = %q", cd)
	}
	rows := opsCSV(t, rec.Body.String())
	want := []string{"'=cmd@example.com", "Exported", "2027-01-02T03:04:05Z", strconv.FormatInt(1<<30, 10), "1", "2", "42"}
	if len(rows) != 2 || !slices.Equal(rows[0], adminuser.ExportHeader) || !slices.Equal(rows[1][:7], want) {
		t.Fatalf("rows = %v", rows)
	}
	if _, err := time.Parse(time.RFC3339, rows[1][7]); err != nil {
		t.Fatalf("created_at = %q", rows[1][7])
	}

	rec = testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users/export", nil, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if rows := opsCSV(t, rec.Body.String()); len(rows) != 5 { // header + root, staff, member, =cmd
		t.Fatalf("all rows = %d", len(rows))
	}
	e.expect(t, http.MethodPost, "/api/v1/admin/users/export", map[string]any{"expired": "bogus"}, e.adminTok, http.StatusBadRequest)
}

func TestAdminUsersMail(t *testing.T) {
	e := newOpsEnv(t)
	testutil.CreateUser(t, e.a, "news1@example.com")
	testutil.CreateUser(t, e.a, "news2@example.com")
	rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users/mail", map[string]any{
		"email": "news", "subject": "Maintenance window", "content": "Line one.\n\nLine two.",
	}, e.adminTok)
	out := testutil.Decode[map[string]int](t, rec)
	if out["queued"] != 2 {
		t.Fatalf("queued = %v", out)
	}
	msgs := testutil.Mails(t, e.a)
	var to []string
	for _, m := range msgs {
		if m.Template != mail.TplNotify || m.Subject != "Maintenance window" || !strings.Contains(m.Text, "Line two.") {
			t.Fatalf("message = %+v", m)
		}
		to = append(to, m.To)
	}
	if !slices.Equal(to, []string{"news1@example.com", "news2@example.com"}) {
		t.Fatalf("recipients = %v", to)
	}
	rec = testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users/mail", map[string]any{
		"email": "nobody", "subject": "x", "content": "y",
	}, e.adminTok)
	if out := testutil.Decode[map[string]int](t, rec); out["queued"] != 0 {
		t.Fatalf("queued = %v", out)
	}
	for _, body := range []map[string]any{{"subject": "x"}, {"content": "y"}, {"subject": "x", "content": "y", "expired": "no"}} {
		e.expect(t, http.MethodPost, "/api/v1/admin/users/mail", body, e.adminTok, http.StatusBadRequest)
	}
}

func TestAdminUsersPatchKeepsAbsentNullables(t *testing.T) {
	e := newOpsEnv(t)
	exp := time.Now().Add(time.Hour)
	u := testutil.CreateUser(t, e.a, "keep@example.com", usersvc.CreateOptions{ExpiredAt: &exp, SpeedLimit: testutil.Ptr(9)})
	body, err := json.Marshal(adminuser.UpdateInput{Remarks: testutil.Ptr("only remarks")})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"remarks":"only remarks"}` {
		t.Fatalf("encoded = %s", body)
	}
	e.expect(t, http.MethodPatch, usersPath(u.ID, ""), string(body), e.adminTok, http.StatusOK)
	if got := e.reload(t, u.ID); got.ExpiredAt == nil || got.SpeedLimit == nil || *got.SpeedLimit != 9 || got.Remarks != "only remarks" {
		t.Fatalf("absent fields changed: %+v", got)
	}
}
