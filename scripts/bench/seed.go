package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Seed markers: everything the benchmark creates is recognisable, so a second run reuses it (idempotent).
const (
	benchGroup        = "Bench"
	benchPlan         = "Bench 200 GB"
	benchEmailPrefix  = "bench-"
	benchEmailSuffix  = "example.com"
	benchHostSuffix   = ".bench.example.com"
	benchUserPassword = "Bench-Passw0rd!!" // throwaway test value for the generated example.com accounts
	generateBatch     = 500                // POST /admin/users/generate limit
)

// benchUser is a seeded account.
type benchUser struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
	Token string `json:"token"`
}

// benchNode is a seeded node and the UniProxy node_type its backend would send.
type benchNode struct {
	ID       uint64
	Type     string
	UniType  string
	Name     string
	Hostname string
}

// dataset is what the scenarios need.
type dataset struct {
	ServerToken string
	PlanID      uint64
	Users       []benchUser
	Nodes       []benchNode
	Created     struct{ Users, Nodes int }
}

var regions = []struct{ code, city string }{
	{"HK", "Hong Kong"}, {"JP", "Tokyo"}, {"SG", "Singapore"}, {"US", "Los Angeles"}, {"TW", "Taipei"},
	{"KR", "Seoul"}, {"GB", "London"}, {"DE", "Frankfurt"}, {"NL", "Amsterdam"}, {"CA", "Toronto"},
}

// nodeKinds is the protocol mix of the seeded nodes (cycled).
var nodeKinds = []struct{ typ, uni, label string }{
	{"vless", "vless", "REALITY"},
	{"vmess", "vmess", "WS"},
	{"trojan", "trojan", "Trojan"},
	{"shadowsocks", "shadowsocks", "SS2022"},
	{"hysteria", "hysteria2", "HY2"},
	{"anytls", "anytls", "AnyTLS"},
}

// uniTypeOf maps a node type to the UniProxy node_type (seeded hysteria nodes are version 2).
func uniTypeOf(typ string) string {
	for _, k := range nodeKinds {
		if k.typ == typ {
			return k.uni
		}
	}
	return typ
}

func randB64(n int, enc *base64.Encoding) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return enc.EncodeToString(b)
}

// nodePayload builds a valid POST /admin/nodes body for the i-th seeded node.
func nodePayload(i int, group uint64) (map[string]any, error) {
	r := regions[i%len(regions)]
	k := nodeKinds[i%len(nodeKinds)]
	host := fmt.Sprintf("%s%02d%s", strings.ToLower(r.code), i+1, benchHostSuffix)
	var ps map[string]any
	switch k.typ {
	case "vless":
		key, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		ps = map[string]any{"network": "tcp", "tls": 2, "flow": "xtls-rprx-vision", "tls_settings": map[string]any{
			"server_name": "www.example.org", "dest": "www.example.org:443", "server_port": "443",
			"public_key":  base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
			"private_key": base64.RawURLEncoding.EncodeToString(key.Bytes()),
			"short_id":    "0123abcd", "fingerprint": "chrome",
		}}
	case "vmess":
		ps = map[string]any{"network": "ws", "network_settings": map[string]any{"path": "/vmess", "headers": map[string]any{"Host": host}},
			"tls": 1, "tls_settings": map[string]any{"server_name": host}}
	case "trojan":
		ps = map[string]any{"network": "tcp", "server_name": host}
	case "shadowsocks":
		ps = map[string]any{"cipher": "2022-blake3-aes-128-gcm", "server_key": randB64(16, base64.StdEncoding)}
	case "hysteria":
		ps = map[string]any{"version": 2, "up_mbps": 500, "down_mbps": 1000, "obfs": "salamander",
			"obfs_password": randB64(12, base64.RawURLEncoding), "server_name": host}
	case "anytls":
		ps = map[string]any{"tls": 1, "server_name": host}
	}
	return map[string]any{
		"type": k.typ, "name": fmt.Sprintf("%s %02d %s", r.city, i/len(regions)+1, k.label),
		"group_ids": []uint64{group}, "host": host, "port": "443", "server_port": 443,
		"tags": []string{r.code}, "rate": 1, "show": true, "protocol_settings": ps,
	}, nil
}

