package admin_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/adminuser"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// connPlan is the part of the admin Plan object this file checks.
type connPlan struct {
	ID          uint64 `json:"id"`
	DeviceLimit *int   `json:"device_limit"`
	ConnLimit   *int   `json:"conn_limit"`
	UserCount   int64  `json:"user_count"`
}

func intOr(p *int, null int) int {
	if p == nil {
		return null
	}
	return *p
}

// docs/API.md Plan: conn_limit is created, kept when absent, cleared by null and pushed to holders by force_update;
// it is bounded to 0..100000.
func TestAdminPlanConnLimit(t *testing.T) {
	e := newCommerceEnv(t)
	gid := e.group(t)

	for _, bad := range []any{-1, model.MaxConnLimit + 1} {
		rec := e.do(t, http.MethodPost, "/plans", map[string]any{"name": "Bad", "group_id": gid, "conn_limit": bad})
		if er := testutil.DecodeError(t, rec); er.Status != http.StatusBadRequest || len(er.Errors["conn_limit"]) == 0 {
			t.Fatalf("conn_limit %v = %d %s", bad, rec.Code, rec.Body.String())
		}
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPost, "/plans", map[string]any{"name": "Bad", "group_id": gid, "conn_limit": "8"}), http.StatusBadRequest)
	rec := e.do(t, http.MethodPost, "/plans", map[string]any{"name": "Relay", "group_id": gid, "transfer_enable": 10, "device_limit": 3, "conn_limit": 64})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	p := testutil.Decode[connPlan](t, rec)
	if intOr(p.ConnLimit, -1) != 64 || intOr(p.DeviceLimit, -1) != 3 {
		t.Fatalf("created = %+v", p)
	}
	path := commercePath("/plans", p.ID)
	if got := testutil.Decode[connPlan](t, e.do(t, http.MethodGet, path, nil)); intOr(got.ConnLimit, -1) != 64 {
		t.Fatalf("get = %+v", got)
	}
	plans, _ := testutil.DecodeList[connPlan](t, e.do(t, http.MethodGet, "/plans", nil))
	if len(plans) != 1 || intOr(plans[0].ConnLimit, -1) != 64 {
		t.Fatalf("list = %+v", plans)
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPatch, path, map[string]any{"conn_limit": model.MaxConnLimit + 1}), http.StatusBadRequest)

	future := time.Now().Add(time.Hour)
	holder := testutil.CreateUser(t, e.a, "holder@example.com", usersvc.CreateOptions{PlanID: &p.ID, GroupID: &gid, ExpiredAt: &future, ConnLimit: testutil.Ptr(64)})
	reload := func() model.User {
		t.Helper()
		var u model.User
		if err := e.a.DB.First(&u, holder.ID).Error; err != nil {
			t.Fatal(err)
		}
		return u
	}

	// Absent: kept. Without force_update the holder keeps its own value.
	got := testutil.Decode[connPlan](t, e.do(t, http.MethodPatch, path, map[string]any{"device_limit": 2}))
	if intOr(got.ConnLimit, -1) != 64 || intOr(got.DeviceLimit, -1) != 2 {
		t.Fatalf("absent conn_limit = %+v", got)
	}
	got = testutil.Decode[connPlan](t, e.do(t, http.MethodPatch, path, map[string]any{"conn_limit": 32}))
	if intOr(got.ConnLimit, -1) != 32 || intOr(reload().ConnLimit, -1) != 64 {
		t.Fatalf("patch without force_update: plan %+v, holder %v", got, reload().ConnLimit)
	}
	// force_update pushes it (with the other limits) to every holder.
	got = testutil.Decode[connPlan](t, e.do(t, http.MethodPatch, path, map[string]any{"force_update": true}))
	if u := reload(); intOr(u.ConnLimit, -1) != 32 || intOr(u.DeviceLimit, -1) != 2 || got.UserCount != 1 {
		t.Fatalf("force_update: holder conn %v device %v, plan %+v", u.ConnLimit, u.DeviceLimit, got)
	}
	// null clears it on the plan, and force_update clears it on the holder.
	got = testutil.Decode[connPlan](t, e.do(t, http.MethodPatch, path, map[string]any{"conn_limit": nil, "force_update": true}))
	if got.ConnLimit != nil || reload().ConnLimit != nil {
		t.Fatalf("null clear: plan %+v, holder %v", got, reload().ConnLimit)
	}
	// 0 is stored as given (unlimited, like null).
	got = testutil.Decode[connPlan](t, e.do(t, http.MethodPatch, path, map[string]any{"conn_limit": 0}))
	if intOr(got.ConnLimit, -1) != 0 {
		t.Fatalf("zero = %+v", got)
	}
}

