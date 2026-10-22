package formats_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/subscription/fixture"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

type goldenCase struct {
	file    string
	format  string
	profile func(c *subscription.Catalog) subscription.RuleProfile
	mutate  func(in *engine.Input)
}

func standard(c *subscription.Catalog) subscription.RuleProfile {
	return rulesets.DefaultProfile(c, rulesets.Defaults{Template: "standard", Source: "blackmatrix7"})
}

func withTemplate(id, source, mirror, dns string) func(c *subscription.Catalog) subscription.RuleProfile {
	return func(c *subscription.Catalog) subscription.RuleProfile {
		p := rulesets.DefaultProfile(c, rulesets.Defaults{Template: id, Source: source})
		p.Mirror, p.DNS = mirror, dns
		return p
	}
}

// input builds the render input of the fixture user and nodes (info nodes first, like Generate).
func input(t testing.TB, profile subscription.RuleProfile, c *subscription.Catalog) engine.Input {
	t.Helper()
	u := fixture.User()
	proxies := subscription.InfoProxies(u, "zh-CN", fixture.Now, time.UTC)
	nodes := map[uint64]*model.Node{}
	for _, n := range fixture.Nodes() {
		eff := n
		if n.Parent != nil {
			eff = n.Parent
		}
		p, err := subscription.BuildProxy(n, eff, u)
		if err != nil {
			t.Fatalf("node %d: %v", n.ID, err)
		}
		p.Name = subscription.DisplayName(p.Name, false)
		proxies = append(proxies, p)
		nodes[n.ID] = n
	}
	return engine.Input{
		User: u, Proxies: proxies, Nodes: nodes, Profile: profile, Catalog: c,
		Defaults: rulesets.Defaults{Template: "standard", Source: "blackmatrix7"},
		AppName:  "GG Bond", Locale: "zh-CN", SelfURL: "https://panel.example.com/api/v1/client/subscribe?token=" + fixture.UserToken,
		Host: "panel.example.com", UpdateHours: 24, Now: fixture.Now, Location: time.UTC,
	}
}

func TestGolden(t *testing.T) {
	c := rulesets.Builtin()
	cases := []goldenCase{
		{file: "clash-meta.yaml", format: subscription.FormatClashMeta, profile: standard},
		{file: "clash-meta-full-mirror-fakeip.yaml", format: subscription.FormatClashMeta, profile: withTemplate("full", "loyalsoldier", "ghfast", subscription.DNSFakeIP)},
		{file: "clash-meta-none.yaml", format: subscription.FormatClashMeta, profile: withTemplate("none", "blackmatrix7", "", subscription.DNSNone)},
		{file: "stash.yaml", format: subscription.FormatStash, profile: standard},
		{file: "clash.yaml", format: subscription.FormatClash, profile: standard},
		{file: "singbox.json", format: subscription.FormatSingbox, profile: standard},
		{file: "singbox-full-mirror-fakeip.json", format: subscription.FormatSingbox, profile: withTemplate("full", "sagernet", "ghfast", subscription.DNSFakeIP)},
		{file: "surge.conf", format: subscription.FormatSurge, profile: standard},
		{file: "surge-list.conf", format: subscription.FormatSurge, profile: standard, mutate: func(in *engine.Input) { in.ListOnly = true }},
		{file: "surfboard.conf", format: subscription.FormatSurfboard, profile: withTemplate("minimal", "acl4ssr", "", subscription.DNSDefault)},
		{file: "quantumultx.conf", format: subscription.FormatQuantumultX, profile: standard},
		{file: "quantumultx-list.conf", format: subscription.FormatQuantumultX, profile: standard, mutate: func(in *engine.Input) { in.ListOnly = true }},
		{file: "loon.conf", format: subscription.FormatLoon, profile: standard},
		{file: "shadowrocket.txt", format: subscription.FormatShadowrocket, profile: standard},
		{file: "v2ray.txt", format: subscription.FormatV2ray, profile: standard},
		{file: "ssr.txt", format: subscription.FormatSSR, profile: standard},
		{file: "v2ray-nodeops.txt", format: subscription.FormatV2ray, profile: standard, mutate: func(in *engine.Input) {
			in.Proxies = subscription.ApplyNodeOps(in.Proxies, subscription.NodeOps{
				Regions: []string{"HK", "JP", "US"}, Exclude: []string{"Relay"}, Emoji: subscription.EmojiAdd,
				Rename: []subscription.RenameRule{{Match: "(?i)hysteria2?", Replace: "HY"}, {Match: `^(\S+) (\S+) `, Replace: "${1} ${2} · "}},
				Sort:   subscription.SortRegion, Dedupe: true, Suffix: " ✈",
			}.Normalized())
		}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			f, ok := subscription.GetFormat(tc.format)
			if !ok {
				t.Fatalf("format %s not registered", tc.format)
			}
			in := input(t, tc.profile(c), c)
			if tc.mutate != nil {
				tc.mutate(&in)
			}
			res, err := engine.Render(f, in)
			if err != nil {
				t.Fatal(err)
			}
			validate(t, tc.format, res.Body)
			readable := res.Body
			if isBase64Format(tc.format) {
				readable = decodeB64(t, res.Body)
			}
			path := filepath.Join("testdata", tc.file+".golden")
			if *update {
				if err := os.WriteFile(path, readable, 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/subscription/formats -update)", err)
			}
			if !bytes.Equal(readable, want) {
				t.Errorf("%s differs from golden file; diff:\n%s", tc.file, diff(string(want), string(readable)))
			}
		})
	}
}

