// Package ordersvc implements the order money flow (docs/PAYMENTS.md): pricing, order creation (user and
// admin-assigned), checkout through a payment gateway, verified payment notifications, fulfilment,
// cancellation, coupons, invite commissions and the order_timeout / commission_settle jobs.
//
// All money is integer cents. Every balance, coupon or plan change runs inside a.DB.Transaction with the
// user's row locked first (lockUser) so concurrent requests serialize on SQLite, PostgreSQL and MySQL.
package ordersvc

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
)

// Job names.
const (
	JobOrderTimeout     = "order_timeout"
	JobCommissionSettle = "commission_settle"
)

// Callback numbers recorded for orders that were not paid through a gateway.
const (
	CallbackFree    = "free"    // nothing to pay
	CallbackBalance = "balance" // fully paid from the account balance
	CallbackManual  = "manual"  // confirmed by an administrator
)

// CommissionHold is how long a paid order's commission waits before commission_auto_check_enable approves it
// (Xboard's refund window).
const CommissionHold = 72 * time.Hour

// Paths of the guest payment endpoints.
const (
	NotifyPath = httpx.APIPrefix + "/guest/payment/notify/"
	ReturnPath = httpx.APIPrefix + "/guest/payment/return/"
)

// batchLimit bounds the rows a job handles per run.
const batchLimit = 500

func init() {
	app.RegisterJob(JobOrderTimeout, "*/1 * * * *", func(ctx context.Context, a *app.App) error {
		_, err := New(a).CancelExpired(ctx)
		return err
	})
	app.RegisterJob(JobCommissionSettle, "0 */1 * * *", func(ctx context.Context, a *app.App) error {
		_, err := New(a).SettleCommissions(ctx)
		return err
	})
}

// Service is cheap to construct; New(a) per request is fine.
type Service struct {
	a *app.App
	// Now is the clock (tests override it).
	Now func() time.Time
}

// New returns a Service bound to a.
func New(a *app.App) *Service { return &Service{a: a, Now: time.Now} }

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// ErrOrderCancelled is returned by Paid for an order that was already cancelled.
var ErrOrderCancelled = errors.New("ordersvc: order is cancelled")

// Pending order statuses: a user may hold at most one such order.
var openStatuses = []model.OrderStatus{model.OrderStatusPending, model.OrderStatusProcessing}

// lockUser takes the user's row lock with a portable no-op UPDATE (SQLite has no SELECT … FOR UPDATE and
// upgrades to a write lock here) and reloads the row inside tx. The reload is a locking read: under MySQL's
// REPEATABLE READ a plain SELECT returns the snapshot taken at the transaction's first read (Paid reads the
// order first), i.e. the user as it was before the lock, and fulfilment would overwrite a change committed in
// between. The SQLite dialect drops the FOR UPDATE clause.
func lockUser(tx *gorm.DB, id uint64) (*model.User, error) {
	if err := tx.Model(&model.User{}).Where("id = ?", id).UpdateColumn("balance", gorm.Expr("balance")).Error; err != nil {
		return nil, err
	}
	var u model.User
	if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.NotFound("user not found")
		}
		return nil, err
	}
	return &u, nil
}

// LockActor locks and rechecks the authenticated account before order reads or writes.
func LockActor(tx *gorm.DB, id uint64, claims *auth.Claims) (*model.User, error) {
	u, err := lockUser(tx, id)
	if err != nil {
		return nil, err
	}
	if claims != nil && (!claims.BoundTo(u) || u.Banned) {
		return nil, httpx.Unauthorized("session expired, please sign in again")
	}
	return u, nil
}

func lockAdminActor(tx *gorm.DB, claims *auth.Claims) error {
	if claims == nil {
		return httpx.Unauthorized("session expired, please sign in again")
	}
	id, err := claims.UserID()
	if err != nil {
		return httpx.Unauthorized("session expired, please sign in again")
	}
	u, err := LockActor(tx, id, claims)
	if err != nil {
		return err
	}
	if !u.IsAdmin {
		return httpx.Unauthorized("administrator session expired, please sign in again")
	}
	return nil
}

