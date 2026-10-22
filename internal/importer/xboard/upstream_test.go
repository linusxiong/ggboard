package xboardimport

import (
	"reflect"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
)

func TestUpstreamCounts(t *testing.T) {
	a, rep := mustImport(t, FlavorUpstream)
	if rep.Flavor != FlavorUpstream || rep.Migrations != 43 || rep.LastMigration != "2026_05_30_000000_rename_refund_amount_to_surplus_credit" {
		t.Fatalf("detection: %s %d %q", rep.Flavor, rep.Migrations, rep.LastMigration)
	}
	want := map[string][3]int{
		"server_groups": {2, 2, 0}, "plans": {2, 2, 0}, "node_routes": {2, 1, 1}, "nodes": {13, 12, 1}, "users": {2, 2, 0},
		"payments": {2, 2, 0}, "tickets": {2, 2, 0}, "ticket_messages": {2, 2, 0}, "notices": {2, 2, 0}, "knowledge": {1, 1, 0},
		"coupons": {1, 1, 0}, "invite_codes": {1, 1, 0}, "commission_logs": {1, 1, 0},
	}
	for name, w := range want {
		st := rep.Lookup(name)
		if st == nil || [3]int{st.Source, st.Imported, st.Skipped} != w {
			t.Errorf("%s summary %+v, want %v", name, st, w)
		}
	}
	if count(t, a, &model.Node{}) != 12 {
		t.Error("node count")
	}
	skipped := map[string]int64{}
	for _, s := range rep.SkippedTables {
		skipped[s.Table] = s.Rows
	}
	if skipped["v2_gift_card_template"] != 1 || skipped["v2_server_machine"] != 1 || skipped["v2_plugins"] != 2 {
		t.Errorf("skipped tables: %+v", rep.SkippedTables)
	}
}

