package formats

import (
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// loon renders a Loon configuration ([General], [Proxy], [Proxy Group], [Rule], [Remote Rule]). Loon reads the
// [Proxy] section when the URL is added as a node subscription, so one body serves both uses; ?list=1 returns
// the proxy lines only. Loon documents no TUIC, mieru or hysteria v1.
type loonFormat struct{}

func (loonFormat) Name() string { return subscription.FormatLoon }
func (loonFormat) Supports(t string) bool {
	switch t {
	case model.NodeShadowsocks, model.NodeShadowsocksR, model.NodeVmess, model.NodeVless, model.NodeTrojan,
		model.NodeHysteria, model.NodeAnyTLS:
		return true
	}
	return false
}
func (loonFormat) RuleFamily() string { return subscription.FormatLoon }

func (loonFormat) SupportsProxy(p *subscription.Proxy) bool {
	switch p.Type {
	case model.NodeShadowsocks:
		k := ssPlugin(p).Kind
		return (k == "" || k == "obfs" || k == "shadow-tls") && p.SS.Cipher != "2022-blake3-chacha20-poly1305"
	case model.NodeVmess, model.NodeVless:
		return p.Network == "tcp" || p.Network == "ws"
	case model.NodeTrojan:
		return p.Network == "ws" || (p.Network == "tcp" && !isHTTPHeaderTCP(p))
	case model.NodeHysteria:
		return p.IsHysteria2()
	}
	return true
}

func (f loonFormat) Render(ctx *subscription.RenderContext) ([]byte, string, error) {
	var b strings.Builder
	b.Grow(2048 + 256*len(ctx.Proxies))
	if ctx.ListOnly {
		for i := range ctx.Proxies {
			b.WriteString(f.proxy(&ctx.Proxies[i]))
			b.WriteByte('\n')
		}
		return []byte(b.String()), subscription.ContentTypePlain, nil
	}
	b.WriteString("[General]\nskip-proxy = 192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,localhost,*.local\n")
	if ctx.Profile.DNS != subscription.DNSNone {
		b.WriteString("dns-server = system,223.5.5.5,119.29.29.29\n")
	}
	b.WriteString("proxy-test-url = " + subscription.DefaultTestURL + "\n")
	b.WriteString("\n[Proxy]\n")
	for i := range ctx.Proxies {
		b.WriteString(f.proxy(&ctx.Proxies[i]))
		b.WriteByte('\n')
	}
	b.WriteString("\n[Proxy Group]\n")
	for _, g := range ctx.Groups {
		b.WriteString(g.Name + " = " + g.Type)
		for _, m := range memberNames(g, subscription.PolicyDirect, subscription.PolicyReject) {
			b.WriteString("," + m)
		}
		if g.IsHealthChecked() {
			b.WriteString(",url=" + g.URL + ",interval=" + strconv.Itoa(g.Interval))
			switch g.Type {
			case subscription.GroupURLTest:
				b.WriteString(",tolerance=50")
			case subscription.GroupLoadBalance:
				b.WriteString(",algorithm=round-robin")
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
		if plan.GeoIPCNDirect {
			b.WriteString("GEOIP,CN,DIRECT\n")
		}
		final = plan.Final
	}
	b.WriteString("FINAL," + final + "\n")
	b.WriteString("\n[Remote Rule]\n")
	if plan := ctx.Rules; plan != nil {
		for _, s := range plan.Sets {
			b.WriteString(s.URL + ", policy=" + s.Target + ", tag=" + s.Tag + ", enabled=true\n")
		}
	}
	return []byte(b.String()), subscription.ContentTypePlain, nil
}

// loonLine builds "Name = Type,host,port,...".
type loonLine struct{ b strings.Builder }

func newLoonLine(name, typ, host string, port int) *loonLine {
	l := &loonLine{}
	l.b.WriteString(name + " = " + typ + "," + host + "," + strconv.Itoa(port))
	return l
}

func (l *loonLine) raw(v string) { l.b.WriteString("," + v) }

func (l *loonLine) quoted(v string) { l.b.WriteString(`,"` + strings.ReplaceAll(v, `"`, ``) + `"`) }

func (l *loonLine) kv(k, v string) {
	if v != "" {
		l.b.WriteString("," + k + "=" + v)
	}
}

func (l *loonLine) tls(p *subscription.Proxy) {
	l.kv("sni", p.SNI())
	l.kv("skip-cert-verify", boolStr(p.Insecure()))
	if pk, sid, ok := p.Reality(); ok {
		l.kv("public-key", pk)
		l.kv("short-id", sid)
	}
}

func (l *loonLine) transport(p *subscription.Proxy) {
	switch {
	case p.Network == "ws":
		l.kv("transport", "ws")
		l.kv("path", first(p.Net.Path, "/"))
		l.kv("host", p.Net.Host)
	case isHTTPHeaderTCP(p):
		l.kv("transport", "http")
		l.kv("path", first(p.Net.Path, "/"))
		l.kv("host", p.Net.Host)
	default:
		l.kv("transport", "tcp")
	}
}

func (loonFormat) proxy(p *subscription.Proxy) string {
	var l *loonLine
	switch p.Type {
	case model.NodeShadowsocks:
		l = newLoonLine(p.Name, "Shadowsocks", p.Host, p.Port)
		l.raw(p.SS.Cipher)
		l.quoted(p.Password)
		switch pi := ssPlugin(p); pi.Kind {
		case "obfs":
			l.kv("obfs-name", pi.Mode)
			l.kv("obfs-host", pi.Host)
			if pi.Mode == "http" {
				l.kv("obfs-uri", pi.Path)
			}
		case "shadow-tls":
			l.kv("shadow-tls-password", pi.Password)
			l.kv("shadow-tls-sni", pi.Host)
			l.kv("shadow-tls-version", strconv.Itoa(pi.Version))
		}
	case model.NodeShadowsocksR:
		l = newLoonLine(p.Name, "ShadowsocksR", p.Host, p.Port)
		l.raw(p.SSR.Cipher)
		l.quoted(p.Password)
		l.kv("protocol", p.SSR.Protocol)
		l.kv("protocol-param", p.SSRProtocolParam)
		l.kv("obfs", p.SSR.Obfs)
		l.kv("obfs-param", p.SSR.ObfsParam)
	case model.NodeVmess:
		l = newLoonLine(p.Name, "VMess", p.Host, p.Port)
		l.raw("auto")
		l.quoted(p.UUID)
		l.transport(p)
		if p.TLSMode() != model.TLSNone {
			l.kv("over-tls", "true")
			l.tls(p)
		}
		l.kv("alterId", "0")
	case model.NodeVless:
		l = newLoonLine(p.Name, "VLESS", p.Host, p.Port)
		l.quoted(p.UUID)
		l.transport(p)
		l.kv("flow", p.Vless.Flow)
		if p.TLSMode() != model.TLSNone {
			l.kv("over-tls", "true")
			l.tls(p)
		}
	case model.NodeTrojan:
		l = newLoonLine(p.Name, "Trojan", p.Host, p.Port)
		l.quoted(p.Password)
		l.transport(p)
		l.tls(p)
	case model.NodeHysteria:
		h := p.Hysteria
		l = newLoonLine(p.Name, "Hysteria2", p.Host, p.Port)
		l.quoted(p.Password)
		l.tls(p)
		l.kv("salamander-password", p.HysteriaObfs())
		if lo, hi, ok := strings.Cut(p.Ports, "-"); ok && !strings.Contains(p.Ports, ",") {
			l.kv("server-ports", `"`+lo+":"+hi+`"`)
			if h.HopInterval > 0 {
				l.kv("hop-interval", strconv.Itoa(h.HopInterval))
			}
		}
		if !h.IgnoreClientBandwidth && h.DownMbps > 0 {
			l.kv("download-bandwidth", strconv.Itoa(h.DownMbps))
		}
	case model.NodeAnyTLS:
		l = newLoonLine(p.Name, "AnyTLS", p.Host, p.Port)
		l.quoted(p.Password)
		l.tls(p)
	default:
		return ""
	}
	l.kv("udp", "true")
	return l.b.String()
}

func init() { subscription.RegisterFormat(loonFormat{}) }
