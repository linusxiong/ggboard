package nodesvc

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/app/online"
	"github.com/linusxiong/ggboard/internal/model"
)

func TestParseUniProxyPush(t *testing.T) {
	got, err := ParseUniProxyPush([]byte(`{"1":[1024,4096],"2":[0,512],"0":[1,1],"x":[1,1],"3":[1],"4":[-1,5],"5":["1","2"],"6":[1.9,2.1],"7":[1e3,0],"8":null,"9":[1,2,3]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint64]Traffic{1: {1024, 4096}, 2: {0, 512}, 6: {1, 2}, 7: {1000, 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, body := range []string{"[]", " [ ] ", "{}"} {
		if got, err := ParseUniProxyPush([]byte(body)); err != nil || len(got) != 0 {
			t.Fatalf("%q: %v %v", body, got, err)
		}
	}
	for _, body := range []string{"", "null", `"x"`, "[1]", "{bad", "123"} {
		if _, err := ParseUniProxyPush([]byte(body)); !errors.Is(err, ErrBadReport) {
			t.Fatalf("%q: err = %v", body, err)
		}
	}
}

func TestParseSogaTraffic(t *testing.T) {
	got, err := ParseSogaTraffic([]byte(`[{"id":1,"u":1024,"d":4096},{"id":1,"u":1,"d":1},{"id":0,"u":5,"d":5},{"id":-2,"u":5,"d":5},{"id":3,"u":-1,"d":5},{"id":4,"d":7},"junk",{"id":"5","u":1,"d":1}]`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint64]Traffic{1: {1025, 4097}, 4: {0, 7}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, body := range []string{"null", "[]"} {
		if got, err := ParseSogaTraffic([]byte(body)); err != nil || len(got) != 0 {
			t.Fatalf("%q: %v %v", body, got, err)
		}
	}
	if _, err := ParseSogaTraffic([]byte(`{"1":[1,2]}`)); !errors.Is(err, ErrBadReport) {
		t.Fatalf("object body: %v", err)
	}
}

func TestParseAlive(t *testing.T) {
	got, err := ParseUniProxyAlive([]byte(`{"1":["203.0.113.7:51234","203.0.113.7","[2001:db8::1]:443","2001:db8::2"],"2":[],"x":["198.51.100.1"],"3":"bad"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint64][]string{1: {"203.0.113.7", "2001:db8::1", "2001:db8::2"}, 2: {}, 3: {}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if _, err := ParseUniProxyAlive([]byte(`[1]`)); !errors.Is(err, ErrBadReport) {
		t.Fatalf("array body: %v", err)
	}
	sg, err := ParseSogaAlive([]byte(`[{"id":1,"ips":["203.0.113.7","198.51.100.9"]},{"id":1,"ips":["203.0.113.7","192.0.2.0"]},{"id":0,"ips":["192.0.2.1"]}]`))
	if err != nil {
		t.Fatal(err)
	}
	if want := map[uint64][]string{1: {"203.0.113.7", "198.51.100.9", "192.0.2.0"}}; !reflect.DeepEqual(sg, want) {
		t.Fatalf("soga got %#v", sg)
	}
}

func TestParseStatus(t *testing.T) {
	load, err := ParseStatus([]byte(`{"cpu":12.5,"mem":{"total":2147483648,"used":734003200},"swap":{"total":0,"used":0},"disk":{"total":42949672960,"used":10737418240},"uptime":86400}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if want := (online.Load{CPU: 12.5, Mem: 34.18, Swap: 0, Disk: 25, Uptime: 86400}); load != want {
		t.Fatalf("load = %+v, want %+v", load, want)
	}
	var se *StatusError
	if _, err := ParseStatus([]byte(`{"cpu":1,"mem":{"total":1,"used":1},"swap":{"total":1,"used":0}}`), true); !errors.As(err, &se) || se.Field != "disk" {
		t.Fatalf("missing disk: %v", err)
	}
	if _, err := ParseStatus([]byte(`{"cpu":"x"}`), false); !errors.As(err, &se) || se.Field != "cpu" {
		t.Fatalf("bad cpu: %v", err)
	}
	if load, err := ParseStatus([]byte(`{"cpu":150}`), false); err != nil || load.CPU != 100 {
		t.Fatalf("lenient: %+v %v", load, err)
	}
	if _, err := ParseStatus([]byte(`[]`), false); !errors.Is(err, ErrBadReport) {
		t.Fatalf("array: %v", err)
	}
}

func TestParseAuditLog(t *testing.T) {
	got, err := ParseAuditLog([]byte(`[{"user_id":1,"audit_id":1000},{"user_id":0,"audit_id":1},{"user_id":2,"audit_id":-1},{"user_id":3,"audit_id":0}]`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []AuditEntry{{1, 1000}, {3, 0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestAuditRing(t *testing.T) {
	var r auditRing
	for i := range AuditRingSize + 5 {
		r.add(AuditHit{UserID: uint64(i)})
	}
	s := r.snapshot()
	if len(s) != AuditRingSize || s[0].UserID != AuditRingSize+4 || s[len(s)-1].UserID != 5 {
		t.Fatalf("len %d first %d last %d", len(s), s[0].UserID, s[len(s)-1].UserID)
	}
}

func TestTypeAliases(t *testing.T) {
	cases := []struct {
		raw, typ string
		hy2      bool
	}{
		{"V2ray", "vmess", false}, {" vless ", "vless", false}, {"hysteria2", "hysteria", true}, {"shadowsocks-plugin", "shadowsocks", false},
		{"ss", "shadowsocks", false}, {"ssr", "shadowsocksr", false}, {"v2node", "", false}, {"", "", false},
	}
	for _, tc := range cases {
		if typ, hy2 := CanonicalType(tc.raw); typ != tc.typ || hy2 != tc.hy2 {
			t.Errorf("%q → %q %v", tc.raw, typ, hy2)
		}
	}
	if _, _, err := UniProxyType("shadowsocksr"); err == nil {
		t.Error("UniProxy accepted shadowsocksr")
	}
	if _, _, err := SogaType("tuic"); err == nil {
		t.Error("soga accepted tuic")
	}
	if typ, _, err := SogaType("ssr"); err != nil || typ != "shadowsocksr" {
		t.Errorf("soga ssr: %q %v", typ, err)
	}
	v1 := &model.Node{ID: 9, Type: "hysteria", ProtocolSettings: model.JSONMap{"version": float64(1)}}
	if err := CheckType(v1, "hysteria", true); err == nil || !strings.Contains(err.Error(), "hysteria2") {
		t.Errorf("hysteria2 vs v1: %v", err)
	}
	if err := CheckSoga(v1, "hysteria", false); err == nil {
		t.Error("soga served hysteria v1")
	}
	if err := CheckUniProxy(&model.Node{Type: "shadowsocksr"}, "", false); err == nil {
		t.Error("UniProxy served shadowsocksr")
	}
}

func TestValidPortSpec(t *testing.T) {
	for _, ok := range []string{"443", "1", "65535", "20000-30000", "443,8443,10000-20000"} {
		if !ValidPortSpec(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "0", "65536", "30000-20000", "443-443", "a", "443,", "-1", "+443", "1-2-3", strings.Repeat("1,", 20) + "1"} {
		if ValidPortSpec(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidateRoute(t *testing.T) {
	ok := &model.NodeRoute{Action: "block", Match: model.JSONSlice[string]{" example.com ", "", "regexp:.*\\.example\\.net"}}
	NormalizeRoute(ok)
	if err := ValidateRoute(ok); err != nil || len(ok.Match) != 2 {
		t.Fatalf("valid route: %v %v", ok.Match, err)
	}
	fields := func(r *model.NodeRoute) []string {
		NormalizeRoute(r)
		var ve ValidationError
		if !errors.As(ValidateRoute(r), &ve) {
			return nil
		}
		var out []string
		for k := range ve {
			out = append(out, k)
		}
		return out
	}
	if f := fields(&model.NodeRoute{Action: "block", Match: model.JSONSlice[string]{"(unclosed"}}); !reflect.DeepEqual(f, []string{"match"}) {
		t.Errorf("bad regexp: %v", f)
	}
	big := make(model.JSONSlice[string], MaxRouteMatch+1)
	for i := range big {
		big[i] = "example.com"
	}
	if f := fields(&model.NodeRoute{Action: "block", Match: big}); !reflect.DeepEqual(f, []string{"match"}) {
		t.Errorf("too many: %v", f)
	}
	if f := fields(&model.NodeRoute{Action: "dns", Match: model.JSONSlice[string]{"example.com"}}); !reflect.DeepEqual(f, []string{"action_value"}) {
		t.Errorf("dns without server: %v", f)
	}
	if f := fields(&model.NodeRoute{Action: "dns", Match: model.JSONSlice[string]{"main", `{"servers":[]}`}}); f != nil {
		t.Errorf("dns main: %v", f)
	}
	if f := fields(&model.NodeRoute{Action: "allow", Match: model.JSONSlice[string]{"x"}}); !reflect.DeepEqual(f, []string{"action"}) {
		t.Errorf("bad action: %v", f)
	}
}

// testX25519 is an obviously fake base64url X25519 key (32 zero bytes).
var testX25519 = strings.Repeat("A", 43)

func TestValidateSettings(t *testing.T) {
	cases := []struct {
		typ, ps string
		fields  []string
	}{
		{"vmess", `{"network":"ws","tls":1}`, nil},
		{"vmess", `{"network":"quic"}`, []string{"protocol_settings.network"}},
		{"vmess", `{"tls":"yes"}`, []string{"protocol_settings"}},
		{"vless", `{"tls":2,"flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.example.org","private_key":"` + testX25519 + `","public_key":"` + testX25519 + `","short_id":"0123abcd","dest":"www.example.org:443"}}`, nil},
		{"vless", `{"tls":2,"tls_settings":{"short_id":"xyz","dest":"a:b:c"}}`, []string{"protocol_settings.tls_settings.dest", "protocol_settings.tls_settings.private_key", "protocol_settings.tls_settings.public_key", "protocol_settings.tls_settings.server_name", "protocol_settings.tls_settings.short_id"}},
		{"vless", `{"flow":"bogus","decryption":"aes.x"}`, []string{"protocol_settings.decryption", "protocol_settings.flow"}},
		{"vless", `{"decryption":"mlkem768x25519plus.native.600s.KEY"}`, nil},
		{"shadowsocks", `{"cipher":"2022-blake3-aes-128-gcm","server_key":"YjVkODdhNWRlMWEzNWIyMg=="}`, nil},
		{"shadowsocks", `{"cipher":"2022-blake3-aes-256-gcm","server_key":"YjVkODdhNWRlMWEzNWIyMg=="}`, []string{"protocol_settings.server_key"}},
		{"shadowsocks", `{"cipher":"rc4-md5","obfs":"weird"}`, []string{"protocol_settings.cipher", "protocol_settings.obfs"}},
		{"shadowsocksr", `{"cipher":"chacha20-ietf","protocol":"auth_chain_a","obfs":"http_simple","single_port_type":"obfs"}`, nil},
		{"shadowsocksr", `{"protocol":"","single_port_type":"x"}`, []string{"protocol_settings.protocol", "protocol_settings.single_port_type"}},
		{"trojan", `{"network":"xhttp"}`, []string{"protocol_settings.network"}},
		{"hysteria", `{"version":2,"obfs":"salamander","obfs_password":"p"}`, nil},
		{"hysteria", `{"version":3,"obfs":"salamander"}`, []string{"protocol_settings.obfs_password", "protocol_settings.version"}},
		{"anytls", `{"tls":2,"server_name":"a.example.com","tls_settings":{"private_key":"` + testX25519 + `","public_key":"` + testX25519 + `"}}`, nil},
		{"anytls", `{"tls":0}`, []string{"protocol_settings.tls"}},
		{"mieru", `{"transport":"QUIC","multiplexing":"MULTIPLEXING_LOW"}`, []string{"protocol_settings.transport"}},
		{"tuic", `{"congestion_control":"reno","udp_relay_mode":"quic"}`, []string{"protocol_settings.congestion_control"}},
	}
	for _, tc := range cases {
		errs := ValidationError{}
		ValidateSettings(tc.typ, ps(t, tc.ps), errs)
		var got []string
		for k := range errs {
			got = append(got, k)
		}
		slices.Sort(got)
		if !reflect.DeepEqual(got, tc.fields) {
			t.Errorf("%s %s: fields %v (%v), want %v", tc.typ, tc.ps, got, errs, tc.fields)
		}
	}
}
