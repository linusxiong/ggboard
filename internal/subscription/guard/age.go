// Package guard implements subscription leak protection (docs/SUBSCRIPTION.md): per-user age key pairs and
// age-encrypted subscription bodies, the subscription access log with its batched writer and daily prune job,
// and leak detection (too many distinct IPs per token in 24 hours → subscription.LeakHook, optionally a token
// rotation).
package guard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/settings"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// Errors of the key store.
var (
	ErrNoKey     = errors.New("guard: no age key")
	ErrKeyExists = errors.New("guard: an age key already exists (regenerate to replace it)")
)

// ContentTypeAge is the content type of encrypted bodies: the age armor is ASCII text (PEM-like).
const ContentTypeAge = subscription.ContentTypePlain

// EncryptionHeader marks encrypted responses ("X-Subscription-Encryption: age").
const EncryptionHeader = "X-Subscription-Encryption"

// sealInfo is the HKDF label of the key that seals stored age identities (mfasvc.SealString).
const sealInfo = "ggboard/subscription/age-identity/v1"

const (
	keyTTL = time.Minute
)

func sealAAD(userID uint64) []byte { return []byte("ggboard:age:" + strconv.FormatUint(userID, 10)) }

// LoadKey returns the user's age key (nil when none). Cached for a minute; every write invalidates it.
func LoadKey(ctx context.Context, a *app.App, userID uint64) (*model.SubscriptionKey, error) {
	ns := usersvc.SubscriptionKeyCacheNamespace(userID)
	lctx := context.WithoutCancel(ctx)
	for {
		key := a.Cache.Key(ns, "")
		k, err := cache.GetOrLoad(a.Cache, key, keyTTL, func() (model.SubscriptionKey, error) {
			var rows []model.SubscriptionKey
			if err := a.DB.WithContext(lctx).Where("user_id = ?", userID).Limit(1).Find(&rows).Error; err != nil {
				return model.SubscriptionKey{}, err
			}
			if len(rows) == 0 {
				return model.SubscriptionKey{}, nil
			}
			return rows[0], nil
		})
		if key != a.Cache.Key(ns, "") {
			continue
		}
		if err != nil || k.UserID == 0 {
			return nil, err
		}
		return &k, nil
	}
}

// LoadKeyForUser reads the private identity while the authenticated account snapshot still owns it.
func LoadKeyForUser(ctx context.Context, a *app.App, u *model.User) (*model.SubscriptionKey, error) {
	if u == nil {
		return nil, httpx.Unauthorized("session expired, please sign in again")
	}
	var row model.SubscriptionKey
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUser(tx, u); err != nil {
			return err
		}
		return tx.Where("user_id = ?", u.ID).First(&row).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func invalidateKey(a *app.App, userID uint64) {
	a.Cache.InvalidateNamespace(usersvc.SubscriptionKeyCacheNamespace(userID))
}

func lockUser(tx *gorm.DB, u *model.User) error {
	if err := tx.Model(&model.User{}).Where("id = ?", u.ID).
		UpdateColumn("password_version", gorm.Expr("password_version")).Error; err != nil {
		return err
	}
	var fresh model.User
	if err := tx.First(&fresh, u.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.Unauthorized("session expired, please sign in again")
		}
		return err
	}
	if u.UUID == "" || fresh.UUID != u.UUID || fresh.PasswordVersion != u.PasswordVersion ||
		usersvc.NormalizeEmail(fresh.Email) != usersvc.NormalizeEmail(u.Email) {
		return httpx.Unauthorized("session expired, please sign in again")
	}
	if fresh.Banned {
		return httpx.Forbidden("account is banned")
	}
	return nil
}

// PrivateKey decrypts the stored identity (AGE-SECRET-KEY-1…). It fails after jwt.secret was rotated; the
// public key keeps working, the user just has to regenerate to see a private key again.
func PrivateKey(a *app.App, k *model.SubscriptionKey) (string, error) {
	return mfasvc.OpenString(a.Cfg.JWT.Secret, sealInfo, sealAAD(k.UserID), k.PrivateKeyEnc)
}

// GenerateKey creates the user's key pair (X25519). An existing key is replaced only with regenerate
// (ErrKeyExists otherwise); enabled nil keeps the current switch (false for a new key). It returns the row
// and the plaintext identity.
func GenerateKey(ctx context.Context, a *app.App, userID uint64, regenerate bool, enabled *bool) (*model.SubscriptionKey, string, error) {
	return generateKey(ctx, a, userID, nil, regenerate, enabled, nil)
}

