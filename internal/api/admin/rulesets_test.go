package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/admin"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type rulesetsView struct {
	Builtin   subscription.Catalog `json:"builtin"`
	Custom    map[string]any       `json:"custom"`
	Effective subscription.Catalog `json:"effective"`
}

func TestAdminRulesets(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := testutil.NewRouterWith(t, a, admin.Register)
	root := testutil.Login(t, a, testutil.CreateAdmin(t, a, "root@example.com"))
	staff := testutil.Login(t, a, testutil.CreateStaff(t, a, "staff@example.com"))

	v := testutil.Decode[rulesetsView](t, testutil.Do(t, r, http.MethodGet, "/api/v1/admin/rulesets", nil, root))
	if len(v.Builtin.Categories) == 0 || len(v.Custom) != 0 || len(v.Effective.Categories) != len(v.Builtin.Categories) {
		t.Fatalf("initial = %d %v", len(v.Builtin.Categories), v.Custom)
	}
	if rec := testutil.Do(t, r, http.MethodGet, "/api/v1/admin/rulesets", nil, staff); rec.Code != http.StatusForbidden {
		t.Fatalf("staff GET = %d", rec.Code)
	}
	if rec := testutil.Do(t, r, http.MethodPut, "/api/v1/admin/rulesets", map[string]any{"custom": nil}, staff); rec.Code != http.StatusForbidden {
		t.Fatalf("staff PUT = %d", rec.Code)
	}

	custom := json.RawMessage(`{
		"categories": [{"id": "corp", "name": {"en": "Corp"}, "default_target": "DIRECT", "order": 1,
			"rules": {"metacubex": {"geosite": "private", "formats": ["clash_mrs_domain"]}}}],
		"templates": [{"id": "standard", "categories": {"corp": "DIRECT"},
			"groups": [{"name": "Proxy", "type": "select", "include": "*"}], "final": "Proxy", "geoip_cn_direct": true}],
		"mirrors": [{"id": "corp-mirror", "prefix": "https://mirror.example.com/"}]
	}`)
	rec := testutil.Do(t, r, http.MethodPut, "/api/v1/admin/rulesets", map[string]any{"custom": custom}, root)
	if ok := testutil.Decode[bool](t, rec); !ok {
		t.Fatal("PUT")
	}
	v = testutil.Decode[rulesetsView](t, testutil.Do(t, r, http.MethodGet, "/api/v1/admin/rulesets", nil, root))
	if v.Effective.Categories[0].ID != "corp" || len(v.Custom["categories"].([]any)) != 1 {
		t.Fatalf("merged = %v", v.Effective.Categories[0].ID)
	}
	if tpl, _ := rulesets.Current(a.Settings).Template("standard"); len(tpl.Categories) != 1 {
		t.Fatal("template override not effective")
	}
	if _, ok := rulesets.Current(a.Settings).Mirror("corp-mirror"); !ok {
		t.Fatal("mirror not merged")
	}

	// Validation: unknown fields, unknown sources, bad targets.
	for _, body := range []string{
		`{"custom": {"categories": [{"id": "x", "default_target": "DIRECT", "rules": {"ghost": {"name": "x", "formats": ["surge"]}}}]}}`,
		`{"custom": {"templates": [{"id": "t", "categories": {}, "groups": [{"name": "Proxy", "type": "select", "include": "*"}], "final": "Nope"}]}}`,
		`{"custom": {"colour": "red"}}`,
		`{"custom": {"mirrors": [{"id": "BAD ID", "prefix": "x"}]}}`,
	} {
		rec := testutil.Do(t, r, http.MethodPut, "/api/v1/admin/rulesets", body, root)
		if e := testutil.DecodeError(t, rec); e.Status != http.StatusBadRequest || len(e.Errors) == 0 {
			t.Errorf("%s: %+v", body, e)
		}
	}
	if _, ok := rulesets.Current(a.Settings).Mirror("corp-mirror"); !ok {
		t.Fatal("a rejected PUT must not change the stored overrides")
	}

	// null resets to the built-in catalog.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPut, "/api/v1/admin/rulesets", `{"custom": null}`, root), http.StatusOK)
	if rulesets.Current(a.Settings) != rulesets.Builtin() {
		t.Fatal("reset")
	}
}
