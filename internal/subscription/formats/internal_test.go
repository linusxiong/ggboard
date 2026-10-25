package formats

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

func TestYAMLScalars(t *testing.T) {
	for in, want := range map[string]string{
		"auto": "auto", "/ws": "/ws", "RULE-SET,a,DIRECT": "RULE-SET,a,DIRECT", "": `""`, "true": `"true"`,
		"No": `"No"`, "443": `"443"`, "1.1.1.1": `"1.1.1.1"`, "a b": `"a b"`, "a: b": `"a: b"`, "#x": `"#x"`,
		"香港": `"香港"`, "x\"y\\z\n": `"x\"y\\z\n"`, "\x01": `"\u0001"`, "-x": `"-x"`, "*.lan": `"*.lan"`,
	} {
		if got := yamlScalar(in); got != want {
			t.Errorf("yamlScalar(%q) = %s, want %s", in, got, want)
		}
	}
	// Every scalar round-trips through a real parser.
	var b strings.Builder
	doc := ymap{{"a", ylist{"x: y", "- z", "{a}", "[b]", "&c", "!d", "|e", ">f", "%g", "@h", "`i", "'j", "null", "~", "0x10", "1e3", "yes"}},
		{"m", ymap{}}, {"l", ylist{}}, {"s", []string{}}, {"n", 1}, {"i64", int64(2)}, {"b", false}, {"skip", nil},
		{"nested", ylist{ymap{{"k", "v"}, {"inner", ymap{{"x", 1}}}}, ymap{}}}}
	writeYAML(&b, doc)
	var back struct {
		A      []string         `yaml:"a"`
		M      map[string]any   `yaml:"m"`
		L      []any            `yaml:"l"`
		N      int              `yaml:"n"`
		Nested []map[string]any `yaml:"nested"`
	}
	if err := yaml.Unmarshal([]byte(b.String()), &back); err != nil {
		t.Fatalf("%v\n%s", err, b.String())
	}
	if back.A[0] != "x: y" || back.A[13] != "~" || back.A[16] != "yes" || back.N != 1 || back.Nested[0]["k"] != "v" || len(back.Nested) != 2 {
		t.Fatalf("round trip = %+v\n%s", back, b.String())
	}
	if strings.Contains(b.String(), "skip") {
		t.Fatal("nil values are skipped")
	}
}

func TestJSONObject(t *testing.T) {
	o := jobj{{"b", 1}, {"a", "<&>"}, {"skip", nil}, {"list", []jobj{{{"x", true}}}}, {"any", []any{jobj{{"y", 2}}, "z"}}}
	b, err := o.MarshalJSON()
	if err != nil || string(b) != `{"b":1,"a":"<&>","list":[{"x":true}],"any":[{"y":2},"z"]}` {
		t.Fatalf("%s %v", b, err)
	}
	var set jobj
	set.setStr("e", "")
	set.setTrue("f", false)
	set.setStr("g", "v")
	set.setTrue("h", true)
	if b, _ := set.MarshalJSON(); string(b) != `{"g":"v","h":true}` {
		t.Fatalf("%s", b)
	}
}

