package model

import "time"

// SubscriptionKey is a user's age key pair for encrypted subscriptions (internal/subscription/guard). PublicKey
// is the age X25519 recipient ("age1…"); PrivateKeyEnc is the AGE-SECRET-KEY-1… identity sealed with
// AES-256-GCM under a key derived from jwt.secret (so the owner can display it again). Enabled switches the
// encryption of supported formats on.
type SubscriptionKey struct {
	UserID        uint64    `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	PublicKey     string    `gorm:"size:128;not null" json:"public_key"`
	PrivateKeyEnc string    `gorm:"type:text;not null" json:"-"`
	Enabled       bool      `gorm:"not null;default:false" json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
}

// SubscriptionAccess is one successful subscription fetch (access log and leak detection). IPBucket is the
// client IP for IPv4 and its /64 prefix for IPv6; TokenTag is the first 8 hex characters of SHA-256 of the
// token that was used, so rotating the token starts a new leak-detection window. Rows are written in batches
// and pruned daily to the newest 100 per user and 30 days.
type SubscriptionAccess struct {
	ID       uint64    `gorm:"primaryKey" json:"id"`
	UserID   uint64    `gorm:"not null;index:idx_subscription_access_user_at,priority:1" json:"user_id"`
	IP       string    `gorm:"column:ip;size:45;not null" json:"ip"`
	IPBucket string    `gorm:"column:ip_bucket;size:64;not null" json:"ip_bucket"`
	TokenTag string    `gorm:"size:16;not null" json:"token_tag"`
	Client   string    `gorm:"size:64;not null" json:"client"`
	Format   string    `gorm:"size:16;not null" json:"format"`
	At       time.Time `gorm:"not null;index;index:idx_subscription_access_user_at,priority:2" json:"at"`
}

// TableName keeps the singular table name from docs/DATA_MODEL.md.
func (SubscriptionAccess) TableName() string { return "subscription_access" }
