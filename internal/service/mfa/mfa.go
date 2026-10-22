// Package mfasvc implements second-factor authentication: TOTP (RFC 6238, ±1 time step, replay guarded) with
// single-use backup codes, WebAuthn passkeys (registration, usernameless and email-scoped login), the
// short-lived MFA tokens of the password + code login and the admin MFA gate.
//
// Errors returned by Service methods are *httpx.Error values (or *account.RetryError for 429s), ready for
// account.Fail. State that must not outlive a ceremony (pending TOTP secrets, MFA tokens, WebAuthn challenges,
// failure counters) lives in a.KV; durable state in the user_mfa and passkeys tables.
package mfasvc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/account"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// Error codes (docs/API.md §Passport, §Security).
const (
	CodeInvalid       = "mfa_invalid"        // 401: wrong TOTP / backup code for an MFA token
	CodeExpired       = "mfa_expired"        // 401: unknown, used or expired MFA token
	CodeSetupRequired = "mfa_setup_required" // 403: AdminGate, admin/staff without TOTP or passkey
)

// Lifetimes and limits.
const (
	TokenTTL          = 5 * time.Minute  // MFA login token
	MaxTokenAttempts  = 5                // wrong codes per MFA token before it is revoked (429)
	SetupTTL          = 10 * time.Minute // pending TOTP secret of /totp/setup
	MaxSetupAttempts  = 5                // wrong codes before a pending secret is discarded
	ChallengeTTL      = 2 * time.Minute  // WebAuthn challenges; also the timeout sent to the browser
	BackupCodeCount   = 10               // codes per batch
	UserFailureLimit  = 20               // wrong second-factor codes per user per UserFailureWindow (429 after)
	UserFailureWindow = time.Hour
	statusTTL         = 30 * time.Second // cache of Configured (AdminGate)
)

// KV key prefixes (a.KV) and cache keys (a.Cache).
const (
	kvToken          = "mfa:token:"            // mfa:token:<token> -> pendingLogin
	kvSetup          = "mfa:totp_setup:"       // mfa:totp_setup:<user id> -> pendingSetup
	kvRegister       = "mfa:passkey_register:" // mfa:passkey_register:<challenge id> -> pendingRegistration
	kvAssertion      = "mfa:passkey_login:"    // mfa:passkey_login:<challenge id> -> pendingAssertion
	kvFailures       = "mfa:failures:"         // mfa:failures:<user id> -> int
	cacheStatusKeyNS = "mfa:configured:"       // mfa:configured:<user id> -> bool
)

// TOTPNow is the clock used to compute TOTP time steps. Tests replace it to get deterministic steps.
var TOTPNow = time.Now

// Service implements the MFA rules. It is cheap: construct one per request with New.
type Service struct {
	a     *app.App
	users *usersvc.Service
}

// New returns a Service bound to a.
func New(a *app.App) *Service { return &Service{a: a, users: usersvc.New(a)} }

func errExpired() *httpx.Error {
	return httpx.NewError(http.StatusUnauthorized, CodeExpired, "verification expired, please sign in again")
}

func errInvalid() *httpx.Error {
	return httpx.NewError(http.StatusUnauthorized, CodeInvalid, "invalid verification code")
}

func errCodeField() *httpx.Error { return httpx.FieldError("code", "invalid verification code") }

func errNotEnabled() *httpx.Error {
	return httpx.Conflict("two-factor authentication is not enabled")
}

// randomID returns n random bytes, hex encoded (tokens and challenge ids).
func randomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("mfa: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func idKey(prefix string, id uint64) string { return prefix + strconv.FormatUint(id, 10) }

// remaining returns the time left until exp (0 when passed).
func remaining(exp time.Time) time.Duration { return max(0, time.Until(exp)) }

// load returns the user_mfa row of userID, or nil when the user never set up a second factor.
func (s *Service) load(ctx context.Context, db *gorm.DB, userID uint64) (*model.UserMFA, error) {
	var row model.UserMFA
	err := db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mfa: load: %w", err)
	}
	return &row, nil
}

// ensureRow creates the (empty) user_mfa row of userID when it does not exist yet.
func ensureRow(ctx context.Context, db *gorm.DB, userID uint64) error {
	err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model.UserMFA{UserID: userID}).Error
	if err != nil && !errors.Is(err, gorm.ErrDuplicatedKey) {
		return fmt.Errorf("mfa: create row: %w", err)
	}
	return nil
}

// TOTPEnabled reports whether userID has TOTP enabled.
func (s *Service) TOTPEnabled(ctx context.Context, userID uint64) (bool, error) {
	row, err := s.load(ctx, s.a.DB, userID)
	if err != nil {
		return false, err
	}
	return row != nil && row.TOTPEnabled, nil
}

// PasskeyCount returns the number of passkeys of userID (every RP id).
func (s *Service) PasskeyCount(ctx context.Context, userID uint64) (int64, error) {
	var n int64
	if err := s.a.DB.WithContext(ctx).Model(&model.Passkey{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("mfa: count passkeys: %w", err)
	}
	return n, nil
}

// Configured reports whether userID has TOTP enabled or at least one passkey. The answer is cached for 30 s
// and dropped whenever the user's second factors change.
func (s *Service) Configured(ctx context.Context, userID uint64) (bool, error) {
	return cache.GetOrLoad(s.a.Cache, idKey(cacheStatusKeyNS, userID), statusTTL, func() (bool, error) {
		on, err := s.TOTPEnabled(ctx, userID)
		if err != nil || on {
			return on, err
		}
		n, err := s.PasskeyCount(ctx, userID)
		return n > 0, err
	})
}

// changed drops cached state after the second factors of userID changed. Sessions stay valid
// (password_version is not bumped); the auth middleware simply reloads the user.
func (s *Service) changed(userID uint64) {
	auth.InvalidateUser(s.a, userID)
	s.a.Cache.Delete(idKey(cacheStatusKeyNS, userID))
}

// checkFailures returns a *account.RetryError once userID made UserFailureLimit wrong second-factor attempts
// within UserFailureWindow.
func (s *Service) checkFailures(userID uint64) error {
	key := idKey(kvFailures, userID)
	v, ok := s.a.KV.Get(key)
	if n, _ := v.(int); !ok || n < UserFailureLimit {
		return nil
	}
	wait, _ := s.a.KV.TTL(key)
	mins := max(1, int(math.Ceil(wait.Minutes())))
	return &account.RetryError{
		Err:        httpx.RateLimited(fmt.Sprintf("too many invalid verification codes, try again in %d minutes", mins)),
		RetryAfter: wait,
	}
}

// recordFailure counts a wrong second-factor code of userID (fixed window starting at the first failure).
func (s *Service) recordFailure(userID uint64) {
	s.a.KV.Update(idKey(kvFailures, userID), UserFailureWindow, true, func(old any, _ bool) any {
		n, _ := old.(int)
		return n + 1
	})
}

func (s *Service) clearFailures(userID uint64) { s.a.KV.Delete(idKey(kvFailures, userID)) }

// checkPassword verifies password for u with the login lockout of internal/service/account (wrong passwords
// count towards password_limit_*): 400 on the "password" field, 429 while locked.
func (s *Service) checkPassword(u *model.User, password string) error {
	acc := account.New(s.a)
	if err := acc.CheckPasswordLock(u.Email); err != nil {
		return err
	}
	if !auth.VerifyPassword(u.PasswordHash, password) {
		acc.RecordPasswordFailure(u.Email)
		return httpx.FieldError("password", "is incorrect")
	}
	acc.ClearPasswordFailures(u.Email)
	return nil
}