func TestSSPlugin(t *testing.T) {
	mk := func(ps model.JSONMap) *subscription.Proxy {
		var s model.ShadowsocksSettings
		_ = s.Decode(ps)
		return &subscription.Proxy{Type: model.NodeShadowsocks, SS: &s}
	}
	cases := []struct {
		ps   model.JSONMap
		want ssPluginInfo
		sip  string
	}{
		{model.JSONMap{}, ssPluginInfo{}, ""},
		{model.JSONMap{"obfs": "tls", "obfs_settings": model.JSONMap{"host": "h"}}, ssPluginInfo{Kind: "obfs", Mode: "tls", Host: "h", Path: "/"}, "obfs-local;obfs=tls;obfs-host=h"},
		{model.JSONMap{"plugin": "obfs-local", "plugin_opts": "obfs=http;obfs-host=h;obfs-uri=/p"}, ssPluginInfo{Kind: "obfs", Mode: "http", Host: "h", Path: "/p"}, "obfs-local;obfs=http;obfs-host=h;obfs-uri=/p"},
		{model.JSONMap{"plugin": "obfs", "plugin_opts": "obfs=quic"}, ssPluginInfo{Kind: "unknown"}, ""},
		{model.JSONMap{"plugin": "v2ray-plugin", "plugin_opts": "tls;host=h;path=/v;mux=4"}, ssPluginInfo{Kind: "v2ray-plugin", Mode: "websocket", Host: "h", Path: "/v", TLS: true, Mux: true}, "v2ray-plugin;mode=websocket;tls;host=h;path=/v;mux=1"},
		{model.JSONMap{"plugin": "v2ray-plugin", "plugin_opts": "mode=quic"}, ssPluginInfo{Kind: "unknown"}, ""},
		{model.JSONMap{"plugin": "shadow-tls", "plugin_opts": "host=h;password=p"}, ssPluginInfo{Kind: "shadow-tls", Host: "h", Password: "p", Version: 2}, "shadow-tls;host=h;password=p;version=2"},
		{model.JSONMap{"plugin": "kcptun"}, ssPluginInfo{Kind: "unknown"}, ""},
	}
	for _, tc := range cases {
		got := ssPlugin(mk(tc.ps))
		if got != tc.want || got.sipPlugin() != tc.sip {
			t.Errorf("ssPlugin(%v) = %+v (%q)", tc.ps, got, got.sipPlugin())
		}
	}
	if ssPlugin(&subscription.Proxy{}) != (ssPluginInfo{}) {
		t.Fatal("non-ss proxy")
	}
}

func TestHelpers(t *testing.T) {
	if strings.Join(sbPorts("20000-30000,443"), " ") != "20000:30000 443:443" {
		t.Fatal("sbPorts")
	}
	if hostPort("2001:db8::1", 443) != "[2001:db8::1]:443" || hostPort("[::1]", 1) != "[::1]:1" || hostPort("a.example.com", 2) != "a.example.com:2" {
		t.Fatal("hostPort")
	}
	for h, want := range map[string]bool{"panel.example.com": true, "192.0.2.1": false, "localhost": false, "": false, "2001:db8::1": false, "a_b.example.com": false} {
		if isDomain(h) != want {
			t.Errorf("isDomain(%q) != %v", h, want)
		}
	}
	if escapeComponent("a b+c/#") != "a%20b%2Bc%2F%23" || b64url("??>") != "Pz8-" {
		t.Fatal("escaping")
	}
	g := subscription.ResolvedGroup{Members: []subscription.Member{{Name: "REJECT", Kind: subscription.MemberReject}}}
	if strings.Join(memberNames(g, "direct", ""), ",") != "direct" || strings.Join(memberNames(g, "DIRECT", "REJECT"), ",") != "REJECT" {
		t.Fatal("memberNames")
	}
	if policyName("DIRECT", "direct", "reject") != "direct" || policyName("REJECT", "direct", "reject") != "reject" || policyName("AI", "d", "r") != "AI" {
		t.Fatal("policyName")
	}
}

func TestSingboxHTTPClients(t *testing.T) {
	for ua, want := range map[string]bool{
		"SFA (sing-box 1.14.4; language zh_CN)":                     true, // current SFA/SFI/SFM; older builds send "SFA/<app> (<build>; sing-box <core>; …)"
		"SFI (sing-box 1.14.0; language en_US)":                     true,
		"SFM (sing-box 1.15.0-alpha.2; language en_US)":             true,
		"SFA/1.14.1-beta.1 (621; sing-box 1.14.1-beta.1; language)": true,
		"sing-box/1.14.4": true,
		"SING-BOX v1.14":  true,
		"sing-box 2.0.0":  true,
		"SFT (sing-box 1.14.0-beta.3; language en)":                 false, // a 1.14.0 pre-release may predate http_clients
		"SFA/1.11.4 (614; sing-box 1.11.4; language zh_CN)":         false,
		"SFA/1.12.0 (sing-box 1.12.0)":                              false,
		"SFI (sing-box 1.13.99; language en_US)":                    false,
		"sing-box 1.9.9":                                            false,
		"sing-box unknown":                                          false,
		"HiddifyNext/2.5.7 (android) like ClashMeta v2ray sing-box": false,
		"sing-box 99999999999999999999.1":                           false,
		"clash-verge/v2.2.3":                                        false,
		"":                                                          false,
	} {
		if got := sbHTTPClients(ua); got != want {
			t.Errorf("sbHTTPClients(%q) = %v, want %v", ua, got, want)
		}
	}
}