// pendingOrder returns the user's open order, if any.
func pendingOrder(tx *gorm.DB, userID uint64) (*model.Order, error) {
	var o model.Order
	err := tx.Where("user_id = ? AND status IN ?", userID, openStatuses).Order("id").Limit(1).Find(&o).Error
	if err != nil {
		return nil, err
	}
	if o.ID == 0 {
		return nil, nil
	}
	return &o, nil
}

func pendingConflict(o *model.Order) error {
	return httpx.Conflict("you have an unpaid order; pay or cancel it first").WithField("trade_no", o.TradeNo)
}

// NewTradeNo returns YYYYMMDDHHMMSS (local time) + 6 random digits.
func NewTradeNo(now time.Time) string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic("ordersvc: crypto/rand failed: " + err.Error())
	}
	return now.Format("20060102150405") + fmt.Sprintf("%06d", n.Int64())
}

func uniqueTradeNo(tx *gorm.DB, now time.Time) (string, error) {
	for range 5 {
		tn := NewTradeNo(now)
		var n int64
		if err := tx.Model(&model.Order{}).Where("trade_no = ?", tn).Count(&n).Error; err != nil {
			return "", err
		}
		if n == 0 {
			return tn, nil
		}
	}
	return "", errors.New("ordersvc: could not allocate a unique trade_no")
}

// percentOf returns round-half-up(amount × pct / 100) for non-negative inputs.
func percentOf(amount, pct int64) int64 {
	if amount <= 0 || pct <= 0 {
		return 0
	}
	pct = min(pct, 100)
	return amount/100*pct + (amount%100*pct+50)/100
}

func chargeAmount(total, handling int64) (int64, bool) {
	if total <= 0 || handling < 0 || handling > math.MaxInt64-total {
		return 0, false
	}
	return total + handling, true
}

func saturatingAmount(a, b int64) int64 {
	a, b = max(a, 0), max(b, 0)
	if b > math.MaxInt64-a {
		return math.MaxInt64
	}
	return a + b
}

func creditUser(tx *gorm.DB, id uint64, column string, amount int64) error {
	if amount <= 0 {
		return nil
	}
	res := tx.Model(&model.User{}).Where("id = ? AND "+column+" >= 0 AND "+column+" <= ?", id, math.MaxInt64-amount).
		UpdateColumn(column, gorm.Expr(column+" + ?", amount))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("ordersvc: %s credit would overflow for user %d", column, id)
	}
	return nil
}

// SiteURL returns settings.app_url without a trailing slash, or fallback (scheme://host of the request).
func (s *Service) SiteURL(fallback string) string {
	if u := strings.TrimRight(strings.TrimSpace(s.a.Settings.String("app_url")), "/"); u != "" {
		return u
	}
	return strings.TrimRight(fallback, "/")
}

// NotifyURL returns <base>/api/v1/guest/payment/notify/<gateway>/<uuid>.
func NotifyURL(base string, p *model.Payment) string {
	return strings.TrimRight(base, "/") + NotifyPath + p.Gateway + "/" + p.UUID
}

// PaymentNotifyURL resolves the notify base (payments.notify_domain, else app_url, else fallback).
func (s *Service) PaymentNotifyURL(p *model.Payment, fallback string) string {
	base := strings.TrimRight(strings.TrimSpace(p.NotifyDomain), "/")
	if base == "" {
		base = s.SiteURL(fallback)
	}
	return NotifyURL(base, p)
}

// OrderPageURL is the SPA page of an order.
func (s *Service) OrderPageURL(tradeNo, fallback string) string {
	return s.SiteURL(fallback) + "/orders/" + tradeNo
}

// alertAdmin mails the first administrator (lowest id) using the notify template.
func (s *Service) alertAdmin(subject, content string) {
	var admin model.User
	if err := s.a.DB.Where("is_admin = ?", true).Order("id").Limit(1).Find(&admin).Error; err != nil || admin.ID == 0 {
		s.a.Log.Error("payment alert: no administrator to notify", "subject", subject, "err", err)
		return
	}
	if err := s.a.Mailer.SendTemplate(admin.Email, mail.TplNotify, mail.Data{"Subject": subject, "Content": content}); err != nil {
		s.a.Log.Error("payment alert: enqueue failed", "subject", subject, "err", err)
	}
}
