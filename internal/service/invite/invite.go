// Package invite implements the user side of the referral programme: invite codes, referral statistics,
// commission history, moving commission to the account balance and withdrawal requests (as tickets).
package invite

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/payment"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
	"github.com/linusxiong/ggboard/internal/settings"
)

// CodeLength is the length of generated invite codes (alphanumeric).
const CodeLength = 32

// Errors.
var (
	ErrCodeLimit              = errors.New("invite: invite code limit reached")
	ErrInsufficientCommission = errors.New("invite: insufficient commission balance")
	ErrWithdrawClosed         = errors.New("invite: commission withdrawal is disabled")
	ErrWithdrawMethod         = errors.New("invite: unsupported withdrawal method")
	ErrWithdrawLimit          = errors.New("invite: commission balance is below the withdrawal minimum")
	ErrUserNotFound           = errors.New("invite: user not found")
)

// Service is cheap to construct: invite.New(a) per request is fine.
type Service struct{ a *app.App }

// New returns a Service bound to a.
func New(a *app.App) *Service { return &Service{a: a} }

// Code is an invite code as shown to its owner.
type Code struct {
	Code      string    `json:"code"`
	PV        int       `json:"pv"`
	CreatedAt time.Time `json:"created_at"`
}

// Stat summarizes a user's referrals. Amounts are cents.
type Stat struct {
	RegisterCount     int64 `json:"register_count"`     // users with invite_user_id = me
	CommissionPending int64 `json:"commission_pending"` // completed orders whose commission is not settled yet
	CommissionPaid    int64 `json:"commission_paid"`    // sum of commission_logs.get_amount
	CommissionRate    int   `json:"commission_rate"`    // percent: users.commission_rate ?? invite_commission
	InviteGenLimit    int   `json:"invite_gen_limit"`   // max unused codes
}

// Overview is GET /user/invite.
type Overview struct {
	Codes []Code `json:"codes"`
	Stat  Stat   `json:"stat"`
}

// Overview returns the user's usable (not consumed) codes, newest first, and the referral statistics.
func (s *Service) Overview(ctx context.Context, userID uint64) (*Overview, error) {
	var rows []model.InviteCode
	if err := s.a.DB.WithContext(ctx).Where(map[string]any{"user_id": userID, "status": false}).
		Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	codes := make([]Code, 0, len(rows))
	for _, c := range rows {
		codes = append(codes, Code{Code: c.Code, PV: c.PV, CreatedAt: c.CreatedAt})
	}
	st, err := s.Stat(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &Overview{Codes: codes, Stat: st}, nil
}

// Stat computes the referral statistics of userID (read fresh: the commission rate may have just changed).
func (s *Service) Stat(ctx context.Context, userID uint64) (Stat, error) {
	db := s.a.DB.WithContext(ctx)
	st := Stat{
		CommissionRate: s.a.Settings.Int("invite_commission"),
		InviteGenLimit: s.a.Settings.Int("invite_gen_limit"),
	}
	var u model.User
	if err := db.Select("id", "commission_rate").Where("id = ?", userID).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Stat{}, ErrUserNotFound
		}
		return Stat{}, err
	}
	if u.CommissionRate != nil {
		st.CommissionRate = *u.CommissionRate
	}
	if err := db.Model(&model.User{}).Where("invite_user_id = ?", u.ID).Count(&st.RegisterCount).Error; err != nil {
		return Stat{}, err
	}
	var paid struct{ Total int64 }
	if err := db.Model(&model.CommissionLog{}).Select("COALESCE(SUM(get_amount), 0) AS total").
		Where("invite_user_id = ?", u.ID).Scan(&paid).Error; err != nil {
		return Stat{}, err
	}
	st.CommissionPaid = paid.Total
	pending, err := s.CommissionPending(ctx, u.ID)
	if err != nil {
		return Stat{}, err
	}
	st.CommissionPending = pending
	return st, nil
}

// CommissionPending sums orders.commission_balance of completed orders referred by userID whose commission is
// pending or processing. With commission_distribution_enable only the first-level share
// (commission_distribution_l1 %) belongs to the direct inviter, as in Xboard.
func (s *Service) CommissionPending(ctx context.Context, userID uint64) (int64, error) {
	var sum struct{ Total int64 }
	err := s.a.DB.WithContext(ctx).Model(&model.Order{}).Select("COALESCE(SUM(commission_balance), 0) AS total").
		Where("invite_user_id = ? AND status = ?", userID, model.OrderStatusCompleted).
		Where("commission_status IN ?", []model.CommissionStatus{model.CommissionStatusPending, model.CommissionStatusProcessing}).
		Scan(&sum).Error
	if err != nil {
		return 0, err
	}
	if s.a.Settings.Bool("commission_distribution_enable") {
		return sum.Total * int64(s.a.Settings.Int("commission_distribution_l1")) / 100, nil
	}
	return sum.Total, nil
}

