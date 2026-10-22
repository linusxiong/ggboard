package subscription

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/app/online"
	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/model"
)

// NodesNamespace is the a.Cache namespace of the per-group node lists (InvalidateNodes clears it).
const NodesNamespace = "subscription_nodes"

// NodesTTL is how long a group's node list is cached.
const NodesTTL = 5 * time.Second

// AvailableNode is a node a user may see in subscriptions.
type AvailableNode struct {
	Node      *model.Node // the entry itself (a child node is an entry of its own); shared, read-only
	Effective *model.Node // the node whose settings, credentials and status apply (the parent for child nodes)
	Status    int         // online.StateOnline / StateIdle / StateOffline of Effective
	IsOnline  bool        // Status >= online.StateIdle

	entry *nodeEntry
}

type nodeEntry struct {
	node, eff *model.Node
	proxy     Proxy  // decoded, user-independent template
	secret    string // ss2022 server key / ssr node password
}

// InvalidateNodes drops every cached node list (call after node or group changes that must show immediately;
// otherwise lists refresh within NodesTTL).
func InvalidateNodes(a *app.App) { a.Cache.InvalidateNamespace(NodesNamespace) }

// AvailableNodes returns the nodes u may use: show=true and u's group in group_ids, sorted by sort, id, child
// nodes included as their own entries (with their parent's settings and status). Nodes whose settings cannot be
// decoded are skipped (and logged). The list is cached per group for NodesTTL; status is evaluated per call.
func AvailableNodes(ctx context.Context, a *app.App, u *model.User) ([]AvailableNode, error) {
	if u == nil || u.GroupID == nil {
		return nil, nil
	}
	gid := *u.GroupID
	key := a.Cache.Key(NodesNamespace, strconv.FormatUint(gid, 10))
	entries, err := cache.GetOrLoad(a.Cache, key, NodesTTL, func() ([]*nodeEntry, error) {
		return loadNodes(ctx, a.DB, a.Log, gid)
	})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	w := online.Window(a.Settings.Duration("server_push_interval", time.Second), a.Settings.Duration("server_pull_interval", time.Second))
	out := make([]AvailableNode, 0, len(entries))
	for _, e := range entries {
		st := online.StateOffline
		if s, ok := a.Online.NodeStatus(e.eff.ID); ok {
			st = s.State(now, w)
		}
		out = append(out, AvailableNode{Node: e.node, Effective: e.eff, Status: st, IsOnline: st >= online.StateIdle, entry: e})
	}
	return out, nil
}

func loadNodes(ctx context.Context, db *gorm.DB, log *slog.Logger, gid uint64) ([]*nodeEntry, error) {
	var nodes []*model.Node
	err := db.WithContext(ctx).Where(map[string]any{"show": true}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "sort"}}).
		Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).
		Find(&nodes).Error
	if err != nil {
		return nil, err
	}
	visible := nodes[:0]
	var parentIDs []uint64
	for _, n := range nodes {
		if !n.InGroup(gid) || !model.IsValidNodeType(n.Type) {
			continue
		}
		visible = append(visible, n)
		if n.ParentID != nil && *n.ParentID != n.ID {
			parentIDs = append(parentIDs, *n.ParentID)
		}
	}
	parents := map[uint64]*model.Node{}
	if len(parentIDs) > 0 {
		var rows []*model.Node
		if err := db.WithContext(ctx).Where("id IN ?", parentIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, p := range rows {
			parents[p.ID] = p
		}
	}
	out := make([]*nodeEntry, 0, len(visible))
	for _, n := range visible {
		eff := n
		if n.ParentID != nil && *n.ParentID != n.ID {
			p, ok := parents[*n.ParentID]
			if !ok || p.Type != n.Type {
				log.Warn("subscription: skipping child node with missing or mismatched parent", "node_id", n.ID)
				continue
			}
			n.Parent = p
			eff = p
		}
		proxy, secret, err := decodeProxy(n, eff)
		if err != nil {
			log.Warn("subscription: skipping node with invalid settings", "node_id", n.ID, "err", err)
			continue
		}
		out = append(out, &nodeEntry{node: n, eff: eff, proxy: proxy, secret: secret})
	}
	return out, nil
}
