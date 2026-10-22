package admin_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestAdminNodeRoutesCRUD(t *testing.T) {
	e := newNodesAdminEnv(t)
	bad := func(body map[string]any, field string) {
		t.Helper()
		if f := nodesAdminFields(t, e.do(http.MethodPost, "/routes", body)); !nodesAdminHas(f, field) {
			t.Fatalf("%v: fields %v", body, f)
		}
	}
	bad(map[string]any{"match": []string{"(unclosed"}, "action": "block"}, "match")
	bad(map[string]any{"match": []string{}, "action": "block"}, "match")
	bad(map[string]any{"match": []string{"x"}, "action": "allow"}, "action")
	bad(map[string]any{"match": []string{"example.org"}, "action": "dns"}, "action_value")
	many := make([]string, 1000)
	for i := range many {
		many[i] = fmt.Sprintf("d%d.example.com", i)
	}
	bad(map[string]any{"match": many, "action": "block"}, "match")

	rec := e.do(http.MethodPost, "/routes", map[string]any{"remarks": " ads ", "match": []string{" ads.example.com ", "", "regexp:.*\\.example\\.net"}, "action": "BLOCK"})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	r := testutil.Decode[model.NodeRoute](t, rec)
	if r.Remarks != "ads" || r.Action != "block" || len(r.Match) != 2 || r.Match[0] != "ads.example.com" || r.ActionValue != "" {
		t.Fatalf("created %+v", r)
	}
	dns := testutil.Decode[model.NodeRoute](t, e.do(http.MethodPost, "/routes", map[string]any{"match": []string{"example.org"}, "action": "dns", "action_value": "198.51.100.53"}))

	list, total := testutil.DecodeList[model.NodeRoute](t, e.do(http.MethodGet, "/routes", nil))
	if total != 2 || list[0].ID != r.ID || list[1].ID != dns.ID {
		t.Fatalf("list %+v", list)
	}

	rec = e.do(http.MethodPatch, fmt.Sprintf("/routes/%d", r.ID), map[string]any{"match": []string{"tracker.example.com"}})
	if got := testutil.Decode[model.NodeRoute](t, rec); got.Remarks != "ads" || len(got.Match) != 1 || got.Match[0] != "tracker.example.com" {
		t.Fatalf("patched %+v", got)
	}
	testutil.ExpectStatus(t, e.do(http.MethodPatch, fmt.Sprintf("/routes/%d", r.ID), map[string]any{"match": []string{"[bad"}}), http.StatusBadRequest)
	testutil.ExpectStatus(t, e.do(http.MethodPatch, "/routes/999", map[string]any{"remarks": "x"}), http.StatusNotFound)

	// Routes in use cannot be deleted; config bodies follow route edits.
	g := e.group("g")
	body := nodesAdminVless("n", []uint64{g})
	body["route_ids"] = []uint64{r.ID}
	n := e.createNode(body)
	cfgURL := fmt.Sprintf("/api/v1/server/UniProxy/config?node_id=%d&token=%s", n.ID, nodesAdminServerToken)
	if rec := testutil.Do(t, e.r, http.MethodGet, cfgURL, nil, ""); !strings.Contains(rec.Body.String(), "tracker.example.com") {
		t.Fatalf("config routes: %s", rec.Body.String())
	}
	testutil.ExpectStatus(t, e.do(http.MethodPatch, fmt.Sprintf("/routes/%d", r.ID), map[string]any{"match": []string{"ads2.example.com"}}), http.StatusOK)
	if rec := testutil.Do(t, e.r, http.MethodGet, cfgURL, nil, ""); !strings.Contains(rec.Body.String(), "ads2.example.com") {
		t.Fatalf("config not refreshed after route edit: %s", rec.Body.String())
	}
	testutil.ExpectStatus(t, e.do(http.MethodDelete, fmt.Sprintf("/routes/%d", r.ID), nil), http.StatusConflict)
	testutil.ExpectStatus(t, e.do(http.MethodDelete, fmt.Sprintf("/routes/%d", dns.ID), nil), http.StatusOK)
	testutil.ExpectStatus(t, e.do(http.MethodDelete, fmt.Sprintf("/routes/%d", dns.ID), nil), http.StatusNotFound)
}
