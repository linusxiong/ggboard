package node_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
)

// Review 2026-10-22: a uid ≥ 2^63 cannot be bound by database/sql; before the fix it was buffered, failed the
// flush transaction, was restored and failed every later flush, so nobody's traffic was ever recorded.
func TestReviewPushHugeUIDDoesNotPoisonFlush(t *testing.T) {
	e := newEnv(t)
	g := e.group("g")
	n := e.node(model.Node{Type: "vmess", Host: "v.example.com", ServerPort: 443, Rate: 1, GroupIDs: model.JSONSlice[uint64]{g}, ProtocolSettings: model.JSONMap{}})
	u := e.user("huge@example.com", uuid1, g, nil, nil)

	body := fmt.Sprintf(`{"9223372036854775808":[1,1],"18446744073709551615":[1,1],"424242":[5,5],"%d":[10,20]}`, u.ID)
	rec := e.do(http.MethodPost, uniURL("push", n.ID, "vmess"), body, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("push = %d %s", rec.Code, rec.Body.String())
	}
	e.flush()
	e.flush() // nothing restored
	if p := e.a.Traffic.Pending(); p != 0 {
		t.Fatalf("pending after flush = %d", p)
	}
	var got model.User
	if err := e.a.DB.First(&got, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.U != 10 || got.D != 20 {
		t.Fatalf("u/d = %d/%d", got.U, got.D)
	}
	var rows int64
	e.a.DB.Model(&model.StatUser{}).Count(&rows)
	if rows != 1 {
		t.Fatalf("stat_users rows = %d, want 1 (unknown uids are ignored)", rows)
	}

	// soga NODE-ID beyond int64 is a bad id (400), never a 500.
	h := sogaHeaders(0, "vmess")
	h["NODE-ID"] = "9223372036854775808"
	if rec := e.do(http.MethodGet, "/api/v1/node", nil, h); rec.Code != http.StatusBadRequest {
		t.Fatalf("huge NODE-ID = %d %s", rec.Code, rec.Body.String())
	}
}

// Review 2026-10-22: counters above MaxReportCount are dropped instead of overflowing users.u.
func TestReviewPushRejectsAbsurdCounters(t *testing.T) {
	e := newEnv(t)
	g := e.group("g")
	n := e.node(model.Node{Type: "vmess", Host: "v.example.com", ServerPort: 443, Rate: 1, GroupIDs: model.JSONSlice[uint64]{g}, ProtocolSettings: model.JSONMap{}})
	u := e.user("big@example.com", uuid1, g, nil, nil)
	for range 2 {
		body := fmt.Sprintf(`{"%d":[9223372036854775807,0]}`, u.ID)
		if rec := e.do(http.MethodPost, uniURL("push", n.ID, "vmess"), body, nil); rec.Code != http.StatusOK {
			t.Fatalf("push = %d", rec.Code)
		}
	}
	e.flush()
	var got model.User
	if err := e.a.DB.First(&got, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.U != 0 {
		t.Fatalf("u = %d, want 0 (absurd counter dropped)", got.U)
	}
}

// Review 2026-10-22: alive reports are bounded per user (count and address length).
func TestReviewAliveBounded(t *testing.T) {
	e := newEnv(t)
	g := e.group("g")
	n := e.node(model.Node{Type: "vmess", Host: "v.example.com", ServerPort: 443, Rate: 1, GroupIDs: model.JSONSlice[uint64]{g}, ProtocolSettings: model.JSONMap{}})
	u := e.user("alive@example.com", uuid1, g, nil, nil)
	ips := make([]string, 0, 1000)
	for i := range 1000 {
		ips = append(ips, fmt.Sprintf(`"10.%d.%d.1"`, i/256, i%256))
	}
	ips = append(ips, `"`+strings.Repeat("a", 10000)+`"`)
	body := fmt.Sprintf(`{"%d":[%s]}`, u.ID, strings.Join(ips, ","))
	if rec := e.do(http.MethodPost, uniURL("alive", n.ID, "vmess"), body, nil); rec.Code != http.StatusOK {
		t.Fatalf("alive = %d %s", rec.Code, rec.Body.String())
	}
	got := e.a.Online.AliveIPs(u.ID)
	if len(got) != nodesvc.MaxAliveIPsPerUser {
		t.Fatalf("alive ips = %d, want %d", len(got), nodesvc.MaxAliveIPsPerUser)
	}
	for _, ip := range got {
		if len(ip) > nodesvc.MaxAliveIPLen {
			t.Fatalf("oversized ip kept: %d bytes", len(ip))
		}
	}
}

// Review 2026-10-22: a request that resolved P before an admin write must not cache a body rendered from the
// old P under the new cache generation.
func TestReviewNodeBodyNotStaleAfterWrite(t *testing.T) {
	e := newEnv(t)
	g := e.group("g")
	n := e.node(model.Node{Type: "vmess", Host: "v.example.com", ServerPort: 443, Rate: 1, GroupIDs: model.JSONSlice[uint64]{g}, ProtocolSettings: model.JSONMap{}})
	svc := nodesvc.New(e.a)
	ctx := context.Background()
	r1, err := svc.Resolve(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Admin write between Resolve and rendering.
	if err := e.a.DB.Model(&model.Node{}).Where("id = ?", n.ID).Updates(map[string]any{"server_port": 8443, "updated_at": time.Now().Add(time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	svc.Invalidate()
	if _, err := svc.UniProxyConfig(ctx, r1); err != nil { // in-flight request renders the old P
		t.Fatal(err)
	}
	r2, err := svc.Resolve(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.UniProxyConfig(ctx, r2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b.JSON), "8443") {
		t.Fatalf("stale config served after write: %s", b.JSON)
	}
}

// Review 2026-10-22: node endpoints had no limit at all, so server_token could be guessed online. Failed checks
// are now limited per client address; once exhausted the token is not even compared.
func TestReviewNodeTokenFailuresLimited(t *testing.T) {
	e := newEnv(t)
	g := e.group("g")
	n := e.node(model.Node{Type: "vmess", Host: "v.example.com", ServerPort: 443, Rate: 1, GroupIDs: model.JSONSlice[uint64]{g}, ProtocolSettings: model.JSONMap{}})
	bad := fmt.Sprintf("/api/v1/server/UniProxy/config?node_id=%d&node_type=vmess&token=wrong", n.ID)
	codes := map[int]int{}
	for range 31 {
		codes[e.do(http.MethodGet, bad, nil, nil).Code]++
	}
	if codes[http.StatusForbidden] != 30 || codes[http.StatusTooManyRequests] != 1 {
		t.Fatalf("codes = %v", codes)
	}
	h := sogaHeaders(n.ID, "vmess")
	if rec := e.do(http.MethodGet, "/soga/api/v1/node", nil, h); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("exhausted address with the right key = %d (the key must not be compared)", rec.Code)
	}
}

// Review 2026-10-22: the admin "online users" view bound every alive uid in one IN list; above 32766 SQLite (65535
// PostgreSQL/MySQL) parameters the query failed and the page answered 500.
func TestReviewOnlineUsersChunked(t *testing.T) {
	e := newEnv(t)
	u := e.user("online@example.com", uuid1, e.group("g"), nil, nil)
	const n = 33000
	alive := make(map[uint64][]string, n)
	for i := range uint64(n) {
		alive[1_000_000+i] = []string{"10.0.0.1"}
	}
	alive[u.ID] = []string{"10.0.0.2"}
	e.a.Online.SetNodeAlive(7, alive)
	got, err := nodesvc.New(e.a).OnlineUsers(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n+1 || got[0].UserID != u.ID || got[0].Email != u.Email {
		t.Fatalf("online users = %d, first %+v", len(got), got[0])
	}
}
