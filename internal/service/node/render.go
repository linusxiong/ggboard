package nodesvc

import (
	"crypto/md5" //nolint:gosec // Xboard-compatible key derivation (docs/NODE_PROTOCOLS.md §4)
	"encoding/base64"
	"encoding/hex"
	"net"
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
)

// DefaultPaddingScheme is the anytls padding scheme sent when protocol_settings.padding_scheme is empty
// (upstream Xboard default).
var DefaultPaddingScheme = []string{
	"stop=8", "0=30-30", "1=100-400", "2=400-500,c,500-1000,c,500-1000,c,500-1000,c,500-1000",
	"3=9-9,500-1000", "4=500-1000", "5=500-1000", "6=500-1000", "7=500-1000",
}

func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func mapOrNil(m model.JSONMap) any {
	if len(m) == 0 {
		return nil
	}
	return m
}

// stringList converts a JSON array value to []string (non-string entries are skipped; never nil).
func stringList(v any) []string {
	out := []string{}
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	case []string:
		for _, s := range x {
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// isV2Type reports whether the node type carries a transport ("network").
func isV2Type(t string) bool {
	return t == model.NodeVmess || t == model.NodeVless || t == model.NodeTrojan
}

// Network returns protocol_settings.network for vmess/vless/trojan (default "tcp"), or "" for other types.
func Network(n *model.Node) string {
	if !isV2Type(n.Type) {
		return ""
	}
	if v := n.ProtocolSettings.String("network"); v != "" {
		return v
	}
	return "tcp"
}

// SplitDest splits a reality handshake target into host and port (docs/NODE_PROTOCOLS.md §2.1.3): the host
// part of dest (or dest itself without a port, or fallback when empty) and the port part of dest, else
// serverPort, else "443".
func SplitDest(dest, serverPort, fallback string) (host, port string) {
	host = strings.TrimSpace(dest)
	if h, p, err := net.SplitHostPort(host); err == nil {
		host, port = h, p
	}
	if host == "" {
		host = fallback
	}
	if port == "" {
		port = strings.TrimSpace(serverPort)
	}
	if port == "" {
		port = "443"
	}
	return host, port
}

func tlsBasic(serverName string, allowInsecure bool) Object {
	return Object{{"server_name", serverName}, {"allow_insecure", allowInsecure}}
}

// reality renders the reality tls_settings object (server_name, dest host, server_port string, keys).
func reality(ts model.JSONMap, serverName string) Object {
	host, port := SplitDest(ts.String("dest"), ts.String("server_port"), serverName)
	o := Object{
		{"server_name", serverName},
		{"dest", host},
		{"server_port", port},
		{"public_key", ts.String("public_key")},
		{"private_key", ts.String("private_key")},
		{"short_id", ts.String("short_id")},
		{"allow_insecure", ts.Bool("allow_insecure")},
	}
	if x := ts.Int("xver"); x > 0 {
		o.Add("xver", strconv.Itoa(x)) // V2bX declares `xver,string`
	}
	return o
}

// SSRNodePassword returns protocol_settings.password of a shadowsocksr node, or base64(md5hex(created_at)[:16]).
func SSRNodePassword(p *model.Node) string {
	if pw := p.ProtocolSettings.String("password"); pw != "" {
		return pw
	}
	sum := md5.Sum([]byte(strconv.FormatInt(p.CreatedAt.Unix(), 10))) //nolint:gosec // Xboard-compatible derivation
	return base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(sum[:])[:16]))
}

// RouteView is one entry of the UniProxy routes array.
type RouteView struct {
	ID          uint64   `json:"id"`
	Match       []string `json:"match"`
	Action      string   `json:"action"`
	ActionValue string   `json:"action_value"`
}

// RouteViews converts node_routes to the UniProxy shape (match never null).
func RouteViews(routes []model.NodeRoute) []RouteView {
	out := make([]RouteView, 0, len(routes))
	for _, r := range routes {
		m := []string(r.Match)
		if m == nil {
			m = []string{}
		}
		out = append(out, RouteView{ID: r.ID, Match: m, Action: r.Action, ActionValue: r.ActionValue})
	}
	return out
}

// vlessDecryption returns protocol_settings.decryption, treating "none" as unset.
func vlessDecryption(ps model.JSONMap) string {
	d := strings.TrimSpace(ps.String("decryption"))
	if d == "none" {
		return ""
	}
	return d
}

