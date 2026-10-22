package subscription

import (
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
)

func testUser() *model.User {
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	return &model.User{ID: 7, UUID: "3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f", TransferEnable: 10 << 30, U: 1 << 30, ExpiredAt: &exp}
}

func TestParsePorts(t *testing.T) {
	cases := []struct {
		in        string
		port      int
		hop       string
		wantError bool
	}{
		{"443", 443, "", false},
		{" 20000-30000 ", 20000, "20000-30000", false},
		{"443,20000-30000", 443, "443,20000-30000", false},
		{"", 0, "", true},
		{"0", 0, "", true},
		{"70000", 0, "", true},
		{"300-200", 0, "", true},
		{"abc", 0, "", true},
	}
	for _, tc := range cases {
		port, hop, err := ParsePorts(tc.in)
		if (err != nil) != tc.wantError || port != tc.port || hop != tc.hop {
			t.Errorf("ParsePorts(%q) = %d %q %v", tc.in, port, hop, err)
		}
	}
}

func TestDisplayNameAndEmoji(t *testing.T) {
	if got := DisplayName("🇭🇰 HK, 01 = fast\t✈️", true); got != "HK， 01 ＝ fast" {
		t.Fatalf("DisplayName strip = %q", got)
	}
	if got := DisplayName("  🇯🇵  JP\n01 ", false); got != "🇯🇵 JP 01" {
		t.Fatalf("DisplayName keep = %q", got)
	}
	for _, r := range []rune{'🇺', '😀', '⭐', '‍', '️', '☁'} {
		if !IsEmoji(r) {
			t.Errorf("IsEmoji(%U) = false", r)
		}
	}
	for _, r := range []rune{'a', '中', '：', '-'} {
		if IsEmoji(r) {
			t.Errorf("IsEmoji(%U) = true", r)
		}
	}
}

func TestUniqueNames(t *testing.T) {
	ps := []Proxy{{Name: "A"}, {Name: "A"}, {Name: "Proxy"}, {Name: "direct"}, {Name: "A 2"}}
	UniqueNames(ps, map[string]bool{"Proxy": true})
	got := []string{}
	for _, p := range ps {
		got = append(got, p.Name)
	}
	if strings.Join(got, "|") != "A|A 2|Proxy 2|direct 2|A 2 2" {
		t.Fatalf("names = %v", got)
	}
}

func TestBuildProxyCredentialsMatchModel(t *testing.T) {
	u := testUser()
	created := time.Unix(1735689600, 0)
	for _, cipher := range []string{"aes-128-gcm", "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305"} {
		n := &model.Node{ID: 1, Type: model.NodeShadowsocks, Host: "ss.example.com", Port: "8388", CreatedAt: created,
			ProtocolSettings: model.JSONMap{"cipher": cipher}}
		p, err := BuildProxy(n, nil, u)
		if err != nil {
			t.Fatal(err)
		}
		if want := model.UserCredentials(u, n).Password; p.Password != want {
			t.Errorf("%s: password %q, model says %q", cipher, p.Password, want)
		}
	}
	// SSR: the node password (derived from created_at when unset) and the single-port protocol parameter.
	n := &model.Node{ID: 2, Type: model.NodeShadowsocksR, Host: "ssr.example.com", Port: "443", CreatedAt: created,
		ProtocolSettings: model.JSONMap{"protocol": "auth_aes128_md5"}}
	p, err := BuildProxy(n, nil, u)
	if err != nil {
		t.Fatal(err)
	}
	if p.Password != "YjVkODdhNWRlMWEzNWIyMg==" || p.SSRProtocolParam != "7:"+u.UUID {
		t.Fatalf("ssr credentials = %q %q", p.Password, p.SSRProtocolParam)
	}
	// Child nodes use the parent's settings and keys but their own address.
	parent := &model.Node{ID: 3, Type: model.NodeShadowsocks, Host: "p.example.com", Port: "1", CreatedAt: created,
		ProtocolSettings: model.JSONMap{"cipher": "2022-blake3-aes-128-gcm"}}
	child := &model.Node{ID: 4, Type: model.NodeShadowsocks, Host: "c.example.com", Port: "2", ParentID: &parent.ID, Parent: parent,
		CreatedAt: time.Unix(1, 0)}
	cp, err := BuildProxy(child, parent, u)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Host != "c.example.com" || cp.Port != 2 || !strings.HasPrefix(cp.Password, "YjVkODdhNWRlMWEzNWIyMg==:") {
		t.Fatalf("child = %+v", cp)
	}
	if _, err := BuildProxy(&model.Node{Type: "socks", Host: "x", Port: "1"}, nil, u); err == nil {
		t.Fatal("unknown type must fail")
	}
	if _, err := BuildProxy(&model.Node{Type: model.NodeVmess, Host: "", Port: "1"}, nil, u); err == nil {
		t.Fatal("empty host must fail")
	}
	if _, err := BuildProxy(&model.Node{Type: model.NodeVmess, Host: "x", Port: "1", ProtocolSettings: model.JSONMap{"tls": "yes"}}, nil, u); err == nil {
		t.Fatal("undecodable settings must fail")
	}
	// Port ranges only survive on QUIC protocols and mieru.
	vm, _ := BuildProxy(&model.Node{Type: model.NodeVmess, Host: "x", Port: "1000-2000"}, nil, u)
	hy, _ := BuildProxy(&model.Node{Type: model.NodeHysteria, Host: "x", Port: "1000-2000"}, nil, u)
	if vm.Port != 1000 || vm.Ports != "" || hy.Ports != "1000-2000" {
		t.Fatalf("ports vmess=%d %q hysteria=%q", vm.Port, vm.Ports, hy.Ports)
	}
}

