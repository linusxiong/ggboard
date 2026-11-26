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
	CodeInvalid       = "mfa_invalid"         // 401: wrong TOTP / backup code for an MFA token
	CodeExpired       = "mfa_expired"         // 401: unknown, used or expired MFA token
	CodeSetupRequired = "mfa_setup_required"  // 403: AdminGate, admin/staff without TOTP or passkey
	CodeReauth        = "mfa_reauth_required" // 403: AdminGate, admin/staff session signed in with the password only
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
	// UserFailureDayLimit caps wrong codes per user per UserFailureDayWindow on top of the hourly limit, so an
	// attacker who knows the password cannot keep guessing TOTP codes at UserFailureLimit an hour forever
	// (20/h would be 480 guesses a day; NIST SP 800-63B §5.2.2 asks for at most 100 before a lock).
	UserFailureDayLimit  = 30
	UserFailureDayWindow = 24 * time.Hour
	statusTTL            = 30 * time.Second // cache of Configured (AdminGate)
)

// KV key prefixes (a.KV) and cache keys (a.Cache).
const (
	kvToken          = "mfa:token:"            // mfa:token:<token> -> pendingLogin
	kvSetup          = "mfa:totp_setup:"       // mfa:totp_setup:<user id> -> pendingSetup
	kvRegister       = "mfa:passkey_register:" // mfa:passkey_register:<challenge id> -> pendingRegistration
	kvAssertion      = "mfa:passkey_login:"    // mfa:passkey_login:<challenge id> -> pendingAssertion
	kvFailures       = "mfa:failures:"         // mfa:failures:<user id> -> int
	kvFailuresDay    = "mfa:failures_day:"     // mfa:failures_day:<user id> -> int
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

// withUser keeps credential writes in the same transaction as the account binding check.
func (s *Service) withUser(ctx context.Context, u *model.User, write func(*gorm.DB) error) error {
	return s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A no-op update takes the user write lock on SQLite, PostgreSQL and MySQL alike.
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
		return write(tx)
	})
}

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

// UsablePasskeyCount returns the number of passkeys of userID that can sign in on this site now: those
// registered for the current relying party ID (0 while passkeys are not configured). A passkey left from an
// earlier passkey_rp_id / app_url cannot produce an assertion, so it must not count as a second factor.
func (s *Service) UsablePasskeyCount(ctx context.Context, userID uint64) (int64, error) {
	rp, ok := RPFromSettings(s.a.Settings)
	if !ok {
		return 0, nil
	}
	var n int64
	if err := s.a.DB.WithContext(ctx).Model(&model.Passkey{}).Where("user_id = ? AND rp_id = ?", userID, rp.ID).
		Count(&n).Error; err != nil {
		return 0, fmt.Errorf("mfa: count passkeys: %w", err)
	}
	return n, nil
}

// Configured reports whether userID has TOTP enabled or at least one usable passkey (UsablePasskeyCount). The
// answer is cached for 30 s and dropped whenever the user's second factors change.
func (s *Service) Configured(ctx context.Context, userID uint64) (bool, error) {
	lctx := context.WithoutCancel(ctx) // singleflight: the load is shared by concurrent callers, so it must not fail because the first caller went away
	ns := idKey(cacheStatusKeyNS, userID)
	for {
		key := s.a.Cache.Key(ns, "")
		configured, err := cache.GetOrLoad(s.a.Cache, key, statusTTL, func() (bool, error) {
			on, err := s.TOTPEnabled(lctx, userID)
			if err != nil || on {
				return on, err
			}
			n, err := s.UsablePasskeyCount(lctx, userID)
			return n > 0, err
		})
		if key == s.a.Cache.Key(ns, "") {
			return configured, err
		}
	}
}

// changed drops cached state after the second factors of userID changed. Sessions stay valid
// (password_version is not bumped); the auth middleware simply reloads the user.
func (s *Service) changed(userID uint64) {
	auth.InvalidateUser(s.a, userID)
	s.a.Cache.InvalidateNamespace(idKey(cacheStatusKeyNS, userID))
}

// reserveFailure reserves one second-factor attempt of userID (account.ReserveAttempt: the attempt counts as a
// failure unless released or cleared) and returns a *account.RetryError once userID made UserFailureLimit wrong
// attempts within UserFailureWindow, or UserFailureDayLimit within UserFailureDayWindow. Reserving before
// verifying keeps concurrent requests from all passing a check made before any failure is recorded.
func (s *Service) reserveFailure(userID uint64) (release func(), err error) {
	key := idKey(kvFailures, userID)
	releaseHour, ok := account.ReserveAttempt(s.a, key, UserFailureLimit, UserFailureWindow)
	if !ok {
		return nil, s.failureLimited(key)
	}
	dayKey := idKey(kvFailuresDay, userID)
	releaseDay, ok := account.ReserveAttempt(s.a, dayKey, UserFailureDayLimit, UserFailureDayWindow)
	if !ok {
		releaseHour()
		return nil, s.failureLimited(dayKey)
	}
	return func() { releaseHour(); releaseDay() }, nil
}

// failureLimited is the 429 of reserveFailure for the exhausted counter key.
func (s *Service) failureLimited(key string) error {
	wait, _ := s.a.KV.TTL(key)
	mins := max(1, int(math.Ceil(wait.Minutes())))
	return &account.RetryError{
		Err:        httpx.RateLimited(fmt.Sprintf("too many invalid verification codes, try again in %d minutes", mins)),
		RetryAfter: wait,
	}
}

func (s *Service) clearFailures(userID uint64) {
	s.a.KV.Delete(idKey(kvFailures, userID))
	s.a.KV.Delete(idKey(kvFailuresDay, userID))
}

// checkPassword verifies password for the signed-in u with the account's confirmation lockout
// (account.ReserveReauthAttempt: wrong passwords count towards password_limit_* per account): 400 on the
// "password" field, 429 while locked.
func (s *Service) checkPassword(u *model.User, password string) error {
	acc := account.New(s.a)
	if _, err := acc.ReserveReauthAttempt(u.ID); err != nil {
		return err
	}
	if !auth.VerifyPassword(u.PasswordHash, password) {
		return httpx.FieldError("password", "is incorrect") // the reserved attempt counts
	}
	acc.ClearReauthFailures(u.ID)
	return nil
}

// ConfirmPassword re-authenticates the signed-in u before a second factor is added (POST /user/security/totp/setup
// and /passkeys/begin): a stolen session alone must not be able to plant a passkey that outlives a password
// change, or a TOTP secret that locks the owner out. Errors as checkPassword.
func (s *Service) ConfirmPassword(u *model.User, password string) error {
	return s.checkPassword(u, password)
}
