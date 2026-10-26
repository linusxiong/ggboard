// Package statsvc computes the admin dashboard statistics: overview counters, daily series, traffic rankings
// and the daily `stats` rows written by the stats_daily job (internal/service/maintenance).
//
// Days are calendar days in the location of the `now` passed by the caller (the process time zone in
// production); Overview reports that day and zone (today.date, timezone) so clients pick days by the server's
// calendar, not the viewer's. Date columns (`stats.record_at`, `stat_nodes.record_at`, `stat_users.record_at`) hold
// model.Day values. Everything is portable SQL (SQLite, PostgreSQL, MySQL): aggregates use COALESCE(SUM(..), 0)
// and per-day bucketing of datetime columns happens in Go.
package statsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/app/online"
	"github.com/linusxiong/ggboard/internal/app/traffic"
	"github.com/linusxiong/ggboard/internal/model"
)

// PaidStatuses are the order statuses that count as paid: processing (paid, being fulfilled), completed and
// discounted (paid, later credited to an upgrade). Paid orders are bucketed by paid_at.
var PaidStatuses = []int{
	int(model.OrderStatusProcessing),
	int(model.OrderStatusCompleted),
	int(model.OrderStatusDiscounted),
}

// Series metrics.
const (
	MetricIncome    = "income"    // sum of total_amount of paid orders (cents)
	MetricOrders    = "orders"    // number of paid orders
	MetricRegisters = "registers" // number of new users
	MetricTraffic   = "traffic"   // stat_nodes u + d (bytes)
)

// Metrics lists the valid series metrics.
var Metrics = []string{MetricIncome, MetricOrders, MetricRegisters, MetricTraffic}

// Ranges maps the series range parameter to a number of days (today included).
var Ranges = map[string]int{"7d": 7, "30d": 30, "90d": 90}

// Service is cheap to construct: statsvc.New(a) per request is fine.
type Service struct{ a *app.App }

// New returns a Service bound to a.
func New(a *app.App) *Service { return &Service{a: a} }

func (s *Service) db(ctx context.Context) *gorm.DB { return s.a.DB.WithContext(ctx) }

// DayStart returns 00:00 of t's calendar day in t's location.
func DayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// DateKey formats the calendar day of t (in t's location) as YYYY-MM-DD.
func DateKey(t time.Time) string { return t.Format(time.DateOnly) }

// recordKey formats a model.Day value (midnight UTC encoding a calendar day) as YYYY-MM-DD.
func recordKey(t time.Time) string { return t.UTC().Format(time.DateOnly) }

// ---------------------------------------------------------------- overview

// Overview is GET /admin/stats/overview.
type Overview struct {
	Today    TodayStats  `json:"today"`
	Month    PeriodStats `json:"month"`
	Totals   Totals      `json:"totals"`
	Nodes    NodeTotals  `json:"nodes"`
	TimeZone TimeZone    `json:"timezone"` // the zone every statistics day is counted in
}

// TodayStats are today's counters (money in cents).
type TodayStats struct {
	// Date is the statistics day these counters cover (YYYY-MM-DD in the server zone): also the last series
	// point and the rankings' default day. The viewer's own calendar day may differ.
	Date       string `json:"date"`
	Income     int64  `json:"income"`
	PaidOrders int64  `json:"paid_orders"`
	Registers  int64  `json:"registers"`
	Commission int64  `json:"commission"` // commission_balance of today's paid orders
}

// TimeZone names the zone statistics days are counted in (the process zone in production).
type TimeZone struct {
	// Name is the IANA name (TZ=Asia/Shanghai), "UTC", or "UTC±hh:mm" of Offset when the zone has none
	// (no TZ: Go loads /etc/localtime and names it "Local").
	Name   string `json:"name"`
	Offset int    `json:"offset"` // seconds east of UTC at the instant it was read
}

// ZoneOf describes t's location at the instant t.
func ZoneOf(t time.Time) TimeZone {
	_, offset := t.Zone()
	name := t.Location().String()
	if strings.HasPrefix(name, "/") { // TZ=/usr/share/zoneinfo/Asia/Shanghai
		_, rest, _ := strings.Cut(name, "zoneinfo/")
		name = rest
	}
	if name == "" || name == "Local" {
		name = offsetName(offset)
	}
	return TimeZone{Name: name, Offset: offset}
}

