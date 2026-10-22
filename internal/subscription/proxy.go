package subscription

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/linusxiong/ggboard/internal/model"
)

// Proxy is one entry of a subscription: a node (child nodes are entries of their own) with its decoded
// protocol settings and the requesting user's credentials, or an info pseudo-node. Exactly one of the typed
// settings pointers is set, matching Type. Formats must treat a Proxy as read-only.
type Proxy struct {
	NodeID uint64
	Name   string
	Type   string // model.Node* type
	Host   string
	Port   int    // client port (first port of a range)
	Ports  string // port-hopping spec such as "20000-30000" or "443,20000-30000" ("" = single port)
	Tags   []string
	Info   bool // account-information pseudo node (127.0.0.1:1080)
	Status int  // online.State* of the effective node (0 for info nodes)

	UUID     string
	Password string // uuid; "<server_key>:<user_key>" for shadowsocks-2022; the node password for shadowsocksr
	// SSRProtocolParam is the single-port protocol parameter "<user id>:<uuid>" (shadowsocksr only).
	SSRProtocolParam string

	Vmess    *model.VmessSettings
	Vless    *model.VlessSettings
	SS       *model.ShadowsocksSettings
	SSR      *model.ShadowsocksRSettings
	Trojan   *model.TrojanSettings
	Hysteria *model.HysteriaSettings
	AnyTLS   *model.AnyTLSSettings
	Mieru    *model.MieruSettings
	Tuic     *model.TuicSettings
	Net      model.NetworkSettings // vmess / vless / trojan transport (normalized)
	Network  string                // vmess / vless / trojan network: tcp | ws | grpc | http | h2 | httpupgrade | xhttp
}

// ProtoName is the short protocol name shown with show_protocol_to_server_enable and used in share links:
// ss, ssr, vmess, vless, trojan, hysteria, hysteria2, tuic, anytls, mieru.
func (p *Proxy) ProtoName() string {
	switch p.Type {
	case model.NodeShadowsocks:
		return "ss"
	case model.NodeShadowsocksR:
		return "ssr"
	case model.NodeHysteria:
		if p.IsHysteria2() {
			return "hysteria2"
		}
		return "hysteria"
	}
	return p.Type
}

// IsHysteria2 reports whether the proxy is a hysteria v2 node.
func (p *Proxy) IsHysteria2() bool {
	return p.Type == model.NodeHysteria && p.Hysteria != nil && p.Hysteria.Version != 1
}

// TLSMode returns the tls mode of vmess/vless/anytls (model.TLSNone/TLSEnabled/TLSReality); trojan,
// hysteria and tuic are always TLS.
func (p *Proxy) TLSMode() int {
	switch {
	case p.Vmess != nil:
		if p.Vmess.TLS != 0 {
			return model.TLSEnabled
		}
		return model.TLSNone
	case p.Vless != nil:
		return p.Vless.TLS
	case p.AnyTLS != nil:
		if p.AnyTLS.TLS == model.TLSReality {
			return model.TLSReality
		}
		return model.TLSEnabled
	case p.Trojan != nil, p.Hysteria != nil, p.Tuic != nil:
		return model.TLSEnabled
	}
	return model.TLSNone
}

// SNI returns the TLS server name of the proxy ("" when unset).
func (p *Proxy) SNI() string {
	switch {
	case p.Vmess != nil:
		return p.Vmess.TLSSettings.ServerName
	case p.Vless != nil:
		return p.Vless.TLSSettings.ServerName
	case p.Trojan != nil:
		return p.Trojan.ServerName
	case p.Hysteria != nil:
		return p.Hysteria.ServerName
	case p.Tuic != nil:
		return p.Tuic.ServerName
	case p.AnyTLS != nil:
		if p.AnyTLS.TLS == model.TLSReality {
			if len(p.AnyTLS.TLSSettings.ServerNames) > 0 && p.AnyTLS.TLSSettings.ServerNames[0] != "" {
				return p.AnyTLS.TLSSettings.ServerNames[0]
			}
			if p.AnyTLS.TLSSettings.ServerName != "" {
				return p.AnyTLS.TLSSettings.ServerName
			}
		}
		return p.AnyTLS.ServerName
	}
	return ""
}

