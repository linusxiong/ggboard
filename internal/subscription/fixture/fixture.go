// Package fixture provides deterministic users and nodes covering every node type and transport, used by the
// subscription golden tests and the subscription API tests. All hosts are example.com names or documentation
// addresses (RFC 5737 / RFC 3849); every secret is an obvious fake.
package fixture

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/model"
)

// Fixed values shared by the tests.
const (
	UserUUID  = "3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f"
	UserToken = "0123456789abcdef0123456789abcdef"
	UserID    = 7
	GroupID   = 1
	GiB       = int64(1) << 30
)

// Now is the render time of golden files.
var Now = time.Date(2026, 10, 22, 8, 0, 0, 0, time.UTC)

// Created is the created_at of every node (shadowsocks-2022 keys derive from it; see NODE_PROTOCOLS §4).
var Created = time.Unix(1735689600, 0).UTC()

// User returns the fixture user (available: 3 GiB used of 100 GiB, expires 2027-01-01, resets 2026-11-01).
func User() *model.User {
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	reset := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	gid := uint64(GroupID)
	return &model.User{
		ID: UserID, Email: "alice@example.com", UUID: UserUUID, Token: UserToken, GroupID: &gid,
		TransferEnable: 100 * GiB, U: 1 * GiB, D: 2 * GiB, ExpiredAt: &exp, NextResetAt: &reset,
	}
}

func node(id uint64, typ, name, host, port string, ps model.JSONMap) *model.Node {
	return &model.Node{
		ID: id, Type: typ, Name: name, Host: host, Port: port, ServerPort: 443, GroupIDs: model.JSONSlice[uint64]{GroupID},
		Rate: 1, Show: true, Sort: int(id), ProtocolSettings: ps, CreatedAt: Created, UpdatedAt: Created,
	}
}

type m = model.JSONMap

