package ordersvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
)

// Coupon usage follows Xboard: coupons.limit_use is the REMAINING number of uses (nil = unlimited). It is
// decremented when an order using the coupon is created and given back when that order is cancelled.
// limit_use_with_user counts the user's orders with the coupon that were paid (status not pending/cancelled).

// findCoupon loads a coupon by code (case-sensitive, trimmed).
func findCoupon(tx *gorm.DB, code, field string) (*model.Coupon, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, httpx.FieldError(field, "is required")
	}
	var c model.Coupon
	if err := tx.Where("code = ?", code).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.FieldError(field, "invalid coupon")
		}
		return nil, err
	}
	return &c, nil
}

// validateCoupon applies the coupon rules for userID buying planID/period at now. field names the request
// field in errors ("code" or "coupon_code").
func validateCoupon(tx *gorm.DB, c *model.Coupon, userID, planID uint64, period, field string, now time.Time) error {
	switch {
	case !c.Show:
		return httpx.FieldError(field, "invalid coupon")
	case c.LimitUse != nil && *c.LimitUse <= 0:
		return httpx.FieldError(field, "this coupon is no longer available")
	case now.Before(c.StartedAt):
		return httpx.FieldError(field, "this coupon has not started yet")
	case now.After(c.EndedAt):
		return httpx.FieldError(field, "this coupon has expired")
	case len(c.LimitPlanIDs) > 0 && !model.SliceContains(c.LimitPlanIDs, planID):
		return httpx.FieldError(field, "this coupon cannot be used for this plan")
	case len(c.LimitPeriods) > 0 && !model.SliceContains(c.LimitPeriods, period):
		return httpx.FieldError(field, "this coupon cannot be used for this period")
	}
	if c.LimitUseWithUser != nil && userID != 0 {
		var used int64
		err := tx.Model(&model.Order{}).
			Where("coupon_id = ? AND user_id = ? AND status NOT IN ?", c.ID, userID,
				[]model.OrderStatus{model.OrderStatusPending, model.OrderStatusCancelled}).
			Count(&used).Error
		if err != nil {
			return err
		}
		if used >= int64(*c.LimitUseWithUser) {
			return httpx.FieldError(field, fmt.Sprintf("this coupon can only be used %d time(s) per user", *c.LimitUseWithUser))
		}
	}
	return nil
}

// consumeCoupon takes one use of a limited coupon atomically.
func consumeCoupon(tx *gorm.DB, c *model.Coupon, field string) error {
	if c.LimitUse == nil {
		return nil
	}
	res := tx.Model(&model.Coupon{}).Where("id = ? AND limit_use > 0", c.ID).
		UpdateColumn("limit_use", gorm.Expr("limit_use - ?", 1))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return httpx.FieldError(field, "this coupon is no longer available")
	}
	return nil
}

// restoreCoupon gives back the use taken by a cancelled order (limited coupons only).
func restoreCoupon(tx *gorm.DB, couponID uint64) error {
	return tx.Model(&model.Coupon{}).Where("id = ? AND limit_use IS NOT NULL", couponID).
		UpdateColumn("limit_use", gorm.Expr("limit_use + ?", 1)).Error
}

// CheckCoupon validates code for userID buying planID/period (POST /user/coupons/check).
func (s *Service) CheckCoupon(ctx context.Context, userID uint64, code string, planID uint64, period string) (*model.Coupon, error) {
	db := s.a.DB.WithContext(ctx)
	var p model.Plan
	if err := db.First(&p, planID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.FieldError("plan_id", "plan not found")
		}
		return nil, err
	}
	if _, ok := p.Price(period); !ok {
		return nil, httpx.FieldError("period", "this period is not available for the plan")
	}
	c, err := findCoupon(db, code, "code")
	if err != nil {
		return nil, err
	}
	if err := validateCoupon(db, c, userID, planID, period, "code", s.now()); err != nil {
		return nil, err
	}
	return c, nil
}

// CouponUsage returns the number of non-cancelled orders per coupon id (admin used_count).
func CouponUsage(ctx context.Context, db *gorm.DB, ids []uint64) (map[uint64]int64, error) {
	out := map[uint64]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		CouponID uint64
		Cnt      int64
	}
	err := db.WithContext(ctx).Model(&model.Order{}).Select("coupon_id, COUNT(*) AS cnt").
		Where("coupon_id IN ? AND status <> ?", ids, model.OrderStatusCancelled).Group("coupon_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.CouponID] = r.Cnt
	}
	return out, nil
}
