// Package formats implements every subscription output format (docs/SUBSCRIPTION.md). Each file registers one
// format from init; importing the package (blank import) makes them available through subscription.GetFormat.
package formats

import (
	"encoding/base64"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// typeSet builds a node-type lookup table.
func typeSet(types ...string) map[string]bool {
	m := make(map[string]bool, len(types))
	for _, t := range types {
		m[t] = true
	}
	return m
}

// allTypes is every node type.
var allTypes = typeSet(model.NodeTypes()...)

// hostPort joins host and port, bracketing IPv6 literals.
func hostPort(host string, port int) string {
	return wrapIPv6(host) + ":" + strconv.Itoa(port)
}

func wrapIPv6(h string) string {
	if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
		return "[" + h + "]"
	}
	return h
}

// isDomain reports whether h is a DNS name (not an IP literal).
func isDomain(h string) bool {
	if h == "" || net.ParseIP(h) != nil || !strings.Contains(h, ".") {
		return false
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}

// b64 is standard base64 with padding; b64url is URL-safe base64 without padding (SIP002 / SSR links).
func b64(s string) string    { return base64.StdEncoding.EncodeToString([]byte(s)) }
func b64url(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

// escapeComponent percent-encodes s for URI userinfo and fragments (spaces as %20).
func escapeComponent(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }

// boolStr renders a bool as true/false.
func boolStr(v bool) string { return strconv.FormatBool(v) }

// ssPluginInfo is the normalized shadowsocks plugin of a proxy: protocol_settings.obfs (simple-obfs) or
// plugin + plugin_opts (obfs-local, v2ray-plugin, shadow-tls).
type ssPluginInfo struct {
	Kind     string // "", "obfs", "v2ray-plugin", "shadow-tls" or "unknown"
	Mode     string // obfs: http | tls; v2ray-plugin: websocket
	Host     string
	Path     string
	TLS      bool
	Mux      bool
	Password string // shadow-tls
	Version  int    // shadow-tls
}

func parsePluginOpts(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func ssPlugin(p *subscription.Proxy) ssPluginInfo {
	if p.SS == nil {
		return ssPluginInfo{}
	}
	if mode := p.SSObfs(); mode != "" {
		return ssPluginInfo{Kind: "obfs", Mode: mode, Host: p.SS.ObfsSettings.Host, Path: first(p.SS.ObfsSettings.Path, "/")}
	}
	name := strings.TrimSpace(p.SS.Plugin)
	if name == "" {
		return ssPluginInfo{}
	}
	o := parsePluginOpts(p.SS.PluginOpts)
	switch name {
	case "obfs", "obfs-local", "simple-obfs":
		mode := first(o["obfs"], o["mode"], "http")
		if mode != "http" && mode != "tls" {
			return ssPluginInfo{Kind: "unknown"}
		}
		return ssPluginInfo{Kind: "obfs", Mode: mode, Host: first(o["obfs-host"], o["host"]), Path: first(o["obfs-uri"], o["path"], "/")}
	case "v2ray-plugin":
		mode := first(o["mode"], "websocket")
		if mode != "websocket" {
			return ssPluginInfo{Kind: "unknown"}
		}
		_, tls := o["tls"]
		_, mux := o["mux"]
		return ssPluginInfo{Kind: "v2ray-plugin", Mode: mode, Host: o["host"], Path: first(o["path"], "/"), TLS: tls, Mux: mux}
	case "shadow-tls":
		v, _ := strconv.Atoi(o["version"])
		if v == 0 {
			v = 2
		}
		return ssPluginInfo{Kind: "shadow-tls", Host: o["host"], Password: o["password"], Version: v}
	}
	return ssPluginInfo{Kind: "unknown"}
}

// sipPlugin returns the SIP003 plugin string of a share link ("" when none).
func (pi ssPluginInfo) sipPlugin() string {
	switch pi.Kind {
	case "obfs":
		s := "obfs-local;obfs=" + pi.Mode
		if pi.Host != "" {
			s += ";obfs-host=" + pi.Host
		}
		if pi.Mode == "http" && pi.Path != "" && pi.Path != "/" {
			s += ";obfs-uri=" + pi.Path
		}
		return s
	case "v2ray-plugin":
		s := "v2ray-plugin;mode=websocket"
		if pi.TLS {
			s += ";tls"
		}
		if pi.Host != "" {
			s += ";host=" + pi.Host
		}
		if pi.Path != "" {
			s += ";path=" + pi.Path
		}
		if pi.Mux {
			s += ";mux=1"
		}
		return s
	case "shadow-tls":
		return "shadow-tls;host=" + pi.Host + ";password=" + pi.Password + ";version=" + strconv.Itoa(pi.Version)
	}
	return ""
}

// memberNames returns a group's member names for one client: DIRECT and REJECT are renamed (reject "" drops
// REJECT members); an emptied group falls back to direct.
func memberNames(g subscription.ResolvedGroup, direct, reject string) []string {
	out := make([]string, 0, len(g.Members))
	for _, m := range g.Members {
		switch m.Kind {
		case subscription.MemberDirect:
			out = append(out, direct)
		case subscription.MemberReject:
			if reject != "" {
				out = append(out, reject)
			}
		default:
			out = append(out, m.Name)
		}
	}
	if len(out) == 0 {
		out = append(out, direct)
	}
	return out
}

// policyName maps DIRECT/REJECT to a client's spelling.
func policyName(name, direct, reject string) string {
	switch name {
	case subscription.PolicyDirect:
		return direct
	case subscription.PolicyReject:
		return reject
	}
	return name
}

// hostRule returns the request host when it should be forced DIRECT ("" for IPs and empty hosts).
func hostRule(ctx *subscription.RenderContext) string {
	if isDomain(ctx.RequestHost) {
		return strings.ToLower(ctx.RequestHost)
	}
	return ""
}

// isHTTPHeaderTCP reports a tcp network with an http header (xray "tcp http obfuscation").
func isHTTPHeaderTCP(p *subscription.Proxy) bool {
	return p.Network == "tcp" && p.Net.HeaderType == "http"
}
