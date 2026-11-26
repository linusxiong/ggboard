// Package plansvc contains plan queries shared by the guest, user, admin and order code: visibility rules,
// active-subscriber counts (capacity, admin user_count) and the public/user/admin plan views.
package plansvc

import (
	"context"
	"fmt"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/model"
)

// OrderBySort orders rows by sort then id (sort is quoted by GORM; never write it in raw SQL).
func OrderBySort() clause.OrderBy {
	return clause.OrderBy{Columns: []clause.OrderByColumn{
		{Column: clause.Column{Name: "sort"}},
		{Column: clause.Column{Name: "id"}},
	}}
}

// activeScope restricts users to holders whose subscription has not expired (expired_at NULL or >= now),
// matching Xboard's PlanService::countActiveUsers.
func activeScope(db *gorm.DB, now time.Time) *gorm.DB {
	return db.Where("plan_id IS NOT NULL").Where("expired_at IS NULL OR expired_at >= ?", now)
}

// ActiveUserCounts returns the number of active subscribers per plan id.
func ActiveUserCounts(ctx context.Context, db *gorm.DB, now time.Time) (map[uint64]int64, error) {
	var rows []struct {
		PlanID uint64
		Cnt    int64
	}
	err := activeScope(db.WithContext(ctx).Model(&model.User{}), now).
		Select("plan_id, COUNT(*) AS cnt").Group("plan_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]int64, len(rows))
	for _, r := range rows {
		out[r.PlanID] = r.Cnt
	}
	return out, nil
}

// ActiveUserCount returns the number of active subscribers of plan, excluding excludeUserID (0 = none).
func ActiveUserCount(ctx context.Context, db *gorm.DB, planID, excludeUserID uint64, now time.Time) (int64, error) {
	q := activeScope(db.WithContext(ctx).Model(&model.User{}), now).Where("plan_id = ?", planID)
	if excludeUserID != 0 {
		q = q.Where("id <> ?", excludeUserID)
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}

// HolderCount returns how many users (active or not) hold plan.
func HolderCount(ctx context.Context, db *gorm.DB, planID uint64) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&model.User{}).Where("plan_id = ?", planID).Count(&n).Error
	return n, err
}

// openOrderStatuses are the unpaid order statuses (pending, processing).
var openOrderStatuses = []model.OrderStatus{model.OrderStatusPending, model.OrderStatusProcessing}

// reservedScope restricts orders to the open ones whose buyer is not an active holder of the order's plan: each
// takes a new seat once paid, so it holds one until it is paid, cancelled or timed out (order_timeout). An active
// holder's renewal or traffic reset takes no new seat.
func reservedScope(db *gorm.DB, now time.Time) *gorm.DB {
	return db.Where("orders.status IN ?", openOrderStatuses).
		Where("NOT EXISTS (SELECT 1 FROM users WHERE users.id = orders.user_id AND users.plan_id = orders.plan_id"+
			" AND (users.expired_at IS NULL OR users.expired_at >= ?))", now)
}

// ReservedSeats returns the number of users with an open order of plan that will take a seat when paid
// (reservedScope), excluding excludeUserID (0 = none).
func ReservedSeats(ctx context.Context, db *gorm.DB, planID, excludeUserID uint64, now time.Time) (int64, error) {
	q := reservedScope(db.WithContext(ctx).Model(&model.Order{}), now).Where("orders.plan_id = ?", planID)
	if excludeUserID != 0 {
		q = q.Where("orders.user_id <> ?", excludeUserID)
	}
	var n int64
	err := q.Distinct("orders.user_id").Count(&n).Error
	return n, err
}

// TakenSeatCounts returns, per plan id, the seats taken: active subscribers plus the users holding a reserved seat
// (ReservedSeats), excluding excludeUserID's reservations (0 = none). It is what the sold-out flags read.
func TakenSeatCounts(ctx context.Context, db *gorm.DB, excludeUserID uint64, now time.Time) (map[uint64]int64, error) {
	out, err := ActiveUserCounts(ctx, db, now)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		PlanID uint64
		Cnt    int64
	}
	q := reservedScope(db.WithContext(ctx).Model(&model.Order{}), now)
	if excludeUserID != 0 {
		q = q.Where("orders.user_id <> ?", excludeUserID)
	}
	if err := q.Select("orders.plan_id AS plan_id, COUNT(DISTINCT orders.user_id) AS cnt").Group("orders.plan_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.PlanID] += r.Cnt
	}
	return out, nil
}

