package model

import (
	"encoding/json"
	"strings"
)

// Typed views of Node.ProtocolSettings. Decode applies the defaults below and then overlays the stored JSON:
//
//	var s model.VlessSettings
//	if err := s.Decode(node.ProtocolSettings); err != nil { ... }
//
// or generically: s, err := model.DecodeSettings[model.VlessSettings](node.ProtocolSettings).

// TLSSettings are the TLS / REALITY options shared by several protocols.
type TLSSettings struct {
	ServerName    string   `json:"server_name"`
	AllowInsecure bool     `json:"allow_insecure"`
	PublicKey     string   `json:"public_key,omitempty"`  // reality
	PrivateKey    string   `json:"private_key,omitempty"` // reality
	ShortID       string   `json:"short_id,omitempty"`    // reality
	Dest          string   `json:"dest,omitempty"`        // reality handshake target host:port
	ServerPort    string   `json:"server_port,omitempty"` // reality handshake target port
	Fingerprint   string   `json:"fingerprint,omitempty"`
	ServerNames   []string `json:"server_names,omitempty"`
}

// TLS modes for vmess/vless/anytls "tls".
const (
	TLSNone    = 0
	TLSEnabled = 1
	TLSReality = 2
)

// VmessSettings for type=vmess.
type VmessSettings struct {
	Network         string      `json:"network"`
	NetworkSettings JSONMap     `json:"network_settings"`
	TLS             int         `json:"tls"`
	TLSSettings     TLSSettings `json:"tls_settings"`
}

// VlessSettings for type=vless.
type VlessSettings struct {
	Network         string      `json:"network"`
	NetworkSettings JSONMap     `json:"network_settings"`
	TLS             int         `json:"tls"` // 0 none, 1 tls, 2 reality
	Flow            string      `json:"flow"`
	Encryption      string      `json:"encryption"` // client-side value used in subscriptions
	Decryption      string      `json:"decryption"` // server-side VLESS encryption string ("" = none)
	TLSSettings     TLSSettings `json:"tls_settings"`
}

// ObfsSettings is the simple-obfs/v2ray-plugin option block of shadowsocks.
type ObfsSettings struct {
	Path string `json:"path"`
	Host string `json:"host"`
}

// ShadowsocksSettings for type=shadowsocks.
type ShadowsocksSettings struct {
	Cipher       string       `json:"cipher"`
	ServerKey    string       `json:"server_key"`
	Obfs         string       `json:"obfs"` // plain | http | tls
	ObfsSettings ObfsSettings `json:"obfs_settings"`
	Plugin       string       `json:"plugin"`
	PluginOpts   string       `json:"plugin_opts"`
}

// Is2022 reports whether the cipher is a shadowsocks-2022 (blake3) cipher.
func (s ShadowsocksSettings) Is2022() bool { return SS2022KeySize(s.Cipher) > 0 }

// ShadowsocksRSettings for type=shadowsocksr.
type ShadowsocksRSettings struct {
	Cipher         string `json:"cipher"`
	Password       string `json:"password"` // node-level SSR password (soga config.password)
	Protocol       string `json:"protocol"`
	ProtocolParam  string `json:"protocol_param"`
	Obfs           string `json:"obfs"`
	ObfsParam      string `json:"obfs_param"`
	SinglePortType string `json:"single_port_type"` // protocol | obfs | ""
}

// TrojanSettings for type=trojan.
type TrojanSettings struct {
	Network         string  `json:"network"`
	NetworkSettings JSONMap `json:"network_settings"`
	ServerName      string  `json:"server_name"`
	AllowInsecure   bool    `json:"allow_insecure"`
}

// HysteriaSettings for type=hysteria (version 1 or 2).
type HysteriaSettings struct {
	Version  int `json:"version"`
	UpMbps   int `json:"up_mbps"`
	DownMbps int `json:"down_mbps"`
	// IgnoreClientBandwidth makes the server ignore client-declared bandwidth (hysteria2 BBR mode).
	IgnoreClientBandwidth bool   `json:"ignore_client_bandwidth"`
	Obfs                  string `json:"obfs"` // plain | salamander
	ObfsPassword          string `json:"obfs_password"`
	ServerName            string `json:"server_name"`
	AllowInsecure         bool   `json:"allow_insecure"`
	ALPN                  string `json:"alpn"`
	HopInterval           int    `json:"hop_interval"`
}