// RenderUniProxyConfig renders GET /api/v1/server/UniProxy/config for the effective node p
// (docs/NODE_PROTOCOLS.md §2.1). routes are p's node_routes sorted by id.
func RenderUniProxyConfig(p *model.Node, routes []model.NodeRoute, iv Intervals) Object {
	ps := p.ProtocolSettings
	ns := mapOrNil(ps.Map("network_settings"))
	o := Object{
		{"protocol", p.Type},
		{"listen_ip", "0.0.0.0"},
		{"server_port", p.ServerPort},
		{"host", p.Host},
		{"network", strOrNil(Network(p))},
		{"networkSettings", ns},
		{"network_settings", ns},
	}
	ts := ps.Map("tls_settings")
	switch p.Type {
	case model.NodeShadowsocks:
		cipher := ps.String("cipher")
		var serverKey any
		if model.SS2022KeySize(cipher) > 0 {
			serverKey = model.NodeServerKey(p)
		}
		o = append(o,
			Field{"cipher", cipher},
			Field{"server_key", serverKey},
			Field{"obfs", strOrNil(ps.String("obfs"))},
			Field{"obfs_settings", mapOrNil(ps.Map("obfs_settings"))},
			Field{"plugin", strOrNil(ps.String("plugin"))},
			Field{"plugin_opts", strOrNil(ps.String("plugin_opts"))},
		)
	case model.NodeVmess:
		tls := tlsBasic(ts.String("server_name"), ts.Bool("allow_insecure"))
		o = append(o, Field{"tls", ps.Int("tls")}, Field{"tls_settings", tls}, Field{"tlsSettings", tls})
	case model.NodeVless:
		mode := ps.Int("tls")
		var tls Object
		if mode == model.TLSReality {
			tls = reality(ts, ts.String("server_name"))
		} else {
			tls = tlsBasic(ts.String("server_name"), ts.Bool("allow_insecure"))
		}
		dec := vlessDecryption(ps)
		o = append(o,
			Field{"tls", mode},
			Field{"flow", ps.String("flow")},
			Field{"server_name", ts.String("server_name")},
			Field{"tls_settings", tls},
			Field{"tlsSettings", tls},
			Field{"decryption", strOrNil(dec)},
		)
		if seg := strings.Split(dec, "."); dec != "" && len(seg) >= 4 {
			n := len(seg)
			o = append(o,
				Field{"encryption", seg[0]},
				Field{"encryption_settings", Object{
					{"mode", seg[1]},
					{"ticket", seg[2]},
					{"server_padding", strings.Join(seg[3:n-1], ".")},
					{"private_key", seg[n-1]},
				}},
			)
		}
	case model.NodeTrojan:
		sn := ps.String("server_name")
		o = append(o, Field{"server_name", sn}, Field{"tls", 1}, Field{"tls_settings", tlsBasic(sn, ps.Bool("allow_insecure"))})
	case model.NodeHysteria:
		version := HysteriaVersion(p)
		sn := ps.String("server_name")
		pw := ps.String("obfs_password")
		o = append(o,
			Field{"version", version},
			Field{"server_name", sn},
			Field{"tls_settings", tlsBasic(sn, ps.Bool("allow_insecure"))},
			Field{"up_mbps", ps.Int("up_mbps")},
			Field{"down_mbps", ps.Int("down_mbps")},
		)
		if version == 2 {
			var obfs any
			if ps.String("obfs") == "salamander" && pw != "" {
				obfs = "salamander"
			}
			o = append(o, Field{"obfs", obfs}, Field{"obfs-password", strOrNil(pw)}, Field{"ignore_client_bandwidth", ps.Bool("ignore_client_bandwidth")})
		} else {
			var obfs any
			if pw != "" && ps.String("obfs") != "plain" {
				obfs = pw
			}
			o = append(o, Field{"obfs", obfs})
		}
	case model.NodeTuic:
		sn := ps.String("server_name")
		cc := ps.String("congestion_control")
		if cc == "" {
			cc = "cubic"
		}
		relay := ps.String("udp_relay_mode")
		if relay == "" {
			relay = "native"
		}
		alpn := []string{}
		for _, a := range strings.Split(ps.String("alpn"), ",") {
			if a = strings.TrimSpace(a); a != "" {
				alpn = append(alpn, a)
			}
		}
		if len(alpn) == 0 {
			alpn = []string{"h3"}
		}
		o = append(o,
			Field{"version", 5},
			Field{"server_name", sn},
			Field{"congestion_control", cc},
			Field{"zero_rtt_handshake", ps.Bool("zero_rtt_handshake")},
			Field{"udp_relay_mode", relay},
			Field{"alpn", alpn},
			Field{"auth_timeout", "3s"},
			Field{"heartbeat", "3s"},
			Field{"tls_settings", tlsBasic(sn, ps.Bool("allow_insecure"))},
		)
	case model.NodeAnyTLS:
		mode := anyTLSMode(ps)
		sn := ps.String("server_name")
		var tls Object
		if mode == model.TLSReality {
			tls = reality(ts, anyTLSRealityName(ps))
		} else {
			tls = tlsBasic(sn, ps.Bool("allow_insecure"))
		}
		padding := stringList(ps["padding_scheme"])
		if len(padding) == 0 {
			padding = DefaultPaddingScheme
		}
		o = append(o, Field{"tls", mode}, Field{"server_name", sn}, Field{"tls_settings", tls}, Field{"padding_scheme", padding})
	case model.NodeMieru:
		transport := ps.String("transport")
		if transport == "" {
			transport = "TCP"
		}
		o = append(o, Field{"transport", transport}, Field{"traffic_pattern", ""}, Field{"multiplexing", ps.String("multiplexing")})
	}
	o = append(o,
		Field{"base_config", Object{{"push_interval", iv.Push}, {"pull_interval", iv.Pull}}},
		Field{"routes", RouteViews(routes)},
	)
	return o
}

