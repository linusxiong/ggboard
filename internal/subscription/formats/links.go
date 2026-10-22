package formats

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// Share links (docs/SUBSCRIPTION.md "Share-link conventions"), used by the v2ray, shadowrocket and ssr formats.

// shareLink returns the URI of p ("" when the type has no link form).
func shareLink(p *subscription.Proxy) string {
	switch p.Type {
	case model.NodeVmess:
		return vmessLink(p)
	case model.NodeVless:
		return vlessLink(p)
	case model.NodeShadowsocks:
		return ssLink(p)
	case model.NodeShadowsocksR:
		return ssrLink(p, "")
	case model.NodeTrojan:
		return trojanLink(p)
	case model.NodeHysteria:
		if p.IsHysteria2() {
			return hysteria2Link(p)
		}
		return hysteriaLink(p)
	case model.NodeTuic:
		return tuicLink(p)
	case model.NodeAnyTLS:
		return anytlsLink(p)
	case model.NodeMieru:
		return mieruLink(p)
	}
	return ""
}

// netParams returns the v2rayN style (type, host, path) of a vmess / vless / trojan transport.
func netParams(p *subscription.Proxy) (typ, host, path string) {
	n := p.Net
	switch p.Network {
	case "ws", "httpupgrade", "xhttp":
		return p.Network, n.Host, first(n.Path, "/")
	case "grpc":
		return "grpc", "", n.ServiceName
	case "http", "h2":
		return "h2", strings.Join(n.Hosts, ","), first(n.Path, "/")
	}
	if isHTTPHeaderTCP(p) {
		return "tcp", strings.Join(n.Hosts, ","), first(n.Path, "/")
	}
	return "tcp", "", ""
}

func vmessLink(p *subscription.Proxy) string {
	net, host, path := netParams(p)
	typ := "none"
	switch {
	case isHTTPHeaderTCP(p):
		typ = "http"
	case net == "grpc":
		typ = "gun"
	case net == "xhttp":
		typ = first(p.Net.Mode, "auto")
	}
	tls := ""
	if p.TLSMode() != model.TLSNone {
		tls = "tls"
	}
	o := jobj{
		{"v", "2"}, {"ps", p.Name}, {"add", p.Host}, {"port", strconv.Itoa(p.Port)}, {"id", p.UUID}, {"aid", "0"},
		{"scy", "auto"}, {"net", net}, {"type", typ}, {"host", host}, {"path", path}, {"tls", tls},
	}
	if tls != "" {
		o.setStr("sni", p.SNI())
		o.setStr("fp", p.Fingerprint())
	}
	var buf bytes.Buffer
	_ = o.encode(&buf) // values are plain strings: cannot fail
	return "vmess://" + b64(buf.String())
}

// linkQuery is an ordered query string (stable output for tests and caches).
type linkQuery struct{ b strings.Builder }

func (q *linkQuery) set(k, v string) {
	if v == "" {
		return
	}
	if q.b.Len() > 0 {
		q.b.WriteByte('&')
	}
	q.b.WriteString(k)
	q.b.WriteByte('=')
	q.b.WriteString(escapeComponent(v))
}

func (q *linkQuery) String() string {
	if q.b.Len() == 0 {
		return ""
	}
	return "?" + q.b.String()
}

func vlessLink(p *subscription.Proxy) string {
	var q linkQuery
	enc := p.Vless.Encryption
	if enc == "" {
		enc = "none"
	}
	q.set("encryption", enc)
	switch p.TLSMode() {
	case model.TLSReality:
		pk, sid, _ := p.Reality()
		q.set("security", "reality")
		q.set("sni", p.SNI())
		q.set("fp", p.Fingerprint())
		q.set("pbk", pk)
		q.set("sid", sid)
	case model.TLSEnabled:
		q.set("security", "tls")
		q.set("sni", p.SNI())
		q.set("fp", p.Fingerprint())
		if p.Insecure() {
			q.set("allowInsecure", "1")
		}
	default:
		q.set("security", "none")
	}
	transportQuery(&q, p)
	q.set("flow", p.Vless.Flow)
	return "vless://" + escapeComponent(p.UUID) + "@" + hostPort(p.Host, p.Port) + q.String() + "#" + escapeComponent(p.Name)
}

// transportQuery adds type/headerType/host/path/serviceName/mode/extra for vless and trojan links.
func transportQuery(q *linkQuery, p *subscription.Proxy) {
	net, host, path := netParams(p)
	if net == "h2" {
		net = "http"
	}
	q.set("type", net)
	if isHTTPHeaderTCP(p) {
		q.set("headerType", "http")
	}
	q.set("host", host)
	if net == "grpc" {
		q.set("serviceName", path)
		q.set("mode", "gun")
		return
	}
	q.set("path", path)
	if net == "xhttp" {
		q.set("mode", first(p.Net.Mode, "auto"))
		if len(p.Net.Extra) > 0 {
			if b, err := json.Marshal(p.Net.Extra); err == nil {
				q.set("extra", string(b))
			}
		}
	}
}