// docs/API.md users rules: plan_id fills conn_limit on POST unless it is given, PATCH sets and clears it, generate copies
// the plan's, and every admin user view carries it.
func TestAdminUsersConnLimit(t *testing.T) {
	e := newOpsEnv(t)
	_, p := opsGroupPlan(t, e, "Relay", 10, nil)
	if err := e.a.DB.Model(p).Update("conn_limit", 64).Error; err != nil {
		t.Fatal(err)
	}
	create := func(email string, extra map[string]any) adminuser.View {
		t.Helper()
		body := map[string]any{"email": email, "password": "password123", "plan_id": p.ID}
		for k, v := range extra {
			body[k] = v
		}
		rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users", body, e.adminTok)
		testutil.ExpectStatus(t, rec, http.StatusCreated)
		return testutil.Decode[adminuser.View](t, rec)
	}

	if v := create("fill@example.com", nil); intOr(v.ConnLimit, -1) != 64 || intOr(e.reload(t, v.ID).ConnLimit, -1) != 64 {
		t.Fatalf("plan fill = %v", v.ConnLimit)
	}
	if v := create("own@example.com", map[string]any{"conn_limit": 8}); intOr(v.ConnLimit, -1) != 8 {
		t.Fatalf("explicit = %v", v.ConnLimit)
	}
	if v := create("zero@example.com", map[string]any{"conn_limit": 0}); intOr(v.ConnLimit, -1) != 0 {
		t.Fatalf("explicit 0 = %v", v.ConnLimit)
	}
	rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users", map[string]any{"email": "noplan@example.com", "password": "password123"}, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	if v := testutil.Decode[adminuser.View](t, rec); v.ConnLimit != nil {
		t.Fatalf("no plan = %v", v.ConnLimit)
	}
	for _, bad := range []any{-1, model.MaxConnLimit + 1} {
		rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users", map[string]any{"email": "bad@example.com", "password": "password123", "conn_limit": bad}, e.adminTok)
		if er := testutil.DecodeError(t, rec); er.Status != http.StatusBadRequest || len(er.Errors["conn_limit"]) == 0 {
			t.Fatalf("create conn_limit %v = %d %s", bad, rec.Code, rec.Body.String())
		}
	}

	u := testutil.CreateUser(t, e.a, "target@example.com", usersvc.CreateOptions{DeviceLimit: testutil.Ptr(2)})
	path := usersPath(u.ID, "")
	rec = testutil.Do(t, e.r, http.MethodPatch, path, map[string]any{"conn_limit": 16}, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if v := testutil.Decode[adminuser.View](t, rec); intOr(v.ConnLimit, -1) != 16 || intOr(v.DeviceLimit, -1) != 2 {
		t.Fatalf("patch = %+v", v)
	}
	e.expect(t, http.MethodPatch, path, map[string]any{"remarks": "absent keeps it"}, e.adminTok, http.StatusOK)
	if got := e.reload(t, u.ID); intOr(got.ConnLimit, -1) != 16 {
		t.Fatalf("absent = %v", got.ConnLimit)
	}
	for _, bad := range []any{-1, model.MaxConnLimit + 1} {
		rec := testutil.Do(t, e.r, http.MethodPatch, path, map[string]any{"conn_limit": bad}, e.adminTok)
		if er := testutil.DecodeError(t, rec); er.Status != http.StatusBadRequest || len(er.Errors["conn_limit"]) == 0 {
			t.Fatalf("patch %v = %d %s", bad, rec.Code, rec.Body.String())
		}
	}
	e.expect(t, http.MethodPatch, path, map[string]any{"conn_limit": model.MaxConnLimit}, e.adminTok, http.StatusOK)
	e.expect(t, http.MethodPatch, path, map[string]any{"conn_limit": nil}, e.adminTok, http.StatusOK)
	if got := e.reload(t, u.ID); got.ConnLimit != nil || intOr(got.DeviceLimit, -1) != 2 {
		t.Fatalf("null clear = conn %v device %v", got.ConnLimit, got.DeviceLimit)
	}

	// Detail and list views.
	e.expect(t, http.MethodPatch, path, map[string]any{"conn_limit": 4}, e.adminTok, http.StatusOK)
	if v := testutil.Decode[adminuser.View](t, testutil.Do(t, e.r, http.MethodGet, path, nil, e.adminTok)); intOr(v.ConnLimit, -1) != 4 {
		t.Fatalf("detail = %v", v.ConnLimit)
	}
	list, _ := testutil.DecodeList[adminuser.View](t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin/users?email=target", nil, e.adminTok))
	if len(list) != 1 || intOr(list[0].ConnLimit, -1) != 4 {
		t.Fatalf("list = %+v", list)
	}

	// Bulk generate copies the plan's limit.
	e.expect(t, http.MethodPost, "/api/v1/admin/users/generate", map[string]any{"email_prefix": "gen", "email_suffix": "example.com", "count": 2, "plan_id": p.ID},
		e.adminTok, http.StatusOK)
	var gen []model.User
	if err := e.a.DB.Where("email LIKE ?", "gen%@example.com").Find(&gen).Error; err != nil {
		t.Fatal(err)
	}
	if len(gen) != 2 || intOr(gen[0].ConnLimit, -1) != 64 || intOr(gen[1].ConnLimit, -1) != 64 {
		t.Fatalf("generated = %+v", gen)
	}
}
