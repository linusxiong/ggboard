package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestJSONSliceValueScan(t *testing.T) {
	var s JSONSlice[uint64]
	if v, _ := s.Value(); v != "[]" {
		t.Fatalf("nil value = %v", v)
	}
	if b, _ := json.Marshal(s); string(b) != "[]" {
		t.Fatalf("nil json = %s", b)
	}
	if err := s.Scan([]byte("[1,2,3]")); err != nil || len(s) != 3 || s[2] != 3 {
		t.Fatalf("scan bytes: %v %v", s, err)
	}
	if err := s.Scan("[4]"); err != nil || len(s) != 1 || s[0] != 4 {
		t.Fatalf("scan string: %v %v", s, err)
	}
	if err := s.Scan(nil); err != nil || s != nil {
		t.Fatalf("scan nil: %v %v", s, err)
	}
	if err := s.Scan(42); err == nil {
		t.Fatal("scan int must fail")
	}
	if !SliceContains(JSONSlice[string]{"a", "b"}, "b") || SliceContains(JSONSlice[string]{"a"}, "c") {
		t.Fatal("SliceContains")
	}
}

func TestJSONMapHelpers(t *testing.T) {
	var m JSONMap
	if v, _ := m.Value(); v != "{}" {
		t.Fatalf("nil value = %v", v)
	}
	if err := m.Scan(`{"s":"x","n":3,"ns":"7","b":true,"bn":1,"o":{"k":"v"}}`); err != nil {
		t.Fatal(err)
	}
	if m.String("s") != "x" || m.String("n") != "3" || m.Int("n") != 3 || m.Int("ns") != 7 {
		t.Fatalf("string/int helpers: %v", m)
	}
	if !m.Bool("b") || !m.Bool("bn") || m.Bool("missing") {
		t.Fatal("bool helper")
	}
	if m.Map("o").String("k") != "v" || m.Map("s") != nil {
		t.Fatal("map helper")
	}
	var dst struct {
		S string `json:"s"`
	}
	if err := m.Decode(&dst); err != nil || dst.S != "x" {
		t.Fatalf("decode: %+v %v", dst, err)
	}

	var o JSONObject[int64]
	if err := o.Scan(`{"monthly":1000}`); err != nil || o["monthly"] != 1000 {
		t.Fatalf("object: %v %v", o, err)
	}
	if b, _ := json.Marshal(JSONObject[int64](nil)); string(b) != "{}" {
		t.Fatalf("nil object json = %s", b)
	}

	var r RawJSON
	if v, _ := r.Value(); v != "null" {
		t.Fatalf("raw nil value = %v", v)
	}
	if err := r.Scan(`{"a":1}`); err != nil || string(r) != `{"a":1}` {
		t.Fatalf("raw scan: %s %v", r, err)
	}
	if _, err := RawJSON("{bad").Value(); err == nil {
		t.Fatal("invalid raw JSON must fail")
	}
}

func TestProtocolDecodeDefaults(t *testing.T) {
	var v VlessSettings
	if err := v.Decode(JSONMap{"tls": 2.0, "flow": "xtls-rprx-vision", "decryption": "mlkem768x25519plus", "encryption": nil,
		"tls_settings": map[string]any{"server_name": "a.example.com", "short_id": "ab"}}); err != nil {
		t.Fatal(err)
	}
	if v.Network != "tcp" || v.Encryption != "none" || v.TLS != TLSReality || v.TLSSettings.ShortID != "ab" || v.Decryption == "" {
		t.Fatalf("vless = %+v", v)
	}
	h, err := DecodeSettings[HysteriaSettings](JSONMap{"version": 1.0, "ignore_client_bandwidth": true})
	if err != nil || h.Version != 1 || h.UpMbps != 100 || h.ALPN != "h3" || !h.IgnoreClientBandwidth {
		t.Fatalf("hysteria = %+v %v", h, err)
	}
	ssr, _ := DecodeSettings[ShadowsocksRSettings](JSONMap{"password": "pw"})
	if ssr.Password != "pw" || ssr.Protocol != "origin" {
		t.Fatalf("ssr = %+v", ssr)
	}
	if _, err := DecodeSettings[TuicSettings](JSONMap{"alpn": 5.0}); err == nil {
		t.Fatal("type mismatch must fail")
	}
	for _, typ := range NodeTypes() {
		if NewSettingsFor(typ) == nil {
			t.Errorf("NewSettingsFor(%s) = nil", typ)
		}
	}
	if NewSettingsFor("bogus") != nil || IsValidNodeType("bogus") || !IsValidNodeType(NodeMieru) {
		t.Fatal("unknown node type handling")
	}
}

