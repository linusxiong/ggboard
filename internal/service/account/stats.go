package account

import (
	"context"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/model"
)

// Stats is the user dashboard summary (GET /user/stats).
type Stats struct {
	PendingOrders     int64 `json:"pending_orders"`
	OpenTickets       int64 `json:"open_tickets"`
	NoticesCount      int64 `json:"notices_count"`
	InviteCount       int64 `json:"invite_count"`
	CommissionPending int64 `json:"commission_pending"`
}

// PaidOrderStatuses are the order statuses whose commission can still be earned (paid orders: processing,
// completed, discounted). Pending and cancelled orders carry a provisional commission_balance that never
// counts.
var PaidOrderStatuses = []model.OrderStatus{model.OrderStatusProcessing, model.OrderStatusCompleted, model.OrderStatusDiscounted}

// CommissionPending sums commission_balance (cents) of paid orders invited by inviterID whose commission is
// not settled yet (commission_status pending or processing).
func CommissionPending(ctx context.Context, db *gorm.DB, inviterID uint64) (int64, error) {
	var sum int64
	err := db.WithContext(ctx).Model(&model.Order{}).
		Select("COALESCE(SUM(commission_balance), 0)").
		Where("invite_user_id = ?", inviterID).
		Where("status IN ?", PaidOrderStatuses).
		Where("commission_status IN ?", []model.CommissionStatus{model.CommissionStatusPending, model.CommissionStatusProcessing}).
		Scan(&sum).Error
	return sum, err
}

// Stats returns the dashboard counters of userID.
func (s *Service) Stats(ctx context.Context, userID uint64) (Stats, error) {
	db := s.a.DB.WithContext(ctx)
	var st Stats
	if err := db.Model(&model.Order{}).
		Where("user_id = ? AND status = ?", userID, model.OrderStatusPending).Count(&st.PendingOrders).Error; err != nil {
		return st, err
	}
	if err := db.Model(&model.Ticket{}).
		Where("user_id = ? AND status = ?", userID, model.TicketStatusOpen).Count(&st.OpenTickets).Error; err != nil {
		return st, err
	}
	if err := db.Model(&model.Notice{}).Where(map[string]any{"show": true}).Count(&st.NoticesCount).Error; err != nil {
		return st, err
	}
	if err := db.Model(&model.User{}).Where("invite_user_id = ?", userID).Count(&st.InviteCount).Error; err != nil {
		return st, err
	}
	var err error
	st.CommissionPending, err = CommissionPending(ctx, s.a.DB, userID)
	return st, err
}
