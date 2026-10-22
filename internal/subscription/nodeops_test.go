package subscription_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

func opsProxy(id uint64, typ, name, host string, port int, tags ...string) subscription.Proxy {
	p := subscription.Proxy{NodeID: id, Type: typ, Name: name, Host: host, Port: port, Tags: tags}
	if typ == model.NodeHysteria {
		p.Hysteria = &model.HysteriaSettings{Version: 2}
	}
	return p
}

func opsInput() []subscription.Proxy {
	return []subscription.Proxy{
		{Name: "剩余流量：10 GB", Type: model.NodeShadowsocks, Info: true, Host: "127.0.0.1", Port: 1080},
		opsProxy(1, model.NodeVmess, "🇭🇰 香港 01", "hk1.example.com", 443),
		opsProxy(2, model.NodeVless, "Japan Tokyo 10", "jp1.example.com", 443),
		opsProxy(3, model.NodeTrojan, "JP-2 Trojan", "jp2.example.com", 443),
		opsProxy(4, model.NodeHysteria, "US Hysteria2", "us1.example.com", 8443),
		opsProxy(5, model.NodeVmess, "Relay 上海-新加坡 IPLC", "sg1.example.com", 443),
		opsProxy(6, model.NodeVmess, "Backup", "hk1.example.com", 443, "HK"),
		opsProxy(7, model.NodeShadowsocks, "Game 专线", "x.example.com", 8388),
	}
}

func names(ps []subscription.Proxy) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestApplyNodeOps(t *testing.T) {
	info := "剩余流量：10 GB"
	cases := []struct {
		name string
		ops  subscription.NodeOps
		want []string
	}{
		{"zero ops keep everything", subscription.NodeOps{}, []string{info, "🇭🇰 香港 01", "Japan Tokyo 10", "JP-2 Trojan", "US Hysteria2", "Relay 上海-新加坡 IPLC", "Backup", "Game 专线"}},
		{"include", subscription.NodeOps{Include: []string{"(?i)japan|JP"}}, []string{info, "Japan Tokyo 10", "JP-2 Trojan"}},
		{"exclude", subscription.NodeOps{Exclude: []string{"Relay", "^Backup$"}}, []string{info, "🇭🇰 香港 01", "Japan Tokyo 10", "JP-2 Trojan", "US Hysteria2", "Game 专线"}},
		{"types with aliases", subscription.NodeOps{Types: []string{"hy2", "ss"}}, []string{info, "US Hysteria2", "Game 专线"}},
		{"regions: flag, names, tokens, tags; UK alias", subscription.NodeOps{Regions: []string{"hk", "SG", "UK"}}, []string{info, "🇭🇰 香港 01", "Relay 上海-新加坡 IPLC", "Backup"}},
		{"rename with groups in order", subscription.NodeOps{Include: []string{"JP|Japan"}, Rename: []subscription.RenameRule{
			{Match: `^Japan (\w+) (\d+)$`, Replace: "JP ${1}-$2"}, {Match: "-", Replace: " · "},
		}}, []string{info, "JP Tokyo · 10", "JP · 2 Trojan"}},
		{"emoji add only when missing", subscription.NodeOps{Emoji: subscription.EmojiAdd, Include: []string{"香港|Japan|Relay|Game"}},
			[]string{info, "🇭🇰 香港 01", "🇯🇵 Japan Tokyo 10", "🇸🇬 Relay 上海-新加坡 IPLC", "Game 专线"}},
		{"emoji strip", subscription.NodeOps{Emoji: subscription.EmojiStrip, Include: []string{"香港"}}, []string{info, "香港 01"}},
		{"prefix and suffix", subscription.NodeOps{Include: []string{"Game"}, Prefix: "[GG] ", Suffix: " ⚡"}, []string{info, "[GG] Game 专线 ⚡"}},
		{"natural name sort", subscription.NodeOps{Sort: subscription.SortName, Include: []string{"JP|Japan|US"}},
			[]string{info, "Japan Tokyo 10", "JP-2 Trojan", "US Hysteria2"}},
		{"region sort by table order", subscription.NodeOps{Sort: subscription.SortRegion},
			[]string{info, "🇭🇰 香港 01", "Backup", "Japan Tokyo 10", "JP-2 Trojan", "Relay 上海-新加坡 IPLC", "US Hysteria2", "Game 专线"}},
		{"region sort by the regions list", subscription.NodeOps{Sort: subscription.SortRegion, Regions: []string{"US", "JP"}},
			[]string{info, "US Hysteria2", "Japan Tokyo 10", "JP-2 Trojan"}},
		{"dedupe on type host port keeps the first", subscription.NodeOps{Dedupe: true}, []string{info, "🇭🇰 香港 01", "Japan Tokyo 10", "JP-2 Trojan", "US Hysteria2", "Relay 上海-新加坡 IPLC", "Game 专线"}},
		{"limit after sort", subscription.NodeOps{Sort: subscription.SortName, Limit: 2}, []string{info, "Backup", "Game 专线"}},
		{"invalid stored expression matches literally", subscription.NodeOps{Include: []string{"(["}}, []string{info}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := opsInput()
			got := names(subscription.ApplyNodeOps(in, tc.ops.Normalized()))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
			if !reflect.DeepEqual(names(in), names(opsInput())) {
				t.Fatal("input modified")
			}
		})
	}
}

