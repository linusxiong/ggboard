package user_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/api/client"
	"github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/fixture"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type profileView struct {
	subscription.RuleProfile
	Customized bool                     `json:"customized"`
	Editable   bool                     `json:"editable"`
	Effective  subscription.RuleProfile `json:"effective"`
}

func TestRuleProfileEndpoints(t *testing.T) {
	a := testutil.NewTestApp(t)
	if _, err := fixture.SeedNodes(a.DB); err != nil {
		t.Fatal(err)
	}
	gid := uint64(fixture.GroupID)
	exp := time.Now().Add(24 * time.Hour)
	u := testutil.CreateUser(t, a, "alice@example.com", usersvc.CreateOptions{GroupID: &gid, TransferEnable: 10 * fixture.GiB, ExpiredAt: &exp})
	tok := testutil.Login(t, a, u)
	r := testutil.NewRouterWith(t, a, user.Register)

	// Defaults when nothing is stored.
	v := testutil.Decode[profileView](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/rule-profile", nil, tok))
	if v.Customized || !v.Editable || v.Template != "standard" || v.Effective.Template != "standard" || v.Categories["openai"] != "AI" {
		t.Fatalf("default view = %+v", v)
	}
	if rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/rule-profile", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatal("auth required")
	}

	// PUT fills omitted parts from the template and stores the complete profile.
	rec := testutil.Do(t, r, http.MethodPut, "/api/v1/user/rule-profile", map[string]any{
		"template": "minimal", "source": "metacubex", "categories": map[string]any{"cn": "DIRECT", "telegram": "Proxy", "ads": false},
		"dns": "fakeip",
	}, tok)
	v = testutil.Decode[profileView](t, rec)
	if !v.Customized || v.Template != "minimal" || len(v.Categories) != 2 || v.Source != "metacubex" || len(v.Groups) != 2 ||
		!v.GeoIPCNDirect || v.DNS != "fakeip" || v.Effective.Categories["telegram"] != "Proxy" {
		t.Fatalf("stored = %+v", v)
	}

	// Validation errors are field errors.
	rec = testutil.Do(t, r, http.MethodPut, "/api/v1/user/rule-profile", map[string]any{
		"template": "standard", "categories": map[string]any{"nope": "DIRECT", "openai": "Missing"},
		"groups": []any{map[string]any{"name": "Proxy", "type": "select", "include": "*"}, map[string]any{"name": "Proxy", "type": "x", "include": "(["}},
		"final":  "Nowhere", "mirror": "ftp://x", "dns": "x",
	}, tok)
	e := testutil.DecodeError(t, rec)
	for _, f := range []string{"categories.nope", "categories.openai", "groups[1].name", "groups[1].type", "groups[1].include", "final", "mirror", "dns"} {
		if len(e.Errors[f]) == 0 {
			t.Errorf("missing field error %s: %+v", f, e)
		}
	}
	for _, body := range []any{"", `{"template":"nope"}`, `{"unknown":1}`, `[`} {
		if rec := testutil.Do(t, r, http.MethodPut, "/api/v1/user/rule-profile", body, tok); rec.Code != http.StatusBadRequest {
			t.Errorf("%v: %d", body, rec.Code)
		}
	}

	// Preview uses the stored profile and the subscribe query parameters.
	rec = testutil.Do(t, r, http.MethodGet, "/api/v1/user/subscription/preview?client=clash-meta", nil, tok)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if body := rec.Body.String(); !strings.Contains(body, "RULE-SET,telegram-domain,Proxy") || !strings.Contains(body, "enhanced-mode: fake-ip") ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || rec.Header().Get("X-Subscription-Format") != "clash-meta" {
		t.Fatalf("preview:\n%.800s", body)
	}
	rec = testutil.Do(t, r, http.MethodGet, "/api/v1/user/subscription/preview?client=v2rayN&types=trojan", nil, tok)
	if body := rec.Body.String(); !strings.HasPrefix(body, "ss://") || !strings.Contains(body, "trojan://") || strings.Contains(body, "vmess://") {
		t.Fatalf("decoded preview:\n%s", body)
	}
	if rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/subscription/preview", nil, tok); !strings.Contains(rec.Body.String(), "proxy-groups:") {
		t.Fatal("default preview is clash-meta")
	}
	if rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/subscription/preview?client=surge&cats=nope", nil, tok); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad override = %d", rec.Code)
	}

	// Catalog.
	cat := testutil.Decode[rulesets.PublicCatalog](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/rulesets/catalog", nil, tok))
	if len(cat.Categories) == 0 || cat.Sources["metacubex"].Name == "" || !cat.Editable || cat.Defaults.Template != "standard" {
		t.Fatalf("catalog = %+v", cat.Defaults)
	}

	// Customization disabled: PUT is forbidden, the effective profile is the default template.
	if err := a.Settings.Set(map[string]any{"allow_user_rule_profile": false}); err != nil {
		t.Fatal(err)
	}
	if rec := testutil.Do(t, r, http.MethodPut, "/api/v1/user/rule-profile", map[string]any{"template": "full"}, tok); rec.Code != http.StatusForbidden {
		t.Fatalf("disabled PUT = %d", rec.Code)
	}
	v = testutil.Decode[profileView](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/rule-profile", nil, tok))
	if v.Editable || !v.Customized || v.Template != "minimal" || v.Effective.Template != "standard" {
		t.Fatalf("disabled view = %+v", v)
	}
}

// Rename rules that copy the name ("$1" × 128) are valid input, and each multiplied the node name by 128: with five
// of them a subscriber's preview or subscription allocated 171 GiB and the panel died with an out-of-memory error.
// Three rules on "HK 01" built a 10 MiB name; now every name is capped, so the bodies stay small.
func TestRuleProfileRenameRulesCannotExhaustMemory(t *testing.T) {
	a := testutil.NewTestApp(t)
	gid := uint64(fixture.GroupID)
	n := &model.Node{Type: model.NodeVmess, Name: "HK 01", Host: "hk.example.com", Port: "443", ServerPort: 443,
		GroupIDs: []uint64{gid}, Rate: 1, Show: true}
	if err := a.DB.Create(n).Error; err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(24 * time.Hour)
	u := testutil.CreateUser(t, a, "alice@example.com", usersvc.CreateOptions{GroupID: &gid, TransferEnable: 10 * fixture.GiB, ExpiredAt: &exp})
	tok := testutil.Login(t, a, u)
	r := testutil.NewRouterWith(t, a, user.Register, client.Register)

	grow := map[string]any{"match": "^(.*)$", "replace": strings.Repeat("$1", 128)}
	rec := testutil.Do(t, r, http.MethodPut, "/api/v1/user/rule-profile", map[string]any{
		"template": "minimal", "nodes": map[string]any{"rename": []any{grow, grow, grow}},
	}, tok)
	testutil.ExpectStatus(t, rec, http.StatusOK)

	const small = 64 << 10
	for app, want := range map[string]string{"clash-meta": "HK 01HK 01", "v2rayN": "vmess://"} {
		rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/subscription/preview?client="+app, nil, tok)
		if rec.Code != http.StatusOK || rec.Body.Len() > small || !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("preview for %s: %d, %d bytes", app, rec.Code, rec.Body.Len())
		}
	}
	for _, ua := range []string{"clash.meta/1.19.0", "v2rayN/7.0"} {
		rec := testutil.DoWithHeaders(t, r, http.MethodGet, "/api/v1/client/subscribe?token="+u.Token, nil, "", map[string]string{"User-Agent": ua})
		if rec.Code != http.StatusOK || rec.Body.Len() > small {
			t.Fatalf("subscribe as %s: %d, %d bytes", ua, rec.Code, rec.Body.Len())
		}
	}
}
