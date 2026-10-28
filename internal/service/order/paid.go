package ordersvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/payment"
	plansvc "github.com/linusxiong/ggboard/internal/service/plan"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// Paid marks the pending order tradeNo as paid with callbackNo and fulfils it in the same transaction.
// It is idempotent: an order that is no longer pending is returned unchanged with alreadyPaid=true, except
// a cancelled order, which yields ErrOrderCancelled.
func (s *Service) Paid(ctx context.Context, tradeNo, callbackNo string) (order *model.Order, alreadyPaid bool, err error) {
	return s.paid(ctx, tradeNo, callbackNo, nil)
}

// verified binds the callback checks to the same update that claims fulfilment.
func (s *Service) paid(ctx context.Context, tradeNo, callbackNo string, verified *model.Order) (order *model.Order, alreadyPaid bool, err error) {
	return s.paidWithActor(ctx, tradeNo, callbackNo, verified, 0, nil, nil)
}

func (s *Service) paidWithActor(ctx context.Context, tradeNo, callbackNo string, verified *model.Order, userID uint64, actor, admin *auth.Claims) (order *model.Order, alreadyPaid bool, err error) {
	now := s.now()
	var (
		o     model.Order
		plan  model.Plan
		user  model.User
		fresh bool
	)
	err = s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&model.Order{}).Where("trade_no = ? AND status = ?", tradeNo, model.OrderStatusPending)
		if actor != nil {
			q = q.Where("user_id = ?", userID)
		}
		if verified != nil {
			q = q.Where("payment_id = ? AND total_amount = ? AND handling_amount = ?",
				verified.PaymentID, verified.TotalAmount, verified.HandlingAmount)
		}
		res := q.Updates(map[string]any{"status": model.OrderStatusCompleted, "paid_at": now, "callback_no": callbackNo})
		if res.Error != nil {
			return res.Error
		}
		if err := tx.Where("trade_no = ?", tradeNo).First(&o).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return httpx.NotFound("order not found")
			}
			return err
		}
		if res.RowsAffected == 0 {
			if o.Status == model.OrderStatusPending {
				return errors.New("ordersvc: payment changed during verification")
			}
			if o.Status == model.OrderStatusCancelled {
				return ErrOrderCancelled
			}
			return nil
		}
		if actor != nil {
			if _, err := LockActor(tx, userID, actor); err != nil {
				return err
			}
		}
		if admin != nil {
			if err := lockAdminActor(tx, admin); err != nil {
				return err
			}
		}
		fresh = true
		return s.fulfil(ctx, tx, &o, &plan, &user, now)
	})
	if err != nil {
		return nil, false, err
	}
	if !fresh {
		return &o, true, nil
	}
	s.afterFulfil(&o, &plan, &user)
	return &o, false, nil
}

