// Package nodesvc contains the node domain logic shared by the node, admin and user APIs: node lookup with
// child-node resolution (docs/NODE_PROTOCOLS.md §5.3), configuration rendering for UniProxy (XrayR/V2bX) and
// soga-v1, user lists, traffic/alive/status reports, runtime status views and admin validation.
package nodesvc

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/app/online"
	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// Namespace is the cache namespace of everything this package caches (resolved nodes, rendered configs, user
// list bodies, visible node lists). Invalidate drops it.
const Namespace = "nodesvc"

// CacheTTL is how long rendered bodies and resolved nodes are cached (docs/NODE_PROTOCOLS.md §6.6).
const CacheTTL = 60 * time.Second

// ErrNotFound is returned when a node does not exist.
var ErrNotFound = errors.New("nodesvc: node not found")

// TypeError reports that a node cannot be served for the requested type (wrong type, unsupported by the
// protocol, or a hysteria version mismatch). Its message is sent to the node backend.
type TypeError struct{ Msg string }

func (e *TypeError) Error() string { return e.Msg }

// Service is cheap to construct: nodesvc.New(a) per request is fine.
type Service struct{ a *app.App }

// New returns a Service bound to a.
func New(a *app.App) *Service { return &Service{a: a} }

// Invalidate drops every cached node body and node list, and the available-user lists of usersvc. Call it after
// any write to nodes, node_routes or server groups.
func (s *Service) Invalidate() {
	s.a.Cache.InvalidateNamespace(Namespace)
	usersvc.New(s.a).InvalidateNodeUsers()
}

func (s *Service) key(parts ...string) string {
	return s.a.Cache.Key(Namespace, strings.Join(parts, ":"))
}

func u64(v uint64) string { return strconv.FormatUint(v, 10) }

// Aliases accepted for node types (UniProxy node_type, soga NODE-TYPE). hysteria2 is handled separately.
var typeAliases = map[string]string{
	"v2ray":              model.NodeVmess,
	"ss":                 model.NodeShadowsocks,
	"shadowsocks-plugin": model.NodeShadowsocks,
	"ssr":                model.NodeShadowsocksR,
}

// CanonicalType normalises a requested node type: lower-case, trimmed, aliases applied (v2ray→vmess,
// ss/shadowsocks-plugin→shadowsocks, ssr→shadowsocksr, hysteria2→hysteria with hy2=true). "v2node" means
// "no type". The result is not checked against the list of supported types.
func CanonicalType(raw string) (typ string, hy2 bool) {
	t := strings.ToLower(strings.TrimSpace(raw))
	switch t {
	case "v2node":
		return "", false
	case "hysteria2":
		return model.NodeHysteria, true
	}
	if alias, ok := typeAliases[t]; ok {
		return alias, false
	}
	return t, false
}

var (
	uniProxyTypes = []string{model.NodeVmess, model.NodeVless, model.NodeTrojan, model.NodeShadowsocks, model.NodeHysteria, model.NodeTuic, model.NodeAnyTLS, model.NodeMieru}
	sogaTypes     = []string{model.NodeVmess, model.NodeVless, model.NodeShadowsocks, model.NodeShadowsocksR, model.NodeTrojan, model.NodeHysteria, model.NodeAnyTLS, model.NodeMieru}
)

// UniProxyType normalises a UniProxy node_type query value (docs/NODE_PROTOCOLS.md §1.2). An empty result means
// "match by id only".
func UniProxyType(raw string) (typ string, hy2 bool, err error) {
	typ, hy2 = CanonicalType(raw)
	if typ != "" && !slices.Contains(uniProxyTypes, typ) {
		return "", false, fmt.Errorf("unsupported node_type %q", strings.TrimSpace(raw))
	}
	return typ, hy2, nil
}

// SogaType normalises a soga NODE-TYPE header (docs/NODE_PROTOCOLS.md §1.3).
func SogaType(raw string) (typ string, hy2 bool, err error) {
	typ, hy2 = CanonicalType(raw)
	if typ != "" && !slices.Contains(sogaTypes, typ) {
		return "", false, fmt.Errorf("unsupported NODE-TYPE %q", strings.TrimSpace(raw))
	}
	return typ, hy2, nil
}

// HysteriaVersion returns protocol_settings.version of a hysteria node (default 2).
func HysteriaVersion(n *model.Node) int {
	if v := n.ProtocolSettings.Int("version"); v != 0 {
		return v
	}
	return 2
}

