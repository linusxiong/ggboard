package ordersvc

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/model"
)

// commissionFor computes the invite commission of a freshly paid order (Xboard setInvite, evaluated at payment
// time). The base is the amount paid through the gateway (total_amount; balance and handling fees excluded).
// Eligibility follows the inviter's commission_type: 0 system → settings.commission_first_time_enable decides
// between "first paid order only" and "every order"; 1 period → every order; 2 one-time → first only. The rate
// is the inviter's commission_rate, or settings.invite_commission when it is NULL or 0 (Xboard treats 0 as unset).
func (s *Service) commissionFor(tx *gorm.DB, o *model.Order) (int64, error) {
	if o.InviteUserID == nil || o.TotalAmount <= 0 {
		return 0, nil
	}
	var inviter model.User
	if err := tx.First(&inviter, *o.InviteUserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	firstOnly := false
	switch inviter.CommissionType {
	case model.CommissionTypeSystem:
		firstOnly = s.a.Settings.Bool("commission_first_time_enable")
	case model.CommissionTypeOneTime:
		firstOnly = true
	}
	if firstOnly {
		var n int64
		err := tx.Model(&model.Order{}).Where("user_id = ? AND id <> ? AND status IN ?", o.UserID, o.ID,
			[]model.OrderStatus{model.OrderStatusProcessing, model.OrderStatusCompleted, model.OrderStatusDiscounted}).Count(&n).Error
		if err != nil {
			return 0, err
		}
		if n > 0 {
			return 0, nil
		}
	}
	rate := int64(s.a.Settings.Int("invite_commission"))
	if inviter.CommissionRate != nil && *inviter.CommissionRate > 0 {
		rate = int64(*inviter.CommissionRate)
	}
	return percentOf(o.TotalAmount, min(rate, 100)), nil
}

// SettleCommissions is the commission_settle job (Xboard check:commission):
//  1. with commission_auto_check_enable, completed orders whose commission has been pending for
//     CommissionHold are approved (commission_status 0 → 1);
//  2. every approved order (1, automatically or by an administrator) is paid out: the commission is split
//     across up to three upline levels when commission_distribution_enable is on (l1/l2/l3 percent of it),
//     otherwise the direct inviter gets all of it; each share goes to commission_balance (or balance when
//     withdraw_close_enable), is logged in commission_logs, and the order becomes 2 (valid).
//
// It returns the number of orders settled.
func (s *Service) SettleCommissions(ctx context.Context) (int, error) {
	db := s.a.DB.WithContext(ctx)
	if s.a.Settings.Bool("commission_auto_check_enable") {
		err := db.Model(&model.Order{}).
			Where("commission_status = ? AND status = ? AND invite_user_id IS NOT NULL AND commission_balance > 0 AND paid_at <= ?",
				model.CommissionStatusPending, model.OrderStatusCompleted, s.now().Add(-CommissionHold)).
			Update("commission_status", model.CommissionStatusProcessing).Error
		if err != nil {
			return 0, err
		}
	}
	var ids []uint64
	err := db.Model(&model.Order{}).Where("commission_status = ? AND invite_user_id IS NOT NULL", model.CommissionStatusProcessing).
		Order("id").Limit(batchLimit).Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}
	settled := 0
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return settled, err
		}
		ok, credited, err := s.settleOne(ctx, id)
		if err != nil {
			s.a.Log.Error("commission settlement failed", "order_id", id, "err", err)
			continue
		}
		if ok {
			settled++
		}
		for _, uid := range credited {
			s.invalidateUser(uid)
		}
	}
	return settled, nil
}

func (s *Service) commissionLevels() []int64 {
	if !s.a.Settings.Bool("commission_distribution_enable") {
		return []int64{100}
	}
	return []int64{
		int64(s.a.Settings.Int("commission_distribution_l1")),
		int64(s.a.Settings.Int("commission_distribution_l2")),
		int64(s.a.Settings.Int("commission_distribution_l3")),
	}
}

func (s *Service) settleOne(ctx context.Context, id uint64) (bool, []uint64, error) {
	var credited []uint64
	settled := false
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Order{}).Where("id = ? AND commission_status = ?", id, model.CommissionStatusProcessing).
			Update("commission_status", model.CommissionStatusValid)
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error
		}
		var o model.Order
		if err := tx.First(&o, id).Error; err != nil {
			return err
		}
		settled = true
		if o.InviteUserID == nil || o.CommissionBalance <= 0 {
			return nil
		}
		column := "commission_balance"
		if s.a.Settings.Bool("withdraw_close_enable") {
			column = "balance"
		}
		inviterID := *o.InviteUserID
		for _, pct := range s.commissionLevels() {
			var inv model.User
			if err := tx.First(&inv, inviterID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					break
				}
				return err
			}
			if share := percentOf(o.CommissionBalance, pct); share > 0 {
				if err := creditUser(tx, inv.ID, column, share); err != nil {
					return err
				}
				if err := tx.Create(&model.CommissionLog{
					InviteUserID: inv.ID, UserID: o.UserID, TradeNo: o.TradeNo, OrderAmount: o.TotalAmount, GetAmount: share,
				}).Error; err != nil {
					return err
				}
				credited = append(credited, inv.ID)
			}
			if inv.InviteUserID == nil || *inv.InviteUserID == inv.ID {
				break
			}
			inviterID = *inv.InviteUserID
		}
		return nil
	})
	if err != nil {
		return false, nil, err
	}
	return settled, credited, nil
}
