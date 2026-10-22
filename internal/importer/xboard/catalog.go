package xboardimport

import (
	"sort"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
)

// server_groups ← v2_server_group (§4.1).
func (im *importer) importGroups() error {
	stat := im.rep.Table("server_groups")
	b := newBatcher[model.ServerGroup](im.tx)
	err := im.src.each(im.ctx, "v2_server_group", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("server_groups", id)
		im.groups[id] = true
		return b.add(model.ServerGroup{
			ID: id, Name: im.fit(scope, "name", r.str("name"), 191),
			CreatedAt: im.ts(scope, r["created_at"]), UpdatedAt: im.ts(scope, r["updated_at"]),
		})
	})
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	return nil
}

// legacyPriceColumns maps the L cents columns to our period keys (§4.2).
var legacyPriceColumns = []struct{ col, period string }{
	{"month_price", model.PeriodMonthly},
	{"quarter_price", model.PeriodQuarterly},
	{"half_year_price", model.PeriodHalfYearly},
	{"year_price", model.PeriodYearly},
	{"two_year_price", model.PeriodTwoYearly},
	{"three_year_price", model.PeriodThreeYearly},
	{"onetime_price", model.PeriodOnetime},
	{"reset_price", model.PeriodResetTraffic},
}

// legacyPeriodNames maps L period names (orders, coupon limit_period) to ours.
var legacyPeriodNames = func() map[string]string {
	m := map[string]string{}
	for _, c := range legacyPriceColumns {
		m[c.col] = c.period
	}
	return m
}()

// plans ← v2_plan (§4.2).
func (im *importer) importPlans() error {
	stat := im.rep.Table("plans")
	jsonPrices := im.src.hasCol("v2_plan", "prices")
	b := newBatcher[model.Plan](im.tx)
	err := im.src.each(im.ctx, "v2_plan", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("plans", id)
		p := model.Plan{
			ID:             id,
			Name:           im.fit(scope, "name", r.str("name"), 191),
			Content:        r.str("content"),
			Tags:           model.JSONSlice[string]{},
			TransferEnable: r.i64("transfer_enable"),
			SpeedLimit:     nz(r["speed_limit"]),
			Show:           r.bool("show"),
			Renew:          r.bool("renew"),
			Sort:           int(r.i64("sort")),
			CreatedAt:      im.ts(scope, r["created_at"]),
			UpdatedAt:      im.ts(scope, r["updated_at"]),
		}
		if gid, ok := r.int("group_id"); ok && gid > 0 && im.groups[uint64(gid)] {
			p.GroupID = uint64(gid)
		} else {
			g := r.str("group_id")
			if r.isNull("group_id") {
				g = "NULL"
			}
			im.warn(scope, "group_id %s is not an imported server group → 0: plan grants no group", g)
		}
		if r.has("tags") {
			tags, bad, err := parseStringList(r["tags"])
			if err != nil {
				im.warn(scope, "tags: %v → []", err)
			}
			for _, x := range bad {
				im.warn(scope, "tags: dropped non-string element %s", x)
			}
			p.Tags = tags
		}
		if r.has("device_limit") {
			p.DeviceLimit = nz(r["device_limit"])
		}
		if n, ok := r.int("capacity_limit"); ok {
			c := int(n)
			p.CapacityLimit = &c
		}
		if n, ok := r.int("reset_traffic_method"); ok {
			if n >= 0 && n <= 4 {
				m := model.ResetMethod(n)
				p.ResetTrafficMethod = &m
			} else {
				im.warn(scope, "reset_traffic_method %d is not 0..4 → follow the setting", n)
			}
		}
		if r.has("sell") {
			p.Sell = r.bool("sell")
		} else {
			p.Sell = p.Show // U's 2025-01 migration did the same
		}
		if jsonPrices {
			p.Prices = im.upstreamPrices(scope, r["prices"])
		} else {
			p.Prices = im.legacyPrices(scope, r)
		}
		im.plans[id] = &planInfo{reset: p.ResetTrafficMethod, capacity: p.CapacityLimit}
		return b.add(p)
	})
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	return nil
}

// legacyPrices reads the 8 cents columns: NULL → not for sale; 0 is kept (free) with a warning.
func (im *importer) legacyPrices(scope string, r row) model.JSONObject[int64] {
	prices := model.JSONObject[int64]{}
	for _, c := range legacyPriceColumns {
		n, ok := r.int(c.col)
		if !ok {
			continue
		}
		if n < 0 {
			im.warn(scope, "%s is negative (%d) → not for sale", c.col, n)
			continue
		}
		if n == 0 {
			im.warn(scope, "%s is 0: the %s period stays purchasable for free", c.col, c.period)
		}
		prices[c.period] = n
	}
	return prices
}

// upstreamPrices converts U's prices JSON (currency units) to cents. NULL, "" or <= 0 → not for sale.
func (im *importer) upstreamPrices(scope string, v any) model.JSONObject[int64] {
	prices := model.JSONObject[int64]{}
	obj, err := jsonObject(v)
	if err != nil {
		im.warn(scope, "prices: %v → no prices", err)
		return prices
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !model.IsValidPeriod(k) {
			im.warn(scope, "prices: unknown period %q dropped", k)
			continue
		}
		f, ok := flexFloat(obj[k])
		if !ok || f <= 0 {
			continue
		}
		prices[k] = cents(f)
	}
	return prices
}

// node_routes ← v2_server_route (§4.3). Only block and dns routes exist in ggboard.
func (im *importer) importRoutes() error {
	stat := im.rep.Table("node_routes")
	if !im.src.has("v2_server_route") {
		return nil
	}
	b := newBatcher[model.NodeRoute](im.tx)
	err := im.src.each(im.ctx, "v2_server_route", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("node_routes", id)
		action := strings.ToLower(strings.TrimSpace(r.str("action")))
		if action != model.RouteActionBlock && action != model.RouteActionDNS {
			im.warn(scope, "action %q is not supported (block/dns only) — route skipped and removed from every node", action)
			im.skippedRoutes[id] = true
			stat.Skipped++
			return nil
		}
		match, bad, err := parseStringList(r["match"])
		if err != nil {
			im.warn(scope, "match: %v → []", err)
		}
		for _, x := range bad {
			im.warn(scope, "match: dropped non-string element %s", x)
		}
		im.routes[id] = true
		return b.add(model.NodeRoute{
			ID: id, Remarks: im.fit(scope, "remarks", r.str("remarks"), 191), Match: match, Action: action,
			ActionValue: im.fit(scope, "action_value", r.str("action_value"), 255),
			CreatedAt:   im.ts(scope, r["created_at"]), UpdatedAt: im.ts(scope, r["updated_at"]),
		})
	})
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	return nil
}

// idList parses an id-list column and keeps only ids accepted by keep (warning about the rest).
func (im *importer) idList(scope, col string, v any, keep func(uint64) (bool, string)) model.JSONSlice[uint64] {
	ids, bad, err := parseIDList(v)
	if err != nil {
		im.warn(scope, "%s: %v → []", col, err)
	}
	for _, x := range bad {
		im.warn(scope, "%s: dropped invalid id %q", col, x)
	}
	out := model.JSONSlice[uint64]{}
	for _, id := range ids {
		if ok, why := keep(id); !ok {
			if why != "" {
				im.warn(scope, "%s: dropped %d (%s)", col, id, why)
			}
			continue
		}
		out = append(out, id)
	}
	return out
}