// GenerateKeyIf checks authorize in the key write transaction before replacing a key.
func GenerateKeyIf(ctx context.Context, a *app.App, userID uint64, regenerate bool, enabled *bool, authorize func(*gorm.DB) error) (*model.SubscriptionKey, string, error) {
	return generateKey(ctx, a, userID, nil, regenerate, enabled, authorize)
}

// GenerateKeyForUser is GenerateKey with an account snapshot checked in the key-write transaction.
func GenerateKeyForUser(ctx context.Context, a *app.App, u *model.User, regenerate bool, enabled *bool) (*model.SubscriptionKey, string, error) {
	if u == nil {
		return nil, "", httpx.Unauthorized("session expired, please sign in again")
	}
	return generateKey(ctx, a, u.ID, u, regenerate, enabled, nil)
}

func generateKey(ctx context.Context, a *app.App, userID uint64, u *model.User, regenerate bool, enabled *bool, authorize func(*gorm.DB) error) (*model.SubscriptionKey, string, error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, "", fmt.Errorf("guard: generate age key: %w", err)
	}
	priv := id.String()
	sealed, err := mfasvc.SealString(a.Cfg.JWT.Secret, sealInfo, sealAAD(userID), priv)
	if err != nil {
		return nil, "", err
	}
	row := model.SubscriptionKey{UserID: userID, PublicKey: id.Recipient().String(), PrivateKeyEnc: sealed, CreatedAt: time.Now()}
	err = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if authorize != nil {
			if err := authorize(tx); err != nil {
				return err
			}
		}
		if u != nil {
			if err := lockUser(tx, u); err != nil {
				return err
			}
		}
		var cur []model.SubscriptionKey
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Where("user_id = ?", userID).
			Limit(1).Find(&cur).Error; err != nil {
			return err
		}
		switch {
		case len(cur) == 0:
			row.Enabled = enabled != nil && *enabled
			return tx.Create(&row).Error
		case !regenerate:
			return ErrKeyExists
		}
		row.Enabled = cur[0].Enabled
		if enabled != nil {
			row.Enabled = *enabled
		}
		return tx.Model(&model.SubscriptionKey{}).Where("user_id = ?", userID).Updates(map[string]any{
			"public_key": row.PublicKey, "private_key_enc": row.PrivateKeyEnc, "enabled": row.Enabled, "created_at": row.CreatedAt,
		}).Error
	})
	invalidateKey(a, userID)
	if err != nil {
		return nil, "", err
	}
	return &row, priv, nil
}

// SetKeyEnabled switches encryption on or off (ErrNoKey without a key).
func SetKeyEnabled(ctx context.Context, a *app.App, userID uint64, enabled bool) (*model.SubscriptionKey, error) {
	return setKeyEnabled(ctx, a, userID, nil, enabled)
}

// SetKeyEnabledForUser is SetKeyEnabled with an account snapshot checked in the key-write transaction.
func SetKeyEnabledForUser(ctx context.Context, a *app.App, u *model.User, enabled bool) (*model.SubscriptionKey, error) {
	if u == nil {
		return nil, httpx.Unauthorized("session expired, please sign in again")
	}
	return setKeyEnabled(ctx, a, u.ID, u, enabled)
}

func setKeyEnabled(ctx context.Context, a *app.App, userID uint64, u *model.User, enabled bool) (*model.SubscriptionKey, error) {
	var row model.SubscriptionKey
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if u != nil {
			if err := lockUser(tx, u); err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ?", userID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNoKey
			}
			return err
		}
		res := tx.Model(&model.SubscriptionKey{}).Where("user_id = ?", userID).Update("enabled", enabled)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			if err := tx.Where("user_id = ?", userID).First(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrNoKey
				}
				return err
			}
		}
		row.Enabled = enabled
		return nil
	})
	invalidateKey(a, userID)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// DeleteKey removes the user's key pair (encryption off). Deleting a missing key is not an error.
func DeleteKey(ctx context.Context, a *app.App, userID uint64) error {
	return deleteKey(ctx, a, userID, nil)
}

// DeleteKeyForUser is DeleteKey with an account snapshot checked in the key-write transaction.
func DeleteKeyForUser(ctx context.Context, a *app.App, u *model.User) error {
	if u == nil {
		return httpx.Unauthorized("session expired, please sign in again")
	}
	return deleteKey(ctx, a, u.ID, u)
}