// offsetName formats a UTC offset in seconds as "UTC", "UTC+08:00" or "UTC-04:00".
func offsetName(offset int) string {
	if offset == 0 {
		return "UTC"
	}
	sign := '+'
	if offset < 0 {
		sign, offset = '-', -offset
	}
	return fmt.Sprintf("UTC%c%02d:%02d", sign, offset/3600, offset%3600/60)
}

// PeriodStats are the counters of the current month.
type PeriodStats struct {
	Income     int64 `json:"income"`
	PaidOrders int64 `json:"paid_orders"`
	Registers  int64 `json:"registers"`
}

// Totals are instantaneous counters.
type Totals struct {
	Users             int64 `json:"users"`
	ActiveUsers       int64 `json:"active_users"` // not banned, not expired, traffic left
	OnlineUsers       int   `json:"online_users"` // users with an alive IP on any node
	PendingTickets    int64 `json:"pending_tickets"`
	PendingOrders     int64 `json:"pending_orders"`
	PendingCommission int64 `json:"pending_commission"` // paid orders whose commission awaits review
}

// NodeTotals counts nodes; Online uses the three-state model (state >= idle, children follow their parent).
type NodeTotals struct {
	Total  int `json:"total"`
	Online int `json:"online"`
}

// PaidSum aggregates paid orders.
type PaidSum struct {
	Count      int64
	Total      int64
	Commission int64
}

// Paid sums the paid orders with paid_at in [from, to).
func (s *Service) Paid(ctx context.Context, from, to time.Time) (PaidSum, error) {
	var r struct {
		N             int64
		TotalSum      int64
		CommissionSum int64
	}
	err := s.db(ctx).Model(&model.Order{}).
		Select("COUNT(*) AS n, COALESCE(SUM(total_amount), 0) AS total_sum, COALESCE(SUM(commission_balance), 0) AS commission_sum").
		Where("status IN ?", PaidStatuses).
		Where("paid_at >= ? AND paid_at < ?", from, to).
		Scan(&r).Error
	if err != nil {
		return PaidSum{}, fmt.Errorf("stat: paid orders: %w", err)
	}
	return PaidSum{Count: r.N, Total: r.TotalSum, Commission: r.CommissionSum}, nil
}

// Registers counts users created in [from, to).
func (s *Service) Registers(ctx context.Context, from, to time.Time) (int64, error) {
	var n int64
	err := s.db(ctx).Model(&model.User{}).Where("created_at >= ? AND created_at < ?", from, to).Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("stat: registers: %w", err)
	}
	return n, nil
}