// Insecure reports whether certificate verification is disabled.
func (p *Proxy) Insecure() bool {
	switch {
	case p.Vmess != nil:
		return p.Vmess.TLSSettings.AllowInsecure
	case p.Vless != nil:
		return p.Vless.TLSSettings.AllowInsecure
	case p.Trojan != nil:
		return p.Trojan.AllowInsecure
	case p.Hysteria != nil:
		return p.Hysteria.AllowInsecure
	case p.Tuic != nil:
		return p.Tuic.AllowInsecure
	case p.AnyTLS != nil:
		return p.AnyTLS.AllowInsecure
	}
	return false
}

// Reality returns the REALITY settings (public key, short id) of a vless/anytls proxy in reality mode.
func (p *Proxy) Reality() (publicKey, shortID string, ok bool) {
	switch {
	case p.Vless != nil && p.Vless.TLS == model.TLSReality:
		return p.Vless.TLSSettings.PublicKey, p.Vless.TLSSettings.ShortID, true
	case p.AnyTLS != nil && p.AnyTLS.TLS == model.TLSReality:
		return p.AnyTLS.TLSSettings.PublicKey, p.AnyTLS.TLSSettings.ShortID, true
	}
	return "", "", false
}

// Fingerprint returns the uTLS fingerprint ("" = client default). REALITY always needs one: "chrome" is
// used when none is configured.
func (p *Proxy) Fingerprint() string {
	fp := ""
	switch {
	case p.Vmess != nil:
		fp = p.Vmess.TLSSettings.Fingerprint
	case p.Vless != nil:
		fp = p.Vless.TLSSettings.Fingerprint
	case p.AnyTLS != nil:
		fp = p.AnyTLS.TLSSettings.Fingerprint
	}
	if fp == "" {
		if _, _, ok := p.Reality(); ok {
			return "chrome"
		}
	}
	return fp
}

// ALPN returns the ALPN list of hysteria/tuic proxies (comma-separated setting; default h3).
func (p *Proxy) ALPN() []string {
	raw := ""
	switch {
	case p.Hysteria != nil:
		raw = p.Hysteria.ALPN
	case p.Tuic != nil:
		raw = p.Tuic.ALPN
	default:
		return nil
	}
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = []string{"h3"}
	}
	return out
}

// SSObfs returns the simple-obfs mode ("http" or "tls") of a shadowsocks proxy, or "".
func (p *Proxy) SSObfs() string {
	if p.SS == nil {
		return ""
	}
	switch p.SS.Obfs {
	case "http", "tls":
		return p.SS.Obfs
	}
	return ""
}

// HysteriaObfs returns the obfuscation password of a hysteria proxy ("" = no obfuscation). For v2 the
// obfuscation type is always salamander; for v1 the value is the xplus obfs string.
func (p *Proxy) HysteriaObfs() string {
	h := p.Hysteria
	if h == nil || h.ObfsPassword == "" {
		return ""
	}
	if p.IsHysteria2() {
		if h.Obfs == "salamander" {
			return h.ObfsPassword
		}
		return ""
	}
	if h.Obfs != "" && h.Obfs != "plain" {
		return h.ObfsPassword
	}
	return ""
}

// ParsePorts splits a node port string into the first port and the port-hopping spec. "443" -> (443, ""),
// "20000-30000" -> (20000, "20000-30000"), "443,20000-30000" -> (443, "443,20000-30000").
func ParsePorts(s string) (int, string, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if s == "" {
		return 0, "", errors.New("empty port")
	}
	first := 0
	for _, part := range strings.Split(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := parsePort(lo)
		if err != nil {
			return 0, "", err
		}
		if isRange {
			b, err := parsePort(hi)
			if err != nil || b < a {
				return 0, "", fmt.Errorf("invalid port range %q", part)
			}
		}
		if first == 0 {
			first = a
		}
	}
	if !strings.ContainsAny(s, "-,") {
		return first, "", nil
	}
	return first, s, nil
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return n, nil
}