// §9 assertions 3 and 4 (upstream): code warning, socks skipped, enabled=0 hidden, keys, protocol_settings mapping.
func TestUpstreamNodes(t *testing.T) {
	a, rep := mustImport(t, FlavorUpstream)
	var none model.Node
	if err := a.DB.First(&none, 10).Error; err == nil {
		t.Error("socks node must be skipped")
	}
	if !hasWarning(rep, "nodes#10", `"socks"`) || !hasWarning(rep, "nodes#2", "backend may use NodeID 5; ggboard resolves by id → set NodeID=2") {
		t.Error("socks / code warnings missing")
	}
	if len(rep.CodeNodes) != 1 {
		t.Errorf("code nodes: %v", rep.CodeNodes)
	}

	// shadowsocks 2022-256: K32 of created_at (2023-11-15 06:13:20 Asia/Shanghai = 1700000000); child uses the parent's
	ss := loadNode(t, a, 1)
	k32 := "MjQ5MjBkZWNmODNmNjk2MGI4MGY3MzdhMjhlZWFmYzE="
	if ss.ProtocolSettings.String("server_key") != k32 || ss.ProtocolSettings.String("obfs") != "plain" || ss.CreatedAt.Unix() != 1700000000 {
		t.Errorf("ss: %v created %v", ss.ProtocolSettings, ss.CreatedAt)
	}
	child := loadNode(t, a, 12)
	if child.ParentID == nil || *child.ParentID != 1 || child.ProtocolSettings.String("server_key") != k32 {
		t.Errorf("ss child: parent %v key %q", child.ParentID, child.ProtocolSettings.String("server_key"))
	}
	if !reflect.DeepEqual([]uint64(ss.RouteIDs), []uint64{1}) {
		t.Errorf("route_ids must drop the skipped direct route: %v", ss.RouteIDs)
	}

	vm := loadNode(t, a, 2)
	var vs model.VmessSettings
	_ = vs.Decode(vm.ProtocolSettings)
	if vs.Network != "ws" || vs.TLS != 1 || vs.TLSSettings.ServerName != "vm.example.com" || vs.NetworkSettings.String("path") != "/vm" ||
		vm.Rate != 1.5 || !reflect.DeepEqual([]uint64(vm.GroupIDs), []uint64{1, 2}) {
		t.Errorf("vmess: %+v rate %v groups %v", vs, vm.Rate, vm.GroupIDs)
	}
	if _, ok := vm.ProtocolSettings["multiplex"]; ok {
		t.Error("multiplex must be dropped")
	}
	for _, w := range []struct{ scope, text string }{{"nodes#2", "multiplex"}, {"nodes#2", "machine_id"}, {"nodes#2", "custom_routes"}} {
		if !hasWarning(rep, w.scope, w.text) {
			t.Errorf("missing warning %s %q", w.scope, w.text)
		}
	}

	vl := loadNode(t, a, 3)
	var vls model.VlessSettings
	_ = vls.Decode(vl.ProtocolSettings)
	if vls.TLS != 2 || vls.TLSSettings.Dest != "www.example.com:443" || vls.TLSSettings.Fingerprint != "firefox" ||
		vls.TLSSettings.PrivateKey != "example-private-key" || vls.TLSSettings.ShortID != "ab" || vls.Encryption != "none" {
		t.Errorf("vless reality: %+v", vls)
	}

	tj := loadNode(t, a, 4)
	var ts model.TrojanSettings
	_ = ts.Decode(tj.ProtocolSettings)
	if tj.Show || ts.ServerName != "tj.example.com" || !ts.AllowInsecure || ts.Network != "grpc" || ts.NetworkSettings.String("serviceName") != "tj" {
		t.Errorf("trojan: show %v %+v", tj.Show, ts)
	}
	for _, text := range []string{"disabled", "transfer_enable", "rate_time_ranges"} {
		if !hasWarning(rep, "nodes#4", text) {
			t.Errorf("missing trojan warning %q", text)
		}
	}

	hy := loadNode(t, a, 5)
	var hs model.HysteriaSettings
	_ = hs.Decode(hy.ProtocolSettings)
	if hy.Type != model.NodeHysteria || hs.Version != 2 || hs.UpMbps != 100 || hs.DownMbps != 500 || hs.Obfs != "salamander" ||
		hs.ObfsPassword != "parent-obfs-pass" || hs.HopInterval != 45 || hs.ServerName != "hy.example.com" {
		t.Errorf("hysteria2: type %s %+v", hy.Type, hs)
	}
	hyc := loadNode(t, a, 6)
	if hyc.ProtocolSettings.String("obfs_password") != "parent-obfs-pass" || !hasWarning(rep, "nodes#6", "differs from parent") {
		t.Errorf("hysteria child password = %q", hyc.ProtocolSettings.String("obfs_password"))
	}
	if !hyc.Show {
		t.Error("enabled NULL means enabled")
	}

	tuic := loadNode(t, a, 7)
	var tu model.TuicSettings
	_ = tu.Decode(tuic.ProtocolSettings)
	if tu.ALPN != "h3,h2" || tu.CongestionControl != "bbr" || tu.ServerName != "tuic.example.com" || tu.ZeroRTTHandshake {
		t.Errorf("tuic: %+v", tu)
	}
	anyTLS := loadNode(t, a, 8)
	var as model.AnyTLSSettings
	_ = as.Decode(anyTLS.ProtocolSettings)
	if as.TLS != 1 || !as.AllowInsecure || as.ServerName != "any.example.com" || !reflect.DeepEqual(as.PaddingScheme, []string{"stop=8", "0=30-30"}) {
		t.Errorf("anytls: %+v", as)
	}
	mieru := loadNode(t, a, 9)
	if mieru.ProtocolSettings.String("transport") != "UDP" || !hasWarning(rep, "nodes#9", "traffic_pattern") {
		t.Errorf("mieru: %v", mieru.ProtocolSettings)
	}
	v2 := loadNode(t, a, 11)
	if v2.Type != model.NodeVmess || len(v2.GroupIDs) != 0 || v2.Show {
		t.Errorf("v2ray alias / NULL group_ids: %+v", v2)
	}
	// malformed protocol_settings JSON → warning, defaults, no crash
	broken := loadNode(t, a, 13)
	if broken.ProtocolSettings.String("network") != "tcp" || !hasWarning(rep, "nodes#13", "protocol_settings") {
		t.Errorf("broken JSON node: %v", broken.ProtocolSettings)
	}
}

// §9 assertion 6 (upstream prices) and plan columns.
func TestUpstreamPlans(t *testing.T) {
	a, rep := mustImport(t, FlavorUpstream)
	var p1, p2 model.Plan
	a.DB.First(&p1, 1)
	a.DB.First(&p2, 2)
	if !reflect.DeepEqual(map[string]int64(p1.Prices), map[string]int64{"monthly": 999, "yearly": 9990}) {
		t.Errorf("upstream prices = %v", p1.Prices)
	}
	if !reflect.DeepEqual([]string(p1.Tags), []string{"hot", "new"}) || p1.DeviceLimit == nil || *p1.DeviceLimit != 3 || p1.SpeedLimit != nil || !p1.Sell {
		t.Errorf("plan 1: %+v", p1)
	}
	if len(p2.Prices) != 0 || p2.GroupID != 0 || p2.TransferEnable != 0 || p2.Sell || *p2.ResetTrafficMethod != model.ResetYearly {
		t.Errorf("plan 2: %+v", p2)
	}
	if !hasWarning(rep, "plans#2", `unknown period "weird"`) || !hasWarning(rep, "plans#2", "group_id NULL") {
		t.Error("plan 2 warnings missing")
	}
}

