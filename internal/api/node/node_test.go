package node_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/api/node"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/router"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const (
	testToken = "node-test-token-0123456789abcdef"
	uuid1     = "3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f"
	uuid2     = "9d4c2e1a-0b3f-4a5c-8e7d-6f1a2b3c4d5e"
)

type env struct {
	t *testing.T
	a *app.App
	r *gin.Engine
}

func newEnv(t *testing.T, settings ...map[string]any) *env {
	t.Helper()
	s := map[string]any{"server_token": testToken}
	for _, m := range settings {
		for k, v := range m {
			s[k] = v
		}
	}
	a := testutil.NewTestApp(t, testutil.WithSettings(s))
	return &env{t: t, a: a, r: testutil.NewRouterWith(t, a, node.Register)}
}

func (e *env) group(name string) uint64 {
	e.t.Helper()
	g := model.ServerGroup{Name: name}
	if err := e.a.DB.Create(&g).Error; err != nil {
		e.t.Fatal(err)
	}
	return g.ID
}

func (e *env) node(n model.Node) *model.Node {
	e.t.Helper()
	if n.Port == "" {
		n.Port = "443"
	}
	if n.Name == "" {
		n.Name = n.Type + " node"
	}
	if err := e.a.DB.Omit(clause.Associations).Create(&n).Error; err != nil {
		e.t.Fatal(err)
	}
	nodesvc.New(e.a).Invalidate()
	return &n
}

func (e *env) route(r model.NodeRoute) *model.NodeRoute {
	e.t.Helper()
	if err := e.a.DB.Create(&r).Error; err != nil {
		e.t.Fatal(err)
	}
	return &r
}

// user creates an available user in group with the given uuid and limits.
func (e *env) user(email, uuid string, group uint64, speed, devices *int) *model.User {
	e.t.Helper()
	u := testutil.CreateUser(e.t, e.a, email, usersvc.CreateOptions{GroupID: &group, TransferEnable: 100 << 30, SpeedLimit: speed, DeviceLimit: devices})
	if uuid != "" {
		if err := e.a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("uuid", uuid).Error; err != nil {
			e.t.Fatal(err)
		}
		u.UUID = uuid
	}
	usersvc.New(e.a).InvalidateNodeUsers()
	return u
}

func uniURL(endpoint string, id uint64, typ string) string {
	return fmt.Sprintf("/api/v1/server/UniProxy/%s?node_id=%d&node_type=%s&token=%s", endpoint, id, typ, testToken)
}

func (e *env) do(method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	return testutil.DoWithHeaders(e.t, e.r, method, path, body, "", headers)
}

func sogaHeaders(id uint64, typ string) map[string]string {
	return map[string]string{"API-KEY": testToken, "NODE-ID": fmt.Sprint(id), "NODE-TYPE": typ}
}

func expectBody(t *testing.T, rec *httptest.ResponseRecorder, status int, body string) {
	t.Helper()
	if rec.Code != status || rec.Body.String() != body {
		t.Fatalf("got %d %s\nwant %d %s", rec.Code, rec.Body.String(), status, body)
	}
}

func (e *env) flush() {
	e.t.Helper()
	if err := e.a.FlushTraffic(context.Background()); err != nil {
		e.t.Fatal(err)
	}
}

// online is the admin view's online count of node id.
func (e *env) online(id uint64) int { return nodesvc.New(e.a).Status(id).Online }

func (e *env) userUD(id uint64) (int64, int64) {
	e.t.Helper()
	var u model.User
	if err := e.a.DB.Where("id = ?", id).First(&u).Error; err != nil {
		e.t.Fatal(err)
	}
	return u.U, u.D
}

// docSetup builds the data of docs/NODE_PROTOCOLS.md §7: users 1 and 2 in group 1 and route 1.
func docSetup(e *env) uint64 {
	g := e.group("default")
	e.user("one@example.com", uuid1, g, nil, testutil.Ptr(3))
	e.user("two@example.com", uuid2, g, testutil.Ptr(100), nil)
	e.route(model.NodeRoute{ID: 1, Match: model.JSONSlice[string]{"example-blocked.com"}, Action: "block"})
	return g
}