// AnyTLSSettings for type=anytls.
type AnyTLSSettings struct {
	TLS           int         `json:"tls"` // 1 tls, 2 reality
	ServerName    string      `json:"server_name"`
	AllowInsecure bool        `json:"allow_insecure"`
	PaddingScheme []string    `json:"padding_scheme"`
	TLSSettings   TLSSettings `json:"tls_settings"`
}

// MieruSettings for type=mieru.
type MieruSettings struct {
	Transport    string `json:"transport"`    // TCP | UDP
	Multiplexing string `json:"multiplexing"` // MULTIPLEXING_LOW | _MIDDLE | _HIGH
}

// TuicSettings for type=tuic.
type TuicSettings struct {
	CongestionControl string `json:"congestion_control"`
	ZeroRTTHandshake  bool   `json:"zero_rtt_handshake"`
	UDPRelayMode      string `json:"udp_relay_mode"`
	ServerName        string `json:"server_name"`
	AllowInsecure     bool   `json:"allow_insecure"`
	ALPN              string `json:"alpn"`
}

// ProtocolSettings is implemented by every *XxxSettings type.
type ProtocolSettings interface {
	SetDefaults()
	Decode(m JSONMap) error
}

// SetDefaults fills sane defaults.
func (s *VmessSettings) SetDefaults() { *s = VmessSettings{Network: "tcp"} }

// SetDefaults fills sane defaults.
func (s *VlessSettings) SetDefaults() { *s = VlessSettings{Network: "tcp", Encryption: "none"} }

// SetDefaults fills sane defaults.
func (s *ShadowsocksSettings) SetDefaults() {
	*s = ShadowsocksSettings{Cipher: "aes-128-gcm", Obfs: "plain", ObfsSettings: ObfsSettings{Path: "/"}}
}

// SetDefaults fills sane defaults.
func (s *ShadowsocksRSettings) SetDefaults() {
	*s = ShadowsocksRSettings{Cipher: "chacha20-ietf", Protocol: "origin", Obfs: "plain"}
}

// SetDefaults fills sane defaults.
func (s *TrojanSettings) SetDefaults() { *s = TrojanSettings{Network: "tcp"} }

// SetDefaults fills sane defaults.
func (s *HysteriaSettings) SetDefaults() {
	*s = HysteriaSettings{Version: 2, UpMbps: 100, DownMbps: 100, Obfs: "plain", ALPN: "h3", HopInterval: 30}
}

// SetDefaults fills sane defaults.
func (s *AnyTLSSettings) SetDefaults() { *s = AnyTLSSettings{TLS: TLSEnabled} }

// SetDefaults fills sane defaults.
func (s *MieruSettings) SetDefaults() {
	*s = MieruSettings{Transport: "TCP", Multiplexing: "MULTIPLEXING_LOW"}
}

// SetDefaults fills sane defaults.
func (s *TuicSettings) SetDefaults() {
	*s = TuicSettings{CongestionControl: "bbr", UDPRelayMode: "native", ALPN: "h3"}
}

// Decode applies defaults then overlays m.
func (s *VmessSettings) Decode(m JSONMap) error { s.SetDefaults(); return decodeOver(m, s) }

// Decode applies defaults then overlays m.
func (s *VlessSettings) Decode(m JSONMap) error { s.SetDefaults(); return decodeOver(m, s) }

// Decode applies defaults then overlays m.
func (s *ShadowsocksSettings) Decode(m JSONMap) error { s.SetDefaults(); return decodeOver(m, s) }

// Decode applies defaults then overlays m.
func (s *ShadowsocksRSettings) Decode(m JSONMap) error { s.SetDefaults(); return decodeOver(m, s) }

// Decode applies defaults then overlays m.
func (s *TrojanSettings) Decode(m JSONMap) error { s.SetDefaults(); return decodeOver(m, s) }

// Decode applies defaults then overlays m.
func (s *HysteriaSettings) Decode(m JSONMap) error { s.SetDefaults(); return decodeOver(m, s) }

