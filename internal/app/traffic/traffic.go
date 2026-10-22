// Package traffic aggregates node traffic reports in memory and flushes them to the database in one
// transaction: users.u/d increments (multiplied by the node rate) plus daily stat_users / stat_nodes upserts.
package traffic

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/model"
)

type key struct{ node, user uint64 }

type entry struct {
	rate     float64
	up, down int64
}

// Accumulator is safe for concurrent use.
type Accumulator struct {
	mu  sync.Mutex
	m   map[key]*entry
	now func() time.Time
}

// New returns an empty accumulator.
func New() *Accumulator { return &Accumulator{m: map[key]*entry{}, now: time.Now} }

// SetClock replaces the time source used to pick record_at (tests).
func (a *Accumulator) SetClock(now func() time.Time) {
	a.mu.Lock()
	a.now = now
	a.mu.Unlock()
}

// Add records raw bytes for (nodeID, userID). rate is the node multiplier applied to user usage
// (the latest rate wins for the pending entry). Negative values are ignored.
func (a *Accumulator) Add(nodeID, userID uint64, rate float64, up, down int64) {
	if up < 0 {
		up = 0
	}
	if down < 0 {
		down = 0
	}
	if up == 0 && down == 0 {
		return
	}
	if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		rate = 1
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	k := key{nodeID, userID}
	e := a.m[k]
	if e == nil {
		e = &entry{}
		a.m[k] = e
	}
	e.rate = rate
	e.up = satAdd(e.up, up)
	e.down = satAdd(e.down, down)
}

// satAdd adds two non-negative counters, saturating at MaxInt64 instead of wrapping to a negative value
// (a wrapped counter would lower the user's usage).
func satAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// Pending returns the number of buffered (node, user) pairs.
func (a *Accumulator) Pending() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.m)
}

// Flush writes everything buffered so far. On failure the data is merged back so nothing is lost.
func (a *Accumulator) Flush(ctx context.Context, db *gorm.DB) error {
	a.mu.Lock()
	batch := a.m
	a.m = map[key]*entry{}
	now := a.now()
	a.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	if err := write(ctx, db, batch, model.Day(now)); err != nil {
		a.restore(batch)
		return err
	}
	return nil
}

func (a *Accumulator) restore(batch map[key]*entry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, e := range batch {
		cur := a.m[k]
		if cur == nil {
			a.m[k] = e
			continue
		}
		cur.up = satAdd(cur.up, e.up)
		cur.down = satAdd(cur.down, e.down)
	}
}

// scaled returns floor(v × rate) (docs/NODE_PROTOCOLS.md §2.3), clamped to [0, MaxInt64]: converting a
// float64 ≥ 2^63 to int64 is implementation-defined in Go.
func scaled(v int64, rate float64) int64 {
	f := math.Floor(float64(v) * rate)
	if !(f > 0) { // also NaN
		return 0
	}
	if f >= math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(f)
}

// maxUserID is the largest id a user row can have (signed BIGINT on every dialect); larger ids cannot be bound
// as query parameters by database/sql and would make every flush fail.
const maxUserID = math.MaxInt64

// knownUsers returns the subset of ids that exist in users (queried in chunks to stay below the
// dialects' bound-parameter limits).
func knownUsers(tx *gorm.DB, ids []uint64) (map[uint64]bool, error) {
	known := make(map[uint64]bool, len(ids))
	valid := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id > 0 && id <= maxUserID {
			valid = append(valid, id)
		}
	}
	const chunk = 500
	for i := 0; i < len(valid); i += chunk {
		var found []uint64
		if err := tx.Model(&model.User{}).Where("id IN ?", valid[i:min(i+chunk, len(valid))]).Pluck("id", &found).Error; err != nil {
			return nil, err
		}
		for _, id := range found {
			known[id] = true
		}
	}
	return known, nil
}