// decodeProxy builds the user-independent part of a Proxy for entry n whose effective node (parent for child
// nodes) is eff. It also returns the shadowsocks-2022 server key or the shadowsocksr node password.
func decodeProxy(n, eff *model.Node) (Proxy, string, error) {
	port, ports, err := ParsePorts(n.Port)
	if err != nil {
		return Proxy{}, "", err
	}
	p := Proxy{NodeID: n.ID, Name: n.Name, Type: eff.Type, Host: strings.TrimSpace(n.Host), Port: port, Ports: ports, Tags: n.Tags}
	if p.Host == "" {
		return Proxy{}, "", errors.New("empty host")
	}
	ps := eff.ProtocolSettings
	secret := ""
	switch eff.Type {
	case model.NodeVmess:
		var s model.VmessSettings
		err = s.Decode(ps)
		p.Vmess, p.Network = &s, normNetwork(s.Network)
		p.Net = model.DecodeNetwork(p.Network, s.NetworkSettings)
	case model.NodeVless:
		var s model.VlessSettings
		err = s.Decode(ps)
		p.Vless, p.Network = &s, normNetwork(s.Network)
		p.Net = model.DecodeNetwork(p.Network, s.NetworkSettings)
	case model.NodeTrojan:
		var s model.TrojanSettings
		err = s.Decode(ps)
		p.Trojan, p.Network = &s, normNetwork(s.Network)
		p.Net = model.DecodeNetwork(p.Network, s.NetworkSettings)
	case model.NodeShadowsocks:
		var s model.ShadowsocksSettings
		err = s.Decode(ps)
		p.SS = &s
		if err == nil && s.Is2022() {
			secret = model.NodeServerKey(eff)
		}
	case model.NodeShadowsocksR:
		var s model.ShadowsocksRSettings
		err = s.Decode(ps)
		p.SSR = &s
		secret = s.Password
		if secret == "" {
			sum := md5.Sum([]byte(strconv.FormatInt(eff.CreatedAt.Unix(), 10)))
			secret = base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(sum[:])[:16]))
		}
	case model.NodeHysteria:
		var s model.HysteriaSettings
		err = s.Decode(ps)
		p.Hysteria = &s
	case model.NodeAnyTLS:
		var s model.AnyTLSSettings
		err = s.Decode(ps)
		p.AnyTLS = &s
	case model.NodeMieru:
		var s model.MieruSettings
		err = s.Decode(ps)
		p.Mieru = &s
	case model.NodeTuic:
		var s model.TuicSettings
		err = s.Decode(ps)
		p.Tuic = &s
	default:
		return Proxy{}, "", fmt.Errorf("unsupported node type %q", eff.Type)
	}
	if err != nil {
		return Proxy{}, "", fmt.Errorf("protocol_settings: %w", err)
	}
	if ports != "" && eff.Type != model.NodeHysteria && eff.Type != model.NodeTuic && eff.Type != model.NodeMieru {
		p.Ports = "" // port hopping only exists for QUIC protocols and mieru
	}
	return p, secret, nil
}

func normNetwork(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	if n == "" {
		return "tcp"
	}
	return n
}

// withUser fills the credentials of u into the template p.
func withUser(p Proxy, secret string, u *model.User) Proxy {
	p.UUID, p.Password = u.UUID, u.UUID
	switch {
	case p.SS != nil && p.SS.Is2022():
		p.Password = secret + ":" + model.SS2022UserKey(u.UUID, p.SS.Cipher)
	case p.SSR != nil:
		p.Password = secret
		p.SSRProtocolParam = strconv.FormatUint(u.ID, 10) + ":" + u.UUID
	}
	return p
}

// ProxyOptions are the per-request node filters of docs/SUBSCRIPTION.md.
type ProxyOptions struct {
	Types        []string // node types (aliases ss, ssr, hy, hy2, hysteria2 ...); empty = all
	Keywords     []string // case-insensitive name/tag substrings; empty = all
	StripEmoji   bool
	ShowProtocol bool // prefix names with "[vmess] " (show_protocol_to_server_enable)
}

// ParseList splits "a,b|c" on commas and pipes, trimming blanks.
func ParseList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '|' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func typeMatches(p *Proxy, want []string) bool {
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		switch strings.ToLower(w) {
		case p.Type:
			return true
		case "ss":
			if p.Type == model.NodeShadowsocks {
				return true
			}
		case "ssr":
			if p.Type == model.NodeShadowsocksR {
				return true
			}
		case "hysteria2", "hy2":
			if p.IsHysteria2() {
				return true
			}
		case "hysteria1", "hy", "hy1":
			if p.Type == model.NodeHysteria && !p.IsHysteria2() {
				return true
			}
		}
	}
	return false
}

