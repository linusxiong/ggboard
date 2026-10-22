package rulesets

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/testutil"
)

var def = Defaults{Template: "standard", Source: "blackmatrix7"}

func TestBuiltinCatalog(t *testing.T) {
	c := Builtin()
	if err := ValidateCatalog(c); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"none", "minimal", "standard", "full"} {
		if _, ok := c.Template(id); !ok {
			t.Fatalf("template %s missing", id)
		}
	}
	full, _ := c.Template("full")
	groups := map[string]bool{}
	for _, g := range full.Groups {
		groups[g.Name] = true
	}
	for i, cat := range c.Categories {
		if full.Categories[cat.ID] == "" {
			t.Errorf("full template lacks category %s", cat.ID)
		}
		if !validTarget(cat.DefaultTarget, groups) {
			t.Errorf("category %s default_target %q is not a full-template group", cat.ID, cat.DefaultTarget)
		}
		if i > 0 && c.Categories[i-1].Order > cat.Order {
			t.Errorf("categories not sorted at %s", cat.ID)
		}
	}
	if c.Mirrors[0].ID != "direct" || c.Mirrors[0].Prefix != "" {
		t.Fatal("first mirror must be direct")
	}
	// The embedded file round-trips through the Go types without losing fields.
	var raw, back map[string]any
	if err := json.Unmarshal(BuiltinJSON(), &raw); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(c)
	_ = json.Unmarshal(b, &back)
	rawCats, _ := json.Marshal(raw["categories"])
	backCats, _ := json.Marshal(back["categories"])
	if string(rawCats) != string(backCats) {
		t.Fatal("categories do not round-trip")
	}
}

