package guest_test

import (
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/guest"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestRulesetsCatalog(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"ruleset_default_template": "minimal", "ruleset_default_source": "metacubex"}))
	r := testutil.NewRouterWith(t, a, guest.Register)
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/guest/rulesets/catalog", nil, "")
	cat := testutil.Decode[rulesets.PublicCatalog](t, rec)
	if cat.Defaults.Template != "minimal" || cat.Defaults.Source != "metacubex" || len(cat.Templates) != 4 || len(cat.Mirrors) == 0 {
		t.Fatalf("catalog = %+v", cat.Defaults)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatal("cache header")
	}
}