func TestDetectRegion(t *testing.T) {
	cases := map[string]string{
		"🇭🇰 Whatever":           "HK",
		"🇬🇧 London":             "GB",
		"🇺🇸 美国 → 香港":            "US", // the flag wins
		"上海-香港 IPLC 01":         "HK", // relays: the domestic entry city does not count
		"印度尼西亚 01":              "ID", // before India
		"印度 孟买":                 "IN",
		"内蒙古 BGP":               "CN", // not Mongolia
		"蒙古 01":                 "MN",
		"HK01":                  "HK",
		"hk01":                  "", // tokens are upper case
		"PHK 1":                 "",
		"CN2 GIA 美西":            "US",
		"CN2 GIA":               "",
		"JP-Osaka":              "JP",
		"Singapore Premium":     "SG",
		"Taipei 101":            "TW",
		"Seoul KR 3":            "KR",
		"Frankfurt DE":          "DE",
		"Generic node":          "",
		"UK London Gateway 07":  "GB",
		"Los Angeles 洛杉矶 Relay": "US",
	}
	for name, want := range cases {
		if got := subscription.DetectRegion(name, nil); got != want {
			t.Errorf("DetectRegion(%q) = %q, want %q", name, got, want)
		}
	}
	if got := subscription.DetectRegion("Backup", []string{"JP"}); got != "JP" {
		t.Errorf("tag region = %q", got)
	}
	if subscription.RegionFlag("HK") != "🇭🇰" || subscription.RegionFlag("hk") != "" || subscription.RegionFlag("X") != "" {
		t.Error("RegionFlag")
	}
	if c, ok := subscription.RegionCode(" uk "); !ok || c != "GB" {
		t.Errorf("RegionCode(uk) = %q %v", c, ok)
	}
	if _, ok := subscription.RegionCode("XX"); ok {
		t.Error("unknown region accepted")
	}
	for _, r := range subscription.Regions() {
		if subscription.RegionFlag(r.Code) == "" || strings.ToUpper(r.Code) != r.Code {
			t.Errorf("bad table code %q", r.Code)
		}
	}
}

func TestNaturalLess(t *testing.T) {
	in := []string{"hk 10", "HK 2", "HK 01", "hk", "JP 1", "HK 2a"}
	want := []string{"hk", "HK 01", "HK 2", "HK 2a", "hk 10", "JP 1"}
	got := append([]string(nil), in...)
	for i := 1; i < len(got); i++ { // insertion sort with NaturalLess
		for j := i; j > 0 && subscription.NaturalLess(got[j], got[j-1]); j-- {
			got[j], got[j-1] = got[j-1], got[j]
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

// Node operations run inside NodeProxies: after the ?filter=/types= filters and the name normalization, and
// before ?emoji=0 and the protocol prefix, so user-supplied text is normalized too.
func TestNodeProxiesAppliesOps(t *testing.T) {
	ops := subscription.NodeOps{Rename: []subscription.RenameRule{{Match: "HK", Replace: "Hong=Kong,"}}, Emoji: subscription.EmojiAdd}.Normalized()
	ps := subscription.ApplyNodeOps([]subscription.Proxy{opsProxy(1, model.NodeVmess, "HK 01", "h", 1)}, ops)
	if ps[0].Name != "🇭🇰 Hong=Kong, 01" {
		t.Fatalf("ops name = %q", ps[0].Name)
	}
	if got := subscription.DisplayName(ps[0].Name, true); got != "Hong＝Kong， 01" {
		t.Fatalf("normalized = %q", got)
	}
}