// anyTLSMode returns protocol_settings.tls of an anytls node (default 1 when absent).
func anyTLSMode(ps model.JSONMap) int {
	if _, ok := ps["tls"]; !ok {
		return model.TLSEnabled
	}
	return ps.Int("tls")
}

// anyTLSServerNames returns tls_settings.server_names, or [server_name].
func anyTLSServerNames(ps model.JSONMap) []string {
	names := stringList(ps.Map("tls_settings")["server_names"])
	if len(names) == 0 {
		names = []string{ps.String("server_name")}
	}
	return names
}

func anyTLSRealityName(ps model.JSONMap) string { return anyTLSServerNames(ps)[0] }

func sogaTLSType(mode int) string {
	switch mode {
	case model.TLSEnabled:
		return "tls"
	case model.TLSReality:
		return "reality"
	}
	return "none"
}

// sogaStreamExtras returns the optional stream keys of a soga config (path / service_name / xhttp_settings).
func sogaStreamExtras(network string, ns model.JSONMap) Object {
	switch network {
	case "ws", "http", "h2", "httpupgrade":
		return Object{{"path", ns.String("path")}}
	case "grpc":
		name := ns.String("serviceName")
		if name == "" {
			name = ns.String("service_name")
		}
		return Object{{"service_name", name}}
	case "xhttp":
		mode := ns.String("mode")
		if mode == "" {
			mode = "auto"
		}
		extra := ns.Map("extra")
		if extra == nil {
			extra = model.JSONMap{}
		}
		return Object{{"xhttp_settings", Object{{"host", ns.String("host")}, {"path", ns.String("path")}, {"mode", mode}, {"extra", extra}}}}
	}
	return nil
}

func sogaSSObfs(obfs string) string {
	switch obfs {
	case "http":
		return "simple_obfs_http"
	case "tls":
		return "simple_obfs_tls"
	}
	return "plain"
}

// sogaReality returns the soga reality keys (server_names, private_key, short_ids, dest "host:port").
func sogaReality(ts model.JSONMap, names []string) Object {
	host, port := SplitDest(ts.String("dest"), ts.String("server_port"), names[0])
	return Object{
		{"server_names", names},
		{"private_key", ts.String("private_key")},
		{"short_ids", []string{ts.String("short_id")}},
		{"dest", net.JoinHostPort(host, port)},
	}
}