// Overview computes the dashboard counters at now.
func (s *Service) Overview(ctx context.Context, now time.Time) (*Overview, error) {
	today := DayStart(now)
	tomorrow := today.AddDate(0, 0, 1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	monthEnd := monthStart.AddDate(0, 1, 0)

	out := Overview{TimeZone: ZoneOf(now)}
	paidToday, err := s.Paid(ctx, today, tomorrow)
	if err != nil {
		return nil, err
	}
	out.Today = TodayStats{
		Date: DateKey(today), Income: paidToday.Total, PaidOrders: paidToday.Count, Commission: paidToday.Commission,
	}
	if out.Today.Registers, err = s.Registers(ctx, today, tomorrow); err != nil {
		return nil, err
	}
	paidMonth, err := s.Paid(ctx, monthStart, monthEnd)
	if err != nil {
		return nil, err
	}
	out.Month = PeriodStats{Income: paidMonth.Total, PaidOrders: paidMonth.Count}
	if out.Month.Registers, err = s.Registers(ctx, monthStart, monthEnd); err != nil {
		return nil, err
	}

	db := s.db(ctx)
	counts := []struct {
		dst *int64
		q   *gorm.DB
	}{
		{&out.Totals.Users, db.Model(&model.User{})},
		{&out.Totals.ActiveUsers, db.Model(&model.User{}).
			Where("banned = ?", false).
			Where("expired_at IS NULL OR expired_at > ?", now).
			Where("u + d < transfer_enable")},
		{&out.Totals.PendingTickets, db.Model(&model.Ticket{}).
			Where("status = ? AND reply_status = ?", model.TicketStatusOpen, model.TicketAwaitingStaff)},
		{&out.Totals.PendingOrders, db.Model(&model.Order{}).Where("status = ?", model.OrderStatusPending)},
		{&out.Totals.PendingCommission, db.Model(&model.Order{}).
			Where("commission_status = ?", model.CommissionStatusPending).
			Where("invite_user_id IS NOT NULL AND commission_balance > 0").
			Where("status IN ?", PaidStatuses)},
	}
	for _, c := range counts {
		if err := c.q.Count(c.dst).Error; err != nil {
			return nil, fmt.Errorf("stat: totals: %w", err)
		}
	}
	out.Totals.OnlineUsers = s.OnlineUsers()
	if out.Nodes.Total, out.Nodes.Online, err = s.NodeCounts(ctx, now); err != nil {
		return nil, err
	}
	return &out, nil
}

// window is the node status window W = 2 x max(server_push_interval, server_pull_interval).
func (s *Service) window() time.Duration {
	return online.Window(
		s.a.Settings.Duration("server_push_interval", time.Second),
		s.a.Settings.Duration("server_pull_interval", time.Second),
	)
}

// OnlineUsers returns the number of distinct users online on any node (docs/NODE_PROTOCOLS.md §5.1).
func (s *Service) OnlineUsers() int { return s.a.Online.OnlineUserCount(s.window()) }

// NodeCounts returns the number of nodes and how many are online (state >= idle within
// W = 2 x max(server_push_interval, server_pull_interval)); a child node reports its parent's state.
func (s *Service) NodeCounts(ctx context.Context, now time.Time) (total, onlineCount int, err error) {
	var nodes []model.Node
	if err := s.db(ctx).Select("id", "parent_id").Find(&nodes).Error; err != nil {
		return 0, 0, fmt.Errorf("stat: nodes: %w", err)
	}
	w := s.window()
	snap := s.a.Online.Snapshot()
	for _, n := range nodes {
		id := n.ID
		if n.ParentID != nil {
			id = *n.ParentID
		}
		if st, ok := snap[id]; ok && st.State(now, w) >= online.StateIdle {
			onlineCount++
		}
	}
	return len(nodes), onlineCount, nil
}

// ---------------------------------------------------------------- series

// Point is one day of a series.
type Point struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Value int64  `json:"value"`
}

// Series returns days points (oldest first, today last) of metric. Past days use the `stats` rows written by
// stats_daily when present; today and days without a row are computed from the live tables. Traffic always
// comes from stat_nodes.
func (s *Service) Series(ctx context.Context, metric string, days int, now time.Time) ([]Point, error) {
	if days < 1 {
		days = 1
	}
	today := DayStart(now)
	first := today.AddDate(0, 0, -(days - 1))
	points := make([]Point, days)
	index := make(map[string]int, days)
	for i := range points {
		k := DateKey(first.AddDate(0, 0, i))
		points[i].Date = k
		index[k] = i
	}

	if metric == MetricTraffic {
		var rows []struct {
			RecordAt time.Time
			Total    int64
		}
		err := s.db(ctx).Model(&model.StatNode{}).
			Select("record_at, COALESCE(SUM(u), 0) + COALESCE(SUM(d), 0) AS total").
			Where("record_at >= ? AND record_at <= ?", model.Day(first), model.Day(today)).
			Group("record_at").
			Scan(&rows).Error
		if err != nil {
			return nil, fmt.Errorf("stat: traffic series: %w", err)
		}
		for _, r := range rows {
			if i, ok := index[recordKey(r.RecordAt)]; ok {
				points[i].Value += r.Total
			}
		}
		return points, nil
	}

	pick, err := statColumn(metric)
	if err != nil {
		return nil, err
	}
	var rows []model.Stat
	if err := s.db(ctx).Where("record_at >= ? AND record_at < ?", model.Day(first), model.Day(today)).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("stat: stats rows: %w", err)
	}
	filled := make(map[int]bool, len(rows))
	for i := range rows {
		if idx, ok := index[recordKey(rows[i].RecordAt)]; ok && idx < days-1 {
			points[idx].Value = pick(&rows[i])
			filled[idx] = true
		}
	}
	// Live data for today and every day without a stats row, fetched once over the smallest span.
	lo, hi := -1, -1
	for i := range points {
		if !filled[i] {
			if lo < 0 {
				lo = i
			}
			hi = i
		}
	}
	if lo < 0 {
		return points, nil
	}
	live, err := s.live(ctx, metric, first.AddDate(0, 0, lo), first.AddDate(0, 0, hi+1))
	if err != nil {
		return nil, err
	}
	for i := lo; i <= hi; i++ {
		if !filled[i] {
			points[i].Value = live[points[i].Date]
		}
	}
	return points, nil
}

