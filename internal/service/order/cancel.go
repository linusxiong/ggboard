package ordersvc

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
)

// Who cancelled an order (CancelBy). Cancellations by the timeout job or an administrator are notified
// (order_cancel); a user cancelling their own order is not.
const (
	CancelByUser    = "user"
	CancelByAdmin   = notify.ReasonAdmin
	CancelByTimeout = notify.ReasonTimeout
)

// Cancel cancels a pending order on its user's request: status 2, balance_amount back to the user's balance and
// the coupon use returned. Orders that are not pending yield 409.
func (s *Service) Cancel(ctx context.Context, orderID uint64) (*model.Order, error) {
	return s.CancelBy(ctx, orderID, CancelByUser)
}

func (s *Service) CancelForUser(ctx context.Context, orderID uint64, actor *auth.Claims) (*model.Order, error) {
	return s.cancelBy(ctx, orderID, CancelByUser, actor)
}

func (s *Service) CancelForAdmin(ctx context.Context, orderID uint64, admin *auth.Claims) (*model.Order, error) {
	if admin == nil {
		return nil, httpx.Unauthorized("session expired, please sign in again")
	}
	return s.cancelBy(ctx, orderID, CancelByAdmin, admin)
}

// CancelBy is Cancel on behalf of by (CancelByUser, CancelByAdmin or CancelByTimeout).
func (s *Service) CancelBy(ctx context.Context, orderID uint64, by string) (*model.Order, error) {
	return s.cancelBy(ctx, orderID, by, nil)
}

func (s *Service) cancelBy(ctx context.Context, orderID uint64, by string, actor *auth.Claims) (*model.Order, error) {
	var o model.Order
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A user's cancel matches only their own order, so another user's order is never touched, not even inside a
		// transaction that the account check below would roll back (same shape and lock order as paid).
		owned := func(q *gorm.DB) *gorm.DB { return q }
		if actor != nil && by == CancelByUser {
			uid, err := actor.UserID()
			if err != nil {
				return httpx.Unauthorized("session expired, please sign in again")
			}
			owned = func(q *gorm.DB) *gorm.DB { return q.Where("user_id = ?", uid) }
		}
		res := owned(tx.Model(&model.Order{}).Where("id = ? AND status = ?", orderID, model.OrderStatusPending)).
			Update("status", model.OrderStatusCancelled)
		if res.Error != nil {
			return res.Error
		}
		if err := owned(tx.Where("id = ?", orderID)).First(&o).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.NotFound("order not found")
			}
			return err
		}
		if res.RowsAffected == 0 {
			return httpx.Conflict("only pending orders can be cancelled")
		}
		if actor != nil {
			if by == CancelByAdmin {
				if err := lockAdminActor(tx, actor); err != nil {
					return err
				}
			} else if _, err := LockActor(tx, o.UserID, actor); err != nil {
				return err
			}
		}
		if o.BalanceAmount > 0 {
			if err := creditUser(tx, o.UserID, "balance", o.BalanceAmount); err != nil {
				return err
			}
		}
		if o.CouponID != nil {
			return restoreCoupon(tx, *o.CouponID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if o.BalanceAmount > 0 {
		s.invalidateUser(o.UserID)
	}
	s.a.Log.Info("order cancelled", "trade_no", o.TradeNo, "user_id", o.UserID, "balance_restored", o.BalanceAmount, "by", by)
	if by != CancelByUser && notify.Enabled(s.a) {
		notify.Emit(s.a, notify.OrderCancel(s.a, s.notifyOrder(ctx, &o, by)))
	}
	return &o, nil
}

// notifyOrder describes o for an order notification (the user's e-mail and the plan name are looked up).
func (s *Service) notifyOrder(ctx context.Context, o *model.Order, reason string) notify.Order {
	n := notify.Order{UserID: o.UserID, Period: o.Period, Amount: o.TotalAmount + o.BalanceAmount, TradeNo: o.TradeNo, Reason: reason}
	var u model.User
	if err := s.a.DB.WithContext(ctx).Select("id", "email").Where("id = ?", o.UserID).Limit(1).Find(&u).Error; err == nil {
		n.Email = u.Email
	}
	var p model.Plan
	if err := s.a.DB.WithContext(ctx).Select("id", "name").Where("id = ?", o.PlanID).Limit(1).Find(&p).Error; err == nil {
		n.Plan = p.Name
	}
	return n
}

// CheckoutGrace is how long a pending order stays payable after its last checkout. Payment pages outlive the
// order deadline (Stripe Checkout Sessions expire no sooner than 30 minutes after creation, others not at all),
// so cancelling a just-checked-out order turns a normal payment into "paid after cancel" needing manual repair.
const CheckoutGrace = 31 * time.Minute

// CancelExpired is the order_timeout job: it cancels pending orders created more than order_timeout_minutes
// ago, except orders checked out (payment_id set, updated_at bumped by Checkout) within CheckoutGrace, and
// returns how many were cancelled.
func (s *Service) CancelExpired(ctx context.Context) (int, error) {
	timeout := time.Duration(s.a.Settings.Int("order_timeout_minutes")) * time.Minute
	if timeout <= 0 {
		return 0, nil
	}
	now := s.now()
	var ids []uint64
	err := s.a.DB.WithContext(ctx).Model(&model.Order{}).
		Where("status = ? AND created_at < ?", model.OrderStatusPending, now.Add(-timeout)).
		Where("(payment_id IS NULL OR updated_at < ?)", now.Add(-CheckoutGrace)).
		Order("id").Limit(batchLimit).Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		if _, err := s.CancelBy(ctx, id, CancelByTimeout); err != nil {
			var he *httpx.Error
			if errors.As(err, &he) && he.Status < 500 {
				continue // paid or cancelled meanwhile
			}
			return n, err
		}
		n++
	}
	return n, nil
}