func deleteKey(ctx context.Context, a *app.App, userID uint64, u *model.User) error {
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if u != nil {
			if err := lockUser(tx, u); err != nil {
				return err
			}
		}
		return tx.Where("user_id = ?", userID).Delete(&model.SubscriptionKey{}).Error
	})
	invalidateKey(a, userID)
	return err
}

// Encrypt returns body encrypted to the age recipient publicKey in the armored age-encryption.org/v1 format
// ("-----BEGIN AGE ENCRYPTED FILE-----" first, as mihomo's age.DecryptBytes requires).
func Encrypt(body []byte, publicKey string) ([]byte, error) {
	rs, err := age.ParseRecipients(strings.NewReader(publicKey))
	if err != nil || len(rs) == 0 {
		return nil, fmt.Errorf("guard: invalid age recipient: %w", err)
	}
	var buf bytes.Buffer
	buf.Grow(len(body)*4/3 + 512)
	aw := armor.NewWriter(&buf)
	w, err := age.Encrypt(aw, rs...)
	if err != nil {
		return nil, fmt.Errorf("guard: age encrypt: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return nil, fmt.Errorf("guard: age encrypt: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("guard: age encrypt: %w", err)
	}
	if err := aw.Close(); err != nil {
		return nil, fmt.Errorf("guard: age armor: %w", err)
	}
	return buf.Bytes(), nil
}

// KeyTag is a short fingerprint of the public key, appended to ETags of encrypted bodies so a new key never
// matches a cached ciphertext.
func KeyTag(publicKey string) string {
	sum := sha256.Sum256([]byte(publicKey))
	return hex.EncodeToString(sum[:4])
}

// Decision is how a subscription response is served.
type Decision int

// Decisions.
const (
	ServePlain   Decision = iota // no key, key disabled, ?age=0 allowed, or an unsupported format (not strict)
	ServeAge                     // encrypt to the user's public key
	RejectFormat                 // unsupported format with subscribe_age_strict: 406
)

// Decide applies docs/SUBSCRIPTION.md "Encrypted subscriptions (age)": with an enabled key, formats that
// support age are encrypted; ?age=0 (any strconv.ParseBool false value) forces plaintext only while
// subscribe_age_optional is on; other formats get plaintext unless subscribe_age_strict refuses them.
func Decide(s *settings.Service, k *model.SubscriptionKey, format, ageParam string) Decision {
	if k == nil || !k.Enabled {
		return ServePlain
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(ageParam)); err == nil && !v && s.Bool("subscribe_age_optional") {
		return ServePlain
	}
	if !subscription.SupportsAge(format) {
		if s.Bool("subscribe_age_strict") {
			return RejectFormat
		}
		return ServePlain
	}
	return ServeAge
}

// KeyView is the age key as shown to its owner (GET/POST/PATCH /user/subscription/age) and, without the
// private key, to admins.
type KeyView struct {
	Enabled    bool       `json:"enabled"`
	PublicKey  *string    `json:"public_key"`
	PrivateKey *string    `json:"private_key"` // owner only (null for admins, and when it cannot be decrypted any more)
	CreatedAt  *time.Time `json:"created_at"`
	Formats    []string   `json:"formats"`  // formats served encrypted
	Optional   bool       `json:"optional"` // ?age=0 is honoured (subscribe_age_optional)
	Strict     bool       `json:"strict"`   // other formats are refused (subscribe_age_strict)
}

// View builds the view of k (nil = no key). withPrivate adds the decrypted identity (null when it cannot be
// decrypted; the failure is logged).
func View(a *app.App, k *model.SubscriptionKey, withPrivate bool) KeyView {
	v := KeyView{Formats: subscription.AgeFormats(), Optional: a.Settings.Bool("subscribe_age_optional"),
		Strict: a.Settings.Bool("subscribe_age_strict")}
	if v.Formats == nil {
		v.Formats = []string{}
	}
	if k == nil {
		return v
	}
	pub, at := k.PublicKey, k.CreatedAt.UTC()
	v.Enabled, v.PublicKey, v.CreatedAt = k.Enabled, &pub, &at
	if withPrivate {
		priv, err := PrivateKey(a, k)
		if err != nil {
			a.Log.Warn("subscription age key: private key unreadable (jwt.secret rotated?)", "user_id", k.UserID, "err", err)
		} else {
			v.PrivateKey = &priv
		}
	}
	return v
}