// fulfil applies a freshly paid order to its user (Xboard OrderService::open):
//   - onetime: plan, group, traffic (u=d=0), expired_at NULL;
//   - reset_traffic: u=d=0;
//   - periods: plan, group, traffic; expiry extends max(now, expired_at) by the period's months; an upgrade
//     starts from now; usage resets for a new purchase, an upgrade or a switch from a one-time plan, while a
//     renewal keeps it;
//   - always: speed/device limits from the plan, next_reset_at, refund_amount → balance, surplus orders →
//     status 4 and the invite commission.
func (s *Service) fulfil(ctx context.Context, tx *gorm.DB, o *model.Order, plan *model.Plan, user *model.User, now time.Time) error {
	if err := tx.First(plan, o.PlanID).Error; err != nil {
		return fmt.Errorf("ordersvc: fulfil %s: plan %d: %w", o.TradeNo, o.PlanID, err)
	}
	u, err := lockUser(tx, o.UserID)
	if err != nil {
		return fmt.Errorf("ordersvc: fulfil %s: %w", o.TradeNo, err)
	}
	upd := map[string]any{}
	resetUsage := func() { upd["u"], upd["d"] = int64(0), int64(0); u.U, u.D = 0, 0 }
	assignPlan := func() {
		upd["plan_id"], upd["group_id"], upd["transfer_enable"] = plan.ID, plan.GroupID, plan.TransferBytes()
		u.PlanID, u.GroupID, u.TransferEnable = &plan.ID, &plan.GroupID, plan.TransferBytes()
	}
	switch o.Period {
	case model.PeriodOnetime:
		resetUsage()
		assignPlan()
		upd["expired_at"], u.ExpiredAt = nil, nil
	case model.PeriodResetTraffic:
		resetUsage()
	default:
		months := model.PeriodMonths(o.Period)
		if months == 0 {
			return fmt.Errorf("ordersvc: fulfil %s: invalid period %q", o.TradeNo, o.Period)
		}
		start := now
		switch {
		case o.Type == model.OrderTypeUpgrade:
			resetUsage()
		case u.ExpiredAt == nil || o.Type == model.OrderTypeNew:
			resetUsage()
		case u.ExpiredAt.After(now):
			start = *u.ExpiredAt
		}
		assignPlan()
		exp := start.AddDate(0, months, 0)
		upd["expired_at"], u.ExpiredAt = exp, &exp
	}
	upd["speed_limit"], upd["device_limit"], upd["conn_limit"] = plan.SpeedLimit, plan.DeviceLimit, plan.ConnLimit
	next := usersvc.New(s.a).ComputeNextResetAt(u, plan, now)
	upd["next_reset_at"] = next
	if o.RefundAmount > 0 {
		if err := creditUser(tx, u.ID, "balance", o.RefundAmount); err != nil {
			return err
		}
	}
	if err := tx.Model(&model.User{}).Where("id = ?", u.ID).Updates(upd).Error; err != nil {
		return err
	}
	if len(o.SurplusOrderIDs) > 0 {
		err := tx.Model(&model.Order{}).Where("id IN ? AND status = ? AND id <> ?", []uint64(o.SurplusOrderIDs), model.OrderStatusCompleted, o.ID).
			Update("status", model.OrderStatusDiscounted).Error
		if err != nil {
			return err
		}
	}
	commission, err := s.commissionFor(tx, o)
	if err != nil {
		return err
	}
	o.CommissionBalance, o.CommissionStatus = commission, model.CommissionStatusPending
	if err := tx.Model(&model.Order{}).Where("id = ?", o.ID).
		Updates(map[string]any{"commission_balance": commission, "commission_status": model.CommissionStatusPending}).Error; err != nil {
		return err
	}
	*user = *u
	return nil
}

func (s *Service) invalidateUser(id uint64) { auth.InvalidateUser(s.a, id) }

// afterFulfil runs after commit: cache invalidation, audit log and the order_paid email.
func (s *Service) afterFulfil(o *model.Order, plan *model.Plan, u *model.User) {
	s.invalidateUser(o.UserID)
	usersvc.New(s.a).InvalidateNodeUsers()
	s.a.Log.Info("order paid", "trade_no", o.TradeNo, "user_id", o.UserID, "plan_id", o.PlanID, "type", o.Type,
		"period", o.Period, "total", o.TotalAmount, "callback_no", o.CallbackNo, "commission", o.CommissionBalance)
	// shortcut: notification amounts saturate at int64's limit; use decimal output if charges ever exceed it.
	paidSum := saturatingAmount(saturatingAmount(o.TotalAmount, o.HandlingAmount), o.BalanceAmount)
	if notify.Enabled(s.a) {
		notify.Emit(s.a, notify.OrderPaid(s.a, notify.Order{
			UserID: o.UserID, Email: u.Email, Plan: plan.Name, Period: o.Period,
			Amount: paidSum, TradeNo: o.TradeNo,
			Gateway: s.paymentName(o.PaymentID), CallbackNo: o.CallbackNo,
		}))
	}
	if u.Email == "" {
		return
	}
	locale := s.a.Settings.String("default_locale")
	amount := s.a.Settings.String("currency_symbol") + payment.FormatAmount(paidSum)
	data := mail.Data{
		"TradeNo":  o.TradeNo,
		"PlanName": plan.Name,
		"Period":   plansvc.PeriodLabel(o.Period, locale),
		"Amount":   amount,
	}
	if base := s.SiteURL(""); base != "" {
		data["URL"] = base + "/orders/" + o.TradeNo
	}
	if err := s.a.Mailer.SendTemplate(u.Email, mail.TplOrderPaid, data); err != nil {
		s.a.Log.Warn("order paid mail not queued", "trade_no", o.TradeNo, "err", err)
	}
}

// paymentName returns the display name of payment id ("" when unset or unknown).
func (s *Service) paymentName(id *uint64) string {
	if id == nil {
		return ""
	}
	var p model.Payment
	if err := s.a.DB.Select("id", "name").Where("id = ?", *id).Limit(1).Find(&p).Error; err != nil {
		return ""
	}
	return p.Name
}