func realityNode(g uint64) model.Node {
	return model.Node{ID: 3, Type: "vless", Host: "hk1.example.com", ServerPort: 443, Rate: 1.5, Show: true,
		GroupIDs: model.JSONSlice[uint64]{g}, RouteIDs: model.JSONSlice[uint64]{1},
		ProtocolSettings: model.JSONMap{"network": "tcp", "tls": 2, "flow": "xtls-rprx-vision", "tls_settings": map[string]any{
			"server_name": "www.example.org", "dest": "www.example.org:443", "server_port": "443",
			"public_key": "PUBLIC_KEY_BASE64URL", "private_key": "PRIVATE_KEY_BASE64URL", "short_id": "0123abcd"}}}
}

// docs/NODE_PROTOCOLS.md §7.1 end to end.
func TestUniProxyWorkedExample(t *testing.T) {
	e := newEnv(t)
	e.node(realityNode(docSetup(e)))

	rec := e.do(http.MethodGet, uniURL("config", 3, "vless"), nil, nil)
	want := `{"protocol":"vless","listen_ip":"0.0.0.0","server_port":443,"host":"hk1.example.com","network":"tcp","networkSettings":null,"network_settings":null,` +
		`"tls":2,"flow":"xtls-rprx-vision","server_name":"www.example.org",` +
		`"tls_settings":{"server_name":"www.example.org","dest":"www.example.org","server_port":"443","public_key":"PUBLIC_KEY_BASE64URL","private_key":"PRIVATE_KEY_BASE64URL","short_id":"0123abcd","allow_insecure":false},` +
		`"tlsSettings":{"server_name":"www.example.org","dest":"www.example.org","server_port":"443","public_key":"PUBLIC_KEY_BASE64URL","private_key":"PRIVATE_KEY_BASE64URL","short_id":"0123abcd","allow_insecure":false},` +
		`"decryption":null,"base_config":{"push_interval":60,"pull_interval":60},` +
		`"routes":[{"id":1,"match":["example-blocked.com"],"action":"block","action_value":""}]}`
	expectBody(t, rec, http.StatusOK, want)
	if et := rec.Header().Get("ETag"); et != `"2bb00202abbadb48db07c406a2a1851180f3a03d"` {
		t.Fatalf("config etag %s", et)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("content type %s", ct)
	}

	rec = e.do(http.MethodGet, uniURL("user", 3, "vless"), nil, map[string]string{"X-Response-Format": "msgpack"})
	expectBody(t, rec, http.StatusOK, `{"users":[{"id":1,"uuid":"`+uuid1+`","speed_limit":0,"device_limit":3},{"id":2,"uuid":"`+uuid2+`","speed_limit":100,"device_limit":0}]}`)
	const userTag = `"19b74f5415250b984cc6359bfd8bd041e146d05f"`
	if et := rec.Header().Get("ETag"); et != userTag {
		t.Fatalf("user etag %s", et)
	}
	for _, inm := range []string{userTag, "W/" + userTag, strings.Trim(userTag, `"`)} {
		rec = e.do(http.MethodGet, uniURL("user", 3, "vless"), nil, map[string]string{"If-None-Match": inm})
		if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != userTag {
			t.Fatalf("If-None-Match %s: %d %q %s", inm, rec.Code, rec.Body.String(), rec.Header().Get("ETag"))
		}
	}

	rec = e.do(http.MethodPost, uniURL("push", 3, "vless"), `{"1":[1048576,52428800],"2":[2048,8192]}`, nil)
	expectBody(t, rec, http.StatusOK, `{"data":true}`)
	st, _ := e.a.Online.NodeStatus(3)
	if e.online(3) != 2 || st.LastPushAt == nil || st.LastCheckAt == nil {
		t.Fatalf("status %+v, online %d", st, e.online(3))
	}
	e.flush()
	if u, d := e.userUD(1); u != 1572864 || d != 78643200 {
		t.Fatalf("user 1 u=%d d=%d", u, d)
	}
	var su model.StatUser
	if err := e.a.DB.Where("user_id = ? AND node_id = ?", 1, 3).First(&su).Error; err != nil || su.Rate != 1.5 || su.U != 1572864 {
		t.Fatalf("stat_users %+v %v", su, err)
	}
	var sn model.StatNode
	if err := e.a.DB.Where("node_id = ?", 3).First(&sn).Error; err != nil || sn.U != 1048576+2048 || sn.D != 52428800+8192 {
		t.Fatalf("stat_nodes %+v %v", sn, err)
	}

	rec = e.do(http.MethodPost, uniURL("alive", 3, "vless"), `{"1":["203.0.113.7"],"2":["198.51.100.9"]}`, nil)
	expectBody(t, rec, http.StatusOK, `{"data":true}`)
	rec = e.do(http.MethodGet, uniURL("alivelist", 3, "vless"), nil, nil)
	expectBody(t, rec, http.StatusOK, `{"alive":{"1":1}}`)
	if rec.Header().Get("ETag") != "" {
		t.Fatal("alivelist must not carry an ETag")
	}
}