// Nodes returns one node per interesting configuration, in display order. Node 21 is a child (relay) of node 3.
func Nodes() []*model.Node {
	reality := m{"server_name": "www.example.org", "public_key": "PUBLIC_KEY_BASE64URL", "short_id": "0123abcd",
		"dest": "www.example.org:443", "private_key": "PRIVATE_KEY_BASE64URL", "fingerprint": "chrome"}
	ns := []*model.Node{
		node(1, model.NodeVmess, "🇭🇰 HK VMess WS", "hk1.example.com", "443", m{
			"network": "ws", "network_settings": m{"path": "/ws", "headers": m{"Host": "cdn.example.com"}},
			"tls": 1, "tls_settings": m{"server_name": "hk1.example.com"}}),
		node(2, model.NodeVmess, "JP VMess TCP", "192.0.2.10", "10086", m{"network": "tcp"}),
		node(3, model.NodeVless, "🇺🇸 US VLESS Reality", "us1.example.com", "443", m{
			"network": "tcp", "tls": 2, "flow": "xtls-rprx-vision", "tls_settings": reality}),
		node(4, model.NodeVless, "SG VLESS XHTTP", "sg1.example.com", "443", m{
			"network": "xhttp", "network_settings": m{"path": "/xh", "host": "x.example.com", "mode": "auto"},
			"tls": 1, "tls_settings": m{"server_name": "sg1.example.com"}}),
		node(5, model.NodeVless, "TW VLESS WS", "tw1.example.com", "443", m{
			"network": "ws", "network_settings": m{"path": "/vl", "headers": m{"Host": "tw1.example.com"}},
			"tls": 1, "tls_settings": m{"server_name": "tw1.example.com", "allow_insecure": true}}),
		node(6, model.NodeVless, "KR VLESS gRPC", "kr1.example.com", "443", m{
			"network": "grpc", "network_settings": m{"serviceName": "vless-grpc"},
			"tls": 1, "tls_settings": m{"server_name": "kr1.example.com"}}),
		node(7, model.NodeShadowsocks, "HK SS obfs", "ss.example.com", "8388", m{
			"cipher": "aes-128-gcm", "obfs": "http", "obfs_settings": m{"path": "/", "host": "www.example.net"}}),
		node(8, model.NodeShadowsocks, "JP SS2022, Premium", "ss2022.example.com", "8389", m{"cipher": "2022-blake3-aes-128-gcm"}),
		node(9, model.NodeShadowsocks, "SS2022 ChaCha", "ss2022c.example.com", "8390", m{"cipher": "2022-blake3-chacha20-poly1305"}),
		node(10, model.NodeShadowsocksR, "HK SSR", "ssr.example.com", "8443", m{ //nolint:gosec // fake fixture secrets
			"cipher": "chacha20-ietf", "password": "ssr-node-pass", "protocol": "auth_chain_a", "obfs": "http_simple",
			"obfs_param": "www.example.net", "single_port_type": "protocol"}),
		node(11, model.NodeTrojan, "US Trojan", "t1.example.com", "443", m{"network": "tcp", "server_name": "t1.example.com"}),
		node(12, model.NodeTrojan, "US Trojan WS", "t2.example.com", "443", m{
			"network": "ws", "network_settings": m{"path": "/tj", "headers": m{"Host": "t2.example.com"}}, "server_name": "t2.example.com"}),
		node(13, model.NodeTrojan, "US Trojan gRPC", "t3.example.com", "443", m{
			"network": "grpc", "network_settings": m{"serviceName": "trojan-grpc"}, "server_name": "t3.example.com", "allow_insecure": true}),
		node(14, model.NodeHysteria, "HK Hysteria1", "hy1.example.com", "36712", m{ //nolint:gosec // fake fixture secrets
			"version": 1, "up_mbps": 200, "down_mbps": 1000, "obfs": "xplus", "obfs_password": "obfs-pass-123",
			"server_name": "hy1.example.com", "alpn": "h3"}),
		node(15, model.NodeHysteria, "JP Hysteria2 Hop", "hy2.example.com", "20000-30000", m{ //nolint:gosec // fake fixture secrets
			"version": 2, "up_mbps": 100, "down_mbps": 500, "obfs": "salamander", "obfs_password": "obfs-pass-456",
			"server_name": "hy2.example.com", "hop_interval": 30}),
		node(16, model.NodeHysteria, "SG Hysteria2", "198.51.100.20", "8443", m{
			"version": 2, "ignore_client_bandwidth": true, "server_name": "hy2s.example.com", "allow_insecure": true}),
		node(17, model.NodeTuic, "US TUIC", "tuic.example.com", "8443", m{
			"congestion_control": "bbr", "udp_relay_mode": "native", "server_name": "tuic.example.com", "alpn": "h3"}),
		node(18, model.NodeAnyTLS, "HK AnyTLS", "anytls.example.com", "8443", m{"tls": 1, "server_name": "anytls.example.com"}),
		node(19, model.NodeAnyTLS, "JP AnyTLS Reality", "anytlsr.example.com", "443", m{
			"tls": 2, "tls_settings": m{"server_names": []any{"www.example.org"}, "public_key": "PUBLIC_KEY_BASE64URL", "short_id": "0123abcd"}}),
		node(20, model.NodeMieru, "DE Mieru", "mieru.example.com", "2999", m{"transport": "TCP", "multiplexing": "MULTIPLEXING_LOW"}),
		node(21, model.NodeVless, "🇺🇸 US Relay", "relay.example.com", "8443", nil),
		node(22, model.NodeShadowsocks, "IPv6 SS", "2001:db8::10", "8388", m{"cipher": "chacha20-ietf-poly1305"}),
		node(23, model.NodeVmess, "VMess HTTP Header", "vh.example.com", "80", m{
			"network": "tcp", "network_settings": m{"header": m{"type": "http", "request": m{
				"path": []any{"/index"}, "headers": m{"Host": []any{"www.example.net"}}}}}}),
		node(24, model.NodeVmess, "VMess HTTPUpgrade", "vu.example.com", "443", m{
			"network": "httpupgrade", "network_settings": m{"path": "/up", "host": "vu.example.com"},
			"tls": 1, "tls_settings": m{"server_name": "vu.example.com"}}),
		node(25, model.NodeVless, "VLESS H2", "vh2.example.com", "443", m{
			"network": "h2", "network_settings": m{"path": "/h2", "host": []any{"vh2.example.com"}},
			"tls": 1, "tls_settings": m{"server_name": "vh2.example.com"}}),
	}
	parent := uint64(3)
	ns[20].ParentID = &parent
	ns[20].Parent = ns[2]
	return ns
}

// SeedNodes inserts Nodes into db (with their fixed ids) and returns them.
func SeedNodes(db *gorm.DB) ([]*model.Node, error) {
	nodes := Nodes()
	for _, n := range nodes {
		if err := db.Omit(clause.Associations).Create(n).Error; err != nil {
			return nil, err
		}
	}
	return nodes, nil
}
