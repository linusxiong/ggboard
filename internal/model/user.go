package model

import (
	"math"
	"time"
)

// User is a panel account. JSON tags describe the admin view; user-facing handlers should build their own DTOs.
type User struct {
	ID                uint64         `gorm:"primaryKey" json:"id"`
	Email             string         `gorm:"size:191;uniqueIndex;not null" json:"email"`
	PasswordHash      string         `gorm:"size:255;not null" json:"-"`
	UUID              string         `gorm:"column:uuid;size:36;uniqueIndex;not null" json:"uuid"`
	Token             string         `gorm:"size:32;uniqueIndex;not null" json:"token"`
	PlanID            *uint64        `gorm:"index" json:"plan_id"`
	Plan              *Plan          `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
	GroupID           *uint64        `gorm:"index" json:"group_id"`
	TransferEnable    int64          `gorm:"not null;default:0" json:"transfer_enable"`
	U                 int64          `gorm:"column:u;not null;default:0" json:"u"`
	D                 int64          `gorm:"column:d;not null;default:0" json:"d"`
	ExpiredAt         *time.Time     `gorm:"index" json:"expired_at"`
	NextResetAt       *time.Time     `gorm:"index" json:"next_reset_at"`
	SpeedLimit        *int           `json:"speed_limit"`
	DeviceLimit       *int           `json:"device_limit"`
	ConnLimit         *int           `json:"conn_limit"` // TCP connections per user (soga tcp_limit); null or 0 = unlimited
	Balance           int64          `gorm:"not null;default:0" json:"balance"`
	CommissionBalance int64          `gorm:"not null;default:0" json:"commission_balance"`
	CommissionType    CommissionType `gorm:"not null;default:0" json:"commission_type"`
	CommissionRate    *int           `json:"commission_rate"`
	Discount          *int           `json:"discount"`
	InviteUserID      *uint64        `gorm:"index" json:"invite_user_id"`
	IsAdmin           bool           `gorm:"not null;default:false" json:"is_admin"`
	IsStaff           bool           `gorm:"not null;default:false" json:"is_staff"`
	Banned            bool           `gorm:"not null;default:false" json:"banned"`
	RemindExpire      bool           `gorm:"not null;default:false" json:"remind_expire"`
	RemindTraffic     bool           `gorm:"not null;default:false" json:"remind_traffic"`
	LastLoginAt       *time.Time     `json:"last_login_at"`
	LastLoginIP       string         `gorm:"size:45" json:"last_login_ip"`
	TelegramID        *int64         `gorm:"index" json:"telegram_id"`         // bound Telegram account (chat id of the private chat)
	TelegramUsername  *string        `gorm:"size:64" json:"telegram_username"` // @username at binding time, without "@"
	Remarks           string         `gorm:"type:text" json:"remarks"`
	PasswordVersion   int            `gorm:"not null;default:0" json:"-"`
	CreatedAt         time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	// EmailVerifiedAt is when whoever controls the account first proved control of its mailbox (email code at
	// registration or password reset; admin-created and imported accounts count as verified). Null = never: the
	// account may have been registered with someone else's address, so the first proof is an ownership change
	// (internal/service/user ReclaimAccount).
	EmailVerifiedAt *time.Time `gorm:"column:email_verified_at" json:"email_verified_at"`
}

// Used returns total usage, saturating on overflow. Negative stored counters fail closed.
func (u *User) Used() int64 {
	if u.U < 0 || u.D < 0 || u.U > math.MaxInt64-u.D {
		return math.MaxInt64
	}
	return u.U + u.D
}

// Remaining returns transfer_enable - (u + d), never negative.
func (u *User) Remaining() int64 {
	used := u.Used()
	if u.TransferEnable > used {
		return u.TransferEnable - used
	}
	return 0
}

// IsExpired reports whether expired_at is set and not after now.
func (u *User) IsExpired(now time.Time) bool {
	return u.ExpiredAt != nil && !u.ExpiredAt.After(now)
}

// IsAvailable reports whether the user may use nodes: not banned, not expired (nil = never) and
// u + d < transfer_enable.
func (u *User) IsAvailable(now time.Time) bool {
	return !u.Banned && !u.IsExpired(now) && u.Used() < u.TransferEnable
}

// IsStaffOrAdmin reports is_admin || is_staff.
func (u *User) IsStaffOrAdmin() bool { return u.IsAdmin || u.IsStaff }