func TestProxyAccessors(t *testing.T) {
	u := testUser()
	mk := func(typ string, ps model.JSONMap) Proxy {
		p, err := BuildProxy(&model.Node{Type: typ, Host: "h.example.com", Port: "443", ProtocolSettings: ps}, nil, u)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	hy1 := mk(model.NodeHysteria, model.JSONMap{"version": 1, "obfs": "xplus", "obfs_password": "p"})
	hy2 := mk(model.NodeHysteria, model.JSONMap{"version": 2, "obfs": "salamander", "obfs_password": "p"})
	hy2plain := mk(model.NodeHysteria, model.JSONMap{"version": 2, "obfs": "plain", "obfs_password": "p"})
	if hy1.ProtoName() != "hysteria" || hy2.ProtoName() != "hysteria2" || hy1.HysteriaObfs() != "p" || hy2.HysteriaObfs() != "p" || hy2plain.HysteriaObfs() != "" {
		t.Fatal("hysteria accessors")
	}
	tuic := mk(model.NodeTuic, model.JSONMap{"alpn": "h3, h2"})
	if a := tuic.ALPN(); len(a) != 2 || a[1] != "h2" || tuic.TLSMode() != model.TLSEnabled {
		t.Fatalf("tuic alpn = %v", a)
	}
	at := mk(model.NodeAnyTLS, model.JSONMap{"tls": 2, "server_name": "fallback.example", "tls_settings": model.JSONMap{"public_key": "k", "short_id": "s"}})
	if pk, sid, ok := at.Reality(); !ok || pk != "k" || sid != "s" || at.SNI() != "fallback.example" || at.Fingerprint() != "chrome" {
		t.Fatal("anytls reality")
	}
	vm := mk(model.NodeVmess, model.JSONMap{"tls": 1, "tls_settings": model.JSONMap{"server_name": "s", "allow_insecure": true, "fingerprint": "ios"}})
	if vm.SNI() != "s" || !vm.Insecure() || vm.Fingerprint() != "ios" || vm.Network != "tcp" {
		t.Fatal("vmess accessors")
	}
	ss := mk(model.NodeShadowsocks, model.JSONMap{"obfs": "tls"})
	if ss.SSObfs() != "tls" || ss.ProtoName() != "ss" {
		t.Fatal("ss obfs")
	}
}

func TestTypeAndKeywordFilters(t *testing.T) {
	hy2 := &Proxy{Type: model.NodeHysteria, Hysteria: &model.HysteriaSettings{Version: 2}, Name: "JP 01", Tags: []string{"Premium"}}
	hy1 := &Proxy{Type: model.NodeHysteria, Hysteria: &model.HysteriaSettings{Version: 1}, Name: "HK"}
	ss := &Proxy{Type: model.NodeShadowsocks, Name: "US"}
	if !typeMatches(hy2, []string{"hy2"}) || typeMatches(hy1, []string{"hysteria2"}) || !typeMatches(hy1, []string{"hysteria"}) ||
		!typeMatches(ss, []string{"vmess", "ss"}) || typeMatches(ss, []string{"ssr"}) || !typeMatches(ss, nil) {
		t.Fatal("type filter")
	}
	if !keywordMatches(hy2, []string{"premium"}) || !keywordMatches(hy2, []string{"hk", "jp"}) || keywordMatches(hy1, []string{"jp"}) {
		t.Fatal("keyword filter")
	}
	if got := ParseList(" a, b|c ,,"); strings.Join(got, ";") != "a;b;c" {
		t.Fatalf("ParseList = %v", got)
	}
}

func TestInfoLines(t *testing.T) {
	now := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	u := testUser()
	next := now.Add(36 * time.Hour)
	u.NextResetAt = &next
	got := strings.Join(InfoLines(u, "zh-CN", now, time.UTC), "|")
	if got != "剩余流量：9 GB|套餐到期：2027-01-01|距离下次重置剩余：2 天" {
		t.Fatalf("zh = %s", got)
	}
	u.ExpiredAt = nil
	u.Banned = true
	if got := strings.Join(InfoLines(u, "en", now, time.UTC), "|"); got != "Account banned|Traffic left: 9 GB|Expires: never|Next reset in: 2 days" {
		t.Fatalf("en = %s", got)
	}
	u.Banned = false
	past := now.Add(-time.Hour)
	u.ExpiredAt = &past
	if got := InfoLines(u, "fr", now, time.UTC); got[0] != "套餐已过期" || len(got) != 3 {
		t.Fatalf("expired = %v", got)
	}
	u.ExpiredAt = nil
	u.D = 20 << 30
	if got := InfoLines(u, "en", now, nil); got[0] != "Traffic exhausted" || got[1] != "Traffic left: 0 GB" {
		t.Fatalf("exhausted = %v", got)
	}
	ps := InfoProxies(u, "en", now, time.UTC)
	if len(ps) != 4 || !ps[0].Info || ps[0].Host != InfoHost || ps[0].SS.Cipher != InfoCipher || ps[0].Password != u.UUID {
		t.Fatalf("info proxies = %+v", ps[0])
	}
	if FormatGB(1<<30+1<<29) != "1.5 GB" || FormatGB(0) != "0 GB" {
		t.Fatal("FormatGB")
	}
}

func TestResolveGroups(t *testing.T) {
	proxies := []Proxy{{Name: "info", Info: true}, {Name: "HK 01"}, {Name: "US 01"}, {Name: "JP 01"}}
	groups := []PolicyGroup{
		{Name: "Proxy", Type: "select", Include: "auto, *"},
		{Name: "AI", Type: "select", Include: "Proxy, US|JP, ([bad"},
		{Name: "Empty", Type: "select", Include: "SG"},
		{Name: "LoopA", Type: "select", Include: "LoopB, HK"},
		{Name: "LoopB", Type: "select", Include: "LoopA"},
		{Name: "Block", Type: "select", Include: "REJECT, DIRECT"},
		{Name: "auto", Type: "url-test", Include: "*"},
		{Name: "Proxy", Type: "select", Include: "DIRECT"}, // duplicate: ignored
	}
	got := ResolveGroups(groups, proxies)
	names := func(g ResolvedGroup) string {
		var out []string
		for _, m := range g.Members {
			out = append(out, m.Name)
		}
		return strings.Join(out, ",")
	}
	want := map[string]string{
		"Proxy": "auto,info,HK 01,US 01,JP 01",
		"AI":    "Proxy,US 01,JP 01",
		"Empty": "Proxy",
		"LoopA": "HK 01", // LoopB closes a loop (LoopB -> LoopA): the first reference in scan order is dropped
		"LoopB": "LoopA",
		"Block": "REJECT,DIRECT",
		"auto":  "HK 01,US 01,JP 01",
	}
	if len(got) != len(want) {
		t.Fatalf("groups = %d", len(got))
	}
	for _, g := range got {
		if names(g) != want[g.Name] {
			t.Errorf("%s = %s, want %s", g.Name, names(g), want[g.Name])
		}
	}
	if got[6].URL != DefaultTestURL || got[6].Interval != DefaultTestInterval {
		t.Fatal("health-check defaults")
	}
	// No nodes at all: auto would loop back to Proxy, so it falls back to DIRECT.
	empty := ResolveGroups([]PolicyGroup{{Name: "Proxy", Include: "auto, *"}, {Name: "auto", Type: "url-test", Include: "*"}}, nil)
	if names(empty[0]) != "auto" || names(empty[1]) != "DIRECT" || empty[0].Type != GroupSelect {
		t.Fatalf("empty = %s / %s", names(empty[0]), names(empty[1]))
	}
}

func TestRulePlanHelpers(t *testing.T) {
	if (RuleSet{URL: "https://x/a/b.mrs?x=1"}).Ext() != "mrs" || (RuleSet{URL: "https://x/a.b/c", Format: "text"}).Ext() != "text" {
		t.Fatal("Ext")
	}
	p := &RulePlan{Sets: []RuleSet{{Category: "a", Target: "X"}, {Category: "a", Target: "X"}, {Category: "b", Target: "Y"}}}
	if g := p.CategoryGroups(); len(g) != 2 || len(g[0]) != 2 {
		t.Fatalf("groups = %v", g)
	}
	var nilPlan *RulePlan
	if nilPlan.CategoryGroups() != nil {
		t.Fatal("nil plan")
	}
	src := Source{URLs: map[string]string{"singbox": "u"}, Aliases: map[string]string{"singbox_geosite": "singbox"}}
	if u, m, ok := src.Template("singbox_geosite"); !ok || u != "u" || m.Field != "name" {
		t.Fatal("alias")
	}
	if _, _, ok := src.Template("surge"); ok {
		t.Fatal("missing key")
	}
	if !IsReservedName(" Direct ") || IsReservedName("Proxy") || !IsBuiltinPolicy("REJECT") {
		t.Fatal("reserved names")
	}
}
