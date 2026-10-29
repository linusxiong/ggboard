package node_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const (
	nodeAToken = "node-a-own-token-0123456789"
	nodeCToken = "node-c-own-token-0123456789"
)

func uniURLWith(endpoint string, id uint64, typ, token string) string {
	return fmt.Sprintf("/api/v1/server/UniProxy/%s?node_id=%d&node_type=%s&token=%s", endpoint, id, typ, token)
}

func sogaHeadersWith(id uint64, typ, key string) map[string]string {
	h := sogaHeaders(id, typ)
	h["API-KEY"] = key
	return h
}

// Security review 2026-10-29: with one shared server_token, the operator of any node could change node_id and pull
// every other node's users (UUIDs, REALITY private keys, SS2022 server keys). A node with its own token now accepts
// only that token, and that token works for no other node.
func TestNodeOwnTokenBindsTheNode(t *testing.T) {
	e := newEnv(t)
	ga, gb := e.group("a"), e.group("b")
	a := e.node(model.Node{Type: "vmess", Host: "a.example.com", ServerPort: 443, Rate: 1, GroupIDs: model.JSONSlice[uint64]{ga}, Token: nodeAToken, ProtocolSettings: model.JSONMap{}})
	child := e.node(model.Node{Type: "vmess", Host: "a2.example.com", ServerPort: 443, Rate: 1, ParentID: &a.ID, ProtocolSettings: model.JSONMap{}})
	b := e.node(model.Node{Type: "vmess", Host: "b.example.com", ServerPort: 443, Rate: 1, GroupIDs: model.JSONSlice[uint64]{gb}, ProtocolSettings: model.JSONMap{}})
	c := e.node(model.Node{Type: "vmess", Host: "c.example.com", ServerPort: 443, Rate: 1, GroupIDs: model.JSONSlice[uint64]{gb}, Token: nodeCToken, ProtocolSettings: model.JSONMap{}})
	ua := e.user("a@example.com", uuid1, ga, nil, nil)
	e.user("b@example.com", uuid2, gb, nil, nil)
	forbidden := `{"code":"forbidden","message":"invalid token"}`

	// The node's own token serves the node and its children.
	for _, id := range []uint64{a.ID, child.ID} {
		rec := e.do(http.MethodGet, uniURLWith("user", id, "vmess", nodeAToken), nil, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), uuid1) || strings.Contains(rec.Body.String(), uuid2) {
			t.Fatalf("own token on node %d = %d %s", id, rec.Code, rec.Body.String())
		}
	}
	if rec := e.do(http.MethodGet, "/soga/api/v1/users", nil, sogaHeadersWith(a.ID, "vmess", nodeAToken)); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), uuid1) {
		t.Fatalf("soga, own token = %d %s", rec.Code, rec.Body.String())
	}

	// ...and nothing else: not a node on the shared token, not a node with another own token, not an unknown id
	// (which would tell the key's holder which ids exist).
	for _, id := range []uint64{b.ID, c.ID, 9999} {
		for _, ep := range []string{"config", "user"} {
			expectBody(t, e.do(http.MethodGet, uniURLWith(ep, id, "vmess", nodeAToken), nil, nil), http.StatusForbidden, forbidden)
		}
		expectBody(t, e.do(http.MethodPost, uniURLWith("push", id, "vmess", nodeAToken), fmt.Sprintf(`{"%d":[1,1]}`, ua.ID), nil), http.StatusForbidden, forbidden)
		expectBody(t, e.do(http.MethodGet, "/soga/api/v1/users", nil, sogaHeadersWith(id, "vmess", nodeAToken)), http.StatusForbidden, `{"code":403,"message":"invalid API-KEY"}`)
	}

	// The shared server_token no longer works for a node with its own token (or its children), and still works for
	// the others.
	for _, id := range []uint64{a.ID, child.ID, c.ID} {
		expectBody(t, e.do(http.MethodGet, uniURL("user", id, "vmess"), nil, nil), http.StatusForbidden, forbidden)
		expectBody(t, e.do(http.MethodGet, "/api/v1/users", nil, sogaHeaders(id, "vmess")), http.StatusForbidden, `{"code":403,"message":"invalid API-KEY"}`)
	}
	testutil.ExpectStatus(t, e.do(http.MethodGet, uniURL("user", b.ID, "vmess"), nil, nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(http.MethodGet, uniURLWith("user", c.ID, "vmess", nodeCToken), nil, nil), http.StatusOK)
	expectBody(t, e.do(http.MethodGet, uniURL("user", 9999, "vmess"), nil, nil), http.StatusBadRequest, `{"code":"validation_failed","message":"server is not exist"}`)

	// Nothing reached node B's traffic from the refused pushes.
	e.flush()
	if u, d := e.userUD(ua.ID); u != 0 || d != 0 {
		t.Fatalf("refused pushes charged user a: %d/%d", u, d)
	}

	// Cross-node attempts are failures like wrong tokens (charged to the address); the node's own requests keep
	// working once the bucket is empty.
	limited := false
	for range 40 {
		if e.do(http.MethodGet, uniURLWith("user", b.ID, "vmess", nodeAToken), nil, nil).Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("cross-node attempts are never limited")
	}
	testutil.ExpectStatus(t, e.do(http.MethodGet, uniURLWith("user", a.ID, "vmess", nodeAToken), nil, nil), http.StatusOK)

	// Own tokens work without a shared server_token.
	if err := e.a.Settings.Set(map[string]any{"server_token": ""}); err != nil {
		t.Fatal(err)
	}
	testutil.ExpectStatus(t, e.do(http.MethodGet, uniURLWith("user", c.ID, "vmess", nodeCToken), nil, nil), http.StatusOK)
	if err := e.a.Settings.Set(map[string]any{"server_token": testToken}); err != nil {
		t.Fatal(err)
	}

	// Clearing the own token hands the node back to server_token; the old key stops working at once.
	if err := e.a.DB.Model(&model.Node{}).Where("id = ?", a.ID).Update("token", "").Error; err != nil {
		t.Fatal(err)
	}
	nodesvc.New(e.a).Invalidate()
	testutil.ExpectStatus(t, e.do(http.MethodGet, uniURL("user", a.ID, "vmess"), nil, nil), http.StatusOK)
	if rec := e.do(http.MethodGet, uniURLWith("user", a.ID, "vmess", nodeAToken), nil, nil); rec.Code != http.StatusForbidden && rec.Code != http.StatusTooManyRequests {
		t.Fatalf("old own token after clearing = %d %s", rec.Code, rec.Body.String())
	}
}