// HasCapacity reports whether plan can take another subscriber (capacity_limit nil = unlimited): active
// subscribers plus reserved seats (open orders that will take one when paid) stay below capacity_limit. The buyer
// (excludeUserID) is not counted. A purchase serializes the check with other purchases of the plan first (the
// order service locks the plan row), so two buyers cannot both take the last seat.
func HasCapacity(ctx context.Context, db *gorm.DB, p *model.Plan, excludeUserID uint64, now time.Time) (bool, error) {
	if p.CapacityLimit == nil {
		return true, nil
	}
	n, err := ActiveUserCount(ctx, db, p.ID, excludeUserID, now)
	if err != nil {
		return false, err
	}
	r, err := ReservedSeats(ctx, db, p.ID, excludeUserID, now)
	if err != nil {
		return false, err
	}
	return n+r < int64(*p.CapacityLimit), nil
}

// soldOut reports whether a capacity-limited plan is full according to counts.
func soldOut(p *model.Plan, counts map[uint64]int64) bool {
	return p.CapacityLimit != nil && counts[p.ID] >= int64(*p.CapacityLimit)
}

// PublicView is a plan as shown to guests and users (no group or capacity figures).
type PublicView struct {
	ID                 uint64             `json:"id"`
	Name               string             `json:"name"`
	Content            string             `json:"content"`
	Tags               []string           `json:"tags"`
	TransferEnable     int64              `json:"transfer_enable"`
	SpeedLimit         *int               `json:"speed_limit"`
	DeviceLimit        *int               `json:"device_limit"`
	ConnLimit          *int               `json:"conn_limit"`
	Prices             map[string]int64   `json:"prices"`
	ResetTrafficMethod *model.ResetMethod `json:"reset_traffic_method"`
	Show               bool               `json:"show"`
	Sell               bool               `json:"sell"`
	Renew              bool               `json:"renew"`
	Sort               int                `json:"sort"`
	SoldOut            bool               `json:"sold_out"`
	IsCurrent          bool               `json:"is_current"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

// Public builds the guest/user view of p.
func Public(p *model.Plan) PublicView {
	tags := []string(p.Tags)
	if tags == nil {
		tags = []string{}
	}
	prices := map[string]int64(p.Prices)
	if prices == nil {
		prices = map[string]int64{}
	}
	return PublicView{
		ID: p.ID, Name: p.Name, Content: p.Content, Tags: tags, TransferEnable: p.TransferEnable,
		SpeedLimit: p.SpeedLimit, DeviceLimit: p.DeviceLimit, ConnLimit: p.ConnLimit, Prices: prices, ResetTrafficMethod: p.ResetTrafficMethod,
		Show: p.Show, Sell: p.Sell, Renew: p.Renew, Sort: p.Sort, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// AdminView is a plan with every column plus its active subscriber count.
type AdminView struct {
	model.Plan
	UserCount int64 `json:"user_count"`
}

// ListForGuest returns plans with show && sell that still have capacity, ordered by sort.
func ListForGuest(ctx context.Context, db *gorm.DB, now time.Time) ([]PublicView, error) {
	var plans []model.Plan
	err := db.WithContext(ctx).Where(map[string]any{"show": true, "sell": true}).Clauses(OrderBySort()).Find(&plans).Error
	if err != nil {
		return nil, err
	}
	counts, err := countsIfNeeded(ctx, db, plans, 0, now)
	if err != nil {
		return nil, err
	}
	out := make([]PublicView, 0, len(plans))
	for i := range plans {
		if soldOut(&plans[i], counts) {
			continue
		}
		out = append(out, Public(&plans[i]))
	}
	return out, nil
}

// ListForUser returns plans with show=true plus u's current plan (even when hidden), with sold_out and
// is_current flags. The current plan is never reported sold out to its holder.
func ListForUser(ctx context.Context, db *gorm.DB, u *model.User, now time.Time) ([]PublicView, error) {
	q := db.WithContext(ctx).Where(map[string]any{"show": true})
	if u.PlanID != nil {
		q = q.Or("id = ?", *u.PlanID)
	}
	var plans []model.Plan
	if err := q.Clauses(OrderBySort()).Find(&plans).Error; err != nil {
		return nil, err
	}
	counts, err := countsIfNeeded(ctx, db, plans, u.ID, now)
	if err != nil {
		return nil, err
	}
	out := make([]PublicView, 0, len(plans))
	for i := range plans {
		v := Public(&plans[i])
		v.IsCurrent = u.PlanID != nil && *u.PlanID == plans[i].ID
		v.SoldOut = !v.IsCurrent && soldOut(&plans[i], counts)
		out = append(out, v)
	}
	return out, nil
}

// GetForUser returns plan id when it is visible to u (show=true or u's current plan).
func GetForUser(ctx context.Context, db *gorm.DB, u *model.User, id uint64, now time.Time) (*PublicView, error) {
	var p model.Plan
	if err := db.WithContext(ctx).First(&p, id).Error; err != nil {
		return nil, err
	}
	current := u.PlanID != nil && *u.PlanID == p.ID
	if !p.Show && !current {
		return nil, gorm.ErrRecordNotFound
	}
	v := Public(&p)
	v.IsCurrent = current
	if !current {
		ok, err := HasCapacity(ctx, db, &p, u.ID, now)
		if err != nil {
			return nil, err
		}
		v.SoldOut = !ok
	}
	return &v, nil
}

// ListForAdmin returns every plan with its active user count.
func ListForAdmin(ctx context.Context, db *gorm.DB, now time.Time) ([]AdminView, error) {
	var plans []model.Plan
	if err := db.WithContext(ctx).Clauses(OrderBySort()).Find(&plans).Error; err != nil {
		return nil, err
	}
	counts, err := ActiveUserCounts(ctx, db, now)
	if err != nil {
		return nil, err
	}
	out := make([]AdminView, len(plans))
	for i, p := range plans {
		out[i] = AdminView{Plan: p, UserCount: counts[p.ID]}
	}
	return out, nil
}

// countsIfNeeded returns TakenSeatCounts (viewer's own reservations excluded) when a plan is capacity-limited.
func countsIfNeeded(ctx context.Context, db *gorm.DB, plans []model.Plan, viewerID uint64, now time.Time) (map[uint64]int64, error) {
	if !slices.ContainsFunc(plans, func(p model.Plan) bool { return p.CapacityLimit != nil }) {
		return nil, nil
	}
	return TakenSeatCounts(ctx, db, viewerID, now)
}

// ValidatePrices checks price keys (model.Periods) and non-negative values; it returns field errors keyed
// "prices.<period>".
func ValidatePrices(prices map[string]int64) map[string][]string {
	errs := map[string][]string{}
	for k, v := range prices {
		if !model.IsValidPeriod(k) {
			errs["prices."+k] = append(errs["prices."+k], "unknown period")
			continue
		}
		if v < 0 {
			errs["prices."+k] = append(errs["prices."+k], "must be greater than or equal to 0")
		}
	}
	return errs
}

// PeriodLabel returns a human name for a price period in locale (zh-CN or en).
func PeriodLabel(period, locale string) string {
	zh := map[string]string{
		model.PeriodMonthly: "月付", model.PeriodQuarterly: "季付", model.PeriodHalfYearly: "半年付", model.PeriodYearly: "年付",
		model.PeriodTwoYearly: "两年付", model.PeriodThreeYearly: "三年付", model.PeriodOnetime: "一次性", model.PeriodResetTraffic: "流量重置包",
	}
	en := map[string]string{
		model.PeriodMonthly: "Monthly", model.PeriodQuarterly: "Quarterly", model.PeriodHalfYearly: "Half-yearly", model.PeriodYearly: "Yearly",
		model.PeriodTwoYearly: "Two years", model.PeriodThreeYearly: "Three years", model.PeriodOnetime: "One-time", model.PeriodResetTraffic: "Traffic reset",
	}
	m := en
	if locale == "zh-CN" {
		m = zh
	}
	if s, ok := m[period]; ok {
		return s
	}
	return fmt.Sprint(period)
}
