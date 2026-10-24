package admin_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/api/admin"
	"github.com/linusxiong/ggboard/internal/api/node"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// nodesAdminEnv is the fixture of the BE-B admin tests (groups, nodes, routes); node.Register is mounted too so
// tests can check that admin writes reach node backends.
type nodesAdminEnv struct {
	t     *testing.T
	a     *app.App
	r     *gin.Engine
	token string
}

const nodesAdminServerToken = "admin-nodes-test-token-0123456789"

func newNodesAdminEnv(t *testing.T) *nodesAdminEnv {
	t.Helper()
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"server_token": nodesAdminServerToken}))
	r := testutil.NewRouterWith(t, a, admin.Register, node.Register)
	root := testutil.CreateAdmin(t, a, "root@example.com")
	return &nodesAdminEnv{t: t, a: a, r: r, token: testutil.Login(t, a, root)}
}

func (e *nodesAdminEnv) do(method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	return testutil.Do(e.t, e.r, method, "/api/v1/admin"+path, body, e.token)
}

func (e *nodesAdminEnv) group(name string) uint64 {
	e.t.Helper()
	rec := e.do(http.MethodPost, "/groups", map[string]any{"name": name})
	testutil.ExpectStatus(e.t, rec, http.StatusCreated)
	return testutil.Decode[model.ServerGroup](e.t, rec).ID
}

type nodesAdminView struct {
	model.Node
	Parent *struct {
		ID   uint64 `json:"id"`
		Name string `json:"name"`
	} `json:"parent"`
	Status nodesvc.StatusView `json:"status"`
}

func (e *nodesAdminEnv) createNode(body map[string]any) nodesAdminView {
	e.t.Helper()
	rec := e.do(http.MethodPost, "/nodes", body)
	testutil.ExpectStatus(e.t, rec, http.StatusCreated)
	return testutil.Decode[nodesAdminView](e.t, rec)
}

func nodesAdminFields(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	er := testutil.DecodeError(t, rec)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out []string
	for k := range er.Errors {
		out = append(out, k)
	}
	return out
}

