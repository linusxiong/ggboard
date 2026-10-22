package xboardimport

import (
	"reflect"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func loadNode(t *testing.T, a *app.App, id uint64) model.Node {
	t.Helper()
	var n model.Node
	if err := a.DB.First(&n, id).Error; err != nil {
		t.Fatalf("node %d: %v", id, err)
	}
	return n
}

func loadUser(t *testing.T, a *app.App, id uint64) model.User {
	t.Helper()
	var u model.User
	if err := a.DB.First(&u, id).Error; err != nil {
		t.Fatalf("user %d: %v", id, err)
	}
	return u
}

func count(t *testing.T, a *app.App, m any) int {
	t.Helper()
	var n int64
	if err := a.DB.Model(m).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return int(n)
}

// §9 assertion 2: per-table counts equal the summary; kept ids; next insert id = max+1.
func TestLegacyCountsAndIDs(t *testing.T) {
	a, rep := mustImport(t, FlavorLegacy)
	if rep.Flavor != FlavorLegacy || rep.Migrations != 33 || rep.LastMigration != "2024_03_19_103149_modify_icon_column_to_v2_payment_table" {
		t.Fatalf("detection/migrations: %s %d %q", rep.Flavor, rep.Migrations, rep.LastMigration)
	}
	tables := map[string]any{
		"server_groups": &model.ServerGroup{}, "plans": &model.Plan{}, "node_routes": &model.NodeRoute{},
		"nodes": &model.Node{}, "users": &model.User{}, "invite_codes": &model.InviteCode{},
		"commission_logs": &model.CommissionLog{}, "coupons": &model.Coupon{}, "payments": &model.Payment{},
		"tickets": &model.Ticket{}, "ticket_messages": &model.TicketMessage{}, "notices": &model.Notice{},
		"knowledge": &model.Knowledge{},
	}
	want := map[string][3]int{ // source, imported, skipped
		"server_groups": {3, 3, 0}, "plans": {4, 4, 0}, "node_routes": {2, 2, 0}, "nodes": {12, 12, 0},
		"users": {14, 14, 0}, "invite_codes": {3, 1, 2}, "commission_logs": {2, 2, 0}, "coupons": {4, 2, 2},
		"payments": {7, 7, 0}, "tickets": {4, 3, 1}, "ticket_messages": {5, 4, 1}, "notices": {3, 3, 0}, "knowledge": {2, 2, 0},
	}
	for name, m := range tables {
		st := rep.Lookup(name)
		if st == nil {
			t.Fatalf("no summary line for %s", name)
		}
		if got := count(t, a, m); got != st.Imported {
			t.Errorf("%s: %d rows in the target, summary says %d", name, got, st.Imported)
		}
		if w := want[name]; [3]int{st.Source, st.Imported, st.Skipped} != w {
			t.Errorf("%s: summary %d/%d/%d, want %v", name, st.Source, st.Imported, st.Skipped, w)
		}
	}
	if count(t, a, &model.Order{}) != 0 {
		t.Error("orders must never be imported")
	}
	if rep.Lookup("stats") != nil {
		t.Error("stats are imported only with --with-stats")
	}
	// kept ids
	var ids []uint64
	a.DB.Model(&model.User{}).Order("id").Pluck("id", &ids)
	if !reflect.DeepEqual(ids, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}) {
		t.Errorf("user ids = %v", ids)
	}
	var cids []uint64
	a.DB.Model(&model.Coupon{}).Order("id").Pluck("id", &cids)
	if !reflect.DeepEqual(cids, []uint64{1, 4}) {
		t.Errorf("coupon ids = %v", cids)
	}
	// next insert id = max+1 (SQLite AUTOINCREMENT follows explicit ids)
	u := model.User{Email: "new@example.com", PasswordHash: "!", UUID: "20000000-0000-4000-8000-000000000001", Token: "b0000000000000000000000000000001"}
	if err := a.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	if u.ID != 15 {
		t.Errorf("next user id = %d, want 15", u.ID)
	}
	n := model.Node{Type: model.NodeVmess, Name: "new", Host: "new.example.com", Port: "1", Rate: 1}
	if err := a.DB.Create(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n.ID != 13 {
		t.Errorf("next node id = %d, want 13", n.ID)
	}
}

// §9 assertions 3, 4, 5: renumbering, keys, group_id variants, protocol settings from legacy columns.
func TestLegacyNodes(t *testing.T) {
	a, rep := mustImport(t, FlavorLegacy)
	// vmess #1 / #2 collide with shadowsocks #1 / #2 and are renumbered past max legacy id 10
	vm := loadNode(t, a, 11)
	relay := loadNode(t, a, 12)
	if vm.Type != model.NodeVmess || vm.Name != "VM-1" || relay.Name != "VM-1-relay" {
		t.Fatalf("renumbered nodes: %+v / %+v", vm, relay)
	}
	if relay.ParentID == nil || *relay.ParentID != 11 {
		t.Errorf("child parent_id = %v, want 11", relay.ParentID)
	}
	if !hasWarning(rep, "nodes#11", `vmess #1 "VM-1" → node 11`) || len(rep.RenumberedNodes) != 2 {
		t.Errorf("renumber warnings: %v", rep.RenumberedNodes)
	}
	var vs model.VmessSettings
	if err := vs.Decode(vm.ProtocolSettings); err != nil {
		t.Fatal(err)
	}
	if vs.Network != "ws" || vs.TLS != 1 || vs.TLSSettings.ServerName != "vm.example.com" || !vs.TLSSettings.AllowInsecure ||
		vs.NetworkSettings.String("path") != "/ws" {
		t.Errorf("vmess settings from camelCase columns: %+v", vs)
	}
	if !hasWarning(rep, "nodes#11", "rules") {
		t.Error("vmess rules must be dropped with a warning")
	}

	// SS2022 server key from created_at 1700000000; the child uses its parent's created_at
	ss := loadNode(t, a, 1)
	child := loadNode(t, a, 2)
	if got := ss.ProtocolSettings.String("server_key"); got != "MjQ5MjBkZWNmODNmNjk2MA==" {
		t.Errorf("ss2022 server_key = %q", got)
	}
	if child.ParentID == nil || *child.ParentID != 1 || child.ProtocolSettings.String("server_key") != "MjQ5MjBkZWNmODNmNjk2MA==" {
		t.Errorf("ss child: parent %v key %q", child.ParentID, child.ProtocolSettings.String("server_key"))
	}
	if child.Rate != 1.5 || child.Port != "8443" || child.ServerPort != 443 {
		t.Errorf("ss child common columns: %+v", child)
	}
	if got := model.NodeServerKey(&ss); got != "MjQ5MjBkZWNmODNmNjk2MA==" {
		t.Errorf("model.NodeServerKey = %q", got)
	}

	// group_id variants
	groups := map[uint64][]uint64{1: {1, 2}, 2: {1}, 3: {1, 3}, 4: {1, 2}, 6: {1, 2}, 8: {}}
	for id, want := range groups {
		n := loadNode(t, a, id)
		got := []uint64(n.GroupIDs)
		if got == nil {
			got = []uint64{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("node %d group_ids = %v, want %v", id, got, want)
		}
	}
	if ss.RouteIDs == nil || !reflect.DeepEqual([]uint64(ss.RouteIDs), []uint64{1, 2}) {
		t.Errorf("route_ids = %v", ss.RouteIDs)
	}
	if !reflect.DeepEqual([]string(ss.Tags), []string{"HK", "SS"}) {
		t.Errorf("tags = %v", ss.Tags)
	}

	// shadowsocks obfs=http, unknown cipher, invalid rate, excludes/ips warnings
	obfs := loadNode(t, a, 3)
	var sss model.ShadowsocksSettings
	_ = sss.Decode(obfs.ProtocolSettings)
	if sss.Obfs != "http" || sss.ObfsSettings.Host != "cdn.example.com" || sss.ServerKey != "" || obfs.Rate != 0.5 {
		t.Errorf("ss obfs settings: %+v rate %v", sss, obfs.Rate)
	}
	text := loadNode(t, a, 4)
	if text.Rate != 1 || text.ProtocolSettings.String("cipher") != "aes-192-gcm" || text.Sort != 0 {
		t.Errorf("ss #4: rate %v cipher %q sort %d", text.Rate, text.ProtocolSettings.String("cipher"), text.Sort)
	}
	for _, w := range []struct{ scope, text string }{{"nodes#4", "rate"}, {"nodes#4", "aes-192-gcm"}, {"nodes#3", "excludes"}, {"nodes#5", "ips"}} {
		if !hasWarning(rep, w.scope, w.text) {
			t.Errorf("missing warning %s %q", w.scope, w.text)
		}
	}

	// trojan with camelCase networkSettings
	tj := loadNode(t, a, 6)
	var ts model.TrojanSettings
	_ = ts.Decode(tj.ProtocolSettings)
	if ts.Network != "ws" || ts.NetworkSettings.String("path") != "/tj" || ts.ServerName != "tj.example.com" || ts.AllowInsecure {
		t.Errorf("trojan settings: %+v", ts)
	}

	// hysteria v1: alpn 3 → h3, obfs password = K16 of its created_at; version 0 → 1
	hy := loadNode(t, a, 7)
	var hs model.HysteriaSettings
	_ = hs.Decode(hy.ProtocolSettings)
	if hs.Version != 1 || hs.ALPN != "h3" || hs.Obfs != "salamander" || hs.ObfsPassword != "MjQ5MjBkZWNmODNmNjk2MA==" ||
		hs.UpMbps != 100 || hs.DownMbps != 200 || !hs.AllowInsecure || hy.Port != "20000-30000" {
		t.Errorf("hysteria v1: %+v port %q", hs, hy.Port)
	}
	hy0 := loadNode(t, a, 8)
	var hs0 model.HysteriaSettings
	_ = hs0.Decode(hy0.ProtocolSettings)
	if hs0.Version != 1 || hs0.Obfs != "plain" || hs0.ObfsPassword != "" || hs0.ALPN != "hysteria" {
		t.Errorf("hysteria version 0: %+v", hs0)
	}
	if !hasWarning(rep, "nodes#8", "→ 1") {
		t.Error("hysteria version 0/NULL must warn")
	}

	// vless reality and an unsupported transport
	vl := loadNode(t, a, 9)
	var vls model.VlessSettings
	_ = vls.Decode(vl.ProtocolSettings)
	if vls.TLS != 2 || vls.Flow != "xtls-rprx-vision" || vls.Encryption != "none" || vls.TLSSettings.Dest != "www.example.com:443" ||
		vls.TLSSettings.PublicKey != "example-public-key" || vls.TLSSettings.ShortID != "abcd" || vls.TLSSettings.Fingerprint != "chrome" ||
		vls.TLSSettings.ServerPort != "443" || vl.Port != "443" {
		t.Errorf("vless reality: %+v", vls)
	}
	kcp := loadNode(t, a, 10)
	if kcp.Show || kcp.ProtocolSettings.String("network") != "kcp" || !hasWarning(rep, "nodes#10", "kcp") {
		t.Errorf("kcp node must be hidden with a warning: show=%v", kcp.Show)
	}

	// routes: JSON object match and newline list
	var routes []model.NodeRoute
	a.DB.Order("id").Find(&routes)
	if len(routes) != 2 || !reflect.DeepEqual([]string(routes[0].Match), []string{"domain:ads.example.com", "domain:track.example.com"}) ||
		!reflect.DeepEqual([]string(routes[1].Match), []string{"geosite:netflix", "domain:video.example.com"}) ||
		routes[1].Action != model.RouteActionDNS || routes[1].ActionValue != "1.1.1.1" {
		t.Errorf("routes: %+v", routes)
	}
}

// §9 assertion 6 (legacy half), plans.
func TestLegacyPlans(t *testing.T) {
	a, rep := mustImport(t, FlavorLegacy)
	var plans []model.Plan
	a.DB.Order("id").Find(&plans)
	if len(plans) != 4 {
		t.Fatalf("plans = %d", len(plans))
	}
	p1 := plans[0]
	if !reflect.DeepEqual(map[string]int64(p1.Prices), map[string]int64{"monthly": 1000, "onetime": 0}) {
		t.Errorf("legacy prices = %v", p1.Prices)
	}
	if !hasWarning(rep, "plans#1", "onetime_price is 0") {
		t.Error("a free legacy price must warn")
	}
	if p1.TransferEnable != 100 || p1.GroupID != 1 || !p1.Sell || !p1.Show || p1.SpeedLimit != nil || p1.ResetTrafficMethod != nil {
		t.Errorf("plan 1: %+v", p1)
	}
	p2 := plans[1]
	if p2.Sell || p2.Show || *p2.ResetTrafficMethod != model.ResetMonthly || p2.CapacityLimit == nil || *p2.CapacityLimit != 0 ||
		!reflect.DeepEqual(map[string]int64(p2.Prices), map[string]int64{"monthly": 2000, "yearly": 20000, "reset_traffic": 500}) {
		t.Errorf("plan 2: %+v", p2)
	}
	if !hasWarning(rep, "plans#2", "capacity_limit 0 is already exceeded") {
		t.Error("exceeded capacity must warn")
	}
	if plans[2].GroupID != 0 || !hasWarning(rep, "plans#3", "plan grants no group") {
		t.Errorf("plan 3 group = %d", plans[2].GroupID)
	}
}

// §9 assertions 7 and 8: passwords, expiry, next_reset_at.
func TestLegacyUsers(t *testing.T) {
	a, rep := mustImport(t, FlavorLegacy)
	for _, id := range []uint64{1, 2, 3, 4} {
		u := loadUser(t, a, id)
		if !auth.VerifyPassword(u.PasswordHash, "password123") {
			t.Errorf("user %d (%s): password123 does not verify against %q", id, u.Email, u.PasswordHash)
		}
		if auth.VerifyPassword(u.PasswordHash, "wrong") {
			t.Errorf("user %d: wrong password verifies", id)
		}
	}
	if u := loadUser(t, a, 5); u.PasswordHash != "!xb-unusable" {
		t.Errorf("unknown algo hash = %q", u.PasswordHash)
	}
	if rep.LegacyHashes != 3 || rep.UnusableHashes != 1 {
		t.Errorf("hash counters %d/%d", rep.LegacyHashes, rep.UnusableHashes)
	}

	u1 := loadUser(t, a, 1)
	if u1.Email != "admin@example.com" || !u1.IsAdmin || u1.TelegramID == nil || *u1.TelegramID != 123456789 ||
		u1.Balance != 1000 || u1.CommissionBalance != 500 || u1.U != 1024 || u1.D != 2048 || u1.TransferEnable != 107374182400 ||
		u1.Remarks != "root" || u1.LastLoginAt == nil || u1.LastLoginAt.Unix() != 1700000300 || u1.CreatedAt.Unix() != 1690000000 ||
		u1.PasswordVersion != 0 || u1.LastLoginIP != "" {
		t.Errorf("user 1: %+v", u1)
	}
	// expired_at NULL → never; 0 → created_at
	if u := loadUser(t, a, 4); u.ExpiredAt != nil || u.NextResetAt != nil {
		t.Errorf("lifetime user: expired %v next %v", u.ExpiredAt, u.NextResetAt)
	}
	if u := loadUser(t, a, 6); u.ExpiredAt == nil || u.ExpiredAt.Unix() != 1690000000 || u.NextResetAt != nil {
		t.Errorf("expired_at 0 user: %v / %v", u.ExpiredAt, u.NextResetAt)
	}
	if u := loadUser(t, a, 7); u.ExpiredAt == nil || u.ExpiredAt.Unix() != 1600000000 || u.NextResetAt != nil {
		t.Errorf("past user: %v / %v", u.ExpiredAt, u.NextResetAt)
	}
	sh := shanghai()
	cases := map[uint64]time.Time{
		2: time.Date(2026, 10, 31, 2, 0, 0, 0, time.UTC), // M=1, E = 2027-01-31 10:00 +08
		1: time.Date(2026, 11, 1, 0, 0, 0, 0, sh),        // plan NULL → setting (absent in L → 0)
		3: time.Date(2027, 1, 1, 0, 0, 0, 0, sh),         // M=3
	}
	for id, want := range cases {
		u := loadUser(t, a, id)
		if u.NextResetAt == nil || !u.NextResetAt.Equal(want) {
			t.Errorf("user %d next_reset_at = %v, want %v", id, u.NextResetAt, want)
		}
	}
	// M=2 → nil (user 4 has plan 4)
	if u := loadUser(t, a, 4); u.PlanID == nil || *u.PlanID != 4 {
		t.Errorf("user 4 plan = %v", u.PlanID)
	}

	// references
	if u := loadUser(t, a, 2); u.InviteUserID == nil || *u.InviteUserID != 1 || u.CommissionType != 1 || u.CommissionRate == nil ||
		*u.CommissionRate != 25 || u.Discount == nil || *u.Discount != 10 || u.SpeedLimit == nil || *u.SpeedLimit != 100 || u.RemindTraffic {
		t.Errorf("user 2: %+v", u)
	}
	if u := loadUser(t, a, 8); u.PlanID != nil || u.InviteUserID != nil || u.GroupID == nil {
		t.Errorf("user 8 (deleted plan, missing inviter): %+v", u)
	}
	if u := loadUser(t, a, 9); u.InviteUserID != nil || u.Token == "00000000000000000000000000000001" || len(u.Token) != 32 {
		t.Errorf("user 9 (self-invite, duplicate token): invite %v token %q", u.InviteUserID, u.Token)
	}
	if u := loadUser(t, a, 11); u.UUID == "00000000-0000-4000-8000-000000000002" || len(u.UUID) != 36 {
		t.Errorf("user 11 duplicate uuid kept: %q", u.UUID)
	}
	if u := loadUser(t, a, 10); u.Email != "mixed@example.com" || u.RemindExpire || u.RemindTraffic {
		t.Errorf("user 10: %q remind %v/%v", u.Email, u.RemindExpire, u.RemindTraffic)
	}
	if u := loadUser(t, a, 12); u.U != 0 || u.D != 10 || u.CommissionType != 0 || !u.Banned {
		t.Errorf("user 12: %+v", u)
	}
	// case-insensitive duplicate emails: the most recently active account keeps the address
	if u := loadUser(t, a, 14); u.Email != "a@example.com" {
		t.Errorf("winner email = %q", u.Email)
	}
	if u := loadUser(t, a, 13); u.Email != "a+dup13@example.com" {
		t.Errorf("loser email = %q", u.Email)
	}
	for _, w := range []struct{ scope, text string }{
		{"users#5", "cannot be verified"}, {"users#8", "plan_id 99"}, {"users#8", "invite_user_id 77"}, {"users#9", "itself"},
		{"users#9", "token duplicates user #1"}, {"users#11", "duplicates user #2"}, {"users#12", "negative traffic"},
		{"users#12", "commission_type 5"}, {"users#13", "collides with user #14"},
	} {
		if !hasWarning(rep, w.scope, w.text) {
			t.Errorf("missing warning %s %q", w.scope, w.text)
		}
	}
}

// next_reset_at with a later clock (2026-11-15): M=1 clamps to the 30th.
func TestNextResetAtLaterClock(t *testing.T) {
	later := time.Date(2026, 11, 15, 12, 0, 0, 0, shanghai())
	a, _ := mustImport(t, FlavorLegacy, func(o *Options) { o.Now = func() time.Time { return later } })
	u := loadUser(t, a, 2)
	want := time.Date(2026, 11, 30, 10, 0, 0, 0, shanghai())
	if u.NextResetAt == nil || !u.NextResetAt.Equal(want) {
		t.Errorf("next_reset_at = %v, want %v", u.NextResetAt, want)
	}
}

// §9 assertion 9 (legacy): reply_status derived from the last message; the no-message ticket is inverted.
func TestLegacyTickets(t *testing.T) {
	a, rep := mustImport(t, FlavorLegacy)
	var tickets []model.Ticket
	a.DB.Order("id").Find(&tickets)
	if len(tickets) != 3 {
		t.Fatalf("tickets = %d", len(tickets))
	}
	check := func(tk model.Ticket, status model.TicketReplyStatus, last *uint64) {
		t.Helper()
		if tk.ReplyStatus != status || !reflect.DeepEqual(tk.LastReplyUserID, last) {
			t.Errorf("ticket %d: reply_status %d last %v; want %d %v", tk.ID, tk.ReplyStatus, tk.LastReplyUserID, status, last)
		}
	}
	one, three := uint64(1), uint64(3)
	check(tickets[0], model.TicketReplied, &one)
	check(tickets[1], model.TicketAwaitingStaff, &three)
	check(tickets[2], model.TicketReplied, nil)
	if tickets[2].Status != model.TicketStatusClosed || tickets[2].Level != model.TicketLevelHigh || tickets[1].Level != model.TicketLevelMedium {
		t.Errorf("ticket status/level: %+v", tickets)
	}
	if count(t, a, &model.TicketMessage{}) != 4 || !hasWarning(rep, "tickets#4", "user 99") {
		t.Error("the orphan ticket and its message must be skipped with a warning")
	}
}

// §9 assertion 10 (legacy): settings translation.
func TestLegacySettings(t *testing.T) {
	a, rep := mustImport(t, FlavorLegacy)
	s := a.Settings
	checks := map[string]any{
		"app_name":                   "Example Board", // highest id wins
		"app_url":                    "https://board.example.com",
		"stop_register":              false, // ""
		"email_verify":               true,  // "1"
		"invite_force":               true,  // "true"
		"plan_change_enable":         false,
		"show_info_to_server_enable": false, // absent → Xboard default
		"invite_never_expire":        false, // absent → Xboard default
		"reset_traffic_method":       0,     // absent in L
		"server_token":               "xboard-example-server-token",
		"try_out_plan_id":            0,
		"try_out_hour":               1,
		"invite_commission":          13,
		"captcha_provider":           "recaptcha",
		"captcha_site_key":           "recaptcha-example-site",
		"captcha_secret":             "recaptcha-example-secret",
		"email_driver":               "smtp",
		"email_host":                 "smtp.example.com",
		"email_port":                 465,
		"email_encryption":           "none",
		"remind_mail_enable":         true, // L: left as ours
		"server_push_interval":       30,
		"currency":                   "CNY",
	}
	for k, want := range checks {
		if got := s.Get(k); !reflect.DeepEqual(got, want) {
			t.Errorf("setting %s = %#v, want %#v", k, got, want)
		}
	}
	if got := s.Strings("subscribe_url"); !reflect.DeepEqual(got, []string{"https://a.example.com"}) {
		t.Errorf("subscribe_url = %v", got)
	}
	if got := s.Strings("email_whitelist_suffix"); len(got) != 9 || got[0] != "gmail.com" {
		t.Errorf("email_whitelist_suffix = %v", got)
	}
	if got := s.Strings("commission_withdraw_method"); !reflect.DeepEqual(got, []string{"支付宝", "USDT", "Paypal"}) {
		t.Errorf("commission_withdraw_method = %v", got)
	}
	for _, text := range []string{"https://*.example.com", "reset_traffic_method not set", "try_out_plan_id 99", "ignored settings: some_unknown_key", "frontend_theme", "12.5 rounded"} {
		if !hasWarning(rep, "settings", text) {
			t.Errorf("missing settings warning %q", text)
		}
	}
	if st := rep.Lookup("settings"); st.Source != 22 || st.Imported != 19 || st.Skipped != 3 {
		t.Errorf("settings summary %+v", st)
	}
}

// §9 assertion 11 (legacy): payments.
// A real legacy install (terabix rehearsal, 2026-10-22) stores email_encryption as "SSL" and a mixed-case setting
// name (theme_Xboard): values and names are matched case-insensitively.
func TestLegacySettingsCaseInsensitive(t *testing.T) {
	a := testutil.NewTestApp(t)
	rep, err := importInto(t, a, newSource(t, FlavorLegacy,
		`UPDATE v2_settings SET value = 'SSL' WHERE name = 'email_encryption';`,
		`INSERT INTO v2_settings ("id", "group", "type", "name", "value", "created_at", "updated_at") VALUES (900, NULL, NULL, 'theme_Xboard', '{"a":1}', NULL, NULL);`))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := a.Settings.String("email_encryption"); got != "ssl" {
		t.Errorf("email_encryption = %q, want ssl", got)
	}
	if hasWarning(rep, "settings", "email_encryption") {
		t.Error("SSL must not be reported as unsupported")
	}
	if !hasWarning(rep, "settings", "ignored settings: ") || !hasWarning(rep, "settings", "theme_xboard") {
		t.Error("theme_Xboard must be listed (lower-cased) among the ignored settings")
	}
}

func TestLegacyPayments(t *testing.T) {
	a, rep := mustImport(t, FlavorLegacy)
	var pays []model.Payment
	a.DB.Order("id").Find(&pays)
	if len(pays) != 7 {
		t.Fatalf("payments = %d", len(pays))
	}
	type want struct {
		gateway string
		enabled bool
		config  map[string]string
	}
	wants := []want{
		{"epay", true, map[string]string{"url": "https://pay.example.com", "pid": "1001", "key": "epay-example-key", "type": "alipay"}},
		{"epusdt", true, map[string]string{"api_url": "https://usdt.example.com", "api_token": "usdt-example-token", "currency_type": "usdt.trc20"}},
		{"alipay_f2f", true, map[string]string{"app_id": "2021000000000000", "private_key": "EXAMPLE-PRIVATE-KEY", "alipay_public_key": "EXAMPLE-PUBLIC-KEY", "sandbox": "0"}},
		{"stripe", false, map[string]string{"secret_key": "sk_test_example", "webhook_secret": "whsec_example", "currency": "usd", "payment_methods": "card"}},
		{"coinbase", false, map[string]string{}},
		{"btcpay", true, map[string]string{"server_url": "https://btcpay.example.com", "store_id": "store1", "api_key": "btc-example-key", "webhook_secret": "btc-example-hook"}},
		{"mgate", false, map[string]string{"mgate_url": "https://mgate.example.com", "mgate_app_id": "app", "mgate_app_secret": "mgate-example-secret"}},
	}
	for i, w := range wants {
		p := pays[i]
		cfg := map[string]string(p.Config)
		if cfg == nil {
			cfg = map[string]string{}
		}
		if p.Gateway != w.gateway || p.Enabled != w.enabled || !reflect.DeepEqual(cfg, w.config) {
			t.Errorf("payment %d: %s enabled=%v config=%v; want %s %v %v", p.ID, p.Gateway, p.Enabled, cfg, w.gateway, w.enabled, w.config)
		}
	}
	if pays[0].UUID != "AbCd1234" || pays[0].HandlingFeeFixed != 100 || pays[0].HandlingFeePercent != 1.5 || pays[1].Icon != "usdt.png" {
		t.Errorf("payment common columns: %+v / %+v", pays[0], pays[1])
	}
	for _, w := range []struct{ scope, text string }{
		{"payments#4", "checkout.session.completed"}, {"payments#4", "exchange_rate"}, {"payments#5", "Coinbase Commerce closed"},
		{"payments#6", "/notify/btcpay/UvWx1234"}, {"payments#7", "not available in ggboard"}, {"payments#3", "product_name"},
		{"payments#1", "config keys not used by ggboard dropped: product_name"}, {"payments#4", "dropped: description"},
	} {
		if !hasWarning(rep, w.scope, w.text) {
			t.Errorf("missing warning %s %q", w.scope, w.text)
		}
	}
	if !hasWarning(rep, "payments#4", "https://pay-notify.example.com/api/v1/guest/payment/notify/stripe/MnOp3456") {
		t.Error("the Stripe webhook URL must use the payment's notify_domain")
	}
}

// Coupons, invite codes, commission logs, notices, knowledge.
func TestLegacyContent(t *testing.T) {
	a, rep := mustImport(t, FlavorLegacy)
	var c model.Coupon
	if err := a.DB.First(&c, 1).Error; err != nil {
		t.Fatal(err)
	}
	if c.Code != "WELCOME" || c.LimitUse == nil || *c.LimitUse != 10 || c.LimitUseWithUser == nil || *c.LimitUseWithUser != 1 ||
		!reflect.DeepEqual([]uint64(c.LimitPlanIDs), []uint64{1, 2}) || !reflect.DeepEqual([]string(c.LimitPeriods), []string{"monthly", "yearly"}) ||
		c.Type != model.CouponTypeAmount || c.Value != 500 || !c.Show || c.EndedAt.Unix() != 1900000000 {
		t.Errorf("coupon 1: %+v", c)
	}
	for _, w := range []struct{ scope, text string }{
		{"coupons#2", `duplicate code "welcome" (kept #1)`}, {"coupons#3", "longer than 64"}, {"coupons#1", "bogus"},
		{"coupons", "limit_use_with_user"}, {"invite_codes#2", "duplicate code"}, {"invite_codes#3", "user 99"},
		{"commission_logs#2", "inviter 77"}, {"knowledge#2", "access start"},
	} {
		if !hasWarning(rep, w.scope, w.text) {
			t.Errorf("missing warning %s %q", w.scope, w.text)
		}
	}
	var ic model.InviteCode
	if err := a.DB.First(&ic, 1).Error; err != nil || ic.Code != "INVITE01" || ic.PV != 3 || ic.Status {
		t.Errorf("invite code: %+v %v", ic, err)
	}
	var logs []model.CommissionLog
	a.DB.Order("id").Find(&logs)
	if len(logs) != 2 || logs[0].TradeNo != "2024010112000012345678" || logs[0].GetAmount != 100 || logs[1].InviteUserID != 77 {
		t.Errorf("commission logs: %+v", logs)
	}
	// legacy notices: sort = rank by created_at DESC, id DESC
	var notices []model.Notice
	a.DB.Order("id").Find(&notices)
	if len(notices) != 3 || notices[0].Sort != 2 || notices[1].Sort != 1 || notices[2].Sort != 0 {
		t.Errorf("notice sort: %+v", notices)
	}
	if !reflect.DeepEqual([]string(notices[0].Tags), []string{"news", "update"}) || !reflect.DeepEqual([]string(notices[2].Tags), []string{"maintenance"}) ||
		notices[1].ImgURL != "" || notices[0].ImgURL != "https://img.example.com/a.png" {
		t.Errorf("notice tags/img: %+v", notices)
	}
	var kb []model.Knowledge
	a.DB.Order("id").Find(&kb)
	if len(kb) != 2 || kb[0].Language != "zh-CN" || kb[1].Sort != 0 || kb[1].Show || rep.AccessArticles != 1 {
		t.Errorf("knowledge: %+v", kb)
	}
}

// §9 assertion 12: stats (legacy charged-bytes aggregation, record_at dates, node remapping).
func TestLegacyStats(t *testing.T) {
	day := time.Date(2024, 10, 22, 12, 0, 0, 0, shanghai())
	a, rep := mustImport(t, FlavorLegacy, func(o *Options) {
		o.WithStats = true
		o.Now = func() time.Time { return day }
	})
	want := model.Day(time.Date(2024, 10, 21, 0, 0, 0, 0, time.UTC))
	var su []model.StatUser
	a.DB.Find(&su)
	if len(su) != 1 {
		t.Fatalf("stat_users = %+v", su)
	}
	if s := su[0]; s.UserID != 1 || s.NodeID != 0 || s.U != 1000+200 || s.D != 2000+300 || s.Rate != 0.88 || !s.RecordAt.Equal(want) {
		t.Errorf("stat_user aggregate: %+v (want u=1200 d=2300 rate 0.88 on %v)", s, want)
	}
	var sn []model.StatNode
	a.DB.Order("node_id").Find(&sn)
	got := map[uint64][2]int64{}
	for _, s := range sn {
		got[s.NodeID] = [2]int64{s.U, s.D}
		if !s.RecordAt.Equal(want) {
			t.Errorf("stat_node record_at = %v", s.RecordAt)
		}
	}
	// vmess #1 → node 11, shadowsocks #1 → node 1, v2ray #2 → vmess #2 → node 12; server 99 dropped; old row filtered
	if !reflect.DeepEqual(got, map[uint64][2]int64{1: {10, 20}, 11: {100, 200}, 12: {5, 5}}) {
		t.Errorf("stat_nodes = %v", got)
	}
	var st []model.Stat
	a.DB.Find(&st)
	if len(st) != 1 || st[0].OrderCount != 3 || st[0].TransferUsedTotal != 123456789 || !st[0].RecordAt.Equal(want) {
		t.Errorf("stats = %+v", st)
	}
	if s := rep.Lookup("stat_nodes"); s.Source != 4 || s.Imported != 3 || s.Skipped != 1 {
		t.Errorf("stat_nodes summary %+v", s)
	}
	if s := rep.Lookup("stat_users"); s.Source != 3 || s.Imported != 1 || s.Skipped != 1 {
		t.Errorf("stat_users summary %+v", s)
	}
}