func TestDecodeNetwork(t *testing.T) {
	ws := DecodeNetwork("ws", JSONMap{"path": "/ws", "headers": map[string]any{"Host": "cdn.example.com"}})
	if ws.Path != "/ws" || ws.Host != "cdn.example.com" {
		t.Fatalf("ws = %+v", ws)
	}
	grpc := DecodeNetwork("grpc", JSONMap{"serviceName": "svc"})
	if grpc.ServiceName != "svc" || grpc.Path != "" {
		t.Fatalf("grpc = %+v", grpc)
	}
	h2 := DecodeNetwork("h2", JSONMap{"host": []any{"a.example.com", "b.example.com"}})
	if h2.Host != "a.example.com" || len(h2.Hosts) != 2 || h2.Path != "/" {
		t.Fatalf("h2 = %+v", h2)
	}
	tcp := DecodeNetwork("tcp", JSONMap{"header": map[string]any{"type": "http", "request": map[string]any{"path": []any{"/p"}, "headers": map[string]any{"Host": []any{"h.example.com"}}}}})
	if tcp.HeaderType != "http" || tcp.Path != "/p" || tcp.Host != "h.example.com" {
		t.Fatalf("tcp = %+v", tcp)
	}
}

func TestCredentials(t *testing.T) {
	const id = "5f2c7f1e-8a7b-4c1d-9e2f-0123456789ab"
	created := time.Unix(1735689600, 0)
	u := &User{UUID: id}

	ss128 := &Node{Type: NodeShadowsocks, CreatedAt: created, ProtocolSettings: JSONMap{"cipher": "2022-blake3-aes-128-gcm"}}
	if got := NodeServerKey(ss128); got != "YjVkODdhNWRlMWEzNWIyMg==" {
		t.Fatalf("server key 16 = %s", got)
	}
	ss256 := &Node{Type: NodeShadowsocks, CreatedAt: created, ProtocolSettings: JSONMap{"cipher": "2022-blake3-aes-256-gcm"}}
	if got := NodeServerKey(ss256); got != "YjVkODdhNWRlMWEzNWIyMmQwMjM0NThjNDYyMGVjYWE=" {
		t.Fatalf("server key 32 = %s", got)
	}
	c := UserCredentials(u, ss128)
	if c.SS2022Key != SS2022UserKey(id, "2022-blake3-aes-128-gcm") || c.SS2022Key != "NWYyYzdmMWUtOGE3Yi00Yw==" {
		t.Fatalf("user key = %s", c.SS2022Key)
	}
	if c.Password != "YjVkODdhNWRlMWEzNWIyMg==:NWYyYzdmMWUtOGE3Yi00Yw==" {
		t.Fatalf("ss2022 password = %s", c.Password)
	}
	if k := SS2022UserKey(id, "2022-blake3-chacha20-poly1305"); len(k) != 44 {
		t.Fatalf("32-byte user key = %s", k)
	}

	custom := &Node{Type: NodeShadowsocks, ProtocolSettings: JSONMap{"cipher": "2022-blake3-aes-128-gcm", "server_key": "c2VydmVyLWtleQ=="}}
	if NodeServerKey(custom) != "c2VydmVyLWtleQ==" {
		t.Fatal("explicit server_key must win")
	}
	parent := &Node{ID: 1, Type: NodeShadowsocks, CreatedAt: created}
	child := &Node{Type: NodeShadowsocks, ParentID: &parent.ID, Parent: parent, CreatedAt: created.Add(time.Hour),
		ProtocolSettings: JSONMap{"cipher": "2022-blake3-aes-128-gcm"}}
	if NodeServerKey(child) != "YjVkODdhNWRlMWEzNWIyMg==" {
		t.Fatal("child must use the parent's created_at")
	}

	plain := &Node{Type: NodeShadowsocks, ProtocolSettings: JSONMap{"cipher": "aes-128-gcm"}}
	if c := UserCredentials(u, plain); c.Password != id || c.SS2022Key != "" || NodeServerKey(plain) != "" {
		t.Fatalf("plain ss = %+v", c)
	}
	if c := UserCredentials(u, &Node{Type: NodeMieru}); c.MieruUser != id || c.MieruPass != id {
		t.Fatalf("mieru = %+v", c)
	}
	for _, typ := range []string{NodeVmess, NodeVless, NodeTrojan, NodeHysteria, NodeAnyTLS, NodeTuic, NodeShadowsocksR} {
		if c := UserCredentials(u, &Node{Type: typ}); c.UUID != id || c.Password != id {
			t.Fatalf("%s = %+v", typ, c)
		}
	}
}