// CheckType verifies that n can be served for the requested (canonical) type.
func CheckType(n *model.Node, typ string, hy2 bool) error {
	if typ != "" && n.Type != typ {
		return &TypeError{Msg: fmt.Sprintf("node %d is %s, requested %s", n.ID, n.Type, typ)}
	}
	if hy2 && n.Type == model.NodeHysteria && HysteriaVersion(n) != 2 {
		return &TypeError{Msg: fmt.Sprintf("node %d is hysteria v%d, requested hysteria2", n.ID, HysteriaVersion(n))}
	}
	return nil
}

// CheckUniProxy is CheckType plus the UniProxy type restrictions (no shadowsocksr).
func CheckUniProxy(n *model.Node, typ string, hy2 bool) error {
	if err := CheckType(n, typ, hy2); err != nil {
		return err
	}
	if !slices.Contains(uniProxyTypes, n.Type) {
		return &TypeError{Msg: fmt.Sprintf("node %d is %s, which UniProxy does not support", n.ID, n.Type)}
	}
	return nil
}

// CheckSoga is CheckType plus the soga restrictions (no tuic, hysteria 2 only).
func CheckSoga(n *model.Node, typ string, hy2 bool) error {
	if err := CheckType(n, typ, hy2); err != nil {
		return err
	}
	if !slices.Contains(sogaTypes, n.Type) {
		return &TypeError{Msg: fmt.Sprintf("node %d is %s, which soga does not support", n.ID, n.Type)}
	}
	if n.Type == model.NodeHysteria && HysteriaVersion(n) != 2 {
		return &TypeError{Msg: fmt.Sprintf("node %d is hysteria v%d, soga supports hysteria2 only", n.ID, HysteriaVersion(n))}
	}
	return nil
}

// Resolved is a node as seen by a node backend (docs/NODE_PROTOCOLS.md §5.3). It is shared between requests
// through the cache: treat it as read-only.
type Resolved struct {
	// Node is the requested node. For a child, Parent is loaded and protocol_settings / route_ids are the
	// parent's (inherited); host, port, server_port, name and tags stay the child's own.
	Node *model.Node
	// P is the effective node: the parent of a child, otherwise Node. Everything is served from and recorded on P.
	P *model.Node
	// Groups is groups(P) ∪ groups(children of P), sorted and unique: the user selection of P.
	Groups []uint64
}

// Inherit applies the child-node inheritance to n in place (no-op for nodes without a loaded parent).
func Inherit(n *model.Node) {
	if n.ParentID == nil || n.Parent == nil {
		return
	}
	n.ProtocolSettings = n.Parent.ProtocolSettings
	n.RouteIDs = n.Parent.RouteIDs
}

// Resolve loads node id with its parent and the groups of P's children (cached for CacheTTL).
func (s *Service) Resolve(ctx context.Context, id uint64) (*Resolved, error) {
	return cache.GetOrLoad(s.a.Cache, s.key("resolve", u64(id)), CacheTTL, func() (*Resolved, error) {
		n := new(model.Node)
		err := s.a.DB.WithContext(ctx).Preload("Parent").Where("id = ?", id).First(n).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		Inherit(n)
		p := n
		if n.ParentID != nil && n.Parent != nil {
			p = n.Parent
		}
		var children []model.Node
		if err := s.a.DB.WithContext(ctx).Select("id", "group_ids").Where("parent_id = ?", p.ID).Find(&children).Error; err != nil {
			return nil, err
		}
		groups := slices.Clone([]uint64(p.GroupIDs))
		for _, c := range children {
			groups = append(groups, c.GroupIDs...)
		}
		slices.Sort(groups)
		return &Resolved{Node: n, P: p, Groups: slices.Compact(groups)}, nil
	})
}

// Find returns node id (parent preloaded, child inheritance applied) after checking the requested type, which
// may be an alias (v2ray, hysteria2, shadowsocks-plugin, ss, ssr) or "" for any type. The returned node is a
// copy, but its maps and slices are shared with the cache: do not mutate them.
func (s *Service) Find(ctx context.Context, id uint64, typ string) (*model.Node, error) {
	r, err := s.Resolve(ctx, id)
	if err != nil {
		return nil, err
	}
	t, hy2 := CanonicalType(typ)
	if err := CheckType(r.Node, t, hy2); err != nil {
		return nil, err
	}
	cp := *r.Node
	return &cp, nil
}