func TestUpstreamUsersTicketsSettingsPayments(t *testing.T) {
	a, rep := mustImport(t, FlavorUpstream)
	u2 := loadUser(t, a, 2)
	if !auth.VerifyPassword(u2.PasswordHash, "password123") || auth.VerifyPassword(u2.PasswordHash, "wrong") {
		t.Errorf("sha256salt hash %q does not verify", u2.PasswordHash)
	}
	if u2.DeviceLimit == nil || *u2.DeviceLimit != 2 || u2.CommissionRate == nil || *u2.CommissionRate != 13 {
		t.Errorf("user 2: %+v", u2)
	}
	// plan reset method NULL → setting; absent in U → 1 (monthly on the expiry day)
	want := time.Date(2026, 10, 31, 10, 0, 0, 0, shanghai())
	if u2.NextResetAt == nil || !u2.NextResetAt.Equal(want) {
		t.Errorf("next_reset_at = %v, want %v (U column ignored)", u2.NextResetAt, want)
	}
	if u1 := loadUser(t, a, 1); !auth.VerifyPassword(u1.PasswordHash, "password123") || u1.ExpiredAt != nil {
		t.Errorf("user 1: %+v", u1)
	}

	var tickets []model.Ticket
	a.DB.Order("id").Find(&tickets)
	one, two := uint64(1), uint64(2)
	if len(tickets) != 2 || tickets[0].ReplyStatus != model.TicketReplied || !reflect.DeepEqual(tickets[0].LastReplyUserID, &one) ||
		tickets[1].ReplyStatus != model.TicketAwaitingStaff || !reflect.DeepEqual(tickets[1].LastReplyUserID, &two) {
		t.Errorf("upstream tickets: %+v", tickets)
	}

	s := a.Settings
	checks := map[string]any{
		"app_name": "Upstream Board", "reset_traffic_method": 1, "show_info_to_server_enable": false,
		"remind_mail_enable": false, "captcha_provider": "turnstile", "captcha_site_key": "turnstile-example-site",
		"captcha_secret": "turnstile-example-secret", "email_encryption": "tls", "device_limit_mode": 1,
		"server_pull_interval": 60, "stop_register": false, "currency": "USD",
	}
	for k, w := range checks {
		if got := s.Get(k); !reflect.DeepEqual(got, w) {
			t.Errorf("setting %s = %#v, want %#v", k, got, w)
		}
	}
	if tok := s.String("server_token"); tok == "" {
		t.Error("an empty Xboard server_token must keep ggboard's token")
	}
	if !reflect.DeepEqual(s.Strings("email_whitelist_suffix"), []string{"example.com", "example.org"}) ||
		!reflect.DeepEqual(s.Strings("commission_withdraw_method"), []string{"USDT"}) {
		t.Errorf("lists: %v %v", s.Strings("email_whitelist_suffix"), s.Strings("commission_withdraw_method"))
	}
	for _, text := range []string{"server_token is empty", "subscribe_path", "server_pull_interval", "reset_traffic_method not set in Xboard → 1"} {
		if !hasWarning(rep, "settings", text) {
			t.Errorf("missing settings warning %q", text)
		}
	}

	var pays []model.Payment
	a.DB.Order("id").Find(&pays)
	if len(pays) != 2 || pays[0].Gateway != "epay" || pays[0].Enabled || pays[0].Config["type"] != "usdt.tron" ||
		pays[1].Gateway != "btcpay" || !pays[1].Enabled || pays[1].Config["server_url"] != "https://btcpay.example.com" {
		t.Errorf("upstream payments: %+v", pays)
	}
	if !hasWarning(rep, "payments#1", `plugin "epay" is disabled`) {
		t.Error("plugin-disabled warning missing")
	}

	var notices []model.Notice
	a.DB.Order("id").Find(&notices)
	if len(notices) != 2 || notices[0].Sort != 5 || notices[1].Sort != 0 {
		t.Errorf("upstream notice sort: %+v", notices)
	}
}

func TestUpstreamStats(t *testing.T) {
	day := time.Date(2024, 10, 22, 12, 0, 0, 0, shanghai())
	a, _ := mustImport(t, FlavorUpstream, func(o *Options) {
		o.WithStats = true
		o.Now = func() time.Time { return day }
	})
	var sn []model.StatNode
	a.DB.Order("node_id").Find(&sn)
	got := map[uint64][2]int64{}
	for _, s := range sn {
		got[s.NodeID] = [2]int64{s.U, s.D}
	}
	// (hysteria, 5) by id; (vmess, 5) has no vmess id 5 → by code → node 2
	if !reflect.DeepEqual(got, map[uint64][2]int64{2: {10, 10}, 5: {50, 60}}) {
		t.Errorf("stat_nodes = %v", got)
	}
	var su []model.StatUser
	a.DB.Find(&su)
	if len(su) != 1 || su[0].U != 200 || su[0].D != 400 || su[0].Rate != 2 {
		t.Errorf("upstream stat_users (already charged): %+v", su)
	}
}