func statColumn(metric string) (func(*model.Stat) int64, error) {
	switch metric {
	case MetricIncome:
		return func(r *model.Stat) int64 { return r.PaidTotal }, nil
	case MetricOrders:
		return func(r *model.Stat) int64 { return r.PaidCount }, nil
	case MetricRegisters:
		return func(r *model.Stat) int64 { return r.RegisterCount }, nil
	}
	return nil, fmt.Errorf("stat: unknown metric %q", metric)
}

// live buckets metric per calendar day (in from's location) over [from, to).
func (s *Service) live(ctx context.Context, metric string, from, to time.Time) (map[string]int64, error) {
	loc := from.Location()
	out := map[string]int64{}
	switch metric {
	case MetricIncome, MetricOrders:
		var rows []struct {
			PaidAt      time.Time
			TotalAmount int64
		}
		err := s.db(ctx).Model(&model.Order{}).Select("paid_at", "total_amount").
			Where("status IN ?", PaidStatuses).
			Where("paid_at >= ? AND paid_at < ?", from, to).
			Scan(&rows).Error
		if err != nil {
			return nil, fmt.Errorf("stat: live orders: %w", err)
		}
		for _, r := range rows {
			k := DateKey(r.PaidAt.In(loc))
			if metric == MetricIncome {
				out[k] += r.TotalAmount
			} else {
				out[k]++
			}
		}
	case MetricRegisters:
		var created []time.Time
		err := s.db(ctx).Model(&model.User{}).
			Where("created_at >= ? AND created_at < ?", from, to).
			Pluck("created_at", &created).Error
		if err != nil {
			return nil, fmt.Errorf("stat: live registers: %w", err)
		}
		for _, t := range created {
			out[DateKey(t.In(loc))]++
		}
	default:
		return nil, fmt.Errorf("stat: unknown metric %q", metric)
	}
	return out, nil
}

// ---------------------------------------------------------------- rankings

// NodeRank is one row of the node traffic ranking (raw bytes).
type NodeRank struct {
	NodeID uint64 `json:"node_id"`
	Name   string `json:"name"`
	U      int64  `json:"u"`
	D      int64  `json:"d"`
	Total  int64  `json:"total"`
}

// UserRank is one row of the user traffic ranking (bytes multiplied by node rates).
type UserRank struct {
	UserID uint64 `json:"user_id"`
	Email  string `json:"email"`
	U      int64  `json:"u"`
	D      int64  `json:"d"`
	Total  int64  `json:"total"`
}

// NodeRanking returns every node's traffic on day (a calendar day; see model.Day), busiest first.
// Deleted nodes keep their rows with an empty name.
func (s *Service) NodeRanking(ctx context.Context, day time.Time) ([]NodeRank, error) {
	out := []NodeRank{}
	err := s.db(ctx).Table("stat_nodes").
		Select("stat_nodes.node_id, COALESCE(nodes.name, '') AS name, stat_nodes.u, stat_nodes.d, "+
			"stat_nodes.u + stat_nodes.d AS total").
		Joins("LEFT JOIN nodes ON nodes.id = stat_nodes.node_id").
		Where("stat_nodes.record_at = ?", model.Day(day)).
		Order("total DESC").Order("stat_nodes.node_id").
		Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("stat: node ranking: %w", err)
	}
	return out, nil
}

