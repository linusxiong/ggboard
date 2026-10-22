package formats

import (
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// quantumultx renders a Quantumult X configuration ([general], [dns], [policy], [server_local],
// [filter_remote], [filter_local]); with ?list=1 only the server lines (for [server_remote]). Built-in
// policies are lowercase direct / reject.
type quantumultxFormat struct{}

const (
	qxDirect = "direct"
	qxReject = "reject"
)

var qxCiphers = typeSet("aes-128-gcm", "aes-192-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "xchacha20-ietf-poly1305")

func (quantumultxFormat) Name() string { return subscription.FormatQuantumultX }
func (quantumultxFormat) Supports(t string) bool {
	switch t {
	case model.NodeShadowsocks, model.NodeShadowsocksR, model.NodeVmess, model.NodeVless, model.NodeTrojan, model.NodeAnyTLS:
		return true
	}
	return false
}
func (quantumultxFormat) RuleFamily() string { return subscription.FormatQuantumultX }

func (quantumultxFormat) SupportsProxy(p *subscription.Proxy) bool {
	switch p.Type {
	case model.NodeShadowsocks:
		k := ssPlugin(p).Kind
		return qxCiphers[p.SS.Cipher] && (k == "" || k == "obfs" || k == "v2ray-plugin")
	case model.NodeVmess, model.NodeVless:
		if p.Network == "ws" {
			return true
		}
		return p.Network == "tcp"
	case model.NodeTrojan:
		return p.Network == "ws" || (p.Network == "tcp" && !isHTTPHeaderTCP(p))
	case model.NodeAnyTLS:
		return p.TLSMode() != model.TLSReality
	}
	return true
}

func (f quantumultxFormat) Render(ctx *subscription.RenderContext) ([]byte, string, error) {
	var b strings.Builder
	b.Grow(2048 + 256*len(ctx.Proxies))
	if ctx.ListOnly {
		for i := range ctx.Proxies {
			b.WriteString(f.server(&ctx.Proxies[i]))
			b.WriteByte('\n')
		}
		return []byte(b.String()), subscription.ContentTypePlain, nil
	}
	b.WriteString("[general]\nserver_check_url=" + subscription.DefaultTestURL + "\n")
	if ctx.Profile.DNS != subscription.DNSNone {
		b.WriteString("\n[dns]\nserver=223.5.5.5\nserver=119.29.29.29\n")
	}
	b.WriteString("\n[policy]\n")
	for _, g := range ctx.Groups {
		kind := "static"
		switch g.Type {
		case subscription.GroupURLTest:
			kind = "url-latency-benchmark"
		case subscription.GroupFallback:
			kind = "available"
		case subscription.GroupLoadBalance:
			kind = "round-robin"
		}
		b.WriteString(kind + "=" + g.Name)
		for _, m := range memberNames(g, qxDirect, qxReject) {
			b.WriteString(", " + m)
		}
		if g.IsHealthChecked() {
			b.WriteString(", check-interval=" + strconv.Itoa(g.Interval))
			if g.Type == subscription.GroupURLTest {
				b.WriteString(", tolerance=50")
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("\n[server_remote]\n\n[filter_remote]\n")
	final := subscription.GroupProxy
	if plan := ctx.Rules; plan != nil {
		for _, s := range plan.Sets {
			b.WriteString(s.URL + ", tag=" + s.Tag + ", force-policy=" + policyName(s.Target, qxDirect, qxReject) +
				", update-interval=86400, opt-parser=false, enabled=true\n")
		}
		final = plan.Final
	}
	b.WriteString("\n[rewrite_remote]\n\n[server_local]\n")
	for i := range ctx.Proxies {
		b.WriteString(f.server(&ctx.Proxies[i]))
		b.WriteByte('\n')
	}
	b.WriteString("\n[filter_local]\n")
	if h := hostRule(ctx); h != "" {
		b.WriteString("host, " + h + ", " + qxDirect + "\n")
	}
	if ctx.Rules != nil && ctx.Rules.GeoIPCNDirect {
		b.WriteString("geoip, cn, " + qxDirect + "\n")
	}
	b.WriteString("final, " + policyName(final, qxDirect, qxReject) + "\n")
	b.WriteString("\n[rewrite_local]\n\n[mitm]\n")
	return []byte(b.String()), subscription.ContentTypePlain, nil
}

// qxLine builds "type=host:port, k=v, ..., tag=Name".
type qxLine struct{ b strings.Builder }

func (l *qxLine) kv(k, v string) {
	if v != "" {
		l.b.WriteString(", " + k + "=" + v)
	}
}

// qxTransport writes obfs / tls options shared by vmess, vless and trojan.
func (l *qxLine) transport(p *subscription.Proxy, trojan bool) {
	tlsOn := p.TLSMode() != model.TLSNone
	host := p.Net.Host
	switch {
	case p.Network == "ws":
		if tlsOn {
			l.kv("obfs", "wss")
		} else {
			l.kv("obfs", "ws")
		}
		l.kv("obfs-host", first(host, p.SNI()))
		l.kv("obfs-uri", first(p.Net.Path, "/"))
	case isHTTPHeaderTCP(p):
		l.kv("obfs", "http")
		l.kv("obfs-host", host)
		l.kv("obfs-uri", first(p.Net.Path, "/"))
	case tlsOn && trojan:
		l.kv("over-tls", "true")
		l.kv("tls-host", p.SNI())
	case tlsOn:
		l.kv("obfs", "over-tls")
		l.kv("obfs-host", p.SNI())
	}
	if tlsOn && p.TLSMode() != model.TLSReality {
		l.kv("tls-verification", boolStr(!p.Insecure()))
	}
	if pk, sid, ok := p.Reality(); ok {
		l.kv("reality-base64-pubkey", pk)
		l.kv("reality-hex-shortid", sid)
	}
}

func (quantumultxFormat) server(p *subscription.Proxy) string {
	l := &qxLine{}
	addr := hostPort(p.Host, p.Port)
	switch p.Type {
	case model.NodeShadowsocks:
		l.b.WriteString("shadowsocks=" + addr)
		l.kv("method", p.SS.Cipher)
		l.kv("password", p.Password)
		switch pi := ssPlugin(p); pi.Kind {
		case "obfs":
			l.kv("obfs", pi.Mode)
			l.kv("obfs-host", pi.Host)
			if pi.Mode == "http" {
				l.kv("obfs-uri", pi.Path)
			}
		case "v2ray-plugin":
			if pi.TLS {
				l.kv("obfs", "wss")
			} else {
				l.kv("obfs", "ws")
			}
			l.kv("obfs-host", pi.Host)
			l.kv("obfs-uri", pi.Path)
		}
		l.kv("udp-relay", "true")
	case model.NodeShadowsocksR:
		l.b.WriteString("shadowsocks=" + addr)
		l.kv("method", p.SSR.Cipher)
		l.kv("password", p.Password)
		l.kv("ssr-protocol", p.SSR.Protocol)
		l.kv("ssr-protocol-param", p.SSRProtocolParam)
		if p.SSR.Obfs != "" && p.SSR.Obfs != "plain" {
			l.kv("obfs", p.SSR.Obfs)
			l.kv("obfs-host", p.SSR.ObfsParam)
		}
	case model.NodeVmess:
		l.b.WriteString("vmess=" + addr)
		l.kv("method", "chacha20-ietf-poly1305")
		l.kv("password", p.UUID)
		l.transport(p, false)
		l.kv("aead", "true")
	case model.NodeVless:
		l.b.WriteString("vless=" + addr)
		l.kv("method", "none")
		l.kv("password", p.UUID)
		l.transport(p, false)
		l.kv("vless-flow", p.Vless.Flow)
	case model.NodeTrojan:
		l.b.WriteString("trojan=" + addr)
		l.kv("password", p.Password)
		l.transport(p, true)
	case model.NodeAnyTLS:
		l.b.WriteString("anytls=" + addr)
		l.kv("password", p.Password)
		l.kv("over-tls", "true")
		l.kv("tls-host", p.SNI())
		l.kv("tls-verification", boolStr(!p.Insecure()))
		l.kv("udp-relay", "true")
	default:
		return ""
	}
	l.kv("fast-open", "false")
	l.kv("tag", p.Name)
	return l.b.String()
}

func init() { subscription.RegisterFormat(quantumultxFormat{}) }
