package ordersvc

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
)

// Cancel cancels a pending order: status 2, balance_amount back to the user's balance and the coupon use
// returned. Orders that are not pending yield 409.
func (s *Service) Cancel(ctx context.Context, orderID uint64) (*model.Order, error) {
	var o model.Order
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Order{}).Where("id = ? AND status = ?", orderID, model.OrderStatusPending).
			Update("status", model.OrderStatusCancelled)
		if res.Error != nil {
			return res.Error
		}
		if err := tx.First(&o, orderID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.NotFound("order not found")
			}
			return err
		}
		if res.RowsAffected == 0 {
			return httpx.Conflict("only pending orders can be cancelled")
		}
		if o.BalanceAmount > 0 {
			if err := tx.Model(&model.User{}).Where("id = ?", o.UserID).
				UpdateColumn("balance", gorm.Expr("balance + ?", o.BalanceAmount)).Error; err != nil {
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
	s.a.Log.Info("order cancelled", "trade_no", o.TradeNo, "user_id", o.UserID, "balance_restored", o.BalanceAmount)
	return &o, nil
}

// CancelExpired is the order_timeout job: it cancels pending orders created more than order_timeout_minutes
// ago and returns how many were cancelled.
func (s *Service) CancelExpired(ctx context.Context) (int, error) {
	timeout := time.Duration(s.a.Settings.Int("order_timeout_minutes")) * time.Minute
	if timeout <= 0 {
		return 0, nil
	}
	var ids []uint64
	err := s.a.DB.WithContext(ctx).Model(&model.Order{}).
		Where("status = ? AND created_at < ?", model.OrderStatusPending, s.now().Add(-timeout)).
		Order("id").Limit(batchLimit).Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		if _, err := s.Cancel(ctx, id); err != nil {
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