func TestMerge(t *testing.T) {
	base := Builtin()
	custom, err := Parse([]byte(`{
		"sources": {"mysrc": {"name": "My rules", "order": 5, "urls": {"surge": "https://rules.example.com/{name}.list"},
			"url_meta": {"surge": {"field": "name", "behavior": "classical", "format": "text"}}}},
		"categories": [
			{"id": "openai", "name": {"en": "OpenAI (custom)"}, "default_target": "AI", "order": 30,
			 "rules": {"mysrc": {"name": "openai", "formats": ["surge"]}}},
			{"id": "corp", "default_target": "DIRECT", "order": 1, "rules": {"mysrc": {"name": "corp", "formats": ["surge"]}}}],
		"templates": [{"id": "custom", "categories": {"corp": "DIRECT"}, "groups": [{"name": "Proxy", "type": "select", "include": "*"}], "final": "Proxy"}],
		"mirrors": [{"id": "ghfast", "prefix": "https://mirror.example.com/"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	m := Merge(base, custom)
	if err := ValidateCatalog(m); err != nil {
		t.Fatal(err)
	}
	if m.Categories[0].ID != "corp" {
		t.Fatal("custom category must sort by order")
	}
	if cat, _ := m.Category("openai"); len(cat.Rules) != 1 || cat.Name.Get("en") != "OpenAI (custom)" {
		t.Fatal("category override")
	}
	if len(m.Categories) != len(base.Categories)+1 || len(m.Templates) != len(base.Templates)+1 || len(m.Sources) != len(base.Sources)+1 {
		t.Fatal("appended entries")
	}
	if mi, _ := m.Mirror("ghfast"); mi.Prefix != "https://mirror.example.com/" || len(m.Mirrors) != len(base.Mirrors) {
		t.Fatal("mirror override")
	}
	if cat, _ := base.Category("openai"); len(cat.Rules) == 1 {
		t.Fatal("base mutated")
	}
	if _, err := Parse([]byte(`{"sources": {}, "unknown": 1}`)); err == nil {
		t.Fatal("unknown fields must be rejected")
	}
	if c, err := ParseCustom(nil); err != nil || !isEmpty(c) {
		t.Fatal("nil custom")
	}
	if c, err := ParseCustom(map[string]any{}); err != nil || !isEmpty(c) {
		t.Fatal("empty custom")
	}
}

func TestValidateCatalogErrors(t *testing.T) {
	custom, err := Parse([]byte(`{
		"sources": {"Bad Id": {"name": "", "urls": {"nope": "ftp://x"}, "url_meta": {"other": {"field": "x", "behavior": "y", "format": "z"}},
			"aliases": {"a": "missing"}}},
		"categories": [{"id": "x", "default_target": "", "rules": {"ghost": {"name": "x"}, "metacubex": {"formats": ["clash_mrs_domain", "surge"]}}}],
		"templates": [{"id": "t", "categories": {"x": "Nowhere", "unknown": "DIRECT"},
			"groups": [{"name": "Proxy", "type": "select", "include": "A"}, {"name": "A", "type": "url-test", "include": "Proxy"}], "final": "Nope"}],
		"mirrors": [{"id": "m", "prefix": "https://no-slash.example.com"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateCatalog(Merge(Builtin(), custom))
	fe, ok := AsFieldErrors(err)
	if !ok {
		t.Fatalf("err = %v", err)
	}
	for _, k := range []string{
		"sources.Bad Id", "sources.Bad Id.name", "sources.Bad Id.urls.nope", "sources.Bad Id.url_meta.other",
		"sources.Bad Id.aliases.a", "categories[0].default_target", "categories[0].rules.ghost",
		"categories[0].rules.metacubex[0].formats", "categories[0].rules.metacubex[0].geosite",
		"templates[4].categories.x", "templates[4].categories.unknown", "templates[4].final", "templates[4].groups",
		"templates[4].groups[1].url", "templates[4].groups[1].interval", "mirrors[5].prefix",
	} {
		if len(fe[k]) == 0 {
			t.Errorf("missing error for %s (got %v)", k, fe)
		}
	}
}

func TestCurrentMergesSetting(t *testing.T) {
	a := testutil.NewTestApp(t)
	if Current(a.Settings) != Builtin() {
		t.Fatal("no overrides: built-in catalog")
	}
	custom := map[string]any{"mirrors": []any{map[string]any{"id": "corp", "prefix": "https://mirror.example.com/"}}}
	if err := a.Settings.Set(map[string]any{SettingCustom: custom}); err != nil {
		t.Fatal(err)
	}
	c1 := Current(a.Settings)
	if _, ok := c1.Mirror("corp"); !ok {
		t.Fatal("custom mirror not merged")
	}
	if Current(a.Settings) != c1 {
		t.Fatal("merge must be cached while the setting is unchanged")
	}
	if err := a.Settings.Set(map[string]any{SettingCustom: map[string]any{"bogus": true}}); err != nil {
		t.Fatal(err)
	}
	if Current(a.Settings) != Builtin() {
		t.Fatal("an invalid stored override is ignored")
	}
}

func TestDecodeProfile(t *testing.T) {
	c := Builtin()
	p, err := DecodeProfile(c, []byte(`{"template":"minimal"}`), def)
	if err != nil {
		t.Fatal(err)
	}
	if p.Categories["cn"] != "DIRECT" || len(p.Groups) != 2 || p.Final != "Proxy" || !p.GeoIPCNDirect ||
		p.Source != "blackmatrix7" || p.Mirror != MirrorDefault || p.DNS != subscription.DNSDefault {
		t.Fatalf("filled = %+v", p)
	}
	p, err = DecodeProfile(c, []byte(`{"categories":{},"geoip_cn_direct":false,"mirror":""}`), def)
	if err != nil || p.Template != "standard" || len(p.Categories) != 0 || p.GeoIPCNDirect || p.Mirror != "" {
		t.Fatalf("explicit = %+v %v", p, err)
	}
	for _, raw := range []string{`{"template":"nope"}`, `{"colour":"red"}`, `not json`} {
		if _, err := DecodeProfile(c, []byte(raw), def); err == nil {
			t.Errorf("%s must fail", raw)
		}
	}
}

func TestValidateProfile(t *testing.T) {
	c := Builtin()
	ok := DefaultProfile(c, def)
	if err := ValidateProfile(c, ok); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		edit  func(p *subscription.RuleProfile)
		field string
	}{
		{"template", func(p *subscription.RuleProfile) { p.Template = "x" }, "template"},
		{"source", func(p *subscription.RuleProfile) { p.Source = "x" }, "source"},
		{"category", func(p *subscription.RuleProfile) { p.Categories["nope"] = "DIRECT" }, "categories.nope"},
		{"target", func(p *subscription.RuleProfile) { p.Categories["openai"] = "Missing" }, "categories.openai"},
		{"final", func(p *subscription.RuleProfile) { p.Final = "Missing" }, "final"},
		{"mirror", func(p *subscription.RuleProfile) { p.Mirror = "ftp://x/" }, "mirror"},
		{"mirror id", func(p *subscription.RuleProfile) { p.Mirror = "unknown" }, "mirror"},
		{"dns", func(p *subscription.RuleProfile) { p.DNS = "magic" }, "dns"},
		{"no proxy", func(p *subscription.RuleProfile) { p.Groups = p.Groups[1:] }, "groups"},
		{"dup", func(p *subscription.RuleProfile) { p.Groups[1].Name = "Proxy" }, "groups[1].name"},
		{"reserved", func(p *subscription.RuleProfile) { p.Groups[1].Name = "DIRECT" }, "groups[1].name"},
		{"comma", func(p *subscription.RuleProfile) { p.Groups[1].Name = "A,B" }, "groups[1].name"},
		{"type", func(p *subscription.RuleProfile) { p.Groups[1].Type = "random" }, "groups[1].type"},
		{"url", func(p *subscription.RuleProfile) { p.Groups[6].URL = "gstatic" }, "groups[6].url"},
		{"interval", func(p *subscription.RuleProfile) { p.Groups[6].Interval = 1 }, "groups[6].interval"},
		{"regex", func(p *subscription.RuleProfile) { p.Groups[1].Include = "Proxy, ([" }, "groups[1].include"},
		{"self", func(p *subscription.RuleProfile) { p.Groups[1].Include = "AI" }, "groups[1].include"},
		{"empty include", func(p *subscription.RuleProfile) { p.Groups[1].Include = " , " }, "groups[1].include"},
		{"loop", func(p *subscription.RuleProfile) { p.Groups[0].Include = "AI, *"; p.Groups[1].Include = "Proxy" }, "groups"},
	}
	for _, tc := range cases {
		p := DefaultProfile(c, def)
		tc.edit(&p)
		fe, ok := AsFieldErrors(ValidateProfile(c, p))
		if !ok || len(fe[tc.field]) == 0 {
			t.Errorf("%s: want error on %s, got %v", tc.name, tc.field, fe)
		}
	}
	// Mirror forms accepted: "", default, a catalog id, a custom prefix.
	for _, m := range []string{"", MirrorDefault, "ghfast", "https://mirror.example.com/"} {
		p := DefaultProfile(c, def)
		p.Mirror = m
		if err := ValidateProfile(c, p); err != nil {
			t.Errorf("mirror %q: %v", m, err)
		}
	}
}

func TestResolveProfile(t *testing.T) {
	c := Builtin()
	p, err := ResolveProfile(c, nil, def, Overrides{})
	if err != nil || p.Template != "standard" || p.Categories["openai"] != "AI" {
		t.Fatalf("default = %+v %v", p, err)
	}
	stored := DefaultProfile(c, def)
	stored.Template = "standard"
	stored.Categories = subscription.CategoryTargets{"openai": "Proxy", "gone": "DIRECT"}
	stored.Source, stored.Mirror, stored.DNS = "loyalsoldier", "", subscription.DNSFakeIP
	p, err = ResolveProfile(c, &stored, def, Overrides{})
	if err != nil || len(p.Categories) != 1 || p.Categories["openai"] != "Proxy" || p.Source != "loyalsoldier" || p.Mirror != "" {
		t.Fatalf("stored = %+v %v", p, err)
	}
	// profile= keeps the stored source, mirror and dns.
	p, _ = ResolveProfile(c, &stored, def, Overrides{Template: "minimal"})
	if p.Template != "minimal" || len(p.Categories) != 2 || p.Source != "loyalsoldier" || p.DNS != subscription.DNSFakeIP {
		t.Fatalf("profile override = %+v", p)
	}
	// cats narrows: stored target, then the template's, then the category default.
	p, _ = ResolveProfile(c, &stored, def, Overrides{HasCats: true, Cats: []string{"openai", "telegram", "steam"}})
	if len(p.Categories) != 3 || p.Categories["openai"] != "Proxy" || p.Categories["telegram"] != "Telegram" || p.Categories["steam"] != "Games" {
		t.Fatalf("cats = %v", p.Categories)
	}
	p, _ = ResolveProfile(c, nil, def, Overrides{HasCats: true})
	if len(p.Categories) != 0 {
		t.Fatal("empty cats disables every category")
	}
	p, _ = ResolveProfile(c, &stored, def, Overrides{Source: "metacubex", Final: "DIRECT", Mirror: "1"})
	if p.Source != "metacubex" || p.Final != "DIRECT" || p.Mirror != MirrorDefault {
		t.Fatalf("overrides = %+v", p)
	}
	p, _ = ResolveProfile(c, nil, def, Overrides{Mirror: "0"})
	if p.Mirror != "" {
		t.Fatal("mirror=0")
	}
	p, _ = ResolveProfile(c, nil, def, Overrides{Mirror: "ghfast"})
	if p.Mirror != "ghfast" {
		t.Fatal("mirror id")
	}
	// Stale stored references fall back silently.
	stale := subscription.RuleProfile{Template: "deleted", Source: "deleted", Final: "Gone", Mirror: "bogus", DNS: "bogus"}
	p, err = ResolveProfile(c, &stale, def, Overrides{})
	if err != nil || p.Template != "standard" || p.Source != "blackmatrix7" || p.Final != "Proxy" || p.Mirror != MirrorDefault ||
		p.DNS != subscription.DNSDefault || len(p.Groups) == 0 || p.Categories["cn"] != "DIRECT" {
		t.Fatalf("stale = %+v %v", p, err)
	}
	// Invalid overrides are 400s.
	_, err = ResolveProfile(c, nil, def, Overrides{Template: "x", Source: "y"})
	if fe, ok := AsFieldErrors(err); !ok || len(fe["profile"]) == 0 {
		t.Fatalf("profile error = %v", err)
	}
	_, err = ResolveProfile(c, nil, def, Overrides{Source: "y", HasCats: true, Cats: []string{"nope"}, Final: "Nope", Mirror: "nope"})
	fe, ok := AsFieldErrors(err)
	if !ok || len(fe["source"]) == 0 || len(fe["cats"]) == 0 || len(fe["final"]) == 0 || len(fe["mirror"]) == 0 {
		t.Fatalf("override errors = %v", err)
	}
	if !strings.Contains(err.Error(), "cats: unknown category nope") {
		t.Fatalf("message = %v", err)
	}
	if !(Overrides{}).IsZero() || (Overrides{HasCats: true}).IsZero() {
		t.Fatal("IsZero")
	}
}

func tags(p *subscription.RulePlan) []string {
	var out []string
	for _, s := range p.Sets {
		out = append(out, s.Tag)
	}
	return out
}

func setByTag(p *subscription.RulePlan, tag string) subscription.RuleSet {
	for _, s := range p.Sets {
		if s.Tag == tag {
			return s
		}
	}
	return subscription.RuleSet{}
}

func TestBuildPlan(t *testing.T) {
	c := Builtin()
	std := DefaultProfile(c, def)

	// clash-meta prefers metacubex mrs files; IP sets outside lan/cn get no-resolve.
	p := BuildPlan(c, std, subscription.FormatClashMeta, "")
	if s := setByTag(p, "telegram-ip"); s.Source != "metacubex" || s.Format != "mrs" || !s.NoResolve || s.Target != "Telegram" {
		t.Fatalf("telegram-ip = %+v", s)
	}
	if s := setByTag(p, "cn-ip"); s.NoResolve || s.Target != "DIRECT" {
		t.Fatalf("cn-ip = %+v", s)
	}
	if got := tags(p); got[0] != "ads-domain" || got[len(got)-1] != "cn-ip" || !p.GeoIPCNDirect || p.Final != "Proxy" {
		t.Fatalf("clash-meta tags = %v", got)
	}

	// stash uses the chosen source; blackmatrix7 split lists bring their _Domain/_IP siblings, IP sets last.
	p = BuildPlan(c, std, subscription.FormatStash, "")
	cn := []string{}
	for _, s := range p.Sets {
		if s.Category == "cn" {
			cn = append(cn, s.Tag+":"+s.Key)
		}
	}
	if strings.Join(cn, ",") != "cn-classical:clash,cn-domain:clash_domain,cn-ip:clash_ipcidr" {
		t.Fatalf("stash cn = %v", cn)
	}
	if s := setByTag(p, "ads-domain"); s.Key != "clash_domain" || !strings.HasSuffix(s.URL, "AdvertisingLite_Domain.yaml") {
		t.Fatalf("stash ads = %+v", s)
	}

	// A source without files for the client falls back to the next source by order.
	ls := std
	ls.Source = "loyalsoldier"
	p = BuildPlan(c, ls, subscription.FormatSurge, "")
	if s := setByTag(p, "openai-classical"); s.Source != "blackmatrix7" {
		t.Fatalf("surge fallback = %+v", s)
	}
	p = BuildPlan(c, std, subscription.FormatSingbox, "")
	if s := setByTag(p, "openai-domain"); s.Source != "metacubex" || s.Format != "binary" {
		t.Fatalf("singbox fallback = %+v", s)
	}
	if p.GeoIPCN != nil {
		t.Fatal("geoip-cn duplicates cn-ip")
	}

	// sing-box gets a geoip-cn rule set when no category already ships it; mirrors apply to GitHub URLs only.
	lan := std
	lan.Categories = subscription.CategoryTargets{"lan": "DIRECT", "facebook": "Social"}
	p = BuildPlan(c, lan, subscription.FormatSingbox, "https://mirror.example.com/")
	if p.GeoIPCN == nil || p.GeoIPCN.URL != "https://mirror.example.com/https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/cn.srs" {
		t.Fatalf("geoip-cn = %+v", p.GeoIPCN)
	}
	if got := tags(p); strings.Join(got, ",") != "lan-domain,lan-ip,facebook-1-domain,facebook-2-domain,facebook-3-domain,facebook-1-ip" {
		t.Fatalf("multi-ref tags = %v", got)
	}
	if s := setByTag(p, "facebook-2-domain"); s.Target != "Proxy" {
		t.Fatalf("missing group routes to final: %+v", s)
	}
	custom, _ := Parse([]byte(`{"sources":{"corp":{"name":"Corp","order":1,"urls":{"singbox":"https://rules.example.com/{name}.srs"},
		"url_meta":{"singbox":{"field":"geosite","behavior":"domain","format":"binary"}}}},
		"categories":[{"id":"corp","default_target":"DIRECT","order":1,"rules":{"corp":{"geosite":"corp","formats":["singbox"]}}}]}`))
	mc := Merge(c, custom)
	cp := std
	cp.Categories = subscription.CategoryTargets{"corp": "DIRECT"}
	p = BuildPlan(mc, cp, subscription.FormatSingbox, "https://mirror.example.com/")
	if p.Sets[0].URL != "https://rules.example.com/corp.srs" {
		t.Fatalf("non-GitHub URL got a mirror: %s", p.Sets[0].URL)
	}

	// Formats without a rule family get only GEOIP and the final policy; categories no source serves are skipped.
	p = BuildPlan(c, std, "", "")
	if len(p.Sets) != 0 || !p.GeoIPCNDirect || p.Final != "Proxy" {
		t.Fatal("no family")
	}
	apps := std
	apps.Categories = subscription.CategoryTargets{"applications": "DIRECT"}
	if p = BuildPlan(c, apps, subscription.FormatQuantumultX, ""); len(p.Sets) != 0 {
		t.Fatal("applications has no QX source")
	}
	bad := std
	bad.Final = "Nope"
	if p = BuildPlan(c, bad, subscription.FormatSurge, ""); p.Final != "Proxy" {
		t.Fatal("invalid final falls back to Proxy")
	}
	if !HasFamily(subscription.FormatLoon) || HasFamily(subscription.FormatV2ray) {
		t.Fatal("HasFamily")
	}
}

func TestMirrorPrefix(t *testing.T) {
	c := Builtin()
	cases := []struct{ profile, setting, want string }{
		{"", "https://x.example.com/", ""},
		{MirrorDefault, "", ""},
		{MirrorDefault, "https://x.example.com", "https://x.example.com/"},
		{MirrorDefault, "ghfast", "https://ghfast.top/"},
		{MirrorDefault, MirrorDefault, ""},
		{"gh-proxy", "", "https://gh-proxy.com/"},
		{"https://y.example.com/", "", "https://y.example.com/"},
		{"unknown", "", ""},
		{"https://bad.example.com/?q", "", ""},
	}
	for _, tc := range cases {
		if got := MirrorPrefix(c, tc.profile, Defaults{Mirror: tc.setting}); got != tc.want {
			t.Errorf("MirrorPrefix(%q, %q) = %q, want %q", tc.profile, tc.setting, got, tc.want)
		}
	}
	if !IsMirrorable("https://github.com/x") || IsMirrorable("https://example.com/x") || WithMirror("", "https://github.com/x") != "https://github.com/x" {
		t.Fatal("mirrorable")
	}
}

func TestPublicView(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"ruleset_mirror": "ghfast", "allow_user_rule_profile": false}))
	v := PublicView(Current(a.Settings), a.Settings)
	if v.Editable || v.Defaults.Template != "standard" || v.Defaults.Source != "blackmatrix7" || v.Defaults.MirrorPrefix != "https://ghfast.top/" {
		t.Fatalf("defaults = %+v editable=%v", v.Defaults, v.Editable)
	}
	if !slices.Equal(v.Sources["sagernet"].Clients, []string{subscription.FormatSingbox}) {
		t.Fatalf("sagernet clients = %v", v.Sources["sagernet"].Clients)
	}
	for _, cat := range v.Categories {
		if cat.ID == "applications" {
			if !slices.Equal(cat.Clients, []string{subscription.FormatClashMeta, subscription.FormatStash}) || !slices.Equal(cat.Sources, []string{"loyalsoldier"}) {
				t.Fatalf("applications = %+v", cat)
			}
		}
		if cat.ID == "cn" && cat.Sources[0] != "blackmatrix7" {
			t.Fatal("sources by order")
		}
	}
	b, _ := json.Marshal(v)
	for _, leak := range []string{"url_meta", "\"urls\"", "aliases", "\"formats\""} {
		if strings.Contains(string(b), leak) {
			t.Errorf("public view leaks %s", leak)
		}
	}
}