func ssLink(p *subscription.Proxy) string {
	s := "ss://" + b64url(p.SS.Cipher+":"+p.Password) + "@" + hostPort(p.Host, p.Port)
	if plugin := ssPlugin(p).sipPlugin(); plugin != "" {
		s += "/?plugin=" + escapeComponent(plugin)
	}
	return s + "#" + escapeComponent(p.Name)
}

// ssrLink builds ssr://base64url(host:port:protocol:method:obfs:base64url(pass)/?obfsparam=&protoparam=&remarks=&group=).
// Plain shadowsocks proxies are expressed as protocol origin / obfs plain.
func ssrLink(p *subscription.Proxy, group string) string {
	method, proto, obfs, obfsParam, protoParam := "", "origin", "plain", "", ""
	switch {
	case p.SSR != nil:
		method, proto, obfs, obfsParam, protoParam = p.SSR.Cipher, p.SSR.Protocol, p.SSR.Obfs, p.SSR.ObfsParam, p.SSRProtocolParam
	case p.SS != nil:
		method = p.SS.Cipher
	}
	var b strings.Builder
	b.WriteString(wrapIPv6(p.Host) + ":" + strconv.Itoa(p.Port) + ":" + proto + ":" + method + ":" + obfs + ":" + b64url(p.Password))
	b.WriteString("/?obfsparam=" + b64url(obfsParam) + "&protoparam=" + b64url(protoParam) + "&remarks=" + b64url(p.Name))
	if group != "" {
		b.WriteString("&group=" + b64url(group))
	}
	return "ssr://" + b64url(b.String())
}

func trojanLink(p *subscription.Proxy) string {
	var q linkQuery
	q.set("security", "tls")
	q.set("sni", p.SNI())
	transportQuery(&q, p)
	if p.Insecure() {
		q.set("allowInsecure", "1")
	}
	return "trojan://" + escapeComponent(p.Password) + "@" + hostPort(p.Host, p.Port) + q.String() + "#" + escapeComponent(p.Name)
}

func hysteria2Link(p *subscription.Proxy) string {
	var q linkQuery
	q.set("sni", p.SNI())
	if p.Insecure() {
		q.set("insecure", "1")
	}
	if obfs := p.HysteriaObfs(); obfs != "" {
		q.set("obfs", "salamander")
		q.set("obfs-password", obfs)
	}
	q.set("mport", p.Ports)
	return "hysteria2://" + escapeComponent(p.Password) + "@" + hostPort(p.Host, p.Port) + q.String() + "#" + escapeComponent(p.Name)
}

func hysteriaLink(p *subscription.Proxy) string {
	h := p.Hysteria
	var q linkQuery
	q.set("protocol", "udp")
	q.set("auth", p.Password)
	q.set("upmbps", strconv.Itoa(max(h.UpMbps, 1)))
	q.set("downmbps", strconv.Itoa(max(h.DownMbps, 1)))
	if obfs := p.HysteriaObfs(); obfs != "" {
		q.set("obfs", "xplus")
		q.set("obfsParam", obfs)
	}
	q.set("peer", p.SNI())
	if p.Insecure() {
		q.set("insecure", "1")
	}
	q.set("alpn", strings.Join(p.ALPN(), ","))
	q.set("mport", p.Ports)
	return "hysteria://" + hostPort(p.Host, p.Port) + q.String() + "#" + escapeComponent(p.Name)
}

func tuicLink(p *subscription.Proxy) string {
	var q linkQuery
	q.set("sni", p.SNI())
	q.set("alpn", strings.Join(p.ALPN(), ","))
	q.set("congestion_control", p.Tuic.CongestionControl)
	q.set("udp_relay_mode", p.Tuic.UDPRelayMode)
	if p.Insecure() {
		q.set("allow_insecure", "1")
	}
	return "tuic://" + escapeComponent(p.UUID) + ":" + escapeComponent(p.Password) + "@" + hostPort(p.Host, p.Port) + q.String() + "#" + escapeComponent(p.Name)
}

func anytlsLink(p *subscription.Proxy) string {
	var q linkQuery
	q.set("sni", p.SNI())
	if p.Insecure() {
		q.set("insecure", "1")
	}
	if pk, sid, ok := p.Reality(); ok {
		q.set("security", "reality")
		q.set("pbk", pk)
		q.set("sid", sid)
		q.set("fp", p.Fingerprint())
	}
	return "anytls://" + escapeComponent(p.Password) + "@" + hostPort(p.Host, p.Port) + q.String() + "#" + escapeComponent(p.Name)
}

func mieruLink(p *subscription.Proxy) string {
	var q linkQuery
	q.set("transport", strings.ToUpper(first(p.Mieru.Transport, "TCP")))
	q.set("multiplexing", p.Mieru.Multiplexing)
	q.set("port-range", p.Ports)
	return "mieru://" + escapeComponent(p.UUID) + ":" + escapeComponent(p.Password) + "@" + hostPort(p.Host, p.Port) + q.String() + "#" + escapeComponent(p.Name)
}

// linksBody joins links with newlines and base64-encodes the result (the v2ray subscription body).
func linksBody(lines []string) []byte {
	return []byte(b64(strings.Join(lines, "\n") + "\n"))
}
