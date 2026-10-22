package xboardimport

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
)

// nodes ← v2_server_<type> (L) / v2_server (U) (§4.4).

// srcNode is one source server row with its resolved target id.
type srcNode struct {
	typ     string
	srcID   uint64 // L: id in its own table; U: v2_server.id
	id      uint64 // target id
	r       row
	created int64  // unix created_at (U: parsed in --source-tz)
	parent  uint64 // source parent id (0 = none)
	code    string // U only
	node    *model.Node
}

func (n *srcNode) scope() string {
	return fmt.Sprintf("nodes#%d", n.id)
}

// hysteriaALPN is legacy v2_server_hysteria.alpn for version 1.
var hysteriaALPN = map[int64]string{0: "hysteria", 1: "http/1.1", 2: "h2", 3: "h3"}

var knownCiphers = map[string]bool{
	"aes-128-gcm": true, "aes-256-gcm": true, "chacha20-ietf-poly1305": true,
	"2022-blake3-aes-128-gcm": true, "2022-blake3-aes-256-gcm": true, "2022-blake3-chacha20-poly1305": true,
}

func (im *importer) importNodes() error {
	stat := im.rep.Table("nodes")
	var nodes []*srcNode
	var err error
	if im.flavor == FlavorLegacy {
		nodes, err = im.readLegacyNodes()
	} else {
		nodes, err = im.readUpstreamNodes()
	}
	if err != nil {
		return err
	}
	stat.Source = len(nodes)

	byKey := map[nodeKey]*srcNode{}
	for _, n := range nodes {
		if n.id != 0 {
			byKey[nodeKey{n.typ, n.srcID}] = n
		}
	}
	byID := map[uint64]*srcNode{}
	for _, n := range nodes {
		if n.id != 0 {
			byID[n.id] = n
		}
	}
	// parents: L parent_id is a same-type legacy id, U parent_id is a v2_server id.
	parentOf := func(n *srcNode) *srcNode {
		if n.parent == 0 {
			return nil
		}
		if im.flavor == FlavorLegacy {
			return byKey[nodeKey{n.typ, n.parent}]
		}
		return byID[n.parent]
	}

	b := newBatcher[model.Node](im.tx)
	types := map[string]int{}
	renumbered := 0
	for _, n := range nodes {
		if n.id == 0 { // skipped while reading (unsupported type)
			stat.Skipped++
			continue
		}
		if n.srcID != n.id {
			renumbered++
		}
		node := im.mapNodeCommon(n)
		keyCreated := n.created
		if n.parent != 0 {
			if p := parentOf(n); p == nil {
				im.warn(n.scope(), "parent_id %d not found → NULL", n.parent)
			} else {
				pid := p.id
				node.ParentID = &pid
				keyCreated = p.created
				if p.parent != 0 {
					im.warn(n.scope(), "parent node %d is itself a child node (grandchild): traffic is counted on node %d only", pid, pid)
				}
			}
		}
		var hide bool
		if im.flavor == FlavorLegacy {
			node.ProtocolSettings, hide = im.legacySettings(n, keyCreated)
		} else {
			node.ProtocolSettings, hide = im.upstreamSettings(n, keyCreated)
		}
		if hide {
			node.Show = false
		}
		n.node = node
		types[n.typ]++
	}
	// U: a hysteria child serves its parent's obfs password (the backend polls the parent).
	if im.flavor == FlavorUpstream {
		for _, n := range nodes {
			if n.node == nil || n.node.ParentID == nil || n.typ != model.NodeHysteria {
				continue
			}
			p := byID[*n.node.ParentID]
			if p == nil || p.node == nil || p.typ != model.NodeHysteria {
				continue
			}
			own := n.node.ProtocolSettings.String("obfs_password")
			parent := p.node.ProtocolSettings.String("obfs_password")
			if own != parent {
				im.warn(n.scope(), "obfs password differs from parent node %d; using the parent's (clients connect through the parent's backend)", p.id)
				n.node.ProtocolSettings["obfs_password"] = parent
			}
		}
	}
	for _, n := range nodes {
		if n.node == nil {
			continue
		}
		if err := b.add(*n.node); err != nil {
			return err
		}
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	names := make([]string, 0, len(types))
	for t := range types {
		names = append(names, t)
	}
	sort.Strings(names)
	var parts []string
	for _, t := range names {
		parts = append(parts, fmt.Sprintf("%s %d", t, types[t]))
	}
	note := strings.Join(parts, " · ")
	if im.flavor == FlavorLegacy {
		note += fmt.Sprintf(" · renumbered %d", renumbered)
	}
	stat.Note = note
	return nil
}

// readLegacyNodes reads the five per-protocol tables and assigns target ids (§3.2): rows keep their id unless
// an earlier row (in Xboard's merge order) took it; colliding rows get max(all legacy ids)+1, +2, …
func (im *importer) readLegacyNodes() ([]*srcNode, error) {
	var nodes []*srcNode
	var maxID uint64
	for _, t := range legacyServerTables {
		if !im.src.has(t.table) {
			continue
		}
		typ := t.typ
		err := im.src.each(im.ctx, t.table, "", nil, func(r row) error {
			n := &srcNode{typ: typ, srcID: r.id(), r: r}
			n.created, _ = r.int("created_at")
			if p, ok := r.int("parent_id"); ok && p > 0 {
				n.parent = uint64(p)
			}
			maxID = max(maxID, n.srcID)
			nodes = append(nodes, n)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	taken := map[uint64]bool{}
	next := maxID + 1
	for _, n := range nodes {
		if !taken[n.srcID] && n.srcID != 0 {
			n.id = n.srcID
		} else {
			n.id = next
			next++
			msg := fmt.Sprintf("%s #%d %q → node %d", n.typ, n.srcID, n.r.str("name"), n.id)
			im.rep.RenumberedNodes = append(im.rep.RenumberedNodes, msg)
			im.warn(n.scope(), "%s: change NodeID in this server's XrayR/soga config", msg)
		}
		taken[n.id] = true
		im.nodeMap[nodeKey{n.typ, n.srcID}] = n.id
	}
	return nodes, nil
}

// upstreamType normalises U types: v2ray → vmess, hysteria2 → hysteria; "" for types ggboard lacks.
func upstreamType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "v2ray":
		return model.NodeVmess
	case "hysteria2":
		return model.NodeHysteria
	}
	if model.IsValidNodeType(t) {
		return t
	}
	return ""
}

// readUpstreamNodes reads v2_server; nodes keep their id. Unsupported types (socks, naive, http) are skipped.
func (im *importer) readUpstreamNodes() ([]*srcNode, error) {
	var nodes []*srcNode
	err := im.src.each(im.ctx, "v2_server", "", nil, func(r row) error {
		n := &srcNode{srcID: r.id(), r: r, code: strings.TrimSpace(r.str("code"))}
		raw := r.str("type")
		n.typ = upstreamType(raw)
		if n.typ == "" {
			im.warn(scopeID("nodes", n.srcID), "type %q is not supported by ggboard — node %q skipped", raw, r.str("name"))
			nodes = append(nodes, n)
			return nil
		}
		n.id = n.srcID
		if t, ok := datetime(r["created_at"], im.loc); ok {
			n.created = t.Unix()
		}
		if p, ok := r.int("parent_id"); ok && p > 0 {
			n.parent = uint64(p)
		}
		im.nodeMap[nodeKey{n.typ, n.id}] = n.id
		if n.code != "" {
			if c, err := strconv.ParseUint(n.code, 10, 64); err == nil {
				if _, dup := im.nodeCodes[nodeKey{n.typ, c}]; !dup {
					im.nodeCodes[nodeKey{n.typ, c}] = n.id
				}
			}
		}
		nodes = append(nodes, n)
		return nil
	})
	return nodes, err
}

// mapNodeCommon maps the columns shared by every type and flavor.
func (im *importer) mapNodeCommon(n *srcNode) *model.Node {
	r := n.r
	scope := n.scope()
	node := &model.Node{
		ID:         n.id,
		Type:       n.typ,
		Name:       im.fit(scope, "name", r.str("name"), 191),
		Host:       im.fit(scope, "host", r.str("host"), 255),
		Port:       im.fit(scope, "port", strings.TrimSpace(r.str("port")), 32),
		ServerPort: int(r.i64("server_port")),
		Show:       r.bool("show"),
		Sort:       int(r.i64("sort")),
		Tags:       model.JSONSlice[string]{},
	}
	rate, ok := flexFloat(r["rate"])
	if !ok || rate < 0 {
		im.warn(scope, "rate %q is not a number → 1.0", r.str("rate"))
		rate = 1
	}
	node.Rate = rate
	tags, bad, err := parseStringList(r["tags"])
	if err != nil {
		im.warn(scope, "tags: %v → []", err)
	}
	for _, x := range bad {
		im.warn(scope, "tags: dropped non-string element %s", x)
	}
	node.Tags = tags

	groupCol, routeCol := "group_ids", "route_ids"
	if im.flavor == FlavorLegacy {
		groupCol, routeCol = "group_id", "route_id"
	}
	node.GroupIDs = im.idList(scope, groupCol, r[groupCol], func(id uint64) (bool, string) {
		if im.groups[id] {
			return true, ""
		}
		return false, "not an imported server group"
	})
	node.RouteIDs = im.idList(scope, routeCol, r[routeCol], func(id uint64) (bool, string) {
		switch {
		case im.routes[id]:
			return true, ""
		case im.skippedRoutes[id]:
			return false, "" // already warned when the route was skipped
		}
		return false, "not an imported route"
	})

	if im.flavor == FlavorLegacy {
		node.CreatedAt = im.ts(scope, r["created_at"])
		node.UpdatedAt = im.ts(scope, r["updated_at"])
		if !isEmptyJSON(r["excludes"]) {
			im.warn(scope, "excludes (hide from visitors in some regions) is not supported — dropped")
		}
		if strings.TrimSpace(r.str("ips")) != "" && !isEmptyJSON(r["ips"]) {
			im.warn(scope, "ips (per-IP traffic accounting of child nodes) is not supported — dropped; traffic is counted on the parent")
		}
		return node
	}

	if t, ok := datetime(r["created_at"], im.loc); ok {
		node.CreatedAt = t.UTC()
	} else {
		node.CreatedAt = im.start
	}
	if t, ok := datetime(r["updated_at"], im.loc); ok {
		node.UpdatedAt = t.UTC()
	} else {
		node.UpdatedAt = node.CreatedAt
	}
	if r.has("enabled") && r["enabled"] != nil && !r.bool("enabled") {
		node.Show = false
		im.warn(scope, "node is disabled in Xboard → show=false")
	}
	if n.code != "" && n.code != strconv.FormatUint(n.id, 10) {
		msg := fmt.Sprintf("backend may use NodeID %s; ggboard resolves by id → set NodeID=%d", n.code, n.id)
		im.rep.CodeNodes = append(im.rep.CodeNodes, fmt.Sprintf("%d (%s %q): %s", n.id, n.typ, node.Name, msg))
		im.warn(scope, "%s", msg)
	}
	if v, ok := r.int("machine_id"); ok && v > 0 {
		im.warn(scope, "machine_id %d dropped (no machine agent in ggboard)", v)
	}
	if v, ok := r.int("transfer_enable"); ok && v > 0 {
		im.warn(scope, "node traffic quota (transfer_enable %d) is not supported — dropped", v)
	}
	if r.bool("rate_time_enable") || !isEmptyJSON(r["rate_time_ranges"]) {
		im.warn(scope, "time-based rates (rate_time_ranges) are not supported — dropped; rate %.2f applies all day", node.Rate)
	}
	for _, c := range []string{"custom_outbounds", "custom_routes", "cert_config"} {
		if !isEmptyJSON(r[c]) {
			im.warn(scope, "%s is not supported — dropped", c)
		}
	}
	return node
}

// network normalises a transport: "" → tcp, splithttp → xhttp. Unsupported transports are kept but hide the
// node (show=false) with a warning.
func (im *importer) network(scope, typ, raw string) (string, bool) {
	net := strings.ToLower(strings.TrimSpace(raw))
	switch net {
	case "":
		return "tcp", false
	case "splithttp":
		net = "xhttp"
	}
	supported := map[string]bool{"tcp": true, "ws": true, "grpc": true, "http": true, "h2": true, "httpupgrade": true, "xhttp": true}
	if typ == model.NodeTrojan {
		supported = map[string]bool{"tcp": true, "ws": true, "grpc": true}
	}
	if supported[net] {
		return net, false
	}
	im.warn(scope, "%s transport %q is not supported by ggboard → kept with show=false", typ, net)
	return net, true
}

// jsonColumn decodes a JSON object column, warning on malformed JSON.
func (im *importer) jsonColumn(scope, col string, v any) map[string]any {
	obj, err := jsonObject(v)
	if err != nil {
		im.warn(scope, "%s: %v → {}", col, err)
		return nil
	}
	return obj
}

func firstPresent(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

// ---- legacy per-protocol columns ----

func (im *importer) legacySettings(n *srcNode, keyCreated int64) (model.JSONMap, bool) {
	r, scope := n.r, n.scope()
	switch n.typ {
	case model.NodeShadowsocks:
		cipher := strings.TrimSpace(r.str("cipher"))
		if !knownCiphers[cipher] {
			im.warn(scope, "cipher %q is not supported by ggboard clients — kept", cipher)
		}
		ps := model.JSONMap{"cipher": cipher, "obfs": "plain", "plugin": "", "plugin_opts": ""}
		if strings.EqualFold(strings.TrimSpace(r.str("obfs")), "http") {
			ps["obfs"] = "http"
			obj := im.jsonColumn(scope, "obfs_settings", r["obfs_settings"])
			ps["obfs_settings"] = map[string]any{"path": str(obj["path"]), "host": str(obj["host"])}
		}
		if size := model.SS2022KeySize(cipher); size > 0 {
			ps["server_key"] = serverKey(keyCreated, size)
		}
		return ps, false

	case model.NodeVmess:
		net, hide := im.network(scope, n.typ, r.str("network"))
		ps := model.JSONMap{"network": net, "tls": int(r.i64("tls"))}
		if ns := im.jsonColumn(scope, "networkSettings", r["networksettings"]); ns != nil {
			ps["network_settings"] = ns
		}
		tls := im.jsonColumn(scope, "tlsSettings", r["tlssettings"])
		ps["tls_settings"] = map[string]any{
			"server_name":    str(firstPresent(tls, "serverName", "server_name")),
			"allow_insecure": flexBool(firstPresent(tls, "allowInsecure", "allow_insecure")),
		}
		for _, c := range []string{"rules", "rulesettings", "dnssettings"} {
			if !isEmptyJSON(r[c]) {
				im.warn(scope, "vmess %s is not supported — dropped", c)
			}
		}
		return ps, hide

	case model.NodeTrojan:
		net, hide := im.network(scope, n.typ, r.str("network"))
		ps := model.JSONMap{"network": net, "server_name": r.str("server_name"), "allow_insecure": r.bool("allow_insecure")}
		if ns := im.jsonColumn(scope, "networkSettings", r["networksettings"]); ns != nil {
			ps["network_settings"] = ns
		}
		return ps, hide

	case model.NodeVless:
		net, hide := im.network(scope, n.typ, r.str("network"))
		tlsMode := int(r.i64("tls"))
		if tlsMode < 0 || tlsMode > 2 {
			im.warn(scope, "vless tls %d is not 0..2 → 0", tlsMode)
			tlsMode = 0
		}
		ps := model.JSONMap{"network": net, "tls": tlsMode, "flow": r.str("flow"), "encryption": "none"}
		if ns := im.jsonColumn(scope, "network_settings", r["network_settings"]); ns != nil {
			ps["network_settings"] = ns
		}
		ts := im.jsonColumn(scope, "tls_settings", r["tls_settings"])
		tls := map[string]any{
			"server_name":    str(ts["server_name"]),
			"allow_insecure": flexBool(firstPresent(ts, "allow_insecure", "allowInsecure")),
		}
		for _, k := range []string{"public_key", "private_key", "short_id", "server_port"} {
			if s := str(ts[k]); s != "" {
				tls[k] = s
			}
		}
		if tlsMode == model.TLSReality {
			port := str(ts["server_port"])
			if port == "" {
				port = "443"
			}
			tls["server_port"] = port
			tls["dest"] = str(ts["server_name"]) + ":" + port
			tls["fingerprint"] = "chrome"
		}
		ps["tls_settings"] = tls
		return ps, hide

	case model.NodeHysteria:
		version := 1
		if v, ok := r.int("version"); ok && v != 0 {
			version = int(v)
		} else {
			im.warn(scope, "hysteria version %q is NULL or 0 → 1", r.str("version"))
		}
		isObfs := true
		if r.has("is_obfs") {
			isObfs = r.bool("is_obfs")
		}
		ps := model.JSONMap{
			"version":        version,
			"up_mbps":        int(r.i64("up_mbps")),
			"down_mbps":      int(r.i64("down_mbps")),
			"server_name":    r.str("server_name"),
			"allow_insecure": r.bool("insecure"),
			"alpn":           "h3",
			"obfs":           "plain",
			"obfs_password":  "",
		}
		if version == 1 {
			alpn, ok := hysteriaALPN[r.i64("alpn")]
			if !ok {
				im.warn(scope, "hysteria alpn %d unknown → hysteria", r.i64("alpn"))
				alpn = "hysteria"
			}
			ps["alpn"] = alpn
		}
		if isObfs {
			ps["obfs"] = "salamander"
			ps["obfs_password"] = serverKey(keyCreated, 16)
		}
		return ps, false
	}
	return model.JSONMap{}, false
}

// ---- upstream protocol_settings JSON ----

func (im *importer) upstreamSettings(n *srcNode, keyCreated int64) (model.JSONMap, bool) {
	scope := n.scope()
	src := im.jsonColumn(scope, "protocol_settings", n.r["protocol_settings"])
	if src == nil {
		src = map[string]any{}
	}
	m := model.JSONMap(src)
	enabled := func(key string) bool { return flexBool(m.Map(key)["enabled"]) }
	tlsOf := func(key string) map[string]any {
		t := m.Map(key)
		return map[string]any{"server_name": str(t["server_name"]), "allow_insecure": flexBool(t["allow_insecure"])}
	}
	switch n.typ {
	case model.NodeShadowsocks:
		cipher := strings.TrimSpace(m.String("cipher"))
		if !knownCiphers[cipher] {
			im.warn(scope, "cipher %q is not supported by ggboard clients — kept", cipher)
		}
		obfs := strings.TrimSpace(m.String("obfs"))
		if obfs == "" {
			obfs = "plain"
		}
		ps := model.JSONMap{"cipher": cipher, "obfs": obfs, "plugin": m.String("plugin"), "plugin_opts": m.String("plugin_opts")}
		if os := m.Map("obfs_settings"); os != nil {
			ps["obfs_settings"] = map[string]any{"path": str(os["path"]), "host": str(os["host"])}
		}
		if size := model.SS2022KeySize(cipher); size > 0 {
			ps["server_key"] = serverKey(keyCreated, size)
		}
		return ps, false

	case model.NodeVmess:
		net, hide := im.network(scope, n.typ, m.String("network"))
		ps := model.JSONMap{"network": net, "tls": int(flexInt0(m["tls"])), "tls_settings": tlsOf("tls_settings")}
		if ns := m.Map("network_settings"); ns != nil {
			ps["network_settings"] = map[string]any(ns)
		}
		if !isEmptyAny(m["rules"]) {
			im.warn(scope, "vmess rules are not supported — dropped")
		}
		im.warnEnabled(scope, m, "multiplex", "utls")
		if flexBool(m.Map("tls_settings").Map("ech")["enabled"]) {
			im.warn(scope, "ECH is not supported — dropped")
		}
		return ps, hide

	case model.NodeTrojan:
		net, hide := im.network(scope, n.typ, m.String("network"))
		ts := m.Map("tls_settings")
		sni := str(ts["server_name"])
		if sni == "" {
			sni = m.String("server_name")
		}
		ps := model.JSONMap{
			"network":        net,
			"server_name":    sni,
			"allow_insecure": flexBool(ts["allow_insecure"]) || flexBool(m["allow_insecure"]),
		}
		if ns := m.Map("network_settings"); ns != nil {
			ps["network_settings"] = map[string]any(ns)
		}
		if flexInt0(m["tls"]) == model.TLSReality {
			im.warn(scope, "trojan over REALITY is not supported by ggboard → show=false")
			hide = true
		}
		im.warnEnabled(scope, m, "multiplex", "utls")
		return ps, hide

	case model.NodeVless:
		net, hide := im.network(scope, n.typ, m.String("network"))
		tlsMode := int(flexInt0(m["tls"]))
		ps := model.JSONMap{"network": net, "tls": tlsMode, "flow": m.String("flow"), "encryption": "none"}
		if ns := m.Map("network_settings"); ns != nil {
			ps["network_settings"] = map[string]any(ns)
		}
		if enabled("encryption") {
			im.warn(scope, "VLESS encryption is not supported — keys dropped, encryption=none")
		}
		switch tlsMode {
		case model.TLSReality:
			rs := m.Map("reality_settings")
			port := str(rs["server_port"])
			if port == "" {
				port = "443"
			}
			tls := map[string]any{
				"server_name":    str(rs["server_name"]),
				"allow_insecure": flexBool(rs["allow_insecure"]),
				"server_port":    port,
				"dest":           str(rs["server_name"]) + ":" + port,
				"fingerprint":    "chrome",
			}
			for _, k := range []string{"public_key", "private_key", "short_id"} {
				if s := str(rs[k]); s != "" {
					tls[k] = s
				}
			}
			if fp := strings.TrimSpace(str(m.Map("utls")["fingerprint"])); fp != "" {
				tls["fingerprint"] = fp
			}
			ps["tls_settings"] = tls
		case model.TLSEnabled:
			ps["tls_settings"] = tlsOf("tls_settings")
		}
		im.warnEnabled(scope, m, "multiplex")
		return ps, hide

	case model.NodeHysteria:
		version := int(flexInt0(m["version"]))
		if version == 0 {
			version = 2
		}
		bw := m.Map("bandwidth")
		obfs := m.Map("obfs")
		tls := m.Map("tls")
		ps := model.JSONMap{
			"version":        version,
			"up_mbps":        int(flexInt0(bw["up"])),
			"down_mbps":      int(flexInt0(bw["down"])),
			"server_name":    str(tls["server_name"]),
			"allow_insecure": flexBool(tls["allow_insecure"]),
			"obfs":           "plain",
			"obfs_password":  str(obfs["password"]),
		}
		if flexBool(obfs["open"]) {
			t := strings.TrimSpace(str(obfs["type"]))
			if t == "" {
				t = "salamander"
			}
			ps["obfs"] = t
			if ps["obfs_password"] == "" {
				im.warn(scope, "hysteria obfs is enabled with an empty password")
			}
		}
		if hop, ok := flexInt(m["hop_interval"]); ok && hop > 0 {
			ps["hop_interval"] = int(hop)
		}
		return ps, false

	case model.NodeTuic:
		tls := m.Map("tls")
		alpn, _, _ := parseStringList(m["alpn"])
		ps := model.JSONMap{
			"congestion_control": m.String("congestion_control"),
			"udp_relay_mode":     m.String("udp_relay_mode"),
			"alpn":               strings.Join(alpn, ","),
			"server_name":        str(tls["server_name"]),
			"allow_insecure":     flexBool(tls["allow_insecure"]),
			"zero_rtt_handshake": false,
		}
		if v, ok := flexInt(m["version"]); ok && v != 5 {
			im.warn(scope, "tuic version %d: ggboard serves TUIC v5 only", v)
		}
		return ps, false

	case model.NodeAnyTLS:
		tls := m.Map("tls")
		padding, _, _ := parseStringList(m["padding_scheme"])
		return model.JSONMap{
			"tls":            model.TLSEnabled,
			"server_name":    str(tls["server_name"]),
			"allow_insecure": flexBool(tls["allow_insecure"]),
			"padding_scheme": padding,
		}, false

	case model.NodeMieru:
		transport := strings.TrimSpace(m.String("transport"))
		if transport == "" {
			transport = "TCP"
		}
		if strings.TrimSpace(m.String("traffic_pattern")) != "" {
			im.warn(scope, "mieru traffic_pattern is not supported — dropped")
		}
		im.warnEnabled(scope, m, "multiplex")
		return model.JSONMap{"transport": transport}, false

	case model.NodeShadowsocksR:
		return model.JSONMap(src), false
	}
	return model.JSONMap{}, false
}

// warnEnabled warns for each option block that is enabled in Xboard but dropped by the importer.
func (im *importer) warnEnabled(scope string, m model.JSONMap, keys ...string) {
	for _, k := range keys {
		if flexBool(m.Map(k)["enabled"]) {
			im.warn(scope, "%s is enabled in Xboard but not supported by ggboard — dropped", k)
		}
	}
}

func flexInt0(v any) int64 {
	n, _ := flexInt(v)
	return n
}

func isEmptyAny(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	case string:
		return strings.TrimSpace(x) == ""
	}
	return false
}
