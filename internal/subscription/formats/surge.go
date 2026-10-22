package formats

import (
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// surgeFormat renders a Surge managed profile (and the Surfboard subset). Surge has no VLESS, SSR, mieru or
// hysteria v1; its VMess and Trojan only know tcp and ws transports.
type surgeFormat struct {
	name      string
	surfboard bool
	types     map[string]bool
}

var surgeCiphers = typeSet("aes-128-gcm", "aes-192-gcm", "aes-256-gcm", "chacha20-ietf-poly1305",
	"xchacha20-ietf-poly1305", "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm")

var surfboardCiphers = typeSet("aes-128-gcm", "aes-192-gcm", "aes-256-gcm", "chacha20-ietf-poly1305",
	"xchacha20-ietf-poly1305")

func (f surgeFormat) Name() string           { return f.name }
func (f surgeFormat) Supports(t string) bool { return f.types[t] }
func (f surgeFormat) RuleFamily() string     { return f.name }

func (f surgeFormat) SupportsProxy(p *subscription.Proxy) bool {
	switch p.Type {
	case model.NodeShadowsocks:
		ciphers := surgeCiphers
		if f.surfboard {
			ciphers = surfboardCiphers
		}
		switch ssPlugin(p).Kind {
		case "", "obfs":
		case "shadow-tls":
			if f.surfboard {
				return false
			}
		default:
			return false
		}
		return ciphers[p.SS.Cipher]
	case model.NodeVmess, model.NodeTrojan:
		return p.Network == "ws" || (p.Network == "tcp" && !isHTTPHeaderTCP(p))
	case model.NodeHysteria:
		return p.IsHysteria2()
	case model.NodeAnyTLS:
		return p.TLSMode() != model.TLSReality
	}
	return true
}

func (f surgeFormat) Render(ctx *subscription.RenderContext) ([]byte, string, error) {
	var b strings.Builder
	b.Grow(2048 + 256*len(ctx.Proxies))
	if ctx.ListOnly {
		for i := range ctx.Proxies {
			b.WriteString(f.proxy(&ctx.Proxies[i]))
			b.WriteByte('\n')
		}
		return []byte(b.String()), subscription.ContentTypePlain, nil
	}
	if ctx.SubscribeURL != "" {
		b.WriteString("#!MANAGED-CONFIG ")
		b.WriteString(ctx.SubscribeURL)
		b.WriteString(" interval=")
		b.WriteString(strconv.Itoa(max(ctx.UpdateHours, 1) * 3600))
		b.WriteString(" strict=false\n\n")
	}
	b.WriteString("[General]\nloglevel = notify\n")
	b.WriteString("skip-proxy = 127.0.0.1, 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, 100.64.0.0/10, localhost, *.local\n")
	if ctx.Profile.DNS != subscription.DNSNone {
		b.WriteString("dns-server = system, 223.5.5.5, 119.29.29.29\n")
	}
	b.WriteString("proxy-test-url = " + subscription.DefaultTestURL + "\n")
	b.WriteString("internet-test-url = " + subscription.DefaultTestURL + "\n")

	b.WriteString("\n[Proxy]\n")
	for i := range ctx.Proxies {
		b.WriteString(f.proxy(&ctx.Proxies[i]))
		b.WriteByte('\n')
	}
	b.WriteString("\n[Proxy Group]\n")
	for _, g := range ctx.Groups {
		b.WriteString(g.Name)
		b.WriteString(" = ")
		b.WriteString(g.Type)
		for _, m := range memberNames(g, subscription.PolicyDirect, subscription.PolicyReject) {
			b.WriteString(", ")
			b.WriteString(m)
		}
		if g.IsHealthChecked() {
			b.WriteString(", url=" + g.URL + ", interval=" + strconv.Itoa(g.Interval))
			if g.Type == subscription.GroupURLTest {
				b.WriteString(", tolerance=50")
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("\n[Rule]\n")
	if h := hostRule(ctx); h != "" {
		b.WriteString("DOMAIN," + h + ",DIRECT\n")
	}
	final := subscription.GroupProxy
	if plan := ctx.Rules; plan != nil {
		for _, s := range plan.Sets {
			switch s.Behavior {
			case subscription.BehaviorDomain:
				b.WriteString("DOMAIN-SET," + s.URL + "," + s.Target + "\n")
			case subscription.BehaviorIPCIDR:
				b.WriteString("RULE-SET," + s.URL + "," + s.Target + ",no-resolve\n")
			default:
				b.WriteString("RULE-SET," + s.URL + "," + s.Target + "\n")
			}
		}
		if plan.GeoIPCNDirect {
			b.WriteString("GEOIP,CN,DIRECT\n")
		}
		final = plan.Final
	}
	b.WriteString("FINAL," + final + ",dns-failed\n")
	return []byte(b.String()), subscription.ContentTypePlain, nil
}

// surgeLine builds "Name = type, host, port, k=v, ...".
type surgeLine struct{ b strings.Builder }

func newSurgeLine(name, typ, host string, port int) *surgeLine {
	l := &surgeLine{}
	l.b.WriteString(name + " = " + typ + ", " + host + ", " + strconv.Itoa(port))
	return l
}

func (l *surgeLine) kv(k, v string) {
	if v != "" {
		l.b.WriteString(", " + k + "=" + v)
	}
}

func (l *surgeLine) flag(k string, on bool) {
	if on {
		l.b.WriteString(", " + k + "=true")
	}
}

func (l *surgeLine) tls(p *subscription.Proxy) {
	l.kv("sni", p.SNI())
	l.flag("skip-cert-verify", p.Insecure())
}

func (l *surgeLine) ws(p *subscription.Proxy) {
	if p.Network != "ws" {
		return
	}
	l.flag("ws", true)
	l.kv("ws-path", first(p.Net.Path, "/"))
	if p.Net.Host != "" {
		l.kv("ws-headers", "Host:"+p.Net.Host)
	}
}

func (l *surgeLine) hopping(p *subscription.Proxy, interval int) {
	if p.Ports == "" {
		return
	}
	l.kv("port-hopping", strings.ReplaceAll(p.Ports, ",", ";"))
	if interval > 0 {
		l.kv("port-hopping-interval", strconv.Itoa(interval))
	}
}

func (f surgeFormat) proxy(p *subscription.Proxy) string {
	var l *surgeLine
	switch p.Type {
	case model.NodeShadowsocks:
		l = newSurgeLine(p.Name, "ss", p.Host, p.Port)
		l.kv("encrypt-method", p.SS.Cipher)
		l.kv("password", p.Password)
		l.flag("udp-relay", true)
		switch pi := ssPlugin(p); pi.Kind {
		case "obfs":
			l.kv("obfs", pi.Mode)
			l.kv("obfs-host", pi.Host)
			if pi.Mode == "http" {
				l.kv("obfs-uri", pi.Path)
			}
		case "shadow-tls":
			l.kv("shadow-tls-password", pi.Password)
			l.kv("shadow-tls-sni", pi.Host)
			l.kv("shadow-tls-version", strconv.Itoa(pi.Version))
		}
	case model.NodeVmess:
		l = newSurgeLine(p.Name, "vmess", p.Host, p.Port)
		l.kv("username", p.UUID)
		l.flag("vmess-aead", true)
		if p.TLSMode() != model.TLSNone {
			l.flag("tls", true)
			l.tls(p)
		}
		l.ws(p)
	case model.NodeTrojan:
		l = newSurgeLine(p.Name, "trojan", p.Host, p.Port)
		l.kv("password", p.Password)
		l.tls(p)
		l.ws(p)
	case model.NodeHysteria:
		h := p.Hysteria
		l = newSurgeLine(p.Name, "hysteria2", p.Host, p.Port)
		l.kv("password", p.Password)
		l.tls(p)
		if !h.IgnoreClientBandwidth && h.DownMbps > 0 {
			l.kv("download-bandwidth", strconv.Itoa(h.DownMbps))
		}
		l.kv("salamander-password", p.HysteriaObfs())
		l.hopping(p, h.HopInterval)
	case model.NodeTuic:
		l = newSurgeLine(p.Name, "tuic-v5", p.Host, p.Port)
		l.kv("uuid", p.UUID)
		l.kv("password", p.Password)
		l.kv("alpn", p.ALPN()[0]) // one value: commas separate parameters
		l.tls(p)
		l.hopping(p, 0)
	case model.NodeAnyTLS:
		l = newSurgeLine(p.Name, "anytls", p.Host, p.Port)
		l.kv("password", p.Password)
		l.tls(p)
	default:
		return ""
	}
	return l.b.String()
}

func init() {
	subscription.RegisterFormat(surgeFormat{name: subscription.FormatSurge, types: typeSet(
		model.NodeShadowsocks, model.NodeVmess, model.NodeTrojan, model.NodeHysteria, model.NodeTuic, model.NodeAnyTLS)})
}
