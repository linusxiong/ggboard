package account

import (
	"context"

	"github.com/linusxiong/ggboard/internal/model"
	invitesvc "github.com/linusxiong/ggboard/internal/service/invite"
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
	st.CommissionPending, err = invitesvc.New(s.a).CommissionPending(ctx, userID)
	return st, err
}