// GenerateCode creates a random invite code for userID. It returns ErrCodeLimit when the user already holds
// invite_gen_limit unused codes.
func (s *Service) GenerateCode(ctx context.Context, userID uint64) (string, error) {
	limit := int64(s.a.Settings.Int("invite_gen_limit"))
	var code string
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUser(tx, userID); err != nil {
			return err
		}
		var unused int64
		if err := tx.Model(&model.InviteCode{}).Where(map[string]any{"user_id": userID, "status": false}).
			Count(&unused).Error; err != nil {
			return err
		}
		if unused >= limit {
			return ErrCodeLimit
		}
		for attempt := 0; ; attempt++ {
			c := model.InviteCode{UserID: userID, Code: settings.RandomToken(CodeLength)}
			err := tx.Create(&c).Error
			if errors.Is(err, gorm.ErrDuplicatedKey) && attempt < 3 {
				continue
			}
			if err != nil {
				return err
			}
			code = c.Code
			return nil
		}
	})
	if err != nil {
		return "", err
	}
	return code, nil
}

// Commission is one commission_logs row as shown to the inviter.
type Commission struct {
	TradeNo     string    `json:"trade_no"`
	OrderAmount int64     `json:"order_amount"`
	GetAmount   int64     `json:"get_amount"`
	CreatedAt   time.Time `json:"created_at"`
}

// CommissionQuery returns the commissions earned by userID (get_amount > 0) for httpx.Paginate; order newest
// first ("created_at DESC, id DESC").
func (s *Service) CommissionQuery(ctx context.Context, userID uint64) *gorm.DB {
	return s.a.DB.WithContext(ctx).Model(&model.CommissionLog{}).
		Select("trade_no", "order_amount", "get_amount", "created_at").
		Where("invite_user_id = ? AND get_amount > ?", userID, 0)
}

// Transfer moves amount cents from commission_balance to balance atomically and returns the reloaded user.
func (s *Service) Transfer(ctx context.Context, userID uint64, amount int64) (*model.User, error) {
	if amount <= 0 {
		return nil, ErrInsufficientCommission
	}
	var u model.User
	err := s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.User{}).
			Where("id = ? AND commission_balance >= ?", userID, amount).
			Updates(map[string]any{
				"commission_balance": gorm.Expr("commission_balance - ?", amount),
				"balance":            gorm.Expr("balance + ?", amount),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrInsufficientCommission
		}
		return tx.Where("id = ?", userID).First(&u).Error
	})
	if err != nil {
		return nil, err
	}
	auth.InvalidateUser(s.a, userID)
	s.a.Log.Info("invite: commission transferred to balance", "user_id", userID, "amount", amount)
	return &u, nil
}

// Withdraw files a withdrawal request: a high-level ticket stating method, account and the current commission
// balance. Nothing is deducted (staff settle it manually, as in Xboard). Errors: ErrWithdrawClosed,
// ErrWithdrawMethod, ErrWithdrawLimit, ticket.ErrOpenExists.
func (s *Service) Withdraw(ctx context.Context, userID uint64, method, account string) (*model.Ticket, error) {
	st := s.a.Settings
	if st.Bool("withdraw_close_enable") {
		return nil, ErrWithdrawClosed
	}
	if !slices.Contains(st.Strings("commission_withdraw_method"), method) {
		return nil, ErrWithdrawMethod
	}
	var u model.User
	if err := s.a.DB.WithContext(ctx).Where("id = ?", userID).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if u.CommissionBalance <= 0 || u.CommissionBalance < st.Int64("commission_withdraw_limit")*100 {
		return nil, ErrWithdrawLimit
	}
	tr := withdrawText(st.String("default_locale"))
	amount := payment.FormatAmount(u.CommissionBalance) + " " + st.String("currency")
	message := fmt.Sprintf("%s%s\n%s%s\n%s%s", tr.method, method, tr.account, account, tr.amount, amount)
	t, err := ticketsvc.New(s.a).Create(ctx, u.ID, tr.subject, model.TicketLevelHigh, message)
	if err != nil {
		return nil, err
	}
	s.a.Log.Info("invite: withdrawal requested", "user_id", u.ID, "ticket_id", t.ID, "amount", u.CommissionBalance)
	return t, nil
}

// WithdrawLimitText formats the withdrawal minimum (setting commission_withdraw_limit, currency units).
func (s *Service) WithdrawLimitText() string {
	return strconv.Itoa(s.a.Settings.Int("commission_withdraw_limit")) + " " + s.a.Settings.String("currency")
}

type withdrawStrings struct{ subject, method, account, amount string }

func withdrawText(locale string) withdrawStrings {
	if len(locale) >= 2 && locale[:2] == "zh" {
		return withdrawStrings{
			subject: "[提现申请] 本工单由系统发出",
			method:  "提现方式：",
			account: "提现账号：",
			amount:  "提现金额：",
		}
	}
	return withdrawStrings{
		subject: "[Withdrawal Request] This ticket was opened by the system",
		method:  "Withdrawal method: ",
		account: "Withdrawal account: ",
		amount:  "Withdrawal amount: ",
	}
}

// lockUser takes a row lock on the user (PostgreSQL/MySQL; SQLite transactions are IMMEDIATE) so per-user
// check-then-insert sequences cannot interleave.
func lockUser(tx *gorm.DB, userID uint64) error {
	var u model.User
	err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Select("id").Where("id = ?", userID).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrUserNotFound
	}
	return err
}