// Security review 2026-10-29: push accepted any existing uid, so a token holder could exhaust anyone's traffic.
// Reports now count only for the users the node serves: its /user list, plus users of its groups that became
// unavailable since the last pull (they still pay for what they used).
func TestNodeReportsOnlyServedUsers(t *testing.T) {
	e := newEnv(t)
	ga, gb, gc := e.group("a"), e.group("b"), e.group("c")
	b := e.node(model.Node{Type: "vmess", Host: "b.example.com", ServerPort: 443, Rate: 1, GroupIDs: model.JSONSlice[uint64]{gb}, ProtocolSettings: model.JSONMap{}})
	e.node(model.Node{Type: "vmess", Host: "b2.example.com", ServerPort: 443, Rate: 1, ParentID: &b.ID, GroupIDs: model.JSONSlice[uint64]{gc}, ProtocolSettings: model.JSONMap{}})
	outsider := e.user("outsider@example.com", "", ga, nil, nil)
	member := e.user("member@example.com", "", gb, nil, nil)
	viaChild := e.user("child@example.com", "", gc, nil, nil)
	spent := testutil.CreateUser(t, e.a, "spent@example.com", usersvc.CreateOptions{GroupID: &gb, TransferEnable: 10})
	if err := e.a.DB.Model(&model.User{}).Where("id = ?", spent.ID).Update("u", 10).Error; err != nil {
		t.Fatal(err)
	}
	usersvc.New(e.a).InvalidateNodeUsers()

	push := fmt.Sprintf(`{"%d":[100,100],"%d":[10,20],"%d":[3,4],"%d":[5,5],"424242":[1,1]}`, outsider.ID, member.ID, viaChild.ID, spent.ID)
	expectBody(t, e.do(http.MethodPost, uniURL("push", b.ID, "vmess"), push, nil), http.StatusOK, `{"data":true}`)
	soga := fmt.Sprintf(`[{"id":%d,"u":100,"d":100},{"id":%d,"u":1,"d":1}]`, outsider.ID, member.ID)
	expectBody(t, e.do(http.MethodPost, "/api/v1/traffic", soga, sogaHeaders(b.ID, "vmess")), http.StatusOK, `{"code":0,"message":""}`)
	if got := e.online(b.ID); got != 1 {
		t.Fatalf("online = %d, want 1 (the soga report named one served user)", got)
	}
	e.flush()
	for _, c := range []struct {
		name string
		id   uint64
		u, d int64
	}{{"outsider", outsider.ID, 0, 0}, {"member", member.ID, 11, 21}, {"child group", viaChild.ID, 3, 4}, {"spent", spent.ID, 15, 5}} {
		if u, d := e.userUD(c.id); u != c.u || d != c.d {
			t.Errorf("%s: u/d = %d/%d, want %d/%d", c.name, u, d, c.u, c.d)
		}
	}

	alive := fmt.Sprintf(`{"%d":["198.51.100.1"],"%d":["198.51.100.2"]}`, outsider.ID, member.ID)
	expectBody(t, e.do(http.MethodPost, uniURL("alive", b.ID, "vmess"), alive, nil), http.StatusOK, `{"data":true}`)
	sogaAlive := fmt.Sprintf(`[{"id":%d,"ips":["198.51.100.3"]}]`, outsider.ID)
	expectBody(t, e.do(http.MethodPost, "/api/v1/alive_ip", sogaAlive, sogaHeaders(b.ID, "vmess")), http.StatusOK, `{"code":0,"message":""}`)
	if ips := e.a.Online.AliveIPs(outsider.ID); len(ips) != 0 {
		t.Fatalf("outsider alive IPs = %v (they would count against the device limit everywhere)", ips)
	}
	if ips := e.a.Online.AliveIPs(member.ID); len(ips) != 1 {
		t.Fatalf("member alive IPs = %v", ips)
	}

	audit := fmt.Sprintf(`[{"user_id":%d,"audit_id":1001},{"user_id":%d,"audit_id":1001}]`, outsider.ID, member.ID)
	expectBody(t, e.do(http.MethodPost, "/api/v1/audit_log", audit, sogaHeaders(b.ID, "vmess")), http.StatusOK, `{"code":0,"message":""}`)
	hits := nodesvc.New(e.a).AuditHits(b.ID)
	if len(hits) != 1 || hits[0].UserID != member.ID {
		t.Fatalf("audit hits = %+v", hits)
	}
}