func TestUniProxyAuthErrors(t *testing.T) {
	e := newEnv(t)
	g := docSetup(e)
	e.node(realityNode(g))
	e.node(model.Node{ID: 4, Type: "hysteria", Host: "hy.example.com", ServerPort: 36712, ProtocolSettings: model.JSONMap{"version": 1}})
	e.node(model.Node{ID: 5, Type: "shadowsocksr", Host: "ssr.example.com", ServerPort: 443, ProtocolSettings: model.JSONMap{}})
	e.node(model.Node{ID: 6, Type: "vmess", Host: "v.example.com", ServerPort: 443, ProtocolSettings: model.JSONMap{}})
	e.node(model.Node{ID: 7, Type: "hysteria", Host: "hy2.example.com", ServerPort: 443, ProtocolSettings: model.JSONMap{"version": 2}})

	forbidden := `{"code":"forbidden","message":"invalid token"}`
	expectBody(t, e.do(http.MethodGet, "/api/v1/server/UniProxy/config?node_id=3&node_type=vless", nil, nil), http.StatusForbidden, forbidden)
	expectBody(t, e.do(http.MethodGet, "/api/v1/server/UniProxy/config?node_id=3&node_type=vless&token=wrong", nil, nil), http.StatusForbidden, forbidden)
	expectBody(t, e.do(http.MethodPost, "/api/v1/server/UniProxy/push?node_id=3&token=x", `{}`, nil), http.StatusForbidden, forbidden)

	bad := func(path, msg string) {
		t.Helper()
		rec := e.do(http.MethodGet, path, nil, nil)
		er := testutil.DecodeError(t, rec)
		if rec.Code != http.StatusBadRequest || er.Code != "validation_failed" || er.Message != msg {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	bad("/api/v1/server/UniProxy/config?token="+testToken, "node_id must be a positive integer")
	bad("/api/v1/server/UniProxy/config?node_id=abc&token="+testToken, "node_id must be a positive integer")
	bad(uniURL("config", 99, "vless"), "server is not exist")
	bad(uniURL("config", 3, "vmess"), "node 3 is vless, requested vmess")
	bad(uniURL("config", 3, "socks"), `unsupported node_type "socks"`)
	bad(uniURL("config", 4, "hysteria2"), "node 4 is hysteria v1, requested hysteria2")
	bad(uniURL("config", 5, "shadowsocksr"), `unsupported node_type "shadowsocksr"`)
	bad(uniURL("config", 5, ""), "node 5 is shadowsocksr, which UniProxy does not support")

	testutil.ExpectStatus(t, e.do(http.MethodGet, uniURL("config", 6, "v2ray"), nil, nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(http.MethodGet, uniURL("config", 7, "hysteria2"), nil, nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(http.MethodGet, uniURL("config", 3, "v2node"), nil, nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(http.MethodGet, uniURL("config", 3, ""), nil, nil), http.StatusOK)

	// An unset server_token rejects everything.
	if err := e.a.Settings.Set(map[string]any{"server_token": ""}); err != nil {
		t.Fatal(err)
	}
	expectBody(t, e.do(http.MethodGet, "/api/v1/server/UniProxy/config?node_id=3&token=", nil, nil), http.StatusForbidden, forbidden)
}

func TestUniProxyPushValidation(t *testing.T) {
	e := newEnv(t)
	e.node(realityNode(docSetup(e)))
	expectBody(t, e.do(http.MethodPost, uniURL("push", 3, "vless"), `{"1":`, nil), http.StatusBadRequest, `{"code":"validation_failed","message":"invalid data format"}`)
	expectBody(t, e.do(http.MethodPost, uniURL("push", 3, "vless"), `null`, nil), http.StatusBadRequest, `{"code":"validation_failed","message":"invalid data format"}`)
	expectBody(t, e.do(http.MethodPost, uniURL("push", 3, "vless"), `[]`, nil), http.StatusOK, `{"data":true}`)
	st, _ := e.a.Online.NodeStatus(3)
	if st.LastPushAt == nil || e.online(3) != 0 {
		t.Fatalf("empty push status %+v, online %d", st, e.online(3))
	}
	expectBody(t, e.do(http.MethodPost, uniURL("push", 3, "vless"), `{"1":[100,200],"2":[-1,5],"x":[1,1],"999":[10,10]}`, nil), http.StatusOK, `{"data":true}`)
	if got := e.online(3); got != 2 {
		t.Fatalf("online = %d, want 2 (users 1 and 999)", got)
	}
	e.flush() // unknown uid 999 is ignored at flush
	if u, d := e.userUD(1); u != 150 || d != 300 {
		t.Fatalf("user 1 u=%d d=%d", u, d)
	}
	if u, d := e.userUD(2); u != 0 || d != 0 {
		t.Fatalf("user 2 charged: %d %d", u, d)
	}
	expectBody(t, e.do(http.MethodPost, uniURL("alive", 3, "vless"), `"x"`, nil), http.StatusBadRequest, `{"code":"validation_failed","message":"invalid online data"}`)
}

func TestUniProxyStatus(t *testing.T) {
	e := newEnv(t)
	e.node(realityNode(docSetup(e)))
	rec := e.do(http.MethodPost, uniURL("status", 3, "vless"), `{"cpu":50,"mem":{"total":100,"used":25},"swap":{"total":0,"used":0},"disk":{"total":200,"used":100}}`, nil)
	expectBody(t, rec, http.StatusOK, `{"data":true,"code":0,"message":"success"}`)
	st, _ := e.a.Online.NodeStatus(3)
	if st.Load == nil || st.Load.CPU != 50 || st.Load.Mem != 25 || st.Load.Disk != 50 {
		t.Fatalf("load %+v", st.Load)
	}
	rec = e.do(http.MethodPost, uniURL("status", 3, "vless"), `{"cpu":50}`, nil)
	if er := testutil.DecodeError(t, rec); rec.Code != http.StatusBadRequest || len(er.Errors["mem"]) == 0 {
		t.Fatalf("missing mem: %d %s", rec.Code, rec.Body.String())
	}
}

func TestChildNodeResolution(t *testing.T) {
	e := newEnv(t)
	ga, gb := e.group("a"), e.group("b")
	ua := e.user("a@example.com", uuid1, ga, nil, nil)
	ub := e.user("b@example.com", uuid2, gb, nil, testutil.Ptr(2))
	parent := e.node(model.Node{Type: "vless", Host: "p.example.com", ServerPort: 443, Rate: 2, GroupIDs: model.JSONSlice[uint64]{ga},
		ProtocolSettings: model.JSONMap{"network": "ws", "tls": 1, "tls_settings": map[string]any{"server_name": "p.example.com"}}})
	child := e.node(model.Node{Type: "vless", Host: "relay.example.com", Port: "8443", ServerPort: 8443, Rate: 5, GroupIDs: model.JSONSlice[uint64]{gb}, ParentID: &parent.ID,
		ProtocolSettings: model.JSONMap{"network": "grpc"}})

	viaParent := e.do(http.MethodGet, uniURL("config", parent.ID, "vless"), nil, nil)
	viaChild := e.do(http.MethodGet, uniURL("config", child.ID, "vless"), nil, nil)
	if viaChild.Code != http.StatusOK || viaChild.Body.String() != viaParent.Body.String() {
		t.Fatalf("child config differs:\n%s\n%s", viaChild.Body.String(), viaParent.Body.String())
	}
	wantUsers := fmt.Sprintf(`{"users":[{"id":%d,"uuid":"%s","speed_limit":0,"device_limit":0},{"id":%d,"uuid":"%s","speed_limit":0,"device_limit":2}]}`, ua.ID, uuid1, ub.ID, uuid2)
	expectBody(t, e.do(http.MethodGet, uniURL("user", child.ID, "vless"), nil, nil), http.StatusOK, wantUsers)
	expectBody(t, e.do(http.MethodGet, uniURL("user", parent.ID, "vless"), nil, nil), http.StatusOK, wantUsers)

	expectBody(t, e.do(http.MethodPost, uniURL("push", child.ID, "vless"), fmt.Sprintf(`{"%d":[100,1000]}`, ub.ID), nil), http.StatusOK, `{"data":true}`)
	e.flush()
	if u, d := e.userUD(ub.ID); u != 200 || d != 2000 {
		t.Fatalf("child traffic must use the parent's rate 2: u=%d d=%d", u, d)
	}
	var su model.StatUser
	if err := e.a.DB.Where("user_id = ?", ub.ID).First(&su).Error; err != nil || su.NodeID != parent.ID || su.Rate != 2 {
		t.Fatalf("stat_users %+v %v", su, err)
	}
	if _, ok := e.a.Online.NodeStatus(child.ID); ok {
		t.Fatal("child must not have its own runtime status")
	}
	if got := e.online(parent.ID); got != 1 {
		t.Fatalf("parent online = %d", got)
	}

	n, err := nodesvc.New(e.a).Find(context.Background(), child.ID, "vless")
	if err != nil || n.Host != "relay.example.com" || n.ProtocolSettings.String("network") != "ws" || n.Parent == nil {
		t.Fatalf("Find child: %+v %v", n, err)
	}
}

func TestAliveListDeviceLimitModes(t *testing.T) {
	for _, tc := range []struct {
		mode int
		want string
	}{{0, `{"alive":{"1":2}}`}, {1, `{"alive":{"1":1}}`}} {
		t.Run(fmt.Sprint(tc.mode), func(t *testing.T) {
			e := newEnv(t, map[string]any{"device_limit_mode": tc.mode})
			g := e.group("g")
			e.user("one@example.com", uuid1, g, nil, testutil.Ptr(5))
			e.user("two@example.com", uuid2, g, nil, nil) // device_limit 0: never listed
			other := e.user("x@example.com", "", e.group("other"), nil, testutil.Ptr(5))
			e.node(model.Node{ID: 1, Type: "vmess", Host: "v.example.com", ServerPort: 443, GroupIDs: model.JSONSlice[uint64]{g}})
			body := fmt.Sprintf(`{"1":["203.0.113.7:1234","203.0.113.8"],"2":["198.51.100.1"],"%d":["192.0.2.1"]}`, other.ID)
			expectBody(t, e.do(http.MethodPost, uniURL("alive", 1, "vmess"), body, nil), http.StatusOK, `{"data":true}`)
			expectBody(t, e.do(http.MethodGet, uniURL("alivelist", 1, "vmess"), nil, nil), http.StatusOK, tc.want)
		})
	}
}

func TestUniProxyETagChangesOnWrites(t *testing.T) {
	e := newEnv(t)
	e.node(realityNode(docSetup(e)))
	tag := func(endpoint string) string {
		rec := e.do(http.MethodGet, uniURL(endpoint, 3, "vless"), nil, nil)
		testutil.ExpectStatus(t, rec, http.StatusOK)
		return rec.Header().Get("ETag")
	}
	cfg, users := tag("config"), tag("user")
	if err := e.a.Settings.Set(map[string]any{"server_push_interval": 30}); err != nil {
		t.Fatal(err)
	}
	if tag("config") == cfg {
		t.Fatal("config ETag must change with base_config")
	}
	if err := e.a.DB.Model(&model.User{}).Where("id = ?", 2).Update("banned", true).Error; err != nil {
		t.Fatal(err)
	}
	usersvc.New(e.a).InvalidateNodeUsers()
	if tag("user") == users {
		t.Fatal("user ETag must change after a ban")
	}
	rec := e.do(http.MethodGet, uniURL("user", 3, "vless"), nil, nil)
	if strings.Contains(rec.Body.String(), uuid2) {
		t.Fatalf("banned user still listed: %s", rec.Body.String())
	}
}

// docs/NODE_PROTOCOLS.md §7.2 end to end, on both mounts.
func TestSogaWorkedExample(t *testing.T) {
	e := newEnv(t)
	g := docSetup(e)
	e.node(model.Node{ID: 7, Type: "shadowsocks", ServerPort: 8388, Host: "ss.example.com", Rate: 1, CreatedAt: time.Unix(1735689600, 0), GroupIDs: model.JSONSlice[uint64]{g},
		RouteIDs: model.JSONSlice[uint64]{1}, ProtocolSettings: model.JSONMap{"cipher": "2022-blake3-aes-128-gcm", "obfs": "plain"}})
	h := sogaHeaders(7, "shadowsocks")
	for _, prefix := range []string{"/soga/api/v1", "/api/v1"} {
		rec := e.do(http.MethodGet, prefix+"/node", nil, h)
		expectBody(t, rec, http.StatusOK, `{"basic":{"pull_interval":60,"push_interval":60,"speed_limit":0},"config":{"port":8388,"cipher":"2022-blake3-aes-128-gcm","password":"YjVkODdhNWRlMWEzNWIyMg==","obfs":"plain"}}`)
		if et := rec.Header().Get("ETag"); et != `"f8d278af3b4f513c6ab09ac8025dbc61e9e60e0c"` {
			t.Fatalf("node etag %s", et)
		}
		rec = e.do(http.MethodGet, prefix+"/users", nil, h)
		expectBody(t, rec, http.StatusOK, `[{"id":1,"password":"M2IxYjZmNWUtN2MyYS00ZA==","speed_limit":0,"device_limit":3,"tcp_limit":0},{"id":2,"password":"OWQ0YzJlMWEtMGIzZi00YQ==","speed_limit":100,"device_limit":0,"tcp_limit":0}]`)
		const usersTag = `"e2e3870b1bd7ccc5eb600870fb0627dce73202d2"`
		if et := rec.Header().Get("ETag"); et != usersTag {
			t.Fatalf("users etag %s", et)
		}
		withINM := map[string]string{"If-None-Match": usersTag}
		for k, v := range h {
			withINM[k] = v
		}
		if rec = e.do(http.MethodGet, prefix+"/users", nil, withINM); rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
			t.Fatalf("users 304: %d %s", rec.Code, rec.Body.String())
		}
	}

	ok := `{"code":0,"message":""}`
	expectBody(t, e.do(http.MethodPost, "/soga/api/v1/traffic", `[{"id":1,"u":1024,"d":4096},{"id":2,"u":0,"d":512}]`, h), http.StatusOK, ok)
	st, _ := e.a.Online.NodeStatus(7)
	if e.online(7) != 2 || st.LastPushAt == nil {
		t.Fatalf("status %+v, online %d", st, e.online(7))
	}
	e.flush()
	if u, d := e.userUD(1); u != 1024 || d != 4096 {
		t.Fatalf("user 1 u=%d d=%d", u, d)
	}
	expectBody(t, e.do(http.MethodPost, "/soga/api/v1/alive_ip", `[{"id":1,"ips":["203.0.113.7","198.51.100.9"]}]`, h), http.StatusOK, ok)
	if got := e.a.Online.AliveIPs(1); len(got) != 2 {
		t.Fatalf("alive ips %v", got)
	}
	expectBody(t, e.do(http.MethodPost, "/soga/api/v1/audit_log", `[{"user_id":1,"audit_id":1000}]`, h), http.StatusOK, ok)
	hits := nodesvc.New(e.a).AuditHits(7)
	if len(hits) != 1 || hits[0].RouteID != 1 || hits[0].Rule != "example-blocked.com" || hits[0].UserID != 1 {
		t.Fatalf("audit hits %+v", hits)
	}
	expectBody(t, e.do(http.MethodPost, "/soga/api/v1/status", `{"cpu":12.5,"mem":{"total":2147483648,"used":734003200},"swap":{"total":0,"used":0},"disk":{"total":42949672960,"used":10737418240},"uptime":86400}`, h), http.StatusOK, ok)
	if st, _ := e.a.Online.NodeStatus(7); st.Load == nil || st.Load.Uptime != 86400 || st.Load.CPU != 12.5 {
		t.Fatalf("load %+v", st.Load)
	}
	for _, body := range []string{"null", "[]"} {
		expectBody(t, e.do(http.MethodPost, "/api/v1/traffic", body, h), http.StatusOK, ok)
	}
	expectBody(t, e.do(http.MethodPost, "/api/v1/traffic", `{"id":1}`, h), http.StatusBadRequest, `{"code":400,"message":"invalid traffic data"}`)
}

func TestSogaAuthAndTypes(t *testing.T) {
	e := newEnv(t)
	g := e.group("g")
	e.node(model.Node{ID: 1, Type: "shadowsocks", Host: "ss.example.com", ServerPort: 8388, GroupIDs: model.JSONSlice[uint64]{g}, ProtocolSettings: model.JSONMap{"cipher": "aes-128-gcm"}})
	e.node(model.Node{ID: 2, Type: "hysteria", Host: "hy.example.com", ServerPort: 443, ProtocolSettings: model.JSONMap{"version": 1}})
	e.node(model.Node{ID: 3, Type: "tuic", Host: "tu.example.com", ServerPort: 443})
	e.node(model.Node{ID: 4, Type: "shadowsocksr", Host: "ssr.example.com", ServerPort: 443, ProtocolSettings: model.JSONMap{"password": "node-pass"}})

	expectBody(t, e.do(http.MethodGet, "/soga/api/v1/node", nil, map[string]string{"API-KEY": "wrong", "NODE-ID": "1"}), http.StatusForbidden, `{"code":403,"message":"invalid API-KEY"}`)
	expectBody(t, e.do(http.MethodPost, "/soga/api/v1/traffic", `[]`, map[string]string{"NODE-ID": "1"}), http.StatusForbidden, `{"code":403,"message":"invalid API-KEY"}`)
	expectBody(t, e.do(http.MethodGet, "/soga/api/v1/node", nil, sogaHeaders(99, "ss")), http.StatusBadRequest, `{"code":400,"message":"node not found"}`)
	expectBody(t, e.do(http.MethodGet, "/soga/api/v1/node", nil, map[string]string{"API-KEY": testToken}), http.StatusBadRequest, `{"code":400,"message":"NODE-ID must be a positive integer"}`)
	expectBody(t, e.do(http.MethodGet, "/soga/api/v1/node", nil, sogaHeaders(1, "vmess")), http.StatusBadRequest, `{"code":400,"message":"node 1 is shadowsocks, requested vmess"}`)
	expectBody(t, e.do(http.MethodGet, "/soga/api/v1/node", nil, sogaHeaders(3, "tuic")), http.StatusBadRequest, `{"code":400,"message":"unsupported NODE-TYPE \"tuic\""}`)
	expectBody(t, e.do(http.MethodGet, "/soga/api/v1/node", nil, sogaHeaders(3, "")), http.StatusBadRequest, `{"code":400,"message":"node 3 is tuic, which soga does not support"}`)
	expectBody(t, e.do(http.MethodGet, "/soga/api/v1/node", nil, sogaHeaders(2, "hysteria")), http.StatusBadRequest, `{"code":400,"message":"node 2 is hysteria v1, soga supports hysteria2 only"}`)

	// Aliases, a missing NODE-TYPE and the doubled slash of a naively joined webapi_url all work.
	for _, path := range []string{"/soga/api/v1/node", "/soga//api/v1/node", "/api/v1/node"} {
		for _, typ := range []string{"ss", "shadowsocks", ""} {
			testutil.ExpectStatus(t, e.do(http.MethodGet, path, nil, sogaHeaders(1, typ)), http.StatusOK)
		}
	}
	rec := e.do(http.MethodGet, "/soga/api/v1/node", nil, sogaHeaders(4, "ssr"))
	if !strings.Contains(rec.Body.String(), `"password":"node-pass"`) {
		t.Fatalf("ssr node: %s", rec.Body.String())
	}
	// Every soga endpoint exists on both mounts (a 404 stops soga).
	for _, prefix := range []string{"/api/v1", "/soga/api/v1"} {
		for _, ep := range []string{"node", "users", "audit_rules", "white_list"} {
			testutil.ExpectStatus(t, e.do(http.MethodGet, prefix+"/"+ep, nil, sogaHeaders(1, "ss")), http.StatusOK)
		}
		for _, ep := range []string{"traffic", "alive_ip", "audit_log"} {
			testutil.ExpectStatus(t, e.do(http.MethodPost, prefix+"/"+ep, `[]`, sogaHeaders(1, "ss")), http.StatusOK)
		}
		testutil.ExpectStatus(t, e.do(http.MethodPost, prefix+"/status", `{"cpu":1}`, sogaHeaders(1, "ss")), http.StatusOK)
	}
}

func TestSogaAuditRulesAndWhiteList(t *testing.T) {
	e := newEnv(t, map[string]any{"server_white_list": []string{"domain:example.com", "ip:192.0.2.0/24"}})
	e.route(model.NodeRoute{ID: 2, Action: "block", Match: model.JSONSlice[string]{"example-blocked.com", "protocol:bittorrent", "regexp:.*\\.example\\.net"}})
	e.route(model.NodeRoute{ID: 3, Action: "dns", ActionValue: "198.51.100.53", Match: model.JSONSlice[string]{"example.org"}})
	e.node(model.Node{ID: 1, Type: "vmess", Host: "v.example.com", ServerPort: 443, RouteIDs: model.JSONSlice[uint64]{3, 2}})
	h := sogaHeaders(1, "v2ray")
	rec := e.do(http.MethodGet, "/soga/api/v1/audit_rules", nil, h)
	expectBody(t, rec, http.StatusOK, `[{"id":2000,"rule":"example-blocked.com"},{"id":2002,"rule":"regexp:.*\\.example\\.net"}]`)
	if rec.Header().Get("ETag") == "" {
		t.Fatal("audit_rules needs an ETag")
	}
	rec = e.do(http.MethodGet, "/soga/api/v1/white_list", nil, h)
	expectBody(t, rec, http.StatusOK, `["domain:example.com","ip:192.0.2.0/24"]`)
	inm := map[string]string{"If-None-Match": rec.Header().Get("ETag")}
	for k, v := range h {
		inm[k] = v
	}
	testutil.ExpectStatus(t, e.do(http.MethodGet, "/soga/api/v1/white_list", nil, inm), http.StatusNotModified)

	var cfg map[string]any
	rec = e.do(http.MethodGet, uniURL("config", 1, "vmess"), nil, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if routes, _ := cfg["routes"].([]any); len(routes) != 2 || routes[0].(map[string]any)["id"] != float64(2) {
		t.Fatalf("routes %v", cfg["routes"])
	}
}

func TestSogaUsersEmptyAndETag(t *testing.T) {
	e := newEnv(t)
	e.node(model.Node{ID: 1, Type: "mieru", Host: "m.example.com", ServerPort: 2999})
	rec := e.do(http.MethodGet, "/soga/api/v1/users", nil, sogaHeaders(1, "mieru"))
	expectBody(t, rec, http.StatusOK, `[]`)
	if rec.Header().Get("ETag") == "" {
		t.Fatal("empty user list needs an ETag")
	}
	expectBody(t, e.do(http.MethodGet, uniURL("user", 1, "mieru"), nil, nil), http.StatusOK, `{"users":[]}`)
}

// The node routes coexist with every other package and the SPA fallback of the full router.
func TestFullRouterMounts(t *testing.T) {
	e := newEnv(t)
	full, err := router.New(e.a)
	if err != nil {
		t.Fatal(err)
	}
	e.node(model.Node{ID: 1, Type: "trojan", Host: "t.example.com", ServerPort: 443})
	for _, path := range []string{"/soga/api/v1/node", "/api/v1/node", "/soga//api/v1/users", "/api/v1/users", "/api/v1/status"} {
		method := http.MethodGet
		if strings.HasSuffix(path, "status") {
			method = http.MethodPost
		}
		rec := testutil.DoWithHeaders(t, full, method, path, `{}`, "", sogaHeaders(1, "trojan"))
		testutil.ExpectStatus(t, rec, http.StatusOK)
		rec = testutil.DoWithHeaders(t, full, method, path, `{}`, "", map[string]string{"API-KEY": "wrong"})
		expectBody(t, rec, http.StatusForbidden, `{"code":403,"message":"invalid API-KEY"}`)
	}
	testutil.ExpectStatus(t, testutil.Do(t, full, http.MethodGet, uniURL("config", 1, "trojan"), nil, ""), http.StatusOK)
}
