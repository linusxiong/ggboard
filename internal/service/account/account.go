// Package account holds the account rules shared by the passport and user API packages: email
// verification codes, captcha checks, login lockout, registration (whitelist, invites, trial plan, per-IP
// limit), dashboard statistics and the user JSON views.
//
// Errors returned by Service methods are *httpx.Error values (or *RetryError for 429s), ready for Fail.
package account

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/captcha"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/mail"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// Email verification code purposes.
const (
	PurposeRegister = "register"
	PurposeForgot   = "forgot"
)

// Verification code parameters.
const (
	CodeTTL         = 10 * time.Minute // lifetime of an email code
	CodeCooldown    = 60 * time.Second // minimum delay between two codes for the same email
	MaxCodeAttempts = 5                // wrong guesses before a code is discarded
)

// KV key prefixes (a.KV).
const (
	kvCode        = "account:code:"          // account:code:<purpose>:<email> -> codeEntry
	kvCodeCool    = "account:code_cooldown:" // account:code_cooldown:<email> -> true
	kvLoginFail   = "account:login_fail:"    // account:login_fail:<email> -> int
	kvRegisterIP  = "account:register_ip:"   // account:register_ip:<ip> -> int
	codeFieldName = "email_code"
)

// CaptchaVerifier returns the captcha verifier for a (captcha.FromSettings). Tests replace it to avoid
// calling the real provider.
var CaptchaVerifier = func(a *app.App) captcha.Verifier { return captcha.FromSettings(a.Settings) }

// codesMu serializes reads and writes of verification codes so attempt counting is atomic.
var codesMu sync.Mutex

type codeEntry struct {
	Code     string
	Attempts int
}

// Service implements the account rules. It is cheap: construct one per request with New.
type Service struct {
	a     *app.App
	users *usersvc.Service
}

// New returns a Service bound to a.
func New(a *app.App) *Service { return &Service{a: a, users: usersvc.New(a)} }

// RetryError is a 429 error that tells the client when to retry (rendered with a Retry-After header by Fail).
type RetryError struct {
	Err        *httpx.Error
	RetryAfter time.Duration
}

func (e *RetryError) Error() string { return e.Err.Error() }

// Unwrap exposes the *httpx.Error so httpx.AsError renders it.
func (e *RetryError) Unwrap() error { return e.Err }

// RetryAfterSeconds returns RetryAfter rounded up to whole seconds (minimum 1).
func (e *RetryError) RetryAfterSeconds() int {
	return max(1, int(math.Ceil(e.RetryAfter.Seconds())))
}

func retryError(msg string, after time.Duration) *RetryError {
	return &RetryError{Err: httpx.NewError(http.StatusTooManyRequests, httpx.CodeRateLimited, msg), RetryAfter: after}
}

// Fail is httpx.Fail plus a Retry-After header for *RetryError.
func Fail(c *gin.Context, err error) {
	var re *RetryError
	if errors.As(err, &re) {
		c.Header("Retry-After", strconv.Itoa(re.RetryAfterSeconds()))
	}
	httpx.Fail(c, err)
}

// errInvalidCode is returned for a wrong, expired or exhausted email code.
func errInvalidCode() *httpx.Error {
	return httpx.FieldError(codeFieldName, "invalid or expired verification code")
}

func codeKey(purpose, email string) string { return kvCode + purpose + ":" + email }