func isBase64Format(f string) bool {
	return f == subscription.FormatV2ray || f == subscription.FormatShadowrocket || f == subscription.FormatSSR
}

func decodeB64(t *testing.T, b []byte) []byte {
	t.Helper()
	out, err := base64.StdEncoding.DecodeString(string(b))
	if err != nil {
		t.Fatalf("body is not base64: %v", err)
	}
	return out
}

// validate parses the body with a real parser for its format.
func validate(t *testing.T, format string, body []byte) {
	t.Helper()
	switch format {
	case subscription.FormatClashMeta, subscription.FormatStash, subscription.FormatClash:
		var doc struct {
			Proxies []map[string]any `yaml:"proxies"`
			Groups  []struct {
				Name    string   `yaml:"name"`
				Proxies []string `yaml:"proxies"`
			} `yaml:"proxy-groups"`
			Providers map[string]map[string]any `yaml:"rule-providers"`
			Rules     []string                  `yaml:"rules"`
		}
		if err := yaml.Unmarshal(body, &doc); err != nil {
			t.Fatalf("invalid YAML: %v\n%s", err, body)
		}
		names := map[string]bool{"DIRECT": true, "REJECT": true}
		for _, p := range doc.Proxies {
			n, _ := p["name"].(string)
			if names[n] {
				t.Fatalf("duplicate proxy name %q", n)
			}
			names[n] = true
		}
		for _, g := range doc.Groups {
			names[g.Name] = true
		}
		for _, g := range doc.Groups {
			if len(g.Proxies) == 0 {
				t.Fatalf("group %s is empty", g.Name)
			}
			for _, m := range g.Proxies {
				if !names[m] {
					t.Fatalf("group %s references unknown %q", g.Name, m)
				}
			}
		}
		for _, r := range doc.Rules {
			parts := strings.Split(r, ",")
			if parts[0] == "RULE-SET" && doc.Providers[parts[1]] == nil {
				t.Fatalf("rule %q references a missing provider", r)
			}
			target := parts[len(parts)-1]
			if target == "no-resolve" {
				target = parts[len(parts)-2]
			}
			if !names[target] {
				t.Fatalf("rule %q targets unknown policy", r)
			}
		}
		if len(doc.Rules) == 0 || !strings.HasPrefix(doc.Rules[len(doc.Rules)-1], "MATCH,") {
			t.Fatal("rules must end with MATCH")
		}
	case subscription.FormatSingbox:
		var doc struct {
			Outbounds []struct {
				Type      string   `json:"type"`
				Tag       string   `json:"tag"`
				Outbounds []string `json:"outbounds"`
			} `json:"outbounds"`
			Route struct {
				Rules []struct {
					RuleSet  []string `json:"rule_set"`
					Outbound string   `json:"outbound"`
				} `json:"rules"`
				RuleSet []struct {
					Tag string `json:"tag"`
				} `json:"rule_set"`
				Final string `json:"final"`
			} `json:"route"`
		}
		dec := json.NewDecoder(bytes.NewReader(body))
		if err := dec.Decode(&doc); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		tags := map[string]bool{}
		for _, o := range doc.Outbounds {
			if tags[o.Tag] {
				t.Fatalf("duplicate outbound tag %q", o.Tag)
			}
			tags[o.Tag] = true
		}
		for _, o := range doc.Outbounds {
			for _, m := range o.Outbounds {
				if !tags[m] {
					t.Fatalf("%s references unknown outbound %q", o.Tag, m)
				}
			}
		}
		sets := map[string]bool{}
		for _, s := range doc.Route.RuleSet {
			sets[s.Tag] = true
		}
		for _, r := range doc.Route.Rules {
			for _, s := range r.RuleSet {
				if !sets[s] {
					t.Fatalf("rule references missing rule_set %q", s)
				}
			}
			if r.Outbound != "" && !tags[r.Outbound] {
				t.Fatalf("rule targets unknown outbound %q", r.Outbound)
			}
		}
		if !tags[doc.Route.Final] {
			t.Fatalf("final %q is not an outbound", doc.Route.Final)
		}
	case subscription.FormatV2ray, subscription.FormatShadowrocket, subscription.FormatSSR:
		for _, line := range strings.Split(strings.TrimSpace(string(decodeB64(t, body))), "\n") {
			if strings.HasPrefix(line, "vmess://") {
				var v map[string]string
				raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "vmess://"))
				if err != nil || json.Unmarshal(raw, &v) != nil || v["id"] != fixture.UserUUID {
					t.Fatalf("bad vmess link %q", line)
				}
			}
		}
	}
}

