package subscription

import (
	"encoding/json"
	"testing"
)

func TestDetectFormat(t *testing.T) {
	cases := []struct{ flag, ua, want string }{
		{"", "", FormatV2ray},
		{"", "Mozilla/5.0 (Macintosh) AppleWebKit Safari/605.1.15", FormatV2ray},
		{"", "clash.meta/1.18", FormatClashMeta},
		{"", "mihomo/1.19.0", FormatClashMeta},
		{"", "ClashMetaForAndroid/2.11", FormatClashMeta},
		{"", "clash-verge/v2.0.0", FormatClashMeta},
		{"", "Clash Verge Rev", FormatClashMeta},
		{"", "FlClash/0.8", FormatClashMeta},
		{"", "Clash Nyanpasu", FormatClashMeta},
		{"", "ClashX Meta/1.3", FormatClashMeta},
		{"", "Stash/2.4.0 Clash/1.9.0", FormatStash},
		{"", "ClashForAndroid/2.5.12", FormatClash},
		{"", "ClashX/1.95", FormatClash},
		{"", "clash", FormatClash},
		{"", "sing-box 1.11.0", FormatSingbox},
		{"", "SFA/1.10.0", FormatSingbox},
		{"", "SFI/1.10", FormatSingbox},
		{"", "SFM/1.10", FormatSingbox},
		{"", "NekoBox/Android 1.3", FormatSingbox},
		{"", "HiddifyNext/2.0", FormatSingbox},
		{"", "Surge iOS/2920", FormatSurge},
		{"", "Surfboard/2.24", FormatSurfboard},
		{"", "Quantumult X/1.4", FormatQuantumultX},
		{"", "Quantumult%20X/1.0.30", FormatQuantumultX},
		{"", "Shadowrocket/2070", FormatShadowrocket},
		{"", "Loon/3.2", FormatLoon},
		{"", "v2rayN/6.42", FormatV2ray},
		{"", "v2rayNG/1.8", FormatV2ray},
		{"", "Karing/1.0", FormatV2ray},
		{"", "Shadowsocks/1.0", FormatV2ray},
		{"clash", "Shadowrocket/2070", FormatClash},
		{"clash-meta", "", FormatClashMeta},
		{"singbox", "ClashX/1.0", FormatSingbox},
		{"SSR", "", FormatSSR},
		{"quantumultx", "", FormatQuantumultX},
		{"meta", "Shadowrocket/1", FormatShadowrocket}, // unknown flag falls back to UA
		{"clashmeta", "", FormatClashMeta},
		{"hiddify", "", FormatSingbox},
		{"  V2RAY ", "Surge", FormatV2ray},
	}
	for _, tc := range cases {
		if got := DetectFormat(tc.flag, tc.ua); got != tc.want {
			t.Errorf("DetectFormat(%q, %q) = %s, want %s", tc.flag, tc.ua, got, tc.want)
		}
	}
}

type fakeFormat struct{}

func (fakeFormat) Name() string           { return "zzz_fake" }
func (fakeFormat) Supports(t string) bool { return t == "vmess" }
func (fakeFormat) Render(*RenderContext) ([]byte, string, error) {
	return []byte("ok"), "text/plain", nil
}

func TestFormatRegistry(t *testing.T) {
	RegisterFormat(fakeFormat{})
	f, ok := GetFormat("zzz_fake")
	if !ok || !f.Supports("vmess") {
		t.Fatal("registry")
	}
	if names := FormatNames(); names[len(names)-1] != "zzz_fake" {
		t.Fatalf("names = %v", names)
	}
}

func TestRuleProfileJSON(t *testing.T) {
	raw := `{"template":"standard","source":"blackmatrix7","categories":{"ads":"REJECT","cn":"DIRECT","netflix":false,"x":null},
		"groups":[{"name":"Proxy","type":"select","include":"auto, *"},{"name":"auto","type":"url-test","include":"*","url":"http://www.gstatic.com/generate_204","interval":300}],
		"final":"Proxy","geoip_cn_direct":true,"mirror":"default","dns":"fakeip"}`
	var p RuleProfile
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Categories) != 2 || p.Categories["ads"] != "REJECT" || p.Groups[1].Interval != 300 || !p.GeoIPCNDirect || p.DNS != DNSFakeIP {
		t.Fatalf("profile = %+v", p)
	}
	if err := json.Unmarshal([]byte(`{"categories":{"ads":true}}`), &p); err == nil {
		t.Fatal("true is not a valid target")
	}
	if err := json.Unmarshal([]byte(`{"categories":{"ads":1}}`), &p); err == nil {
		t.Fatal("numbers are not valid targets")
	}
	if err := json.Unmarshal([]byte(`{"categories":null}`), &p); err != nil || p.Categories != nil {
		t.Fatal("null categories")
	}
}

func TestCatalogLookups(t *testing.T) {
	c := &Catalog{
		Categories: []Category{{ID: "openai", Name: LocalizedText{"zh-CN": "OpenAI 中文", "en": "OpenAI"}, DefaultTarget: "AI"}},
		Templates:  []Template{{ID: "standard", Final: "Proxy"}},
	}
	if tpl, ok := c.Template("standard"); !ok || tpl.Final != "Proxy" {
		t.Fatal("template")
	}
	if _, ok := c.Template("none"); ok {
		t.Fatal("missing template")
	}
	cat, ok := c.Category("openai")
	if !ok || cat.Name.Get("zh-CN") != "OpenAI 中文" || cat.Name.Get("fr") != "OpenAI" {
		t.Fatal("category")
	}
	if (LocalizedText{"ja": "x"}).Get("en") != "x" || (LocalizedText{}).Get("en") != "" {
		t.Fatal("localized fallback")
	}
	var nilCat *Catalog
	if _, ok := nilCat.Category("x"); ok {
		t.Fatal("nil catalog")
	}
}
