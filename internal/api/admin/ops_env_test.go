package admin_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/admin"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// opsEnv is the fixture of the BE-E1 admin tests (stats, users, settings, system).
type opsEnv struct {
	a                           *app.App
	r                           http.Handler
	admin, staff, user          *model.User
	adminTok, staffTok, userTok string
}

func newOpsEnv(t *testing.T, opts ...testutil.Option) *opsEnv {
	t.Helper()
	a := testutil.NewTestApp(t, opts...)
	e := &opsEnv{a: a, r: testutil.NewRouterWith(t, a, admin.Register)}
	e.admin = testutil.CreateAdmin(t, a, "root@example.com")
	e.staff = testutil.CreateStaff(t, a, "staff@example.com")
	e.user = testutil.CreateUser(t, a, "member@example.com")
	e.adminTok = testutil.Login(t, a, e.admin)
	e.staffTok = testutil.Login(t, a, e.staff)
	e.userTok = testutil.Login(t, a, e.user)
	return e
}

func (e *opsEnv) expect(t *testing.T, method, path string, body any, tok string, status int) []byte {
	t.Helper()
	rec := testutil.Do(t, e.r, method, path, body, tok)
	testutil.ExpectStatus(t, rec, status)
	return rec.Body.Bytes()
}

func (e *opsEnv) reload(t *testing.T, id uint64) model.User {
	t.Helper()
	var u model.User
	if err := e.a.DB.First(&u, id).Error; err != nil {
		t.Fatalf("reload user %d: %v", id, err)
	}
	return u
}

func TestOpsPermissions(t *testing.T) {
	e := newOpsEnv(t)
	id := strconv.FormatUint(e.user.ID, 10)
	adminOnly := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/stats/overview"},
		{http.MethodGet, "/api/v1/admin/stats/series?metric=income"},
		{http.MethodGet, "/api/v1/admin/stats/nodes/ranking"},
		{http.MethodGet, "/api/v1/admin/stats/users/ranking"},
		{http.MethodPost, "/api/v1/admin/users"},
		{http.MethodPatch, "/api/v1/admin/users/" + id},
		{http.MethodDelete, "/api/v1/admin/users/" + id},
		{http.MethodPost, "/api/v1/admin/users/" + id + "/reset-secret"},
		{http.MethodPost, "/api/v1/admin/users/" + id + "/login-token"},
		{http.MethodPost, "/api/v1/admin/users/" + id + "/traffic-reset"},
		{http.MethodPost, "/api/v1/admin/users/generate"},
		{http.MethodPost, "/api/v1/admin/users/export"},
		{http.MethodPost, "/api/v1/admin/users/mail"},
		{http.MethodGet, "/api/v1/admin/settings"},
		{http.MethodPut, "/api/v1/admin/settings"},
		{http.MethodPost, "/api/v1/admin/settings/mail/test"},
		{http.MethodGet, "/api/v1/admin/system/status"},
		{http.MethodGet, "/api/v1/admin/system/logs"},
	}
	for _, rt := range adminOnly {
		e.expect(t, rt.method, rt.path, nil, "", http.StatusUnauthorized)
		e.expect(t, rt.method, rt.path, nil, e.userTok, http.StatusForbidden)
		e.expect(t, rt.method, rt.path, nil, e.staffTok, http.StatusForbidden)
	}
	for _, path := range []string{"/api/v1/admin/users", "/api/v1/admin/users/" + id} {
		e.expect(t, http.MethodGet, path, nil, "", http.StatusUnauthorized)
		e.expect(t, http.MethodGet, path, nil, e.userTok, http.StatusForbidden)
		e.expect(t, http.MethodGet, path, nil, e.staffTok, http.StatusOK)
		e.expect(t, http.MethodGet, path, nil, e.adminTok, http.StatusOK)
	}
}