func nodesAdminHas(fields []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, f := range fields {
			if f == w {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

var nodesAdminKey = strings.Repeat("A", 43) // obviously fake base64url X25519 key

func nodesAdminVless(name string, groups []uint64) map[string]any {
	return map[string]any{
		"type": "vless", "name": name, "host": "vl.example.com", "port": 443, "server_port": 443, "group_ids": groups, "show": true, "tags": []string{" HK ", "HK", ""},
		"protocol_settings": map[string]any{"network": "tcp", "tls": 2, "flow": "xtls-rprx-vision", "tls_settings": map[string]any{
			"server_name": "www.example.org", "dest": "www.example.org:443", "private_key": nodesAdminKey, "public_key": nodesAdminKey, "short_id": "0123abcd"}},
	}
}

func TestAdminNodesRequireAdmin(t *testing.T) {
	e := newNodesAdminEnv(t)
	staff := testutil.Login(t, e.a, testutil.CreateStaff(t, e.a, "staff@example.com"))
	for _, p := range []string{"/nodes", "/groups", "/routes"} {
		testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin"+p, nil, staff), http.StatusForbidden)
		testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodGet, "/api/v1/admin"+p, nil, ""), http.StatusUnauthorized)
	}
}

func TestAdminNodesValidation(t *testing.T) {
	e := newNodesAdminEnv(t)
	g := e.group("g")
	if f := nodesAdminFields(t, e.do(http.MethodPost, "/nodes", map[string]any{})); !nodesAdminHas(f, "type", "name", "host", "port", "server_port") {
		t.Fatalf("empty body fields %v", f)
	}
	body := nodesAdminVless("n", []uint64{g, 999})
	body["route_ids"] = []uint64{42}
	body["port"] = "30000-20000"
	body["rate"] = -1
	if f := nodesAdminFields(t, e.do(http.MethodPost, "/nodes", body)); !nodesAdminHas(f, "group_ids", "route_ids", "port", "rate") {
		t.Fatalf("fields %v", f)
	}
	body = nodesAdminVless("n", []uint64{g})
	body["protocol_settings"] = map[string]any{"tls": 2, "tls_settings": map[string]any{"server_name": "www.example.org"}}
	if f := nodesAdminFields(t, e.do(http.MethodPost, "/nodes", body)); !nodesAdminHas(f, "protocol_settings.tls_settings.private_key", "protocol_settings.tls_settings.public_key") {
		t.Fatalf("reality fields %v", f)
	}
	ss := map[string]any{"type": "shadowsocks", "name": "ss", "host": "ss.example.com", "port": "8388", "server_port": 8388, "protocol_settings": map[string]any{"cipher": "rc4"}}
	if f := nodesAdminFields(t, e.do(http.MethodPost, "/nodes", ss)); !nodesAdminHas(f, "protocol_settings.cipher") {
		t.Fatalf("cipher fields %v", f)
	}
	ss["protocol_settings"] = map[string]any{"cipher": "aes-128-gcm", "obfs": 3}
	if f := nodesAdminFields(t, e.do(http.MethodPost, "/nodes", ss)); !nodesAdminHas(f, "protocol_settings") {
		t.Fatalf("type error fields %v", f)
	}
	ss["port"] = "20000-30000,443"
	ss["protocol_settings"] = map[string]any{"cipher": "aes-128-gcm"}
	n := e.createNode(ss)
	if n.Port != "20000-30000,443" || n.Rate != 1 || n.Show {
		t.Fatalf("defaults %+v", n.Node)
	}
	// Child nodes: same type as the parent, the parent is no child itself, server_port defaults to the parent's.
	child := map[string]any{"type": "vless", "name": "c", "host": "relay.example.com", "port": "443", "parent_id": n.ID}
	if f := nodesAdminFields(t, e.do(http.MethodPost, "/nodes", child)); !nodesAdminHas(f, "parent_id") {
		t.Fatalf("type mismatch fields %v", f)
	}
	child["type"] = "shadowsocks"
	c := e.createNode(child)
	if c.ServerPort != 8388 || c.Parent == nil || c.Parent.Name != "ss" {
		t.Fatalf("child %+v parent %+v", c.Node, c.Parent)
	}
	grandChild := map[string]any{"type": "shadowsocks", "name": "gc", "host": "x.example.com", "port": "443", "parent_id": c.ID}
	if f := nodesAdminFields(t, e.do(http.MethodPost, "/nodes", grandChild)); !nodesAdminHas(f, "parent_id") {
		t.Fatalf("grandchild fields %v", f)
	}
	if f := nodesAdminFields(t, e.do(http.MethodPatch, fmt.Sprintf("/nodes/%d", n.ID), map[string]any{"type": "vmess", "protocol_settings": map[string]any{}})); !nodesAdminHas(f, "type") {
		t.Fatalf("type change with children %v", f)
	}
	if f := nodesAdminFields(t, e.do(http.MethodPatch, fmt.Sprintf("/nodes/%d", n.ID), map[string]any{"parent_id": n.ID})); !nodesAdminHas(f, "parent_id") {
		t.Fatalf("self parent %v", f)
	}
	testutil.ExpectStatus(t, e.do(http.MethodPatch, "/nodes/999", map[string]any{"name": "x"}), http.StatusNotFound)
}

func TestAdminNodesCRUD(t *testing.T) {
	e := newNodesAdminEnv(t)
	g := e.group("g")
	n := e.createNode(nodesAdminVless("HK 01", []uint64{g}))
	if n.Type != "vless" || n.Status.State != nodesvc.StateOffline || n.Parent != nil || len(n.Tags) != 1 || n.Tags[0] != "HK" {
		t.Fatalf("created %+v", n)
	}
	// Node backends see the node immediately; an admin edit invalidates the cached config.
	cfgURL := fmt.Sprintf("/api/v1/server/UniProxy/config?node_id=%d&node_type=vless&token=%s", n.ID, nodesAdminServerToken)
	rec := testutil.Do(t, e.r, http.MethodGet, cfgURL, nil, "")
	testutil.ExpectStatus(t, rec, http.StatusOK)
	tag := rec.Header().Get("ETag")

	rec = e.do(http.MethodPatch, fmt.Sprintf("/nodes/%d", n.ID), map[string]any{"name": "HK 02", "server_port": 8443})
	testutil.ExpectStatus(t, rec, http.StatusOK)
	got := testutil.Decode[nodesAdminView](t, rec)
	if got.Name != "HK 02" || got.ServerPort != 8443 || got.Host != "vl.example.com" || got.ProtocolSettings.Int("tls") != 2 {
		t.Fatalf("patched %+v", got.Node)
	}
	rec = testutil.Do(t, e.r, http.MethodGet, cfgURL, nil, "")
	if rec.Header().Get("ETag") == tag || !strings.Contains(rec.Body.String(), `"server_port":8443`) {
		t.Fatalf("config not refreshed: %s", rec.Body.String())
	}

	e.a.Online.TouchPush(n.ID)
	e.a.Online.SetTrafficUsers(n.ID, []uint64{9001, 9002, 9003, 9004})
	e.a.Online.SetVersion(n.ID, "V2bX/1.0")
	rec = e.do(http.MethodGet, fmt.Sprintf("/nodes/%d", n.ID), nil)
	got = testutil.Decode[nodesAdminView](t, rec)
	if got.Status.State != nodesvc.StateOnline || got.Status.Online != 4 || got.Status.Version != "V2bX/1.0" || got.Status.LastPushAt == nil {
		t.Fatalf("status %+v", got.Status)
	}

	child := e.createNode(map[string]any{"type": "vless", "name": "relay", "host": "relay.example.com", "port": "443", "parent_id": n.ID, "sort": 5})
	if child.Status.State != nodesvc.StateOnline {
		t.Fatalf("child status must be the parent's: %+v", child.Status)
	}
	list, total := testutil.DecodeList[nodesAdminView](t, e.do(http.MethodGet, "/nodes", nil))
	if total != 2 || list[0].ID != n.ID || list[1].Parent == nil || list[1].Parent.Name != "HK 02" {
		t.Fatalf("list %+v", list)
	}

	rec = e.do(http.MethodDelete, fmt.Sprintf("/nodes/%d", n.ID), nil)
	testutil.ExpectStatus(t, rec, http.StatusConflict)

	rec = e.do(http.MethodPost, fmt.Sprintf("/nodes/%d/copy", n.ID), nil)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	cp := testutil.Decode[nodesAdminView](t, rec)
	if cp.ID == n.ID || cp.Name != "HK 02 (copy)" || cp.Show || cp.Type != "vless" || cp.ProtocolSettings.Map("tls_settings").String("short_id") != "0123abcd" {
		t.Fatalf("copy %+v", cp.Node)
	}

	testutil.ExpectStatus(t, e.do(http.MethodPost, "/nodes/sort", map[string]any{"ids": []uint64{cp.ID, child.ID, n.ID}}), http.StatusOK)
	list, _ = testutil.DecodeList[nodesAdminView](t, e.do(http.MethodGet, "/nodes", nil))
	if list[0].ID != cp.ID || list[1].ID != child.ID || list[2].ID != n.ID || list[2].Sort != 2 {
		t.Fatalf("sorted %v %v %v", list[0].ID, list[1].ID, list[2].ID)
	}
	testutil.ExpectStatus(t, e.do(http.MethodPost, "/nodes/sort", map[string]any{"ids": []uint64{}}), http.StatusBadRequest)

	// Detach the child, then both can be deleted.
	rec = e.do(http.MethodPatch, fmt.Sprintf("/nodes/%d", child.ID), map[string]any{"parent_id": nil, "server_port": 443, "protocol_settings": map[string]any{"tls": 1}})
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if got := testutil.Decode[nodesAdminView](t, rec); got.ParentID != nil || got.Parent != nil {
		t.Fatalf("detached %+v", got)
	}
	testutil.ExpectStatus(t, e.do(http.MethodDelete, fmt.Sprintf("/nodes/%d", n.ID), nil), http.StatusOK)
	if _, ok := e.a.Online.NodeStatus(n.ID); ok {
		t.Fatal("runtime status must be forgotten")
	}
	testutil.ExpectStatus(t, e.do(http.MethodGet, fmt.Sprintf("/nodes/%d", n.ID), nil), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodGet, cfgURL, nil, ""), http.StatusBadRequest)
}