// seed creates (or finds) the benchmark group, plan, nodes and users and loads every user's token.
func seed(ctx context.Context, a *api, wantUsers, wantNodes int, logf func(string, ...any)) (*dataset, error) {
	ds := &dataset{}

	var settings struct {
		Data struct {
			Server struct {
				ServerToken string `json:"server_token"`
			} `json:"server"`
		} `json:"data"`
	}
	if err := a.call(ctx, http.MethodGet, "/api/v1/admin/settings", nil, &settings); err != nil {
		return nil, err
	}
	if ds.ServerToken = settings.Data.Server.ServerToken; len(ds.ServerToken) < 16 {
		return nil, errors.New("server_token is not set")
	}

	// group
	var groups struct {
		Data []struct {
			ID   uint64 `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := a.call(ctx, http.MethodGet, "/api/v1/admin/groups", nil, &groups); err != nil {
		return nil, err
	}
	var groupID uint64
	for _, g := range groups.Data {
		if g.Name == benchGroup {
			groupID = g.ID
		}
	}
	if groupID == 0 {
		var g struct {
			Data struct {
				ID uint64 `json:"id"`
			} `json:"data"`
		}
		if err := a.call(ctx, http.MethodPost, "/api/v1/admin/groups", map[string]string{"name": benchGroup}, &g); err != nil {
			return nil, err
		}
		groupID = g.Data.ID
	}

	// plan
	var plans struct {
		Data []struct {
			ID   uint64 `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := a.call(ctx, http.MethodGet, "/api/v1/admin/plans", nil, &plans); err != nil {
		return nil, err
	}
	for _, p := range plans.Data {
		if p.Name == benchPlan {
			ds.PlanID = p.ID
		}
	}
	if ds.PlanID == 0 {
		var p struct {
			Data struct {
				ID uint64 `json:"id"`
			} `json:"data"`
		}
		body := map[string]any{"group_id": groupID, "name": benchPlan, "content": "- 200 GB / month\n- 3 devices",
			"transfer_enable": 200, "device_limit": 3, "prices": map[string]int{"monthly": 1500, "yearly": 15000},
			"show": true, "sell": true, "renew": true}
		if err := a.call(ctx, http.MethodPost, "/api/v1/admin/plans", body, &p); err != nil {
			return nil, err
		}
		ds.PlanID = p.Data.ID
	}

	// nodes
	nodes, err := listBenchNodes(ctx, a)
	if err != nil {
		return nil, err
	}
	for i := len(nodes); i < wantNodes; i++ {
		body, err := nodePayload(i, groupID)
		if err != nil {
			return nil, err
		}
		if err := a.call(ctx, http.MethodPost, "/api/v1/admin/nodes", body, nil); err != nil {
			return nil, err
		}
		ds.Created.Nodes++
	}
	if ds.Created.Nodes > 0 {
		if nodes, err = listBenchNodes(ctx, a); err != nil {
			return nil, err
		}
	}
	ds.Nodes = nodes

	// users
	have, err := countBenchUsers(ctx, a)
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().AddDate(1, 0, 0).Truncate(time.Second)
	for conflicts := 0; have < wantUsers; {
		n := min(generateBatch, wantUsers-have)
		body := map[string]any{"email_prefix": benchEmailPrefix, "email_suffix": benchEmailSuffix, "count": n,
			"plan_id": ds.PlanID, "expired_at": expires.Format(time.RFC3339), "password": benchUserPassword}
		if err := a.call(ctx, http.MethodPost, "/api/v1/admin/users/generate", body, nil); err != nil {
			var ae *apiError
			if errors.As(err, &ae) && ae.Status == http.StatusConflict && conflicts < 5 {
				conflicts++ // a random email collided: draw again
				continue
			}
			return nil, err
		}
		have += n
		ds.Created.Users += n
		if logf != nil && (have%2000 == 0 || have == wantUsers) {
			logf("  users %d / %d", have, wantUsers)
		}
	}
	if ds.Users, err = listBenchUsers(ctx, a); err != nil {
		return nil, err
	}
	if len(ds.Users) < wantUsers {
		return nil, fmt.Errorf("expected %d benchmark users, found %d", wantUsers, len(ds.Users))
	}
	ds.Users = ds.Users[:wantUsers]
	if len(ds.Nodes) < wantNodes {
		return nil, fmt.Errorf("expected %d benchmark nodes, found %d", wantNodes, len(ds.Nodes))
	}
	ds.Nodes = ds.Nodes[:wantNodes]
	return ds, nil
}

func listBenchNodes(ctx context.Context, a *api) ([]benchNode, error) {
	var out struct {
		Data []struct {
			ID   uint64 `json:"id"`
			Type string `json:"type"`
			Name string `json:"name"`
			Host string `json:"host"`
		} `json:"data"`
	}
	if err := a.call(ctx, http.MethodGet, "/api/v1/admin/nodes", nil, &out); err != nil {
		return nil, err
	}
	var nodes []benchNode
	for _, n := range out.Data {
		if strings.HasSuffix(n.Host, benchHostSuffix) {
			nodes = append(nodes, benchNode{ID: n.ID, Type: n.Type, UniType: uniTypeOf(n.Type), Name: n.Name, Hostname: n.Host})
		}
	}
	return nodes, nil
}

func countBenchUsers(ctx context.Context, a *api) (int, error) {
	var out struct {
		Total int `json:"total"`
	}
	err := a.call(ctx, http.MethodGet, "/api/v1/admin/users?page_size=1&email="+benchEmailPrefix, nil, &out)
	return out.Total, err
}

// listBenchUsers pages through every benchmark user (id order).
func listBenchUsers(ctx context.Context, a *api) ([]benchUser, error) {
	var users []benchUser
	for page := 1; ; page++ {
		var out struct {
			Data  []benchUser `json:"data"`
			Total int         `json:"total"`
		}
		path := fmt.Sprintf("/api/v1/admin/users?page_size=200&sort=id&page=%d&email=%s", page, benchEmailPrefix)
		if err := a.call(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, err
		}
		for _, u := range out.Data {
			if strings.HasPrefix(u.Email, benchEmailPrefix) && strings.HasSuffix(u.Email, "@"+benchEmailSuffix) && u.Token != "" {
				users = append(users, u)
			}
		}
		if len(out.Data) == 0 || page*200 >= out.Total {
			return users, nil
		}
	}
}
