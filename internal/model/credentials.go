package model

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"strconv"
)

// Credentials are the per-user secrets a client needs to connect to a node.
type Credentials struct {
	UUID string `json:"uuid"`
	// Password is what the client sends: uuid for trojan/hysteria/anytls/tuic/ssr/plain shadowsocks;
	// SS2022Password for shadowsocks-2022.
	Password string `json:"password"`
	// SS2022Key is the user PSK for shadowsocks-2022 ciphers ("" otherwise).
	SS2022Key string `json:"ss2022_key,omitempty"`
	MieruUser string `json:"mieru_user,omitempty"`
	MieruPass string `json:"mieru_pass,omitempty"`
}

// SS2022KeySize returns the PSK length for a shadowsocks-2022 cipher, or 0 for any other cipher.
func SS2022KeySize(cipher string) int {
	switch cipher {
	case "2022-blake3-aes-128-gcm":
		return 16
	case "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		return 32
	}
	return 0
}

// SS2022UserKey derives the user PSK: base64(uuid[:16]) for aes-128-gcm, base64(uuid[:32]) otherwise
// (Xboard Helper::uuidToBase64). Returns "" for non-2022 ciphers.
func SS2022UserKey(uuid, cipher string) string {
	n := SS2022KeySize(cipher)
	if n == 0 {
		return ""
	}
	if len(uuid) < n {
		n = len(uuid)
	}
	return base64.StdEncoding.EncodeToString([]byte(uuid[:n]))
}

// SS2022Password is the client password of a shadowsocks-2022 node: "<server_key>:<user_key>" (multi-user EIH,
// identical to Xboard) for the AES ciphers, the user key alone for 2022-blake3-chacha20-poly1305. SIP022 has no
// EIH for ChaCha, and sing-box and mihomo reject the whole config when such an outbound carries two keys.
func SS2022Password(cipher, serverKey, userKey string) string {
	if cipher == "2022-blake3-chacha20-poly1305" {
		return userKey
	}
	return serverKey + ":" + userKey
}

// NodeServerKey returns the shadowsocks-2022 server PSK of a node: protocol_settings.server_key when set,
// otherwise base64(md5hex(created_at unix)[:16|32]) (Xboard Helper::getServerKey). Child nodes use their
// parent's created_at when Parent is loaded. Returns "" for non-2022 nodes.
func NodeServerKey(n *Node) string {
	if n == nil || n.Type != NodeShadowsocks {
		return ""
	}
	var ss ShadowsocksSettings
	if err := ss.Decode(n.ProtocolSettings); err != nil {
		return ""
	}
	size := SS2022KeySize(ss.Cipher)
	if size == 0 {
		return ""
	}
	if ss.ServerKey != "" {
		return ss.ServerKey
	}
	created := n.CreatedAt
	if n.ParentID != nil && n.Parent != nil {
		created = n.Parent.CreatedAt
	}
	sum := md5.Sum([]byte(strconv.FormatInt(created.Unix(), 10)))
	return base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(sum[:])[:size]))
}

// UserCredentials derives the connection credentials of u on node n (see docs/DATA_MODEL.md).
func UserCredentials(u *User, n *Node) Credentials {
	c := Credentials{UUID: u.UUID, Password: u.UUID}
	if n == nil {
		return c
	}
	switch n.Type {
	case NodeShadowsocks:
		var ss ShadowsocksSettings
		if err := ss.Decode(n.ProtocolSettings); err == nil && ss.Is2022() {
			c.SS2022Key = SS2022UserKey(u.UUID, ss.Cipher)
			c.Password = SS2022Password(ss.Cipher, NodeServerKey(n), c.SS2022Key)
		}
	case NodeMieru:
		c.MieruUser = u.UUID
		c.MieruPass = u.UUID
	}
	return c
}