func TestUserAvailability(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Second), now.Add(time.Hour)
	cases := []struct {
		name string
		u    User
		want bool
	}{
		{"ok", User{TransferEnable: 100, U: 10, D: 10, ExpiredAt: &future}, true},
		{"never expires", User{TransferEnable: 100}, true},
		{"banned", User{TransferEnable: 100, Banned: true}, false},
		{"expired", User{TransferEnable: 100, ExpiredAt: &past}, false},
		{"expires now", User{TransferEnable: 100, ExpiredAt: &now}, false},
		{"traffic used up", User{TransferEnable: 100, U: 60, D: 40}, false},
		{"no traffic", User{}, false},
	}
	for _, tc := range cases {
		if got := tc.u.IsAvailable(now); got != tc.want {
			t.Errorf("%s: IsAvailable = %v, want %v", tc.name, got, tc.want)
		}
	}
	u := User{TransferEnable: 100, U: 70, D: 50}
	if u.Used() != 120 || u.Remaining() != 0 {
		t.Fatal("Used/Remaining")
	}
}

func TestEnumsAndHelpers(t *testing.T) {
	if PeriodMonths(PeriodYearly) != 12 || PeriodMonths(PeriodOnetime) != 0 || !IsValidPeriod(PeriodResetTraffic) || IsValidPeriod("weekly") {
		t.Fatal("periods")
	}
	p := Plan{TransferEnable: 2, Prices: JSONObject[int64]{PeriodMonthly: 1000}}
	if p.TransferBytes() != 2<<30 {
		t.Fatal("TransferBytes")
	}
	if v, ok := p.Price(PeriodMonthly); !ok || v != 1000 {
		t.Fatal("Price")
	}
	if _, ok := p.Price(PeriodYearly); ok {
		t.Fatal("absent price must not be for sale")
	}
	loc := time.FixedZone("UTC+8", 8*3600)
	d := Day(time.Date(2026, 10, 22, 1, 0, 0, 0, loc))
	if !d.Equal(time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Day = %v", d)
	}
	n := Node{GroupIDs: JSONSlice[uint64]{2, 3}}
	if !n.InGroup(3) || n.InGroup(1) || !n.InAnyGroup(9, 2) {
		t.Fatal("node groups")
	}
	if len(All()) != 20 {
		t.Fatalf("All() = %d models", len(All()))
	}
}