func keywordMatches(p *Proxy, kws []string) bool {
	if len(kws) == 0 {
		return true
	}
	name := strings.ToLower(p.Name)
	for _, k := range kws {
		k = strings.ToLower(k)
		if strings.Contains(name, k) {
			return true
		}
		for _, t := range p.Tags {
			if strings.Contains(strings.ToLower(t), k) {
				return true
			}
		}
	}
	return false
}

// NodeProxies turns available nodes into proxies carrying u's credentials, applying opt. Nodes whose settings
// cannot be decoded were already dropped by AvailableNodes.
func NodeProxies(u *model.User, nodes []AvailableNode, opt ProxyOptions) []Proxy {
	out := make([]Proxy, 0, len(nodes))
	for _, n := range nodes {
		if n.entry == nil {
			continue
		}
		p := n.entry.proxy
		if !typeMatches(&p, opt.Types) || !keywordMatches(&p, opt.Keywords) {
			continue
		}
		p = withUser(p, n.entry.secret, u)
		p.Status = n.Status
		p.Name = DisplayName(p.Name, opt.StripEmoji)
		if p.Name == "" {
			p.Name = "node-" + strconv.FormatUint(p.NodeID, 10)
		}
		if opt.ShowProtocol {
			p.Name = "[" + p.ProtoName() + "] " + p.Name
		}
		out = append(out, p)
	}
	return out
}

// DisplayName normalizes a node name: optional emoji removal, control characters and separators that
// break line-based client configs (",", "=") replaced, whitespace collapsed.
func DisplayName(name string, stripEmoji bool) string {
	var b strings.Builder
	b.Grow(len(name))
	space := false
	for _, r := range name {
		switch {
		case stripEmoji && IsEmoji(r):
			continue
		case r == ',':
			r = '，'
		case r == '=':
			r = '＝'
		case unicode.IsControl(r) || unicode.IsSpace(r):
			if !space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// IsEmoji reports whether r is an emoji or emoji component (flags, pictographs, dingbats, variation selectors,
// joiners, keycaps, tags).
func IsEmoji(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF, // pictographs, emoticons, transport, flags (regional indicators), ...
		r >= 0x2600 && r <= 0x27BF,   // misc symbols, dingbats
		r >= 0x2300 && r <= 0x23FF,   // misc technical (⌚ ⏰)
		r >= 0x2B00 && r <= 0x2BFF,   // arrows, ⭐
		r >= 0xE0020 && r <= 0xE007F, // tag characters (subdivision flags)
		r == 0x200D, r == 0xFE0E, r == 0xFE0F, r == 0x20E3:
		return true
	}
	return false
}

// UniqueNames renames proxies whose names collide with each other, with reserved (group) names or with
// built-in policies by appending " 2", " 3", ... (first occurrence keeps its name).
func UniqueNames(proxies []Proxy, reserved map[string]bool) {
	used := make(map[string]bool, len(proxies)+len(reserved))
	for k := range reserved {
		used[k] = true
	}
	for i := range proxies {
		name := proxies[i].Name
		if !used[name] && !IsReservedName(name) {
			used[name] = true
			continue
		}
		for n := 2; ; n++ {
			cand := name + " " + strconv.Itoa(n)
			if !used[cand] && !IsReservedName(cand) {
				proxies[i].Name = cand
				used[cand] = true
				break
			}
		}
	}
}

// BuildProxy builds the proxy of node n (eff is its effective node: the parent for child nodes, else n) with
// u's credentials, without display-name processing. AvailableNodes + NodeProxies is the cached path used by
// subscriptions; BuildProxy serves previews of single nodes and tests.
func BuildProxy(n, eff *model.Node, u *model.User) (Proxy, error) {
	if eff == nil {
		eff = n
	}
	p, secret, err := decodeProxy(n, eff)
	if err != nil {
		return Proxy{}, err
	}
	return withUser(p, secret, u), nil
}