// EffectiveID returns the id of P for n (its parent id for a child).
func EffectiveID(n *model.Node) uint64 {
	if n.ParentID != nil {
		return *n.ParentID
	}
	return n.ID
}

// visible returns every node with show=true ordered by sort, id (cached).
func (s *Service) visible(ctx context.Context) ([]*model.Node, error) {
	return cache.GetOrLoad(s.a.Cache, s.key("visible"), CacheTTL, func() ([]*model.Node, error) {
		var nodes []*model.Node
		err := s.a.DB.WithContext(ctx).
			Where(map[string]any{"show": true}).
			Order(clause.OrderByColumn{Column: clause.Column{Name: "sort"}}).
			Order("id").
			Find(&nodes).Error
		return nodes, err
	})
}

// UserNode is a node visible to a user together with its online flag (status ≥ idle, from P).
type UserNode struct {
	*model.Node
	IsOnline bool
}

// AvailableForUser returns the nodes with show=true whose group_ids contain u's group, sorted by sort, id.
// It does not check whether u itself is available (banned, expired, out of traffic): callers decide.
// The nodes are shared with the cache: do not mutate them.
func (s *Service) AvailableForUser(ctx context.Context, u *model.User) ([]UserNode, error) {
	out := []UserNode{}
	if u == nil || u.GroupID == nil {
		return out, nil
	}
	nodes, err := s.visible(ctx)
	if err != nil {
		return nil, err
	}
	now, w := time.Now(), s.Window()
	for _, n := range nodes {
		if !n.InGroup(*u.GroupID) {
			continue
		}
		st, _ := s.a.Online.NodeStatus(EffectiveID(n))
		out = append(out, UserNode{Node: n, IsOnline: st.State(now, w) >= online.StateIdle})
	}
	return out, nil
}

// Intervals are the push/pull intervals (seconds) sent to node backends.
type Intervals struct{ Push, Pull int }

// Intervals reads server_push_interval / server_pull_interval.
func (s *Service) Intervals() Intervals {
	return Intervals{Push: s.a.Settings.Int("server_push_interval"), Pull: s.a.Settings.Int("server_pull_interval")}
}

// Window is W = 2 × max(push, pull) (docs/NODE_PROTOCOLS.md §5.1).
func (s *Service) Window() time.Duration {
	iv := s.Intervals()
	return online.Window(time.Duration(iv.Push)*time.Second, time.Duration(iv.Pull)*time.Second)
}

// Node states as strings.
const (
	StateOnline  = "online"
	StateIdle    = "idle"
	StateOffline = "offline"
)

// StatusView is the runtime status of a node for the admin API.
type StatusView struct {
	State       string       `json:"state"` // online | idle | offline
	Online      int          `json:"online"`
	LastCheckAt *time.Time   `json:"last_check_at"`
	LastPushAt  *time.Time   `json:"last_push_at"`
	Load        *online.Load `json:"load"`
	Version     string       `json:"version"`
}

// Status returns the status of node id (pass EffectiveID for children).
func (s *Service) Status(id uint64) StatusView {
	st, _ := s.a.Online.NodeStatus(id)
	now, w := time.Now(), s.Window()
	v := StatusView{Online: st.Online(now, w), LastCheckAt: st.LastCheckAt, LastPushAt: st.LastPushAt, Load: st.Load, Version: st.Version}
	switch st.State(now, w) {
	case online.StateOnline:
		v.State = StateOnline
	case online.StateIdle:
		v.State = StateIdle
	default:
		v.State = StateOffline
	}
	return v
}

// IsOnline reports status ≥ idle for node id (pass EffectiveID for children).
func (s *Service) IsOnline(id uint64) bool {
	st, _ := s.a.Online.NodeStatus(id)
	return st.State(time.Now(), s.Window()) >= online.StateIdle
}

// Routes returns the node_routes whose ids are in ids, sorted by id (missing ids are skipped).
func (s *Service) Routes(ctx context.Context, ids []uint64) ([]model.NodeRoute, error) {
	routes := []model.NodeRoute{}
	if len(ids) == 0 {
		return routes, nil
	}
	err := s.a.DB.WithContext(ctx).Where("id IN ?", ids).Order("id").Find(&routes).Error
	return routes, err
}
