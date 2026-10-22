package db_test

import (
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Review 2026-10-22: indexes for the dashboard/stats and admin lookups that scanned whole tables.
func TestReviewHotQueryIndexes(t *testing.T) {
	a := testutil.NewTestApp(t)
	m := a.DB.Migrator()
	for _, c := range []struct {
		model any
		field string
	}{
		{&model.Order{}, "PaidAt"},   // stats: paid orders by paid_at range
		{&model.Order{}, "CouponID"}, // coupon used_count
		{&model.User{}, "CreatedAt"}, // stats: registers by day
		{&model.CommissionLog{}, "TradeNo"},
		{&model.CommissionLog{}, "CreatedAt"},
	} {
		if !m.HasIndex(c.model, c.field) {
			t.Errorf("%T.%s has no index", c.model, c.field)
		}
	}
}
