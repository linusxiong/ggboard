package model

import "time"

// Order is a purchase. Money columns are integer cents.
type Order struct {
	ID                uint64            `gorm:"primaryKey" json:"id"`
	TradeNo           string            `gorm:"size:32;uniqueIndex;not null" json:"trade_no"`
	UserID            uint64            `gorm:"index;not null" json:"user_id"`
	User              *User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	PlanID            uint64            `gorm:"index;not null" json:"plan_id"`
	Plan              *Plan             `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
	InviteUserID      *uint64           `gorm:"index" json:"invite_user_id"`
	CouponID          *uint64           `json:"coupon_id"`
	PaymentID         *uint64           `json:"payment_id"`
	Payment           *Payment          `gorm:"foreignKey:PaymentID" json:"payment,omitempty"`
	Type              OrderType         `gorm:"not null" json:"type"`
	Period            string            `gorm:"size:16;not null" json:"period"`
	CallbackNo        string            `gorm:"size:191" json:"callback_no"`
	TotalAmount       int64             `gorm:"not null;default:0" json:"total_amount"`
	HandlingAmount    int64             `gorm:"not null;default:0" json:"handling_amount"`
	DiscountAmount    int64             `gorm:"not null;default:0" json:"discount_amount"`
	SurplusAmount     int64             `gorm:"not null;default:0" json:"surplus_amount"`
	RefundAmount      int64             `gorm:"not null;default:0" json:"refund_amount"`
	BalanceAmount     int64             `gorm:"not null;default:0" json:"balance_amount"`
	SurplusOrderIDs   JSONSlice[uint64] `json:"surplus_order_ids"`
	Status            OrderStatus       `gorm:"index;not null;default:0" json:"status"`
	CommissionStatus  CommissionStatus  `gorm:"index;not null;default:0" json:"commission_status"`
	CommissionBalance int64             `gorm:"not null;default:0" json:"commission_balance"`
	PaidAt            *time.Time        `json:"paid_at"`
	CreatedAt         time.Time         `gorm:"index" json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// Payment is a configured payment method (gateway instance).
type Payment struct {
	ID                 uint64             `gorm:"primaryKey" json:"id"`
	UUID               string             `gorm:"column:uuid;size:32;uniqueIndex;not null" json:"uuid"`
	Gateway            string             `gorm:"size:32;not null" json:"gateway"`
	Name               string             `gorm:"size:191;not null" json:"name"`
	Icon               string             `gorm:"size:255" json:"icon"`
	Config             JSONObject[string] `json:"config"` // secret: admin only
	NotifyDomain       string             `gorm:"size:255" json:"notify_domain"`
	HandlingFeeFixed   int64              `gorm:"not null;default:0" json:"handling_fee_fixed"`
	HandlingFeePercent float64            `gorm:"not null;default:0" json:"handling_fee_percent"`
	Enabled            bool               `gorm:"not null;default:false" json:"enabled"`
	Sort               int                `gorm:"not null;default:0" json:"sort"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

// Coupon is a discount code. Usage is counted from orders with status != cancelled.
type Coupon struct {
	ID               uint64            `gorm:"primaryKey" json:"id"`
	Code             string            `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name             string            `gorm:"size:191;not null" json:"name"`
	Type             CouponType        `gorm:"not null" json:"type"`
	Value            int64             `gorm:"not null" json:"value"`
	Show             bool              `gorm:"not null;default:false" json:"show"`
	LimitUse         *int              `json:"limit_use"`
	LimitUseWithUser *int              `json:"limit_use_with_user"`
	LimitPlanIDs     JSONSlice[uint64] `json:"limit_plan_ids"`
	LimitPeriods     JSONSlice[string] `json:"limit_periods"`
	StartedAt        time.Time         `gorm:"not null" json:"started_at"`
	EndedAt          time.Time         `gorm:"not null" json:"ended_at"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// InviteCode is a user's invitation code.
type InviteCode struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"index;not null" json:"user_id"`
	Code      string    `gorm:"size:32;uniqueIndex;not null" json:"code"`
	Status    bool      `gorm:"not null;default:false" json:"status"` // consumed (only when invite_never_expire=false)
	PV        int       `gorm:"column:pv;not null;default:0" json:"pv"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CommissionLog records a commission paid to an inviter.
type CommissionLog struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	InviteUserID uint64    `gorm:"index;not null" json:"invite_user_id"`
	UserID       uint64    `gorm:"not null" json:"user_id"`
	TradeNo      string    `gorm:"size:32;not null" json:"trade_no"`
	OrderAmount  int64     `gorm:"not null" json:"order_amount"`
	GetAmount    int64     `gorm:"not null" json:"get_amount"`
	CreatedAt    time.Time `json:"created_at"`
}
