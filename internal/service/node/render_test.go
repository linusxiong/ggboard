package nodesvc

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// created1735689600 is the created_at of the doc examples (md5("1735689600") = b5d87a5d...).
var created1735689600 = time.Unix(1735689600, 0).UTC()

var docIntervals = Intervals{Push: 60, Pull: 60}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func ps(t *testing.T, s string) model.JSONMap {
	t.Helper()
	var m model.JSONMap
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("protocol settings %q: %v", s, err)
	}
	return m
}

// jsonEq compares got (marshaled) with the JSON document want semantically.
func jsonEq(t *testing.T, got any, want string) {
	t.Helper()
	gb := mustJSON(t, got)
	var g, w any
	if err := json.Unmarshal(gb, &g); err != nil {
		t.Fatalf("decode got: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("decode want: %v\n%s", err, want)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", gb, compact(t, want))
	}
}

func compact(t *testing.T, s string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		t.Fatalf("compact: %v", err)
	}
	return buf.String()
}

func TestRenderUniProxyConfigDocExamples(t *testing.T) {
	cases := []struct {
		name string
		node model.Node
		want string
	}{
		{"shadowsocks obfs", model.Node{Type: "shadowsocks", Host: "ss.example.com", ServerPort: 8388,
			ProtocolSettings: ps(t, `{"cipher":"aes-128-gcm","obfs":"http","obfs_settings":{"path":"/","host":"www.example.net"}}`)},
			`{"protocol":"shadowsocks","listen_ip":"0.0.0.0","server_port":8388,"host":"ss.example.com","network":null,"networkSettings":null,"network_settings":null,
			 "cipher":"aes-128-gcm","server_key":null,"obfs":"http","obfs_settings":{"path":"/","host":"www.example.net"},"plugin":null,"plugin_opts":null,
			 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
		{"shadowsocks 2022", model.Node{Type: "shadowsocks", Host: "ss.example.com", ServerPort: 8389, CreatedAt: created1735689600,
			ProtocolSettings: ps(t, `{"cipher":"2022-blake3-aes-256-gcm"}`)},
			`{"protocol":"shadowsocks","listen_ip":"0.0.0.0","server_port":8389,"host":"ss.example.com","network":null,"networkSettings":null,"network_settings":null,
			 "cipher":"2022-blake3-aes-256-gcm","server_key":"YjVkODdhNWRlMWEzNWIyMmQwMjM0NThjNDYyMGVjYWE=","obfs":null,"obfs_settings":null,"plugin":null,"plugin_opts":null,
			 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
		{"vmess ws tls", model.Node{Type: "vmess", Host: "v.example.com", ServerPort: 10086,
			ProtocolSettings: ps(t, `{"network":"ws","network_settings":{"path":"/ws","headers":{"Host":"v.example.com"}},"tls":1,"tls_settings":{"server_name":"v.example.com","allow_insecure":false}}`)},
			`{"protocol":"vmess","listen_ip":"0.0.0.0","server_port":10086,"host":"v.example.com","network":"ws",
			 "networkSettings":{"path":"/ws","headers":{"Host":"v.example.com"}},"network_settings":{"path":"/ws","headers":{"Host":"v.example.com"}},
			 "tls":1,"tls_settings":{"server_name":"v.example.com","allow_insecure":false},"tlsSettings":{"server_name":"v.example.com","allow_insecure":false},
			 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
		{"vless tls vision", model.Node{Type: "vless", Host: "vl.example.com", ServerPort: 443,
			ProtocolSettings: ps(t, `{"network":"tcp","tls":1,"flow":"xtls-rprx-vision","encryption":"none","tls_settings":{"server_name":"vl.example.com"}}`)},
			`{"protocol":"vless","listen_ip":"0.0.0.0","server_port":443,"host":"vl.example.com","network":"tcp","networkSettings":null,"network_settings":null,
			 "tls":1,"flow":"xtls-rprx-vision","server_name":"vl.example.com","tls_settings":{"server_name":"vl.example.com","allow_insecure":false},
			 "tlsSettings":{"server_name":"vl.example.com","allow_insecure":false},"decryption":null,
			 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
		{"vless xhttp encryption", model.Node{Type: "vless", Host: "vl.example.com", ServerPort: 8080,
			ProtocolSettings: ps(t, `{"network":"xhttp","network_settings":{"path":"/xh","host":"vl.example.com","mode":"auto","extra":{}},"tls":0,"decryption":"mlkem768x25519plus.native.600s.PRIVATE_KEY_BASE64URL"}`)},
			`{"protocol":"vless","listen_ip":"0.0.0.0","server_port":8080,"host":"vl.example.com","network":"xhttp",
			 "networkSettings":{"path":"/xh","host":"vl.example.com","mode":"auto","extra":{}},"network_settings":{"path":"/xh","host":"vl.example.com","mode":"auto","extra":{}},
			 "tls":0,"flow":"","server_name":"","tls_settings":{"server_name":"","allow_insecure":false},"tlsSettings":{"server_name":"","allow_insecure":false},
			 "decryption":"mlkem768x25519plus.native.600s.PRIVATE_KEY_BASE64URL","encryption":"mlkem768x25519plus",
			 "encryption_settings":{"mode":"native","ticket":"600s","server_padding":"","private_key":"PRIVATE_KEY_BASE64URL"},
			 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
		{"trojan grpc", model.Node{Type: "trojan", Host: "t.example.com", ServerPort: 443,
			ProtocolSettings: ps(t, `{"network":"grpc","network_settings":{"serviceName":"trojan-grpc"},"server_name":"t.example.com"}`)},
			`{"protocol":"trojan","listen_ip":"0.0.0.0","server_port":443,"host":"t.example.com","network":"grpc",
			 "networkSettings":{"serviceName":"trojan-grpc"},"network_settings":{"serviceName":"trojan-grpc"},"server_name":"t.example.com",
			 "tls":1,"tls_settings":{"server_name":"t.example.com","allow_insecure":false},"base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
		{"hysteria v1", model.Node{Type: "hysteria", Host: "hy.example.com", ServerPort: 36712,
			ProtocolSettings: ps(t, `{"version":1,"server_name":"hy.example.com","up_mbps":200,"down_mbps":1000,"obfs_password":"obfs-pass-123"}`)},
			`{"protocol":"hysteria","listen_ip":"0.0.0.0","server_port":36712,"host":"hy.example.com","network":null,"networkSettings":null,"network_settings":null,
			 "version":1,"server_name":"hy.example.com","tls_settings":{"server_name":"hy.example.com","allow_insecure":false},"up_mbps":200,"down_mbps":1000,
			 "obfs":"obfs-pass-123","base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
		{"hysteria v2", model.Node{Type: "hysteria", Host: "hy2.example.com", ServerPort: 36712,
			ProtocolSettings: ps(t, `{"version":2,"server_name":"hy2.example.com","obfs":"salamander","obfs_password":"obfs-pass-123"}`)},
			`{"protocol":"hysteria","listen_ip":"0.0.0.0","server_port":36712,"host":"hy2.example.com","network":null,"networkSettings":null,"network_settings":null,
			 "version":2,"server_name":"hy2.example.com","tls_settings":{"server_name":"hy2.example.com","allow_insecure":false},"up_mbps":0,"down_mbps":0,
			 "obfs":"salamander","obfs-password":"obfs-pass-123","ignore_client_bandwidth":false,"base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
		{"tuic", model.Node{Type: "tuic", Host: "tu.example.com", ServerPort: 8443,
			ProtocolSettings: ps(t, `{"server_name":"tu.example.com","congestion_control":"bbr"}`)},
			`{"protocol":"tuic","listen_ip":"0.0.0.0","server_port":8443,"host":"tu.example.com","network":null,"networkSettings":null,"network_settings":null,
			 "version":5,"server_name":"tu.example.com","congestion_control":"bbr","zero_rtt_handshake":false,"udp_relay_mode":"native","alpn":["h3"],
			 "auth_timeout":"3s","heartbeat":"3s","tls_settings":{"server_name":"tu.example.com","allow_insecure":false},
			 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
		{"anytls", model.Node{Type: "anytls", Host: "a.example.com", ServerPort: 8443,
			ProtocolSettings: ps(t, `{"tls":1,"server_name":"a.example.com"}`)},
			`{"protocol":"anytls","listen_ip":"0.0.0.0","server_port":8443,"host":"a.example.com","network":null,"networkSettings":null,"network_settings":null,
			 "tls":1,"server_name":"a.example.com","tls_settings":{"server_name":"a.example.com","allow_insecure":false},
			 "padding_scheme":["stop=8","0=30-30","1=100-400","2=400-500,c,500-1000,c,500-1000,c,500-1000,c,500-1000","3=9-9,500-1000","4=500-1000","5=500-1000","6=500-1000","7=500-1000"],
			 "base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
		{"mieru", model.Node{Type: "mieru", Host: "m.example.com", ServerPort: 2999,
			ProtocolSettings: ps(t, `{"transport":"TCP","multiplexing":"MULTIPLEXING_LOW"}`)},
			`{"protocol":"mieru","listen_ip":"0.0.0.0","server_port":2999,"host":"m.example.com","network":null,"networkSettings":null,"network_settings":null,
			 "transport":"TCP","traffic_pattern":"","multiplexing":"MULTIPLEXING_LOW","base_config":{"push_interval":60,"pull_interval":60},"routes":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jsonEq(t, RenderUniProxyConfig(&tc.node, nil, docIntervals), tc.want)
		})
	}
}

// §7.1: exact bytes and ETag of the vless + reality worked example.
func TestRenderUniProxyVlessRealityWorkedExample(t *testing.T) {
	n := model.Node{ID: 3, Type: "vless", Host: "hk1.example.com", ServerPort: 443, RouteIDs: model.JSONSlice[uint64]{1},
		ProtocolSettings: ps(t, `{"network":"tcp","tls":2,"flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.example.org","dest":"www.example.org:443","server_port":"443","public_key":"PUBLIC_KEY_BASE64URL","private_key":"PRIVATE_KEY_BASE64URL","short_id":"0123abcd"}}`)}
	routes := []model.NodeRoute{{ID: 1, Match: model.JSONSlice[string]{"example-blocked.com"}, Action: "block"}}
	b, err := NewBody(RenderUniProxyConfig(&n, routes, docIntervals))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"protocol":"vless","listen_ip":"0.0.0.0","server_port":443,"host":"hk1.example.com","network":"tcp","networkSettings":null,"network_settings":null,` +
		`"tls":2,"flow":"xtls-rprx-vision","server_name":"www.example.org",` +
		`"tls_settings":{"server_name":"www.example.org","dest":"www.example.org","server_port":"443","public_key":"PUBLIC_KEY_BASE64URL","private_key":"PRIVATE_KEY_BASE64URL","short_id":"0123abcd","allow_insecure":false},` +
		`"tlsSettings":{"server_name":"www.example.org","dest":"www.example.org","server_port":"443","public_key":"PUBLIC_KEY_BASE64URL","private_key":"PRIVATE_KEY_BASE64URL","short_id":"0123abcd","allow_insecure":false},` +
		`"decryption":null,"base_config":{"push_interval":60,"pull_interval":60},` +
		`"routes":[{"id":1,"match":["example-blocked.com"],"action":"block","action_value":""}]}`
	if string(b.JSON) != want {
		t.Fatalf("body\n got: %s\nwant: %s", b.JSON, want)
	}
	if b.ETag != "2bb00202abbadb48db07c406a2a1851180f3a03d" {
		t.Fatalf("etag = %s", b.ETag)
	}
}

func TestRealityDestFallbacksAndXver(t *testing.T) {
	cases := []struct {
		ts         string
		host, port string
	}{
		{`{"server_name":"sni.example.org"}`, "sni.example.org", "443"},
		{`{"server_name":"sni.example.org","dest":"d.example.org","server_port":"8443"}`, "d.example.org", "8443"},
		{`{"server_name":"sni.example.org","dest":"d.example.org:9443","server_port":"8443"}`, "d.example.org", "9443"},
		{`{"server_name":"sni.example.org","dest":"[2001:db8::1]:443"}`, "2001:db8::1", "443"},
		{`{"server_name":"sni.example.org","server_port":443}`, "sni.example.org", "443"},
	}
	for _, tc := range cases {
		o := reality(ps(t, tc.ts), "sni.example.org")
		if h, _ := o.Get("dest"); h != tc.host {
			t.Errorf("%s: dest = %v, want %s", tc.ts, h, tc.host)
		}
		if p, _ := o.Get("server_port"); p != tc.port {
			t.Errorf("%s: server_port = %v, want %s", tc.ts, p, tc.port)
		}
		if o.Has("xver") {
			t.Errorf("%s: xver must not be emitted", tc.ts)
		}
	}
	o := reality(ps(t, `{"xver":1}`), "x.example.org")
	if v, _ := o.Get("xver"); v != "1" {
		t.Fatalf("xver = %#v, want string \"1\"", v)
	}
}

func TestRenderUniProxyNeverSendsEncryptionNone(t *testing.T) {
	for _, dec := range []string{"", "none"} {
		n := model.Node{Type: "vless", Host: "h.example.com", ServerPort: 443, ProtocolSettings: model.JSONMap{"encryption": "none", "decryption": dec}}
		o := RenderUniProxyConfig(&n, nil, docIntervals)
		if o.Has("encryption") || o.Has("encryption_settings") {
			t.Fatalf("decryption %q: encryption emitted: %s", dec, mustJSON(t, o))
		}
		if v, _ := o.Get("decryption"); v != nil {
			t.Fatalf("decryption %q: got %v, want null", dec, v)
		}
	}
	n := model.Node{Type: "vless", ProtocolSettings: model.JSONMap{"decryption": "mlkem768x25519plus.xorpub.0rtt.100-111-1111.75-0-111.50-0-3333.KEY"}}
	es, _ := RenderUniProxyConfig(&n, nil, docIntervals).Get("encryption_settings")
	jsonEq(t, es, `{"mode":"xorpub","ticket":"0rtt","server_padding":"100-111-1111.75-0-111.50-0-3333","private_key":"KEY"}`)
}

func TestRenderUniProxyAnyTLSReality(t *testing.T) {
	n := model.Node{Type: "anytls", Host: "a.example.com", ServerPort: 443,
		ProtocolSettings: ps(t, `{"tls":2,"server_name":"fallback.example.org","padding_scheme":["stop=1"],"tls_settings":{"server_names":["r.example.org"],"private_key":"PK","public_key":"PUB","short_id":"ab","dest":"r.example.org:443"}}`)}
	o := RenderUniProxyConfig(&n, nil, docIntervals)
	ts, _ := o.Get("tls_settings")
	jsonEq(t, ts, `{"server_name":"r.example.org","dest":"r.example.org","server_port":"443","public_key":"PUB","private_key":"PK","short_id":"ab","allow_insecure":false}`)
	pad, _ := o.Get("padding_scheme")
	jsonEq(t, pad, `["stop=1"]`)
}

func TestRenderSogaNodeDocExamples(t *testing.T) {
	cases := []struct {
		name string
		node model.Node
		want string
	}{
		{"vmess", model.Node{Type: "vmess", ServerPort: 10086, ProtocolSettings: ps(t, `{"network":"ws","network_settings":{"path":"/ws","headers":{"Host":"v.example.com"}},"tls":1}`)},
			`{"port":10086,"stream_type":"ws","tls_type":"tls","path":"/ws"}`},
		{"trojan", model.Node{Type: "trojan", ServerPort: 443, ProtocolSettings: ps(t, `{"network":"grpc","network_settings":{"serviceName":"trojan-grpc"},"server_name":"t.example.com"}`)},
			`{"port":443,"stream_type":"grpc","service_name":"trojan-grpc"}`},
		{"ss obfs", model.Node{Type: "shadowsocks", ServerPort: 8388, ProtocolSettings: ps(t, `{"cipher":"aes-128-gcm","obfs":"http","obfs_settings":{"path":"/","host":"www.example.net"}}`)},
			`{"port":8388,"cipher":"aes-128-gcm","password":"","obfs":"simple_obfs_http","path":"/","host":"www.example.net"}`},
		{"ssr", model.Node{Type: "shadowsocksr", ServerPort: 443, CreatedAt: created1735689600, ProtocolSettings: ps(t, `{"cipher":"chacha20-ietf","protocol":"auth_chain_a","obfs":"plain"}`)},
			`{"port":443,"method":"chacha20-ietf","password":"YjVkODdhNWRlMWEzNWIyMg==","protocol":"auth_chain_a","obfs":"plain","single_port_type":"protocol"}`},
		{"vless reality", model.Node{Type: "vless", ServerPort: 443, ProtocolSettings: ps(t, `{"network":"tcp","tls":2,"flow":"xtls-rprx-vision","tls_settings":{"server_name":"www.example.org","dest":"www.example.org:443","private_key":"PRIVATE_KEY_BASE64URL","short_id":"0123abcd"}}`)},
			`{"port":443,"stream_type":"tcp","tls_type":"reality","flow":"xtls-rprx-vision","decryption":"","server_names":["www.example.org"],
			  "private_key":"PRIVATE_KEY_BASE64URL","short_ids":["0123abcd"],"dest":"www.example.org:443"}`},
		{"vless xhttp", model.Node{Type: "vless", ServerPort: 443, ProtocolSettings: ps(t, `{"network":"xhttp","network_settings":{"host":"x.example.com","path":"/xh"},"tls":1}`)},
			`{"port":443,"stream_type":"xhttp","tls_type":"tls","flow":"","decryption":"","xhttp_settings":{"host":"x.example.com","path":"/xh","mode":"auto","extra":{}}}`},
		{"hysteria", model.Node{Type: "hysteria", ServerPort: 36712, ProtocolSettings: ps(t, `{"version":2,"obfs":"salamander","obfs_password":"obfs-pass-123"}`)},
			`{"port":36712,"obfs":"salamander","obfs_password":"obfs-pass-123","up_mbps":0,"down_mbps":0}`},
		{"hysteria plain", model.Node{Type: "hysteria", ServerPort: 36712, ProtocolSettings: ps(t, `{"version":2,"obfs":"salamander","up_mbps":100,"down_mbps":500}`)},
			`{"port":36712,"obfs":"plain","obfs_password":"","up_mbps":100,"down_mbps":500}`},
		{"anytls", model.Node{Type: "anytls", ServerPort: 8443, ProtocolSettings: ps(t, `{"tls":1,"server_name":"a.example.com"}`)},
			`{"port":8443,"tls_type":"tls","padding_scheme":[]}`},
		{"anytls reality", model.Node{Type: "anytls", ServerPort: 8443, ProtocolSettings: ps(t, `{"tls":2,"server_name":"a.example.com","tls_settings":{"private_key":"PK","short_id":"ab","dest":"r.example.org"}}`)},
			`{"port":8443,"tls_type":"reality","server_names":["a.example.com"],"private_key":"PK","short_ids":["ab"],"dest":"r.example.org:443","padding_scheme":[]}`},
		{"mieru", model.Node{Type: "mieru", ServerPort: 2999, ProtocolSettings: ps(t, `{}`)},
			`{"port":2999,"transport":"TCP"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderSogaNode(&tc.node, docIntervals)
			basic, _ := got.Get("basic")
			jsonEq(t, basic, `{"pull_interval":60,"push_interval":60,"speed_limit":0}`)
			cfg, _ := got.Get("config")
			jsonEq(t, cfg, tc.want)
		})
	}
}

// §7.2: exact bytes and ETags of the soga shadowsocks-2022 worked example (node body and user list).
func TestSogaSS2022WorkedExample(t *testing.T) {
	n := model.Node{ID: 7, Type: "shadowsocks", ServerPort: 8388, CreatedAt: created1735689600, ProtocolSettings: ps(t, `{"cipher":"2022-blake3-aes-128-gcm","obfs":"plain"}`)}
	b, err := NewBody(RenderSogaNode(&n, docIntervals))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"basic":{"pull_interval":60,"push_interval":60,"speed_limit":0},"config":{"port":8388,"cipher":"2022-blake3-aes-128-gcm","password":"YjVkODdhNWRlMWEzNWIyMg==","obfs":"plain"}}`
	if string(b.JSON) != want || b.ETag != "f8d278af3b4f513c6ab09ac8025dbc61e9e60e0c" {
		t.Fatalf("node body %s etag %s", b.JSON, b.ETag)
	}
	users := []usersvc.NodeUser{
		{ID: 1, UUID: "3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f", DeviceLimit: 3},
		{ID: 2, UUID: "9d4c2e1a-0b3f-4a5c-8e7d-6f1a2b3c4d5e", SpeedLimit: 100, ConnLimit: 64},
	}
	ub, err := NewBody(SogaUserList(&n, users))
	if err != nil {
		t.Fatal(err)
	}
	wantUsers := `[{"id":1,"password":"M2IxYjZmNWUtN2MyYS00ZA==","speed_limit":0,"device_limit":3,"tcp_limit":0},{"id":2,"password":"OWQ0YzJlMWEtMGIzZi00YQ==","speed_limit":100,"device_limit":0,"tcp_limit":64}]`
	if string(ub.JSON) != wantUsers || ub.ETag != "67a29b192abd3479b4387418891f742f66e9bca0" {
		t.Fatalf("users body %s etag %s", ub.JSON, ub.ETag)
	}
}

func TestSogaUserShapes(t *testing.T) {
	users := []usersvc.NodeUser{{ID: 1, UUID: "3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f", DeviceLimit: 3}}
	const id = "3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f"
	cases := map[string]string{
		"vmess":        `[{"id":1,"uuid":"` + id + `","speed_limit":0,"device_limit":3,"tcp_limit":0}]`,
		"vless":        `[{"id":1,"uuid":"` + id + `","speed_limit":0,"device_limit":3,"tcp_limit":0}]`,
		"trojan":       `[{"id":1,"password":"` + id + `","speed_limit":0,"device_limit":3,"tcp_limit":0}]`,
		"hysteria":     `[{"id":1,"password":"` + id + `","speed_limit":0,"device_limit":3,"tcp_limit":0}]`,
		"anytls":       `[{"id":1,"password":"` + id + `","speed_limit":0,"device_limit":3,"tcp_limit":0}]`,
		"shadowsocksr": `[{"id":1,"password":"` + id + `","speed_limit":0,"device_limit":3,"tcp_limit":0}]`,
		"shadowsocks":  `[{"id":1,"password":"` + id + `","speed_limit":0,"device_limit":3,"tcp_limit":0}]`,
		"mieru":        `[{"id":1,"username":"` + id + `","password":"` + id + `","speed_limit":0,"device_limit":3,"tcp_limit":0}]`,
	}
	for typ, want := range cases {
		n := model.Node{Type: typ, ProtocolSettings: model.JSONMap{"cipher": "aes-256-gcm"}}
		if got := string(mustJSON(t, SogaUserList(&n, users))); got != want {
			t.Errorf("%s:\n got %s\nwant %s", typ, got, want)
		}
	}
	n := model.Node{Type: "shadowsocks", ProtocolSettings: model.JSONMap{"cipher": "2022-blake3-aes-256-gcm"}}
	got := string(mustJSON(t, SogaUserList(&n, users)))
	if want := `[{"id":1,"password":"M2IxYjZmNWUtN2MyYS00ZDhlLTlmMTAtMmEzYjRjNWQ=","speed_limit":0,"device_limit":3,"tcp_limit":0}]`; got != want {
		t.Fatalf("ss2022-256:\n got %s\nwant %s", got, want)
	}
}

func TestRenderAuditRules(t *testing.T) {
	routes := []model.NodeRoute{
		{ID: 1, Action: "block", Match: model.JSONSlice[string]{"example-blocked.com", "protocol:bittorrent", "regexp:.*\\.example\\.net"}},
		{ID: 2, Action: "dns", ActionValue: "198.51.100.53", Match: model.JSONSlice[string]{"example.org"}},
		{ID: 3, Action: "block", Match: model.JSONSlice[string]{"domain:example.com"}},
	}
	jsonEq(t, RenderAuditRules(routes), `[{"id":1000,"rule":"example-blocked.com"},{"id":1002,"rule":"regexp:.*\\.example\\.net"},{"id":3000,"rule":"domain:example.com"}]`)
	jsonEq(t, RenderAuditRules(nil), `[]`)
}

func TestObjectMarshalKeepsOrder(t *testing.T) {
	o := Object{{"b", 1}, {"a", Object{{"z", nil}, {"y", []string{"x"}}}}}
	if got := string(mustJSON(t, o)); got != `{"b":1,"a":{"z":null,"y":["x"]}}` {
		t.Fatalf("got %s", got)
	}
	if got := string(mustJSON(t, Object{})); got != `{}` {
		t.Fatalf("empty: %s", got)
	}
}