// diff returns the first differing lines (enough to locate a regression).
func diff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			return "line " + strconv.Itoa(i+1) + ":\n- " + a + "\n+ " + b
		}
	}
	return ""
}

// TestSupportMatrix pins which fixture nodes each format renders (see docs/SUBSCRIPTION.md for the reasons:
// classic Clash has no 2022 ciphers, sing-box no xhttp/ssr/mieru, Surge no VLESS, mihomo no AnyTLS+REALITY, ...).
func TestSupportMatrix(t *testing.T) {
	want := map[string]string{
		subscription.FormatClash:        "1 2 7 10 11 12 13 22 23",
		subscription.FormatClashMeta:    "1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 20 21 22 23 24 25",
		subscription.FormatLoon:         "1 2 3 5 7 8 10 11 12 15 16 18 19 21 22 23",
		subscription.FormatQuantumultX:  "1 2 3 5 7 10 11 12 18 21 22 23",
		subscription.FormatShadowrocket: "1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 21 22 23 24 25",
		subscription.FormatSingbox:      "1 2 3 5 6 7 8 9 11 12 13 14 15 16 17 18 19 21 22 24 25",
		subscription.FormatSSR:          "10 22",
		subscription.FormatStash:        "1 2 3 4 5 6 7 8 10 11 12 13 14 15 16 17 21 22 23 25",
		subscription.FormatSurfboard:    "1 2 7 11 12 22",
		subscription.FormatSurge:        "1 2 7 8 11 12 15 16 17 18 22",
		subscription.FormatV2ray:        "1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25",
	}
	c := rulesets.Builtin()
	in := input(t, standard(c), c)
	for name, ids := range want {
		f, ok := subscription.GetFormat(name)
		if !ok {
			t.Fatalf("%s not registered", name)
		}
		pf, _ := f.(subscription.ProxyFilter)
		var got []string
		for i := range in.Proxies {
			p := &in.Proxies[i]
			if !p.Info && f.Supports(p.Type) && (pf == nil || pf.SupportsProxy(p)) {
				got = append(got, strconv.FormatUint(p.NodeID, 10))
			}
		}
		if strings.Join(got, " ") != ids {
			t.Errorf("%s renders %v, want %s", name, got, ids)
		}
	}
	for _, name := range subscription.FormatNames() {
		if _, ok := want[name]; !ok && !strings.HasPrefix(name, "zzz") {
			t.Errorf("format %s has no support matrix entry", name)
		}
	}
}

// TestFinalPolicies covers REJECT as the final policy (sing-box has no reject outbound: a reject action is
// appended) and DIRECT finals.
func TestFinalPolicies(t *testing.T) {
	c := rulesets.Builtin()
	for _, final := range []string{"REJECT", "DIRECT"} {
		p := standard(c)
		p.Final = final
		in := input(t, p, c)
		sb, _ := subscription.GetFormat(subscription.FormatSingbox)
		res, err := engine.Render(sb, in)
		if err != nil {
			t.Fatal(err)
		}
		validate(t, subscription.FormatSingbox, res.Body)
		if final == "REJECT" && !strings.Contains(string(res.Body), `"action": "reject"`+"\n      }\n    ],\n    \"rule_set\"") {
			t.Fatalf("missing trailing reject action:\n%s", res.Body)
		}
		cm, _ := subscription.GetFormat(subscription.FormatClashMeta)
		res, _ = engine.Render(cm, in)
		if !strings.Contains(string(res.Body), "  - MATCH,"+final+"\n") {
			t.Fatal("clash final")
		}
	}
}

// TestInfoOnly renders info nodes only (what Generate does for expired users) in every format.
func TestInfoOnly(t *testing.T) {
	c := rulesets.Builtin()
	in := input(t, standard(c), c)
	var info []subscription.Proxy
	for _, p := range in.Proxies {
		if p.Info {
			info = append(info, p)
		}
	}
	in.Proxies = info
	for _, name := range subscription.FormatNames() {
		f, _ := subscription.GetFormat(name)
		if strings.HasPrefix(name, "zzz") {
			continue
		}
		res, err := engine.Render(f, in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		validate(t, name, res.Body)
		if len(res.Body) == 0 {
			t.Fatalf("%s: empty body", name)
		}
	}
	in.Proxies = nil
	for _, name := range []string{subscription.FormatClashMeta, subscription.FormatSingbox} {
		f, _ := subscription.GetFormat(name)
		res, err := engine.Render(f, in)
		if err != nil {
			t.Fatal(err)
		}
		validate(t, name, res.Body)
	}
}

func BenchmarkRender(b *testing.B) {
	c := rulesets.Builtin()
	in := input(b, standard(c), c)
	for _, name := range []string{subscription.FormatClashMeta, subscription.FormatSingbox, subscription.FormatSurge, subscription.FormatV2ray} {
		f, _ := subscription.GetFormat(name)
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := engine.Render(f, in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
