package xboardimport

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
)

// stats, stat_nodes, stat_users ← v2_stat, v2_stat_server, v2_stat_user (§4.14, only with --with-stats).
// Rows: record_type 'd' within the last --stats-days days. record_at is a unix *local* midnight, converted to a
// date in --source-tz (1729440000 → 2024-10-21 in Asia/Shanghai; UTC would give the 20th).

func (im *importer) importStats() error {
	midnight := time.Date(im.start.In(im.loc).Year(), im.start.In(im.loc).Month(), im.start.In(im.loc).Day(), 0, 0, 0, 0, im.loc)
	cutoff := midnight.AddDate(0, 0, -im.opts.StatsDays).Unix()
	if err := im.importStatDaily(cutoff); err != nil {
		return err
	}
	if err := im.importStatNodes(cutoff); err != nil {
		return err
	}
	return im.importStatUsers(cutoff)
}

const statFilter = "record_type = ? AND record_at >= ?"

func (im *importer) recordDay(v any) (time.Time, bool) {
	n, ok := flexInt(v)
	if !ok || n <= 0 {
		return time.Time{}, false
	}
	if n > msThreshold {
		n /= 1000
	}
	return model.Day(time.Unix(n, 0).In(im.loc)), true
}

func (im *importer) importStatDaily(cutoff int64) error {
	stat := im.rep.Table("stats")
	if !im.src.has("v2_stat") {
		return nil
	}
	byDay := map[time.Time]model.Stat{}
	err := im.src.each(im.ctx, "v2_stat", statFilter, []any{"d", cutoff}, func(r row) error {
		stat.Source++
		day, ok := im.recordDay(r["record_at"])
		if !ok {
			stat.Skipped++
			return nil
		}
		if _, dup := byDay[day]; dup {
			stat.Skipped++ // the highest id wins (rows are ordered by id)
		}
		transfer, _ := flexInt(r["transfer_used_total"])
		byDay[day] = model.Stat{
			RecordAt:          day,
			OrderCount:        r.i64("order_count"),
			OrderTotal:        r.i64("order_total"),
			PaidCount:         r.i64("paid_count"),
			PaidTotal:         r.i64("paid_total"),
			CommissionCount:   r.i64("commission_count"),
			CommissionTotal:   r.i64("commission_total"),
			RegisterCount:     r.i64("register_count"),
			InviteCount:       r.i64("invite_count"),
			TransferUsedTotal: transfer,
		}
		return nil
	})
	if err != nil {
		return err
	}
	days := make([]time.Time, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	b := newBatcher[model.Stat](im.tx)
	for _, d := range days {
		if err := b.add(byDay[d]); err != nil {
			return err
		}
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	return nil
}

// statNodeID resolves (server_type, server_id) of a stat row: L through the renumbering map, U by id and then
// by code (both with the type).
func (im *importer) statNodeID(serverType string, serverID int64) (uint64, bool) {
	typ := strings.ToLower(strings.TrimSpace(serverType))
	switch typ {
	case "v2ray":
		typ = model.NodeVmess
	case "hysteria2":
		typ = model.NodeHysteria
	}
	if serverID <= 0 || !model.IsValidNodeType(typ) {
		return 0, false
	}
	if id, ok := im.nodeMap[nodeKey{typ, uint64(serverID)}]; ok {
		return id, true
	}
	if im.flavor == FlavorUpstream {
		if id, ok := im.nodeCodes[nodeKey{typ, uint64(serverID)}]; ok {
			return id, true
		}
	}
	return 0, false
}

type dayKey struct {
	id, node uint64
	day      time.Time
}

func (im *importer) importStatNodes(cutoff int64) error {
	stat := im.rep.Table("stat_nodes")
	if !im.src.has("v2_stat_server") {
		return nil
	}
	sums := map[dayKey]*model.StatNode{}
	dropped := 0
	err := im.src.each(im.ctx, "v2_stat_server", statFilter, []any{"d", cutoff}, func(r row) error {
		stat.Source++
		day, ok := im.recordDay(r["record_at"])
		node, found := im.statNodeID(r.str("server_type"), r.i64("server_id"))
		if !ok || !found {
			dropped++
			stat.Skipped++
			return nil
		}
		k := dayKey{node: node, day: day}
		s := sums[k]
		if s == nil {
			s = &model.StatNode{NodeID: node, RecordAt: day}
			sums[k] = s
		}
		s.U += r.i64("u")
		s.D += r.i64("d")
		return nil
	})
	if err != nil {
		return err
	}
	keys := sortedDayKeys(sums)
	b := newBatcher[model.StatNode](im.tx)
	for _, k := range keys {
		if err := b.add(*sums[k]); err != nil {
			return err
		}
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	if dropped > 0 {
		im.warn("stat_nodes", "%d rows reference servers that were not imported (or unknown types) — dropped", dropped)
	}
	stat.Note = "summed per node and day"
	return nil
}

type userAgg struct {
	chargedU, chargedD float64
	raw                float64
	rawKnown           bool
}

// importStatUsers aggregates per (user, day). ggboard stores charged bytes (as users.u/d do): L rows hold raw
// bytes per server_rate (charged = raw·rate); U rows are already charged (raw = charged/rate). rate is
// Σcharged/Σraw rounded to 2 decimals, or 1 when nothing is derivable. node_id is 0 ("unknown node") unless the
// row carries a server id that maps to an imported node.
func (im *importer) importStatUsers(cutoff int64) error {
	stat := im.rep.Table("stat_users")
	if !im.src.has("v2_stat_user") {
		return nil
	}
	hasServer := im.src.hasCol("v2_stat_user", "server_id")
	hasType := im.src.hasCol("v2_stat_user", "server_type")
	aggs := map[dayKey]*userAgg{}
	droppedUsers := 0
	err := im.src.each(im.ctx, "v2_stat_user", statFilter, []any{"d", cutoff}, func(r row) error {
		stat.Source++
		uid := uint64(max(r.i64("user_id"), 0))
		if im.banned[uid] {
			stat.Skipped++
			im.rep.Banned.StatUsers++
			return nil
		}
		day, ok := im.recordDay(r["record_at"])
		if !ok || !im.users[uid] {
			droppedUsers++
			stat.Skipped++
			return nil
		}
		var node uint64
		if hasServer {
			typ := ""
			if hasType {
				typ = r.str("server_type")
			}
			if typ != "" {
				node, _ = im.statNodeID(typ, r.i64("server_id"))
			} else if im.flavor == FlavorUpstream {
				if sid := r.i64("server_id"); sid > 0 {
					for _, t := range model.NodeTypes() { // U ids are unique across types
						if id, ok := im.nodeMap[nodeKey{t, uint64(sid)}]; ok {
							node = id
							break
						}
					}
				}
			}
		}
		rate, ok := flexFloat(r["server_rate"])
		if !ok || rate < 0 {
			rate = 1
		}
		k := dayKey{id: uid, node: node, day: day}
		a := aggs[k]
		if a == nil {
			a = &userAgg{}
			aggs[k] = a
		}
		u, d := float64(r.i64("u")), float64(r.i64("d"))
		if im.flavor == FlavorLegacy {
			a.chargedU += u * rate
			a.chargedD += d * rate
			a.raw += u + d
			a.rawKnown = true
		} else {
			a.chargedU += u
			a.chargedD += d
			if rate > 0 {
				a.raw += (u + d) / rate
				a.rawKnown = true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	keys := make([]dayKey, 0, len(aggs))
	for k := range aggs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if !keys[i].day.Equal(keys[j].day) {
			return keys[i].day.Before(keys[j].day)
		}
		if keys[i].id != keys[j].id {
			return keys[i].id < keys[j].id
		}
		return keys[i].node < keys[j].node
	})
	b := newBatcher[model.StatUser](im.tx)
	for _, k := range keys {
		a := aggs[k]
		rate := 1.0
		if a.rawKnown && a.raw > 0 {
			rate = math.Round((a.chargedU+a.chargedD)/a.raw*100) / 100
		}
		if err := b.add(model.StatUser{
			UserID: k.id, NodeID: k.node, Rate: rate, RecordAt: k.day,
			U: int64(math.Round(a.chargedU)), D: int64(math.Round(a.chargedD)),
		}); err != nil {
			return err
		}
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	stat.Note = "aggregated per user and day (charged bytes)"
	if n := im.rep.Banned.StatUsers; n > 0 {
		stat.addNote(fmt.Sprintf("skipped banned %d", n))
	}
	if droppedUsers > 0 {
		im.warn("stat_users", "%d rows of users that were not imported — dropped", droppedUsers)
	}
	return nil
}

func sortedDayKeys(m map[dayKey]*model.StatNode) []dayKey {
	keys := make([]dayKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if !keys[i].day.Equal(keys[j].day) {
			return keys[i].day.Before(keys[j].day)
		}
		return keys[i].node < keys[j].node
	})
	return keys
}
