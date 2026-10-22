package formats

import (
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// singbox renders a sing-box >= 1.12 configuration: typed DNS servers, rule actions (sniff, hijack-dns,
// reject) instead of the removed block/dns outbounds, remote rule sets and selector/urltest groups.
// sing-box has no shadowsocksr, mieru, xhttp or tcp-http-header transport.
type singboxFormat struct{}

const sbDirect = "direct"

var singboxTypes = typeSet(model.NodeShadowsocks, model.NodeVmess, model.NodeVless, model.NodeTrojan,
	model.NodeHysteria, model.NodeTuic, model.NodeAnyTLS)

func (singboxFormat) Name() string           { return subscription.FormatSingbox }
func (singboxFormat) Supports(t string) bool { return singboxTypes[t] }
func (singboxFormat) RuleFamily() string     { return subscription.FormatSingbox }
func (singboxFormat) contentType() string    { return subscription.ContentTypeJSON }
func (singboxFormat) SupportsProxy(p *subscription.Proxy) bool {
	switch p.Type {
	case model.NodeShadowsocks:
		k := ssPlugin(p).Kind
		return k != "unknown" && k != "shadow-tls"
	case model.NodeVmess, model.NodeVless, model.NodeTrojan:
		switch p.Network {
		case "ws", "grpc", "http", "h2", "httpupgrade":
			return true
		case "tcp":
			return !isHTTPHeaderTCP(p)
		}
		return false
	}
	return true
}

func (f singboxFormat) Render(ctx *subscription.RenderContext) ([]byte, string, error) {
	root := jobj{{"log", jobj{{"level", "warn"}, {"timestamp", true}}}}
	plan := ctx.Rules
	dnsOn := ctx.Profile.DNS != subscription.DNSNone && ctx.Profile.DNS != ""
	fake := ctx.Profile.DNS == subscription.DNSFakeIP
	if dnsOn {
		root.set("dns", f.dns(ctx, fake))
	}
	root.set("inbounds", []jobj{
		{{"type", "tun"}, {"tag", "tun-in"}, {"address", []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"}},
			{"auto_route", true}, {"strict_route", true}, {"stack", "mixed"}},
		{{"type", "mixed"}, {"tag", "mixed-in"}, {"listen", "127.0.0.1"}, {"listen_port", 2080}},
	})

	outbounds := make([]jobj, 0, len(ctx.Groups)+len(ctx.Proxies)+1)
	for _, g := range ctx.Groups {
		members := memberNames(g, sbDirect, "")
		if g.IsHealthChecked() {
			outbounds = append(outbounds, jobj{{"type", "urltest"}, {"tag", g.Name}, {"outbounds", members},
				{"url", g.URL}, {"interval", strconv.Itoa(g.Interval) + "s"}, {"tolerance", 50}})
			continue
		}
		outbounds = append(outbounds, jobj{{"type", "selector"}, {"tag", g.Name}, {"outbounds", members}})
	}
	for i := range ctx.Proxies {
		outbounds = append(outbounds, f.proxy(&ctx.Proxies[i]))
	}
	outbounds = append(outbounds, jobj{{"type", "direct"}, {"tag", sbDirect}})
	root.set("outbounds", outbounds)

	rules := []jobj{
		{{"action", "sniff"}},
		{{"protocol", "dns"}, {"action", "hijack-dns"}},
		{{"ip_is_private", true}, {"outbound", sbDirect}},
	}
	if h := hostRule(ctx); h != "" {
		rules = append(rules, jobj{{"domain", []string{h}}, {"outbound", sbDirect}})
	}
	var ruleSets []jobj
	final := subscription.GroupProxy
	if plan != nil {
		detour := subscription.GroupProxy
		if plan.MirrorPrefix != "" {
			detour = sbDirect
		}
		addSet := func(s subscription.RuleSet) {
			ruleSets = append(ruleSets, jobj{{"type", "remote"}, {"tag", s.Tag}, {"format", s.Format}, {"url", s.URL},
				{"download_detour", detour}, {"update_interval", "24h"}})
		}
		for _, group := range plan.CategoryGroups() {
			tags := make([]string, 0, len(group))
			for _, s := range group {
				addSet(s)
				tags = append(tags, s.Tag)
			}
			rules = append(rules, sbRouteRule(jobj{{"rule_set", tags}}, group[0].Target))
		}
		if plan.GeoIPCN != nil {
			addSet(*plan.GeoIPCN)
			rules = append(rules, jobj{{"rule_set", []string{plan.GeoIPCN.Tag}}, {"outbound", sbDirect}})
		}
		final = plan.Final
	}
	switch final {
	case subscription.PolicyReject:
		rules = append(rules, jobj{{"action", "reject"}})
		final = sbDirect
	case subscription.PolicyDirect:
		final = sbDirect
	}
	route := jobj{{"rules", rules}}
	if len(ruleSets) > 0 {
		route.set("rule_set", ruleSets)
	}
	route.set("final", final)
	route.set("auto_detect_interface", true)
	if dnsOn {
		route.set("default_domain_resolver", "local")
	}
	root.set("route", route)
	cache := jobj{{"enabled", true}}
	cache.setTrue("store_fakeip", fake)
	root.set("experimental", jobj{{"cache_file", cache}})

	body, err := marshalIndent(root)
	if err != nil {
		return nil, "", err
	}
	return body, f.contentType(), nil
}

// sbRouteRule finishes a route rule for target (REJECT becomes the reject action).
func sbRouteRule(r jobj, target string) jobj {
	switch target {
	case subscription.PolicyReject:
		r.set("action", "reject")
	case subscription.PolicyDirect:
		r.set("outbound", sbDirect)
	default:
		r.set("outbound", target)
	}
	return r
}

func (singboxFormat) dns(ctx *subscription.RenderContext, fake bool) jobj {
	servers := []jobj{
		{{"type", "https"}, {"tag", "remote"}, {"server", "1.1.1.1"}, {"detour", subscription.GroupProxy}},
		{{"type", "https"}, {"tag", "local"}, {"server", "223.5.5.5"}},
	}
	if fake {
		servers = append(servers, jobj{{"type", "fakeip"}, {"tag", "fakeip"}, {"inet4_range", "198.18.0.0/15"}, {"inet6_range", "fc00::/18"}})
	}
	var rules []jobj
	if h := hostRule(ctx); h != "" {
		rules = append(rules, jobj{{"domain", []string{h}}, {"server", "local"}})
	}
	if ctx.Rules != nil {
		var direct []string
		for _, s := range ctx.Rules.Sets {
			if s.Target == subscription.PolicyDirect && s.Behavior != subscription.BehaviorIPCIDR {
				direct = append(direct, s.Tag)
			}
		}
		if len(direct) > 0 {
			rules = append(rules, jobj{{"rule_set", direct}, {"server", "local"}})
		}
	}
	if fake {
		rules = append(rules, jobj{{"query_type", []string{"A", "AAAA"}}, {"server", "fakeip"}})
	}
	d := jobj{{"servers", servers}}
	if len(rules) > 0 {
		d.set("rules", rules)
	}
	d.set("final", "remote")
	return d
}

func sbTLS(p *subscription.Proxy, alpn []string) jobj {
	t := jobj{{"enabled", true}}
	t.setStr("server_name", p.SNI())
	t.setTrue("insecure", p.Insecure())
	if len(alpn) > 0 {
		t.set("alpn", alpn)
	}
	if fp := p.Fingerprint(); fp != "" {
		t.set("utls", jobj{{"enabled", true}, {"fingerprint", fp}})
	}
	if pk, sid, ok := p.Reality(); ok {
		t.set("reality", jobj{{"enabled", true}, {"public_key", pk}, {"short_id", sid}})
	}
	return t
}

func sbTransport(p *subscription.Proxy) jobj {
	n := p.Net
	switch p.Network {
	case "ws":
		t := jobj{{"type", "ws"}, {"path", first(n.Path, "/")}}
		if n.Host != "" {
			t.set("headers", jobj{{"Host", n.Host}})
		}
		return t
	case "grpc":
		return jobj{{"type", "grpc"}, {"service_name", n.ServiceName}}
	case "http", "h2":
		t := jobj{{"type", "http"}}
		if len(n.Hosts) > 0 {
			t.set("host", n.Hosts)
		}
		t.set("path", first(n.Path, "/"))
		return t
	case "httpupgrade":
		t := jobj{{"type", "httpupgrade"}}
		t.setStr("host", n.Host)
		t.set("path", first(n.Path, "/"))
		return t
	}
	return nil
}

// sbPorts converts "20000-30000,443" to sing-box server_ports ["20000:30000", "443:443"].
func sbPorts(spec string) []string {
	var out []string
	for _, part := range strings.Split(spec, ",") {
		lo, hi, ok := strings.Cut(part, "-")
		if !ok {
			hi = lo
		}
		out = append(out, lo+":"+hi)
	}
	return out
}

func (singboxFormat) proxy(p *subscription.Proxy) jobj {
	o := jobj{{"type", ""}, {"tag", p.Name}, {"server", p.Host}, {"server_port", p.Port}}
	setType := func(t string) { o[0].v = t }
	switch p.Type {
	case model.NodeShadowsocks:
		setType("shadowsocks")
		o.set("method", p.SS.Cipher)
		o.set("password", p.Password)
		switch pi := ssPlugin(p); pi.Kind {
		case "obfs":
			opts := "obfs=" + pi.Mode
			if pi.Host != "" {
				opts += ";obfs-host=" + pi.Host
			}
			o.set("plugin", "obfs-local")
			o.set("plugin_opts", opts)
		case "v2ray-plugin":
			opts := "mode=websocket"
			if pi.TLS {
				opts += ";tls"
			}
			if pi.Host != "" {
				opts += ";host=" + pi.Host
			}
			opts += ";path=" + pi.Path
			o.set("plugin", "v2ray-plugin")
			o.set("plugin_opts", opts)
		}
	case model.NodeVmess:
		setType("vmess")
		o.set("uuid", p.UUID)
		o.set("security", "auto")
		o.set("alter_id", 0)
		if p.TLSMode() != model.TLSNone {
			o.set("tls", sbTLS(p, nil))
		}
		if t := sbTransport(p); t != nil {
			o.set("transport", t)
		}
	case model.NodeVless:
		setType("vless")
		o.set("uuid", p.UUID)
		o.setStr("flow", p.Vless.Flow)
		o.set("packet_encoding", "xudp")
		if p.TLSMode() != model.TLSNone {
			o.set("tls", sbTLS(p, nil))
		}
		if t := sbTransport(p); t != nil {
			o.set("transport", t)
		}
	case model.NodeTrojan:
		setType("trojan")
		o.set("password", p.Password)
		o.set("tls", sbTLS(p, nil))
		if t := sbTransport(p); t != nil {
			o.set("transport", t)
		}
	case model.NodeHysteria:
		h := p.Hysteria
		if p.Ports != "" {
			o = o[:3] // server_ports conflicts with server_port
			o.set("server_ports", sbPorts(p.Ports))
			if h.HopInterval > 0 {
				o.set("hop_interval", strconv.Itoa(h.HopInterval)+"s")
			}
		}
		if p.IsHysteria2() {
			setType("hysteria2")
			if !h.IgnoreClientBandwidth && h.UpMbps > 0 && h.DownMbps > 0 {
				o.set("up_mbps", h.UpMbps)
				o.set("down_mbps", h.DownMbps)
			}
			if obfs := p.HysteriaObfs(); obfs != "" {
				o.set("obfs", jobj{{"type", "salamander"}, {"password", obfs}})
			}
			o.set("password", p.Password)
		} else {
			setType("hysteria")
			o.set("up_mbps", max(h.UpMbps, 1))
			o.set("down_mbps", max(h.DownMbps, 1))
			o.setStr("obfs", p.HysteriaObfs())
			o.set("auth_str", p.Password)
		}
		o.set("tls", sbTLS(p, nil))
	case model.NodeTuic:
		setType("tuic")
		o.set("uuid", p.UUID)
		o.set("password", p.Password)
		o.set("congestion_control", p.Tuic.CongestionControl)
		o.set("udp_relay_mode", p.Tuic.UDPRelayMode)
		o.setTrue("zero_rtt_handshake", p.Tuic.ZeroRTTHandshake)
		o.set("heartbeat", "10s")
		o.set("tls", sbTLS(p, p.ALPN()))
	case model.NodeAnyTLS:
		setType("anytls")
		o.set("password", p.Password)
		o.set("tls", sbTLS(p, nil))
	}
	return o
}

func init() { subscription.RegisterFormat(singboxFormat{}) }