func TestAdminNodeStatsOnlineAudit(t *testing.T) {
	e := newNodesAdminEnv(t)
	g := e.group("g")
	n := e.createNode(nodesAdminVless("n", []uint64{g}))
	child := e.createNode(map[string]any{"type": "vless", "name": "c", "host": "c.example.com", "port": "443", "parent_id": n.ID})
	today := model.Today()
	for _, s := range []model.StatNode{
		{NodeID: n.ID, U: 10, D: 20, RecordAt: today},
		{NodeID: n.ID, U: 1, D: 2, RecordAt: today.AddDate(0, 0, -2)},
		{NodeID: n.ID, U: 5, D: 5, RecordAt: today.AddDate(0, 0, -40)},
	} {
		if err := e.a.DB.Create(&s).Error; err != nil {
			t.Fatal(err)
		}
	}
	type point struct {
		Date string `json:"date"`
		U    int64  `json:"u"`
		D    int64  `json:"d"`
	}
	for _, id := range []uint64{n.ID, child.ID} { // a child reports its parent's traffic
		pts := testutil.Decode[[]point](t, e.do(http.MethodGet, fmt.Sprintf("/nodes/%d/stats", id), nil))
		if len(pts) != 7 || pts[6].Date != today.Format(time.DateOnly) || pts[6].U != 10 || pts[4].D != 2 || pts[5].U != 0 {
			t.Fatalf("stats %+v", pts)
		}
	}
	pts := testutil.Decode[[]point](t, e.do(http.MethodGet, fmt.Sprintf("/nodes/%d/stats?range=30d", n.ID), nil))
	if len(pts) != 30 {
		t.Fatalf("30d points %d", len(pts))
	}
	testutil.ExpectStatus(t, e.do(http.MethodGet, fmt.Sprintf("/nodes/%d/stats?range=1y", n.ID), nil), http.StatusBadRequest)

	u := testutil.CreateUser(t, e.a, "alice@example.com", usersvc.CreateOptions{GroupID: &g, TransferEnable: 1 << 30})
	e.a.Online.SetAliveIPs(n.ID, u.ID, []string{"203.0.113.9", "198.51.100.7"})
	e.a.Online.TouchPush(n.ID)
	e.a.Online.SetTrafficUsers(n.ID, []uint64{u.ID, 9001, 9002, 9003, 9004})
	type onlineUser struct {
		UserID uint64   `json:"user_id"`
		Email  string   `json:"email"`
		IPs    []string `json:"ips"`
	}
	// The list is the one behind status.online: alive users plus the latest traffic report's users, who have no IPs
	// (an empty array, never null).
	rec := e.do(http.MethodGet, fmt.Sprintf("/nodes/%d/online", child.ID), nil)
	ou := testutil.Decode[[]onlineUser](t, rec)
	if len(ou) != 5 || ou[0].Email != "alice@example.com" || len(ou[0].IPs) != 2 || ou[0].IPs[0] != "198.51.100.7" ||
		ou[1].UserID != 9001 || ou[1].IPs == nil || strings.Contains(rec.Body.String(), "null") {
		t.Fatalf("online %s", rec.Body.String())
	}
	if v := testutil.Decode[nodesAdminView](t, e.do(http.MethodGet, fmt.Sprintf("/nodes/%d", child.ID), nil)); v.Status.Online != len(ou) {
		t.Fatalf("status.online = %d, list has %d", v.Status.Online, len(ou))
	}

	rt := e.do(http.MethodPost, "/routes", map[string]any{"remarks": "ads", "match": []string{"ads.example.com"}, "action": "block"})
	testutil.ExpectStatus(t, rt, http.StatusCreated)
	route := testutil.Decode[model.NodeRoute](t, rt)
	testutil.ExpectStatus(t, e.do(http.MethodPatch, fmt.Sprintf("/nodes/%d", n.ID), map[string]any{"route_ids": []uint64{route.ID}}), http.StatusOK)
	rec = testutil.DoWithHeaders(t, e.r, http.MethodPost, "/soga/api/v1/audit_log", fmt.Sprintf(`[{"user_id":%d,"audit_id":%d}]`, u.ID, route.ID*1000), "",
		map[string]string{"API-KEY": nodesAdminServerToken, "NODE-ID": fmt.Sprint(child.ID), "NODE-TYPE": "vless"})
	testutil.ExpectStatus(t, rec, http.StatusOK)
	hits := testutil.Decode[[]nodesvc.AuditHit](t, e.do(http.MethodGet, fmt.Sprintf("/nodes/%d/audit", n.ID), nil))
	if len(hits) != 1 || hits[0].Rule != "ads.example.com" || hits[0].NodeID != n.ID || hits[0].UserID != u.ID {
		t.Fatalf("audit %+v", hits)
	}
	testutil.ExpectStatus(t, e.do(http.MethodGet, "/nodes/999/online", nil), http.StatusNotFound)
}
