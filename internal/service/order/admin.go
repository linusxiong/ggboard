package ordersvc

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
)

func (s *Service) loadOrder(ctx context.Context, id uint64) (*model.Order, error) {
	var o model.Order
	if err := s.a.DB.WithContext(ctx).First(&o, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.NotFound("order not found")
		}
		return nil, err
	}
	return &o, nil
}

// MarkPaid confirms a pending order manually (callback_no "manual") and fulfils it.
func (s *Service) MarkPaid(ctx context.Context, orderID uint64) (*model.Order, error) {
	return s.markPaid(ctx, orderID, nil)
}

func (s *Service) MarkPaidAs(ctx context.Context, orderID uint64, admin *auth.Claims) (*model.Order, error) {
	if admin == nil {
		return nil, httpx.Unauthorized("session expired, please sign in again")
	}
	return s.markPaid(ctx, orderID, admin)
}

func (s *Service) markPaid(ctx context.Context, orderID uint64, admin *auth.Claims) (*model.Order, error) {
	o, err := s.loadOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if o.Status != model.OrderStatusPending {
		return nil, httpx.Conflict("only pending orders can be marked paid")
	}
	paid, already, err := s.paidWithActor(ctx, o.TradeNo, CallbackManual, nil, 0, nil, admin)
	if errors.Is(err, ErrOrderCancelled) {
		return nil, httpx.Conflict("only pending orders can be marked paid")
	}
	if err != nil {
		return nil, err
	}
	if already {
		return nil, httpx.Conflict("only pending orders can be marked paid")
	}
	return paid, nil
}

// SetCommissionStatus lets an administrator approve (1), invalidate (3) or reset (0) an order's commission.
// Status 2 (paid out) is set only by the commission_settle job, and a paid-out commission cannot change.
func (s *Service) SetCommissionStatus(ctx context.Context, orderID uint64, status model.CommissionStatus) (*model.Order, error) {
	return s.setCommissionStatus(ctx, orderID, status, nil)
}

func (s *Service) SetCommissionStatusAs(ctx context.Context, orderID uint64, status model.CommissionStatus, admin *auth.Claims) (*model.Order, error) {
	if admin == nil {
		return nil, httpx.Unauthorized("session expired, please sign in again")
	}
	return s.setCommissionStatus(ctx, orderID, status, admin)
}

func (s *Service) setCommissionStatus(ctx context.Context, orderID uint64, status model.CommissionStatus, admin *auth.Claims) (*model.Order, error) {
	switch status {
	case model.CommissionStatusPending, model.CommissionStatusProcessing, model.CommissionStatusInvalid:
	case model.CommissionStatusValid:
		return nil, httpx.FieldError("commission_status", "2 is set by the commission settlement; use 1 to approve")
	default:
		return nil, httpx.FieldError("commission_status", "must be one of: 0, 1, 3")
	}
	var o model.Order
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Order{}).
			Where("id = ? AND commission_status <> ?", orderID, model.CommissionStatusValid).
			Update("commission_status", status)
		if res.Error != nil {
			return res.Error
		}
		if admin != nil {
			if err := lockAdminActor(tx, admin); err != nil {
				return err
			}
		}
		if err := tx.First(&o, orderID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.NotFound("order not found")
			}
			return err
		}
		if res.RowsAffected == 0 && o.CommissionStatus == model.CommissionStatusValid {
			return httpx.Conflict("the commission has already been paid out")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.a.Log.Info("order commission status changed", "trade_no", o.TradeNo, "commission_status", status)
	return &o, nil
}
