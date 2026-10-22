package formats

import (
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
)

// clashFlavor selects the Clash dialect: mihomo (Clash Meta), Stash or the classic Clash core.
type clashFlavor int

const (
	flavorMeta clashFlavor = iota
	flavorStash
	flavorClassic
)

// clashFormat renders Clash-family YAML (clash-meta, stash, clash).
type clashFormat struct {
	name   string
	flavor clashFlavor
	types  map[string]bool
}

func (f clashFormat) Name() string                  { return f.name }
func (f clashFormat) Supports(nodeType string) bool { return f.types[nodeType] }

// RuleFamily implements subscription.RuleConsumer (classic Clash has no rule providers).
func (f clashFormat) RuleFamily() string {
	if f.flavor == flavorClassic {
		return ""
	}
	return f.name
}

// SupportsAge implements subscription.AgeCapable: mihomo >= 1.19.27 decrypts armored age configs and proxy
// providers (age-secret-key), as do Clash Party and FlClash; Stash and the classic core do not.
func (f clashFormat) SupportsAge() bool { return f.flavor == flavorMeta }

// SupportsProxy drops configurations the dialect cannot express.
func (f clashFormat) SupportsProxy(p *subscription.Proxy) bool {
	pi := ssPlugin(p)
	switch p.Type {
	case model.NodeShadowsocks:
		if pi.Kind == "unknown" || (f.flavor == flavorClassic && (p.SS.Is2022() || pi.Kind == "shadow-tls")) {
			return false
		}
		if f.flavor == flavorStash && p.SS.Cipher == "2022-blake3-chacha20-poly1305" {
			return false
		}
	case model.NodeVmess:
		switch p.Network {
		case "tcp", "ws", "grpc", "http", "h2":
		case "httpupgrade":
			return f.flavor == flavorMeta
		default:
			return false
		}
	case model.NodeVless:
		switch p.Network {
		case "tcp", "ws", "grpc", "http", "h2":
		case "httpupgrade":
			return f.flavor == flavorMeta
		case "xhttp":
			return f.flavor != flavorClassic
		default:
			return false
		}
	case model.NodeTrojan:
		switch p.Network {
		case "tcp", "ws", "grpc":
		case "httpupgrade":
			return f.flavor == flavorMeta
		default:
			return false
		}
	case model.NodeAnyTLS:
		return p.TLSMode() != model.TLSReality // mihomo has no AnyTLS+REALITY
	}
	return true
}

func (f clashFormat) Render(ctx *subscription.RenderContext) ([]byte, string, error) {
	var root ymap
	switch f.flavor {
	case flavorMeta:
		root.set("mixed-port", 7890)
		root.set("allow-lan", false)
		root.set("mode", "rule")
		root.set("log-level", "info")
		root.set("unified-delay", true)
		root.set("tcp-concurrent", true)
		if ctx.Mirror != "" {
			root.set("geox-url", geoxURL(ctx.Mirror))
		}
	case flavorStash:
		root.set("mode", "rule")
		root.set("log-level", "info")
	default:
		root.set("mixed-port", 7890)
		root.set("allow-lan", false)
		root.set("mode", "rule")
		root.set("log-level", "info")
	}
	if dns := f.dns(ctx.Profile.DNS); dns != nil {
		root.set("dns", dns)
	}
	proxies := make(ylist, 0, len(ctx.Proxies))
	for i := range ctx.Proxies {
		proxies = append(proxies, f.proxy(&ctx.Proxies[i]))
	}
	root.set("proxies", proxies)
	groups := make(ylist, 0, len(ctx.Groups))
	for _, g := range ctx.Groups {
		groups = append(groups, f.group(g))
	}
	root.set("proxy-groups", groups)
	providers, rules := f.rules(ctx)
	if len(providers) > 0 {
		root.set("rule-providers", providers)
	}
	root.set("rules", rules)

	var b strings.Builder
	b.Grow(4096 + 512*len(ctx.Proxies))
	writeYAML(&b, root)
	return []byte(b.String()), subscription.ContentTypeYAML, nil
}