// UserRanking returns the limit heaviest users on day (summed over nodes).
func (s *Service) UserRanking(ctx context.Context, day time.Time, limit int) ([]UserRank, error) {
	out := []UserRank{}
	err := s.db(ctx).Table("stat_users").
		Select("stat_users.user_id, COALESCE(users.email, '') AS email, "+
			"COALESCE(SUM(stat_users.u), 0) AS u, COALESCE(SUM(stat_users.d), 0) AS d, "+
			"COALESCE(SUM(stat_users.u), 0) + COALESCE(SUM(stat_users.d), 0) AS total").
		Joins("LEFT JOIN users ON users.id = stat_users.user_id").
		Where("stat_users.record_at = ?", model.Day(day)).
		Group("stat_users.user_id, users.email").
		Order("total DESC").Order("stat_users.user_id").
		Limit(limit).
		Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("stat: user ranking: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------- daily aggregation

// Aggregate computes the `stats` row of the calendar day containing day (in day's location):
//   - order_count / order_total: orders created that day (any status)
//   - paid_count / paid_total: orders paid that day (PaidStatuses, by paid_at)
//   - commission_count / commission_total: commission_logs created that day (get_amount)
//   - register_count / invite_count: users created that day / of which invited
//   - transfer_used_total: stat_nodes u + d of that day
func (s *Service) Aggregate(ctx context.Context, day time.Time) (model.Stat, error) {
	from := DayStart(day)
	to := from.AddDate(0, 0, 1)
	st := model.Stat{RecordAt: model.Day(from)}
	db := s.db(ctx)

	type sum struct {
		N int64
		T int64
	}
	var orders, commissions sum
	if err := db.Model(&model.Order{}).Select("COUNT(*) AS n, COALESCE(SUM(total_amount), 0) AS t").
		Where("created_at >= ? AND created_at < ?", from, to).Scan(&orders).Error; err != nil {
		return st, fmt.Errorf("stat: aggregate orders: %w", err)
	}
	paid, err := s.Paid(ctx, from, to)
	if err != nil {
		return st, err
	}
	if err := db.Model(&model.CommissionLog{}).Select("COUNT(*) AS n, COALESCE(SUM(get_amount), 0) AS t").
		Where("created_at >= ? AND created_at < ?", from, to).Scan(&commissions).Error; err != nil {
		return st, fmt.Errorf("stat: aggregate commissions: %w", err)
	}
	if st.RegisterCount, err = s.Registers(ctx, from, to); err != nil {
		return st, err
	}
	if err := db.Model(&model.User{}).Where("created_at >= ? AND created_at < ?", from, to).
		Where("invite_user_id IS NOT NULL").Count(&st.InviteCount).Error; err != nil {
		return st, fmt.Errorf("stat: aggregate invites: %w", err)
	}
	var transfer struct{ T int64 }
	if err := db.Model(&model.StatNode{}).Select("COALESCE(SUM(u), 0) + COALESCE(SUM(d), 0) AS t").
		Where("record_at = ?", st.RecordAt).Scan(&transfer).Error; err != nil {
		return st, fmt.Errorf("stat: aggregate traffic: %w", err)
	}
	st.OrderCount, st.OrderTotal = orders.N, orders.T
	st.PaidCount, st.PaidTotal = paid.Count, paid.Total
	st.CommissionCount, st.CommissionTotal = commissions.N, commissions.T
	st.TransferUsedTotal = transfer.T
	return st, nil
}

// statColumns are the value columns of `stats` overwritten by SaveDay.
var statColumns = []string{
	"order_count", "order_total", "paid_count", "paid_total", "commission_count", "commission_total",
	"register_count", "invite_count", "transfer_used_total",
}

// SaveDay aggregates the calendar day containing day and upserts its `stats` row (idempotent).
func (s *Service) SaveDay(ctx context.Context, day time.Time) (model.Stat, error) {
	st, err := s.Aggregate(ctx, day)
	if err != nil {
		return st, err
	}
	db := s.db(ctx)
	set := make(map[string]any, len(statColumns))
	for _, c := range statColumns {
		set[c] = traffic.Excluded(db, c)
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "record_at"}},
		DoUpdates: clause.Assignments(set),
	}).Create(&st).Error; err != nil {
		return st, fmt.Errorf("stat: save day: %w", err)
	}
	return st, nil
}
