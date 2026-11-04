package admin_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
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

// A list pasted straight from a JSON array (quotes, trailing commas) is stored as clean entries, on create and edit.
func TestAdminNodeRoutesAcceptPastedList(t *testing.T) {
	e := newNodesAdminEnv(t)
	pasted := "\"geosite:falungong\",\n\"geosite:bbc\",\n\"domain:rfa.org\",\n'domain:rsf.org',\nregexp:.*\\.example\\.net,\n\"domain:landofhope.tv\""
	rec := e.do(http.MethodPost, "/routes", map[string]any{"remarks": "news", "match": []string{pasted}, "action": "block"})
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	r := testutil.Decode[model.NodeRoute](t, rec)
	want := []string{"geosite:falungong", "geosite:bbc", "domain:rfa.org", "domain:rsf.org", "regexp:.*\\.example\\.net", "domain:landofhope.tv"}
	if strings.Join(r.Match, "|") != strings.Join(want, "|") {
		t.Fatalf("created %q", r.Match)
	}

	rec = e.do(http.MethodPatch, fmt.Sprintf("/routes/%d", r.ID), map[string]any{"match": []string{"[", `"a.example.com",`, `"b.example.com"`, "]"}})
	if got := testutil.Decode[model.NodeRoute](t, rec); strings.Join(got.Match, "|") != "a.example.com|b.example.com" {
		t.Fatalf("patched %q", got.Match)
	}
}

func routeApply(t *testing.T, e *nodesAdminEnv, id uint64, body map[string]any) nodesvc.RouteApplied {
	t.Helper()
	rec := e.do(http.MethodPost, fmt.Sprintf("/routes/%d/apply", id), body)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	return testutil.Decode[nodesvc.RouteApplied](t, rec)
}

func TestAdminRouteApply(t *testing.T) {
	e := newNodesAdminEnv(t)
	g := e.group("g")
	r := testutil.Decode[model.NodeRoute](t, e.do(http.MethodPost, "/routes", map[string]any{"match": []string{"pushed.example.com"}, "action": "block"}))
	other := testutil.Decode[model.NodeRoute](t, e.do(http.MethodPost, "/routes", map[string]any{"match": []string{"kept.example.com"}, "action": "block"}))
	first := nodesAdminVless("first", []uint64{g})
	first["route_ids"] = []uint64{other.ID}
	a := e.createNode(first)
	b := e.createNode(nodesAdminVless("second", []uint64{g}))
	child := e.createNode(map[string]any{"type": "vless", "name": "relay", "host": "relay.example.com", "port": "443", "parent_id": a.ID})

	config := func(id uint64) string {
		t.Helper()
		url := fmt.Sprintf("/api/v1/server/UniProxy/config?node_id=%d&token=%s", id, nodesAdminServerToken)
		return testutil.Do(t, e.r, http.MethodGet, url, nil, "").Body.String()
	}
	// Warm the cache: a push must reach backends that already hold the old body.
	if strings.Contains(config(a.ID), "pushed.example.com") {
		t.Fatal("route present before the push")
	}

	if got := routeApply(t, e, r.ID, map[string]any{"all": true}); got != (nodesvc.RouteApplied{NodeCount: 2, Added: 2}) {
		t.Fatalf("all: %+v", got)
	}
	for _, id := range []uint64{a.ID, b.ID, child.ID} {
		if !strings.Contains(config(id), "pushed.example.com") {
			t.Fatalf("node %d did not get the route: %s", id, config(id))
		}
	}
	if !strings.Contains(config(a.ID), "kept.example.com") {
		t.Fatalf("the node's other route was lost: %s", config(a.ID))
	}
	var stored model.Node
	if err := e.a.DB.First(&stored, child.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored.RouteIDs) != 0 {
		t.Fatalf("a child takes its parent's routes, got its own %v", stored.RouteIDs)
	}

	// Pushing again changes nothing; the list is exact, so a node left out loses the route.
	if got := routeApply(t, e, r.ID, map[string]any{"all": true}); got != (nodesvc.RouteApplied{NodeCount: 2}) {
		t.Fatalf("repeat: %+v", got)
	}
	if got := routeApply(t, e, r.ID, map[string]any{"node_ids": []uint64{b.ID, b.ID}}); got != (nodesvc.RouteApplied{NodeCount: 1, Removed: 1}) {
		t.Fatalf("subset: %+v", got)
	}
	if strings.Contains(config(a.ID), "pushed.example.com") || !strings.Contains(config(b.ID), "pushed.example.com") {
		t.Fatalf("subset configs: %s / %s", config(a.ID), config(b.ID))
	}
	if !strings.Contains(config(a.ID), "kept.example.com") {
		t.Fatalf("the node's other route was lost: %s", config(a.ID))
	}

	// A reference left on a child by older data is invisible here but would block the delete.
	if err := e.a.DB.Model(&model.Node{}).Where("id = ?", child.ID).Update("route_ids", model.JSONSlice[uint64]{r.ID}).Error; err != nil {
		t.Fatal(err)
	}
	testutil.ExpectStatus(t, e.do(http.MethodDelete, fmt.Sprintf("/routes/%d", r.ID), nil), http.StatusConflict)
	if got := routeApply(t, e, r.ID, map[string]any{"node_ids": []uint64{}}); got != (nodesvc.RouteApplied{Removed: 1}) {
		t.Fatalf("withdraw: %+v", got)
	}
	testutil.ExpectStatus(t, e.do(http.MethodDelete, fmt.Sprintf("/routes/%d", r.ID), nil), http.StatusOK)
}

func TestAdminRouteApplyRejectsBadTargets(t *testing.T) {
	e := newNodesAdminEnv(t)
	r := testutil.Decode[model.NodeRoute](t, e.do(http.MethodPost, "/routes", map[string]any{"match": []string{"a.example.com"}, "action": "block"}))
	parent := e.createNode(nodesAdminVless("parent", nil))
	child := e.createNode(map[string]any{"type": "vless", "name": "relay", "host": "relay.example.com", "port": "443", "parent_id": parent.ID})
	path := fmt.Sprintf("/routes/%d/apply", r.ID)

	for _, body := range []map[string]any{
		{},
		{"all": false},
		{"all": true, "node_ids": []uint64{parent.ID}},
		{"node_ids": []uint64{parent.ID, 9999}},
		{"node_ids": []uint64{parent.ID, child.ID}},
	} {
		if f := nodesAdminFields(t, e.do(http.MethodPost, path, body)); !nodesAdminHas(f, "node_ids") {
			t.Fatalf("%v: fields %v", body, f)
		}
	}
	// A rejected list changes nothing, not even for its valid ids.
	var stored model.Node
	if err := e.a.DB.First(&stored, parent.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored.RouteIDs) != 0 {
		t.Fatalf("rejected push was applied: %v", stored.RouteIDs)
	}
	testutil.ExpectStatus(t, e.do(http.MethodPost, "/routes/9999/apply", map[string]any{"all": true}), http.StatusNotFound)

	staff := testutil.Login(t, e.a, testutil.CreateStaff(t, e.a, "staff@example.com"))
	url := "/api/v1/admin" + path
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPost, url, map[string]any{"all": true}, staff), http.StatusForbidden)
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPost, url, map[string]any{"all": true}, ""), http.StatusUnauthorized)
}