// NewCode returns a uniformly random 6-digit code.
func NewCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		panic("account: crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// SendCode emails a verification code to email, valid CodeTTL for both registration and password reset
// (the response never reveals whether the email is registered). Codes for the same email are limited to
// one per CodeCooldown (*RetryError otherwise).
func (s *Service) SendCode(email string) error {
	email = usersvc.NormalizeEmail(email)
	coolKey := kvCodeCool + email
	busy := false
	s.a.KV.Update(coolKey, CodeCooldown, true, func(old any, ok bool) any {
		busy = ok
		return true
	})
	if busy {
		wait, _ := s.a.KV.TTL(coolKey)
		secs := max(1, int(math.Ceil(wait.Seconds())))
		return retryError(fmt.Sprintf("please wait %d seconds before requesting another code", secs), wait)
	}
	code := NewCode()
	codesMu.Lock()
	for _, p := range []string{PurposeRegister, PurposeForgot} {
		s.a.KV.Set(codeKey(p, email), codeEntry{Code: code}, CodeTTL)
	}
	codesMu.Unlock()
	data := mail.Data{"Code": code, "Minutes": int(CodeTTL / time.Minute)}
	if err := s.a.Mailer.SendTemplate(email, mail.TplVerifyCode, data); err != nil {
		s.a.KV.Delete(coolKey)
		s.dropCodes(email)
		return httpx.Internal(fmt.Errorf("account: queue verification mail: %w", err))
	}
	return nil
}

func (s *Service) dropCodes(email string) {
	codesMu.Lock()
	defer codesMu.Unlock()
	for _, p := range []string{PurposeRegister, PurposeForgot} {
		s.a.KV.Delete(codeKey(p, email))
	}
}

// ConsumeCode checks the email code for purpose. A correct code is single use (it is removed for every
// purpose); a wrong guess counts as an attempt and the code is discarded after MaxCodeAttempts.
func (s *Service) ConsumeCode(purpose, email, code string) error {
	email = usersvc.NormalizeEmail(email)
	if code == "" {
		return httpx.FieldError(codeFieldName, "is required")
	}
	key := codeKey(purpose, email)
	codesMu.Lock()
	defer codesMu.Unlock()
	v, ok := s.a.KV.Get(key)
	e, _ := v.(codeEntry)
	if !ok || e.Code == "" {
		return errInvalidCode()
	}
	if subtle.ConstantTimeCompare([]byte(e.Code), []byte(code)) == 1 {
		for _, p := range []string{PurposeRegister, PurposeForgot} {
			s.a.KV.Delete(codeKey(p, email))
		}
		return nil
	}
	e.Attempts++
	ttl, live := s.a.KV.TTL(key)
	if e.Attempts >= MaxCodeAttempts || !live || ttl <= 0 {
		s.a.KV.Delete(key)
	} else {
		s.a.KV.Set(key, e, ttl)
	}
	return errInvalidCode()
}

// VerifyCaptcha checks token when a captcha provider is configured (400 on the "captcha" field otherwise).
func (s *Service) VerifyCaptcha(ctx context.Context, token, ip string) error {
	if !captcha.Enabled(s.a.Settings) {
		return nil
	}
	if err := CaptchaVerifier(s.a).Verify(ctx, token, ip); err != nil {
		if !errors.Is(err, captcha.ErrFailed) {
			s.a.Log.Warn("account: captcha provider error", "err", err)
		}
		return httpx.FieldError("captcha", "captcha verification failed")
	}
	return nil
}

func minutes(n int) time.Duration { return time.Duration(max(n, 1)) * time.Minute }

func waitMinutes(d time.Duration) int { return max(1, int(math.Ceil(d.Minutes()))) }

// CheckPasswordLock returns a *RetryError when email has reached password_limit_count failed password
// attempts within password_limit_expire minutes (only when password_limit_enable).
func (s *Service) CheckPasswordLock(email string) error {
	st := s.a.Settings
	if !st.Bool("password_limit_enable") {
		return nil
	}
	key := kvLoginFail + usersvc.NormalizeEmail(email)
	v, ok := s.a.KV.Get(key)
	n, _ := v.(int)
	if !ok || n < st.Int("password_limit_count") {
		return nil
	}
	wait, _ := s.a.KV.TTL(key)
	return retryError(fmt.Sprintf("too many failed password attempts, try again in %d minutes", waitMinutes(wait)), wait)
}

// RecordPasswordFailure counts a failed password attempt for email (fixed window of password_limit_expire
// minutes starting at the first failure).
func (s *Service) RecordPasswordFailure(email string) {
	st := s.a.Settings
	if !st.Bool("password_limit_enable") {
		return
	}
	s.a.KV.Update(kvLoginFail+usersvc.NormalizeEmail(email), minutes(st.Int("password_limit_expire")), true,
		func(old any, _ bool) any {
			n, _ := old.(int)
			return n + 1
		})
}

// ClearPasswordFailures resets the failed-attempt counter of email.
func (s *Service) ClearPasswordFailures(email string) {
	s.a.KV.Delete(kvLoginFail + usersvc.NormalizeEmail(email))
}