func write(ctx context.Context, db *gorm.DB, batch map[key]*entry, day time.Time) error {
	type usage struct{ u, d int64 }
	users := map[uint64]*usage{}
	nodes := map[uint64]*usage{}
	statUsers := make([]model.StatUser, 0, len(batch))
	for k, e := range batch {
		su, sd := scaled(e.up, e.rate), scaled(e.down, e.rate)
		if users[k.user] == nil {
			users[k.user] = &usage{}
		}
		users[k.user].u = satAdd(users[k.user].u, su)
		users[k.user].d = satAdd(users[k.user].d, sd)
		if nodes[k.node] == nil {
			nodes[k.node] = &usage{}
		}
		nodes[k.node].u = satAdd(nodes[k.node].u, e.up)
		nodes[k.node].d = satAdd(nodes[k.node].d, e.down)
		statUsers = append(statUsers, model.StatUser{UserID: k.user, NodeID: k.node, Rate: e.rate, U: su, D: sd, RecordAt: day})
	}
	// Deterministic order keeps lock acquisition consistent across concurrent writers.
	sort.Slice(statUsers, func(i, j int) bool {
		if statUsers[i].UserID != statUsers[j].UserID {
			return statUsers[i].UserID < statUsers[j].UserID
		}
		return statUsers[i].NodeID < statUsers[j].NodeID
	})
	userIDs := make([]uint64, 0, len(users))
	for id := range users {
		userIDs = append(userIDs, id)
	}
	sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })
	statNodes := make([]model.StatNode, 0, len(nodes))
	for id, u := range nodes {
		statNodes = append(statNodes, model.StatNode{NodeID: id, U: u.u, D: u.d, RecordAt: day})
	}
	sort.Slice(statNodes, func(i, j int) bool { return statNodes[i].NodeID < statNodes[j].NodeID })

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Unknown uids are ignored (docs/NODE_PROTOCOLS.md §2.3): a node may report a deleted user, and a
		// bogus id must not poison the batch (it would be restored and fail every later flush).
		known, err := knownUsers(tx, userIDs)
		if err != nil {
			return fmt.Errorf("traffic: load users: %w", err)
		}
		rows := statUsers[:0:0]
		for _, su := range statUsers {
			if known[su.UserID] {
				rows = append(rows, su)
			}
		}
		for _, id := range userIDs {
			if !known[id] {
				continue
			}
			u := users[id]
			if err := tx.Model(&model.User{}).Where("id = ?", id).UpdateColumns(map[string]any{
				"u": gorm.Expr("u + ?", u.u),
				"d": gorm.Expr("d + ?", u.d),
			}).Error; err != nil {
				return fmt.Errorf("traffic: update user %d: %w", id, err)
			}
		}
		if len(rows) == 0 {
			return upsertNodes(tx, statNodes)
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "node_id"}, {Name: "record_at"}},
			DoUpdates: clause.Assignments(map[string]any{
				"u":    AddExcluded(tx, "stat_users", "u"),
				"d":    AddExcluded(tx, "stat_users", "d"),
				"rate": Excluded(tx, "rate"),
			}),
		}).CreateInBatches(&rows, 500).Error; err != nil {
			return fmt.Errorf("traffic: upsert stat_users: %w", err)
		}
		return upsertNodes(tx, statNodes)
	})
}

func upsertNodes(tx *gorm.DB, statNodes []model.StatNode) error {
	if len(statNodes) == 0 {
		return nil
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "node_id"}, {Name: "record_at"}},
		DoUpdates: clause.Assignments(map[string]any{
			"u": AddExcluded(tx, "stat_nodes", "u"),
			"d": AddExcluded(tx, "stat_nodes", "d"),
		}),
	}).CreateInBatches(&statNodes, 500).Error; err != nil {
		return fmt.Errorf("traffic: upsert stat_nodes: %w", err)
	}
	return nil
}

// Excluded returns the dialect-specific reference to the proposed insert value of col inside an upsert
// (excluded.col on SQLite/PostgreSQL, VALUES(col) on MySQL).
func Excluded(db *gorm.DB, col string) clause.Expr {
	if db.Dialector.Name() == "mysql" {
		return gorm.Expr("VALUES(" + col + ")")
	}
	return gorm.Expr("excluded." + col)
}

// AddExcluded returns "table.col + <excluded col>" for accumulating upserts on any supported dialect.
func AddExcluded(db *gorm.DB, table, col string) clause.Expr {
	ex := Excluded(db, col)
	return gorm.Expr(table+"."+col+" + "+ex.SQL, ex.Vars...)
}
