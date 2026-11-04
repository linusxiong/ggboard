package nodesvc

import (
	"slices"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/model"
)

// RouteApplied is the outcome of ApplyRoute, counted over top-level nodes.
type RouteApplied struct {
	NodeCount int `json:"node_count"` // nodes that use the route afterwards
	Added     int `json:"added"`
	Removed   int `json:"removed"`
}

// ApplyRoute makes the top-level nodes in ids, or all of them when all is set, the only nodes that use the route, and
// leaves every node's other routes alone. Child nodes take their parent's routes, so they cannot be targets; a leftover
// reference on a child is cleared, because it does nothing except keep the route from being deleted. The caller
// invalidates the caches after the transaction commits.
func ApplyRoute(tx *gorm.DB, routeID uint64, all bool, ids []uint64) (RouteApplied, error) {
	var res RouteApplied
	if err := tx.Where("id = ?", routeID).First(&model.NodeRoute{}).Error; err != nil {
		return res, err
	}
	// Locked: route_ids is rewritten whole, so a second push must not work from the same snapshot.
	var nodes []model.Node
	err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
		Select("id", "parent_id", "route_ids").Order("id").Find(&nodes).Error
	if err != nil {
		return res, err
	}
	targets, err := routeTargets(nodes, all, ids)
	if err != nil {
		return res, err
	}
	res.NodeCount = len(targets)
	for i := range nodes {
		n := &nodes[i]
		has, want := model.SliceContains(n.RouteIDs, routeID), slices.Contains(targets, n.ID)
		if has == want {
			continue
		}
		if want {
			n.RouteIDs = append(n.RouteIDs, routeID)
			res.Added++
		} else {
			n.RouteIDs = slices.DeleteFunc(n.RouteIDs, func(id uint64) bool { return id == routeID })
			if n.ParentID == nil {
				res.Removed++
			}
		}
		if err := tx.Model(&model.Node{}).Where("id = ?", n.ID).Update("route_ids", n.RouteIDs).Error; err != nil {
			return res, err
		}
	}
	return res, nil
}

// routeTargets resolves the requested nodes against nodes, the whole table.
func routeTargets(nodes []model.Node, all bool, ids []uint64) ([]uint64, error) {
	if all {
		targets := make([]uint64, 0, len(nodes))
		for i := range nodes {
			if nodes[i].ParentID == nil {
				targets = append(targets, nodes[i].ID)
			}
		}
		return targets, nil
	}
	targets := uniqueIDs(ids)
	var missing, children []uint64
	for _, id := range targets {
		i := slices.IndexFunc(nodes, func(n model.Node) bool { return n.ID == id })
		switch {
		case i < 0:
			missing = append(missing, id)
		case nodes[i].ParentID != nil:
			children = append(children, id)
		}
	}
	errs := ValidationError{}
	if len(missing) > 0 {
		errs.Add("node_ids", "unknown node ids: "+joinIDs(missing))
	}
	if len(children) > 0 {
		errs.Add("node_ids", "child nodes use their parent's routes: "+joinIDs(children))
	}
	return targets, errs.Err()
}