// geoxURL points mihomo's GeoIP/GeoSite databases at the mirror (GEOIP,CN,DIRECT needs country.mmdb).
func geoxURL(mirror string) ymap {
	const base = "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/release/"
	return ymap{
		{"geoip", rulesets.WithMirror(mirror, base+"geoip.dat")},
		{"geosite", rulesets.WithMirror(mirror, base+"geosite.dat")},
		{"mmdb", rulesets.WithMirror(mirror, base+"country.mmdb")},
		{"asn", rulesets.WithMirror(mirror, base+"GeoLite2-ASN.mmdb")},
	}
}

var (
	domesticDoH  = []string{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query"}
	bootstrapDNS = []string{"223.5.5.5", "119.29.29.29"}
	fakeIPFilter = []string{"*.lan", "*.local", "+.msftconnecttest.com", "+.msftncsi.com", "localhost.ptlink.com", "+.ntp.org", "time.*.com", "+.stun.*.*"}
)

func (f clashFormat) dns(mode string) ymap {
	if mode == subscription.DNSNone || mode == "" {
		return nil
	}
	if f.flavor == flavorStash {
		return ymap{{"default-nameserver", bootstrapDNS}, {"nameserver", domesticDoH}}
	}
	fake := mode == subscription.DNSFakeIP
	m := ymap{{"enable", true}, {"ipv6", false}}
	if fake {
		m.set("enhanced-mode", "fake-ip")
		m.set("fake-ip-range", "198.18.0.1/16")
		m.set("fake-ip-filter", fakeIPFilter)
	} else {
		m.set("enhanced-mode", "redir-host")
	}
	m.set("default-nameserver", bootstrapDNS)
	m.set("nameserver", domesticDoH)
	if f.flavor == flavorMeta {
		m.set("proxy-server-nameserver", domesticDoH)
	}
	if !fake {
		m.set("fallback", []string{"https://1.1.1.1/dns-query", "https://dns.google/dns-query"})
		m.set("fallback-filter", ymap{{"geoip", true}, {"geoip-code", "CN"}, {"ipcidr", []string{"240.0.0.0/4"}}})
	}
	return m
}

func (f clashFormat) group(g subscription.ResolvedGroup) ymap {
	m := ymap{{"name", g.Name}, {"type", g.Type}, {"proxies", memberNames(g, subscription.PolicyDirect, subscription.PolicyReject)}}
	if g.IsHealthChecked() {
		m.set("url", g.URL)
		m.set("interval", g.Interval)
		switch g.Type {
		case subscription.GroupURLTest:
			m.set("tolerance", 50)
		case subscription.GroupLoadBalance:
			m.set("strategy", "consistent-hashing")
		}
	}
	return m
}

// rules returns the rule providers and rules of the plan.
func (f clashFormat) rules(ctx *subscription.RenderContext) (ymap, []string) {
	var providers ymap
	var rules []string
	if h := hostRule(ctx); h != "" {
		rules = append(rules, "DOMAIN,"+h+","+subscription.PolicyDirect)
	}
	plan := ctx.Rules
	if plan == nil {
		return nil, append(rules, "MATCH,"+subscription.GroupProxy)
	}
	if f.flavor != flavorClassic {
		for _, s := range plan.Sets {
			p := ymap{}
			if f.flavor == flavorMeta {
				p.set("type", "http")
			}
			p.set("behavior", s.Behavior)
			p.set("format", s.Format)
			p.set("url", s.URL)
			p.set("path", "./rules/"+s.Tag+"."+s.Ext())
			p.set("interval", 86400)
			providers.set(s.Tag, p)
			r := "RULE-SET," + s.Tag + "," + s.Target
			if s.NoResolve {
				r += ",no-resolve"
			}
			rules = append(rules, r)
		}
	}
	if plan.GeoIPCNDirect {
		rules = append(rules, "GEOIP,CN,"+subscription.PolicyDirect)
	}
	return providers, append(rules, "MATCH,"+plan.Final)
}

func (f clashFormat) proxy(p *subscription.Proxy) ymap {
	m := ymap{{"name", p.Name}}
	switch p.Type {
	case model.NodeShadowsocks:
		m.set("type", "ss")
		f.server(&m, p)
		m.set("cipher", p.SS.Cipher)
		m.set("password", p.Password)
		m.set("udp", true)
		f.ssPlugin(&m, ssPlugin(p))
	case model.NodeShadowsocksR:
		m.set("type", "ssr")
		f.server(&m, p)
		m.set("cipher", p.SSR.Cipher)
		m.set("password", p.Password)
		m.set("obfs", p.SSR.Obfs)
		m.set("protocol", p.SSR.Protocol)
		m.setStr("obfs-param", p.SSR.ObfsParam)
		m.setStr("protocol-param", p.SSRProtocolParam)
		m.set("udp", true)
	case model.NodeVmess:
		m.set("type", "vmess")
		f.server(&m, p)
		m.set("uuid", p.UUID)
		m.set("alterId", 0)
		m.set("cipher", "auto")
		m.set("udp", true)
		if p.TLSMode() != model.TLSNone {
			m.set("tls", true)
			m.setStr("servername", p.SNI())
			m.setTrue("skip-cert-verify", p.Insecure())
			if f.flavor == flavorMeta {
				m.setStr("client-fingerprint", p.Fingerprint())
			}
		}
		f.transport(&m, p)
	case model.NodeVless:
		m.set("type", "vless")
		f.server(&m, p)
		m.set("uuid", p.UUID)
		m.set("udp", true)
		m.setStr("flow", p.Vless.Flow)
		if enc := p.Vless.Encryption; enc != "" && enc != "none" {
			m.set("encryption", enc)
		}
		if p.TLSMode() != model.TLSNone {
			m.set("tls", true)
			if f.flavor == flavorStash {
				m.setStr("sni", p.SNI())
			} else {
				m.setStr("servername", p.SNI())
			}
			m.setTrue("skip-cert-verify", p.Insecure())
			m.setStr("client-fingerprint", p.Fingerprint())
			if pk, sid, ok := p.Reality(); ok {
				m.set("reality-opts", ymap{{"public-key", pk}, {"short-id", sid}})
			}
		}
		f.transport(&m, p)
	case model.NodeTrojan:
		m.set("type", "trojan")
		f.server(&m, p)
		m.set("password", p.Password)
		m.set("udp", true)
		m.setStr("sni", p.SNI())
		m.setTrue("skip-cert-verify", p.Insecure())
		f.transport(&m, p)
	case model.NodeHysteria:
		f.hysteria(&m, p)
	case model.NodeTuic:
		m.set("type", "tuic")
		f.server(&m, p)
		if f.flavor == flavorStash {
			m.set("version", 5)
		}
		m.set("uuid", p.UUID)
		m.set("password", p.Password)
		m.set("alpn", p.ALPN())
		m.setStr("sni", p.SNI())
		m.setTrue("skip-cert-verify", p.Insecure())
		m.set("congestion-controller", p.Tuic.CongestionControl)
		if f.flavor == flavorMeta {
			m.set("udp-relay-mode", p.Tuic.UDPRelayMode)
			m.setTrue("reduce-rtt", p.Tuic.ZeroRTTHandshake)
		}
	case model.NodeAnyTLS:
		m.set("type", "anytls")
		f.server(&m, p)
		m.set("password", p.Password)
		m.set("udp", true)
		m.setStr("sni", p.SNI())
		m.setTrue("skip-cert-verify", p.Insecure())
		m.setStr("client-fingerprint", p.Fingerprint())
	case model.NodeMieru:
		m.set("type", "mieru")
		m.set("server", p.Host)
		if p.Ports != "" {
			m.set("port-range", p.Ports)
		} else {
			m.set("port", p.Port)
		}
		m.set("transport", strings.ToUpper(first(p.Mieru.Transport, "TCP")))
		m.set("username", p.UUID)
		m.set("password", p.Password)
		m.setStr("multiplexing", p.Mieru.Multiplexing)
	}
	return m
}

func (f clashFormat) server(m *ymap, p *subscription.Proxy) {
	m.set("server", p.Host)
	m.set("port", p.Port)
}

func (f clashFormat) ssPlugin(m *ymap, pi ssPluginInfo) {
	switch pi.Kind {
	case "obfs":
		m.set("plugin", "obfs")
		opts := ymap{{"mode", pi.Mode}}
		opts.setStr("host", pi.Host)
		m.set("plugin-opts", opts)
	case "v2ray-plugin":
		m.set("plugin", "v2ray-plugin")
		opts := ymap{{"mode", pi.Mode}}
		opts.setTrue("tls", pi.TLS)
		opts.setStr("host", pi.Host)
		opts.setStr("path", pi.Path)
		opts.setTrue("mux", pi.Mux)
		m.set("plugin-opts", opts)
	case "shadow-tls":
		m.set("plugin", "shadow-tls")
		opts := ymap{}
		opts.setStr("host", pi.Host)
		opts.setStr("password", pi.Password)
		opts.set("version", pi.Version)
		m.set("plugin-opts", opts)
	}
}

// transport writes network and *-opts for vmess / vless / trojan.
func (f clashFormat) transport(m *ymap, p *subscription.Proxy) {
	n := p.Net
	switch p.Network {
	case "ws":
		m.set("network", "ws")
		opts := ymap{{"path", first(n.Path, "/")}}
		if n.Host != "" {
			opts.set("headers", ymap{{"Host", n.Host}})
		}
		m.set("ws-opts", opts)
	case "httpupgrade":
		m.set("network", "ws")
		opts := ymap{{"path", first(n.Path, "/")}}
		if n.Host != "" {
			opts.set("headers", ymap{{"Host", n.Host}})
		}
		opts.set("v2ray-http-upgrade", true)
		m.set("ws-opts", opts)
	case "grpc":
		m.set("network", "grpc")
		m.set("grpc-opts", ymap{{"grpc-service-name", n.ServiceName}})
	case "http", "h2":
		m.set("network", "h2")
		opts := ymap{}
		if len(n.Hosts) > 0 {
			opts.set("host", n.Hosts)
		}
		opts.set("path", first(n.Path, "/"))
		m.set("h2-opts", opts)
	case "xhttp":
		m.set("network", "xhttp")
		opts := ymap{{"path", first(n.Path, "/")}}
		opts.setStr("host", n.Host)
		opts.setStr("mode", n.Mode)
		m.set("xhttp-opts", opts)
	default:
		if isHTTPHeaderTCP(p) {
			m.set("network", "http")
			opts := ymap{{"method", "GET"}, {"path", []string{first(n.Path, "/")}}}
			if len(n.Hosts) > 0 {
				opts.set("headers", ymap{{"Host", n.Hosts}})
			}
			m.set("http-opts", opts)
		} else if p.Type == model.NodeVless {
			m.set("network", "tcp")
		}
	}
}

func mbps(n int) string { return strconv.Itoa(n) + " Mbps" }

func (f clashFormat) hysteria(m *ymap, p *subscription.Proxy) {
	h := p.Hysteria
	v2 := p.IsHysteria2()
	if v2 {
		m.set("type", "hysteria2")
	} else {
		m.set("type", "hysteria")
	}
	f.server(m, p)
	if p.Ports != "" {
		m.set("ports", p.Ports)
		if h.HopInterval > 0 {
			m.set("hop-interval", h.HopInterval)
		}
	}
	stash := f.flavor == flavorStash
	if v2 {
		if stash {
			m.set("auth", p.Password)
		} else {
			m.set("password", p.Password)
		}
		if !h.IgnoreClientBandwidth && h.UpMbps > 0 && h.DownMbps > 0 {
			if stash {
				m.set("up-speed", h.UpMbps)
				m.set("down-speed", h.DownMbps)
			} else {
				m.set("up", mbps(h.UpMbps))
				m.set("down", mbps(h.DownMbps))
			}
		}
		if obfs := p.HysteriaObfs(); obfs != "" {
			m.set("obfs", "salamander")
			m.set("obfs-password", obfs)
		}
	} else {
		m.set("auth-str", p.Password)
		if stash {
			m.set("up-speed", max(h.UpMbps, 1))
			m.set("down-speed", max(h.DownMbps, 1))
		} else {
			m.set("up", mbps(max(h.UpMbps, 1)))
			m.set("down", mbps(max(h.DownMbps, 1)))
		}
		m.set("protocol", "udp")
		m.setStr("obfs", p.HysteriaObfs())
	}
	m.setStr("sni", p.SNI())
	m.setTrue("skip-cert-verify", p.Insecure())
}
