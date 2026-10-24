package node_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/admin"
	"github.com/linusxiong/ggboard/internal/api/node"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// docs/NODE_PROTOCOLS.md §3.2: soga's tcp_limit is users.conn_limit, and every admin write that changes it (user
// create / PATCH, plan force_update) yields a new soga body and ETag at once. The UniProxy list has no such field
// (§2.2), so its body and ETag stay the same.
func TestSogaTCPLimitFollowsConnLimit(t *testing.T) {
	e := newEnv(t)
	e.r = testutil.NewRouterWith(t, e.a, node.Register, admin.Register)
	adminTok := testutil.Login(t, e.a, testutil.CreateAdmin(t, e.a, "root@example.com"))
	g := e.group("relay")
	plan := model.Plan{GroupID: g, Name: "Relay", TransferEnable: 10, ConnLimit: testutil.Ptr(64)}
	if err := e.a.DB.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	e.node(model.Node{ID: 7, Type: "vless", Host: "v.example.com", ServerPort: 443, GroupIDs: model.JSONSlice[uint64]{g}})
	rec := testutil.Do(t, e.r, http.MethodPost, "/api/v1/admin/users", map[string]any{"email": "relay@example.com", "password": "password123", "plan_id": plan.ID}, adminTok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	created := testutil.Decode[struct {
		ID uint64 `json:"id"`
	}](t, rec)
	patch := func(path string, body map[string]any) {
		t.Helper()
		testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPatch, "/api/v1/admin"+path, body, adminTok), http.StatusOK)
	}
	userPath := fmt.Sprintf("/users/%d", created.ID)
	planPath := fmt.Sprintf("/plans/%d", plan.ID)

	h := sogaHeaders(7, "vless")
	soga := func(prevTag string) (tcpLimit int, tag string) {
		t.Helper()
		headers := map[string]string{"If-None-Match": prevTag}
		for k, v := range h {
			headers[k] = v
		}
		rec := e.do(http.MethodGet, "/soga/api/v1/users", nil, headers)
		testutil.ExpectStatus(t, rec, http.StatusOK) // a changed list never matches the previous tag
		var users []struct {
			ID       uint64 `json:"id"`
			TCPLimit *int   `json:"tcp_limit"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil || len(users) != 1 || users[0].ID != created.ID || users[0].TCPLimit == nil {
			t.Fatalf("soga users = %s (%v)", rec.Body.String(), err)
		}
		return *users[0].TCPLimit, rec.Header().Get("ETag")
	}
	uniproxy := func() (body, tag string) {
		t.Helper()
		rec := e.do(http.MethodGet, uniURL("user", 7, "vless"), nil, nil)
		testutil.ExpectStatus(t, rec, http.StatusOK)
		if strings.Contains(rec.Body.String(), "conn_limit") || strings.Contains(rec.Body.String(), "tcp_limit") {
			t.Fatalf("UniProxy users carry a connection limit: %s", rec.Body.String())
		}
		return rec.Body.String(), rec.Header().Get("ETag")
	}

	// Created from the plan: tcp_limit 64.
	limit, tag := soga("")
	uniBody, uniTag := uniproxy()
	if limit != 64 {
		t.Fatalf("tcp_limit from the plan = %d", limit)
	}
	steps := []struct {
		name string
		do   func()
		want int
	}{
		{"user PATCH", func() { patch(userPath, map[string]any{"conn_limit": 8}) }, 8},
		{"plan force_update", func() { patch(planPath, map[string]any{"conn_limit": 16, "force_update": true}) }, 16},
		{"user PATCH null", func() { patch(userPath, map[string]any{"conn_limit": nil}) }, 0},
		{"user PATCH again", func() { patch(userPath, map[string]any{"conn_limit": 100000}) }, 100000},
	}
	for _, s := range steps {
		s.do()
		got, newTag := soga(tag)
		if got != s.want || newTag == tag || newTag == "" {
			t.Fatalf("%s: tcp_limit %d (want %d), ETag %s → %s", s.name, got, s.want, tag, newTag)
		}
		tag = newTag
		if b, ut := uniproxy(); b != uniBody || ut != uniTag {
			t.Fatalf("%s: UniProxy list changed: %s (%s → %s)", s.name, b, uniTag, ut)
		}
	}

	// A plan change without force_update leaves holders alone: same body, 304 for the current tag.
	patch(planPath, map[string]any{"conn_limit": 32})
	headers := map[string]string{"If-None-Match": tag}
	for k, v := range h {
		headers[k] = v
	}
	if rec := e.do(http.MethodGet, "/soga/api/v1/users", nil, headers); rec.Code != http.StatusNotModified {
		t.Fatalf("plan PATCH without force_update changed the soga list: %d %s", rec.Code, rec.Body.String())
	}
}
