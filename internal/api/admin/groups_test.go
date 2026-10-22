package admin_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type serverGroupCounts struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	UserCount int64  `json:"user_count"`
	PlanCount int64  `json:"plan_count"`
	NodeCount int64  `json:"node_count"`
}

func TestAdminGroupsCRUD(t *testing.T) {
	e := newNodesAdminEnv(t)
	testutil.ExpectStatus(t, e.do(http.MethodPost, "/groups", map[string]any{"name": "  "}), http.StatusBadRequest)
	testutil.ExpectStatus(t, e.do(http.MethodPost, "/groups", map[string]any{}), http.StatusBadRequest)
	g1, g2 := e.group("Basic"), e.group("Premium")

	testutil.CreateUser(t, e.a, "a@example.com", usersvc.CreateOptions{GroupID: &g1})
	testutil.CreateUser(t, e.a, "b@example.com", usersvc.CreateOptions{GroupID: &g1})
	if err := e.a.DB.Create(&model.Plan{Name: "Plan", GroupID: g1}).Error; err != nil {
		t.Fatal(err)
	}
	e.createNode(nodesAdminVless("n1", []uint64{g1, g2}))
	e.createNode(nodesAdminVless("n2", []uint64{g2}))

	list, total := testutil.DecodeList[serverGroupCounts](t, e.do(http.MethodGet, "/groups", nil))
	want := []serverGroupCounts{{g1, "Basic", 2, 1, 1}, {g2, "Premium", 0, 0, 2}}
	if total != 2 || len(list) != 2 || list[0] != want[0] || list[1] != want[1] {
		t.Fatalf("groups %+v", list)
	}

	rec := e.do(http.MethodPatch, fmt.Sprintf("/groups/%d", g2), map[string]any{"name": "Gold"})
	if got := testutil.Decode[serverGroupCounts](t, rec); got.Name != "Gold" || got.NodeCount != 2 {
		t.Fatalf("patched %+v", got)
	}
	testutil.ExpectStatus(t, e.do(http.MethodPatch, "/groups/999", map[string]any{"name": "x"}), http.StatusNotFound)

	rec = e.do(http.MethodDelete, fmt.Sprintf("/groups/%d", g1), nil)
	if er := testutil.DecodeError(t, rec); rec.Code != http.StatusConflict || er.Message != "group is still used by 2 users, 1 plans and 1 nodes" {
		t.Fatalf("delete in use: %d %s", rec.Code, rec.Body.String())
	}
	g3 := e.group("Empty")
	testutil.ExpectStatus(t, e.do(http.MethodDelete, fmt.Sprintf("/groups/%d", g3), nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(http.MethodDelete, fmt.Sprintf("/groups/%d", g3), nil), http.StatusNotFound)
}
