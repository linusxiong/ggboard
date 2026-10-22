package model

import "time"

// UserMFA is the second-factor state of a user (one row per user, created on first use). The TOTP secret is
// AES-256-GCM encrypted (internal/service/mfa); backup codes are stored as SHA-256 hashes of the unused codes;
// WebAuthnHandle is the WebAuthn user handle (hex of 16 random bytes) shared by every passkey of the user.
type UserMFA struct {
	UserID         uint64            `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	TOTPSecret     string            `gorm:"column:totp_secret;type:text" json:"-"`
	TOTPEnabled    bool              `gorm:"column:totp_enabled;not null;default:false" json:"totp_enabled"`
	TOTPEnabledAt  *time.Time        `gorm:"column:totp_enabled_at" json:"totp_enabled_at"`
	TOTPLastStep   int64             `gorm:"column:totp_last_step;not null;default:0" json:"-"` // last accepted time step (replay guard)
	BackupCodes    JSONSlice[string] `gorm:"column:backup_codes" json:"-"`
	WebAuthnHandle *string           `gorm:"column:webauthn_handle;size:64;uniqueIndex" json:"-"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// TableName keeps the singular table name from docs/DATA_MODEL.md.
func (UserMFA) TableName() string { return "user_mfa" }

// Passkey is a discoverable WebAuthn credential registered by a user. Credential holds the JSON-encoded
// go-webauthn Credential record (public key, flags, attestation); SignCount, BackupState and LastUsedAt are
// updated after every successful assertion.
type Passkey struct {
	ID             uint64            `gorm:"primaryKey" json:"id"`
	UserID         uint64            `gorm:"index;not null" json:"user_id"`
	RPID           string            `gorm:"column:rp_id;size:255;not null" json:"rp_id"`
	CredentialHash string            `gorm:"size:64;uniqueIndex;not null" json:"-"` // hex SHA-256 of the raw credential id
	CredentialID   string            `gorm:"type:text;not null" json:"-"`           // base64url (unpadded) credential id
	Credential     string            `gorm:"type:text;not null" json:"-"`
	Name           string            `gorm:"size:64;not null" json:"name"`
	SignCount      int64             `gorm:"not null;default:0" json:"sign_count"`
	BackupEligible bool              `gorm:"not null;default:false" json:"backup_eligible"`
	BackupState    bool              `gorm:"not null;default:false" json:"backup_state"`
	Transports     JSONSlice[string] `json:"transports"`
	LastUsedAt     *time.Time        `json:"last_used_at"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}
