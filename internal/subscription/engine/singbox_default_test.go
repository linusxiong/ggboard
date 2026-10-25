package engine_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type singboxConfig struct {
	DNS struct {
		Rules []map[string]any `json:"rules"`
	} `json:"dns"`
	Route struct {
		Rules   []map[string]any `json:"rules"`
		RuleSet []struct {
			Type, Tag, Format, URL string
			DownloadDetour         string `json:"download_detour"`
			UpdateInterval         string `json:"update_interval"`
		} `json:"rule_set"`
	} `json:"route"`
}

// A user who never opened the rule builder (default source blackmatrix7, which has no sing-box files) gets a sing-box
// config with a working rule set for every category of the default template; choosing metacubex, per request or
// saved, changes nothing.
func TestDefaultProfileSingbox(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, a, "alice@example.com")
	gen := func(q url.Values) *engine.Result {
		t.Helper()
		res, err := engine.Generate(ctx, a, u, engine.Request{Format: subscription.FormatSingbox, Query: q, Host: "panel.example.com"})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := gen(nil)
	if res.Format != subscription.FormatSingbox || res.Profile.Source != "blackmatrix7" || res.Profile.Template != "standard" {
		t.Fatalf("format %s, profile %s/%s", res.Format, res.Profile.Template, res.Profile.Source)
	}
	var cfg singboxConfig
	if err := json.Unmarshal(res.Body, &cfg); err != nil {
		t.Fatal(err)
	}
	defined := map[string]bool{}
	for _, rs := range cfg.Route.RuleSet {
		if rs.Type != "remote" || rs.Tag == "" || defined[rs.Tag] || rs.UpdateInterval != "24h" || rs.DownloadDetour != subscription.GroupProxy {
			t.Errorf("rule set %+v", rs)
		}
		defined[rs.Tag] = true
		if !(rs.Format == "binary" && strings.HasSuffix(rs.URL, ".srs")) && !(rs.Format == "source" && strings.HasSuffix(rs.URL, ".json")) {
			t.Errorf("rule set %s: format %q for %s", rs.Tag, rs.Format, rs.URL)
		}
		if !strings.HasPrefix(rs.URL, "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/") {
			t.Errorf("rule set %s from %s", rs.Tag, rs.URL)
		}
	}
	used := map[string]bool{}
	for _, r := range append(cfg.Route.Rules, cfg.DNS.Rules...) {
		tags, _ := r["rule_set"].([]any)
		for _, v := range tags {
			tag, _ := v.(string)
			if !defined[tag] {
				t.Errorf("rule references undefined rule set %q", tag)
			}
			used[tag] = true
		}
	}
	for tag := range defined {
		if !used[tag] {
			t.Errorf("rule set %s is never used", tag)
		}
	}
	std, _ := rulesets.Current(a.Settings).Template("standard")
	for id := range std.Categories {
		found := false
		for tag := range defined {
			found = found || strings.HasPrefix(tag, id+"-")
		}
		if !found {
			t.Errorf("category %s has no sing-box rule set", id)
		}
	}
	if len(std.Categories) < 10 || len(defined) < len(std.Categories) {
		t.Fatalf("%d rule sets for %d categories", len(defined), len(std.Categories))
	}

	if alt := gen(url.Values{"source": {"metacubex"}}); alt.ETag != res.ETag {
		t.Fatal("source=metacubex changes the sing-box config")
	}
	if err := engine.SaveProfile(ctx, a, u.ID, []byte(`{"template":"standard","source":"metacubex"}`)); err != nil {
		t.Fatal(err)
	}
	if saved := gen(nil); saved.Profile.Source != "metacubex" || saved.ETag != res.ETag {
		t.Fatalf("saving source metacubex changes the sing-box config (source %s)", saved.Profile.Source)
	}
}