// Decode applies defaults then overlays m.
func (s *AnyTLSSettings) Decode(m JSONMap) error { s.SetDefaults(); return decodeOver(m, s) }

// Decode applies defaults then overlays m.
func (s *MieruSettings) Decode(m JSONMap) error { s.SetDefaults(); return decodeOver(m, s) }

// Decode applies defaults then overlays m.
func (s *TuicSettings) Decode(m JSONMap) error { s.SetDefaults(); return decodeOver(m, s) }

// DecodeSettings is the generic form of XxxSettings.Decode.
func DecodeSettings[T any, P interface {
	*T
	ProtocolSettings
}](m JSONMap) (T, error) {
	var v T
	err := P(&v).Decode(m)
	return v, err
}

// NewSettingsFor returns a pointer to the zero-defaulted settings struct for a node type (nil if unknown).
func NewSettingsFor(nodeType string) ProtocolSettings {
	var s ProtocolSettings
	switch nodeType {
	case NodeVmess:
		s = &VmessSettings{}
	case NodeVless:
		s = &VlessSettings{}
	case NodeShadowsocks:
		s = &ShadowsocksSettings{}
	case NodeShadowsocksR:
		s = &ShadowsocksRSettings{}
	case NodeTrojan:
		s = &TrojanSettings{}
	case NodeHysteria:
		s = &HysteriaSettings{}
	case NodeAnyTLS:
		s = &AnyTLSSettings{}
	case NodeMieru:
		s = &MieruSettings{}
	case NodeTuic:
		s = &TuicSettings{}
	default:
		return nil
	}
	s.SetDefaults()
	return s
}

// decodeOver unmarshals m into dst without resetting fields absent from m. Explicit JSON nulls keep defaults.
func decodeOver(m JSONMap, dst any) error {
	if len(m) == 0 {
		return nil
	}
	clean := make(map[string]any, len(m))
	for k, v := range m {
		if v != nil {
			clean[k] = v
		}
	}
	b, err := json.Marshal(clean)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// NetworkSettings is a normalized view of the xray-style network_settings object.
type NetworkSettings struct {
	Path        string         // ws/http/h2/httpupgrade/xhttp path
	Host        string         // first host (ws headers.Host, httpupgrade/xhttp host, http/h2 host[0])
	Hosts       []string       // all hosts
	ServiceName string         // grpc
	Mode        string         // xhttp mode
	HeaderType  string         // tcp header.type (none|http)
	Extra       map[string]any // xhttp extra
}

// DecodeNetwork normalizes network_settings for network (tcp|ws|grpc|http|h2|httpupgrade|xhttp).
func DecodeNetwork(network string, m JSONMap) NetworkSettings {
	ns := NetworkSettings{Path: m.String("path"), ServiceName: m.String("serviceName"), Mode: m.String("mode")}
	if ns.ServiceName == "" {
		ns.ServiceName = m.String("service_name")
	}
	if extra := m.Map("extra"); extra != nil {
		ns.Extra = extra
	}
	if hdr := m.Map("header"); hdr != nil {
		ns.HeaderType = hdr.String("type")
		if req := hdr.Map("request"); req != nil {
			if headers := req.Map("headers"); headers != nil {
				ns.Hosts = append(ns.Hosts, anyStrings(headers["Host"])...)
			}
			if p := anyStrings(req["path"]); len(p) > 0 && ns.Path == "" {
				ns.Path = p[0]
			}
		}
	}
	ns.Hosts = append(ns.Hosts, anyStrings(m["host"])...)
	if headers := m.Map("headers"); headers != nil {
		ns.Hosts = append(ns.Hosts, anyStrings(headers["Host"])...)
		ns.Hosts = append(ns.Hosts, anyStrings(headers["host"])...)
	}
	if len(ns.Hosts) > 0 {
		ns.Host = ns.Hosts[0]
	}
	if ns.Path == "" && network != "tcp" && network != "grpc" && network != "" {
		ns.Path = "/"
	}
	return ns
}

func anyStrings(v any) []string {
	switch x := v.(type) {
	case string:
		var out []string
		for _, p := range strings.Split(x, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return x
	}
	return nil
}