// RenderSogaNode renders GET /api/v1/node for the effective node p (docs/NODE_PROTOCOLS.md §3.1).
func RenderSogaNode(p *model.Node, iv Intervals) Object {
	ps := p.ProtocolSettings
	ts := ps.Map("tls_settings")
	ns := ps.Map("network_settings")
	cfg := Object{{"port", p.ServerPort}}
	switch p.Type {
	case model.NodeVmess:
		network := Network(p)
		mode := sogaTLSType(min(ps.Int("tls"), model.TLSEnabled))
		cfg = append(cfg, Field{"stream_type", network}, Field{"tls_type", mode})
		cfg = append(cfg, sogaStreamExtras(network, ns)...)
	case model.NodeTrojan:
		network := Network(p)
		cfg = append(cfg, Field{"stream_type", network})
		cfg = append(cfg, sogaStreamExtras(network, ns)...)
	case model.NodeVless:
		network := Network(p)
		mode := ps.Int("tls")
		cfg = append(cfg,
			Field{"stream_type", network},
			Field{"tls_type", sogaTLSType(mode)},
			Field{"flow", ps.String("flow")},
			Field{"decryption", ps.String("decryption")},
		)
		cfg = append(cfg, sogaStreamExtras(network, ns)...)
		if mode == model.TLSReality {
			cfg = append(cfg, sogaReality(ts, []string{ts.String("server_name")})...)
		}
	case model.NodeShadowsocks:
		cipher := ps.String("cipher")
		pw := ""
		if model.SS2022KeySize(cipher) > 0 {
			pw = model.NodeServerKey(p)
		}
		obfs := sogaSSObfs(ps.String("obfs"))
		cfg = append(cfg, Field{"cipher", cipher}, Field{"password", pw}, Field{"obfs", obfs})
		if obfs != "plain" {
			obfsSet := ps.Map("obfs_settings")
			cfg = append(cfg, Field{"path", obfsSet.String("path")}, Field{"host", obfsSet.String("host")})
		}
	case model.NodeShadowsocksR:
		single := ps.String("single_port_type")
		if single == "" {
			single = "protocol"
		}
		var ssr model.ShadowsocksRSettings
		_ = ssr.Decode(ps) // defaults for missing keys; invalid types were rejected by admin validation
		cfg = append(cfg,
			Field{"method", ssr.Cipher},
			Field{"password", SSRNodePassword(p)},
			Field{"protocol", ssr.Protocol},
			Field{"obfs", ssr.Obfs},
			Field{"single_port_type", single},
		)
	case model.NodeHysteria:
		pw := ps.String("obfs_password")
		obfs := "plain"
		if ps.String("obfs") == "salamander" && pw != "" {
			obfs = "salamander"
		}
		cfg = append(cfg, Field{"obfs", obfs}, Field{"obfs_password", pw}, Field{"up_mbps", ps.Int("up_mbps")}, Field{"down_mbps", ps.Int("down_mbps")})
	case model.NodeAnyTLS:
		mode := anyTLSMode(ps)
		cfg = append(cfg, Field{"tls_type", sogaTLSType(mode)})
		if mode == model.TLSReality {
			cfg = append(cfg, sogaReality(ts, anyTLSServerNames(ps))...)
		}
		cfg = append(cfg, Field{"padding_scheme", stringList(ps["padding_scheme"])})
	case model.NodeMieru:
		transport := ps.String("transport")
		if transport == "" {
			transport = "TCP"
		}
		cfg = append(cfg, Field{"transport", transport})
	}
	return Object{
		{"basic", Object{{"pull_interval", iv.Pull}, {"push_interval", iv.Push}, {"speed_limit", 0}}},
		{"config", cfg},
	}
}

// AuditRule is one soga audit rule.
type AuditRule struct {
	ID   uint64 `json:"id"`
	Rule string `json:"rule"`
}

// AuditRuleID is the soga audit id of match entry i of route routeID (route.id × 1000 + i).
func AuditRuleID(routeID uint64, i int) uint64 { return routeID*1000 + uint64(i) }

// RenderAuditRules renders GET /api/v1/audit_rules: one entry per match line of every block route, skipping
// "protocol:" entries (docs/NODE_PROTOCOLS.md §3.3). routes must be sorted by id.
func RenderAuditRules(routes []model.NodeRoute) []AuditRule {
	out := []AuditRule{}
	for _, r := range routes {
		if r.Action != model.RouteActionBlock {
			continue
		}
		for i, m := range r.Match {
			if m == "" || strings.HasPrefix(m, "protocol:") || i >= 1000 {
				continue
			}
			out = append(out, AuditRule{ID: AuditRuleID(r.ID, i), Rule: m})
		}
	}
	return out
}
