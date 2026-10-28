package mfasvc

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/account"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// MethodTOTP is the only second-step method of the password login (backup codes are accepted by the same
// endpoint).
const MethodTOTP = "totp"

// LoginChallenge is the response of POST /passport/login when the account has TOTP enabled.
type LoginChallenge struct {
	MFARequired bool     `json:"mfa_required"`
	MFAToken    string   `json:"mfa_token"`
	Methods     []string `json:"methods"`
	HasPasskey  bool     `json:"has_passkey"`
}

// pendingLogin binds an MFA token to the account instance and security version that proved the password.
type pendingLogin struct {
	UserID   uint64
	UUID     string
	Email    string
	Version  int
	Attempts int
	Expires  time.Time
}

// Challenge returns the second-step challenge for u (who just proved the password) when TOTP is enabled, or
// nil when the password alone completes the login. The MFA token is valid for TokenTTL and single use.
func (s *Service) Challenge(ctx context.Context, u *model.User) (*LoginChallenge, error) {
	on, err := s.TOTPEnabled(ctx, u.ID)
	if err != nil || !on {
		return nil, err
	}
	n, err := s.PasskeyCount(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	tok := randomID(32)
	s.a.KV.Set(kvToken+tok, pendingLogin{UserID: u.ID, UUID: u.UUID, Email: u.Email, Version: u.PasswordVersion,
		Expires: time.Now().Add(TokenTTL)}, TokenTTL)
	return &LoginChallenge{MFARequired: true, MFAToken: tok, Methods: []string{MethodTOTP}, HasPasskey: n > 0}, nil
}

// CompleteLogin exchanges an MFA token and a TOTP or backup code for the user. Unknown, used or expired tokens
// (or a password change since the token was issued) are 401 mfa_expired; a wrong code is 401 mfa_invalid and
// the MaxTokenAttempts-th wrong code revokes the token with 429; banned users are 403. A correct code consumes
// the token.
func (s *Service) CompleteLogin(ctx context.Context, token, code string) (*model.User, error) {
	key := kvToken + token
	v, ok := s.a.KV.Take(key) // claim the token: concurrent requests cannot both succeed
	p, _ := v.(pendingLogin)
	if !ok || token == "" || p.UserID == 0 {
		return nil, errExpired()
	}
	u, err := s.users.FindByID(ctx, p.UserID)
	if errors.Is(err, usersvc.ErrNotFound) {
		return nil, errExpired()
	}
	if err != nil {
		s.putBack(key, p)
		return nil, err
	}
	if p.UUID == "" || u.UUID != p.UUID || u.PasswordVersion != p.Version || usersvc.NormalizeEmail(u.Email) != usersvc.NormalizeEmail(p.Email) {
		return nil, errExpired()
	}
	if u.Banned {
		return nil, httpx.Forbidden("account is banned")
	}
	row, err := s.load(ctx, s.a.DB, u.ID)
	if err != nil {
		s.putBack(key, p)
		return nil, err
	}
	if row == nil || !row.TOTPEnabled {
		return nil, errExpired()
	}
	release, err := s.reserveFailure(u.ID)
	if err != nil {
		return nil, err
	}
	err = s.withUser(ctx, u, func(tx *gorm.DB) error {
		var verifyErr error
		ok, verifyErr = s.verifyCode(ctx, tx, row, code)
		return verifyErr
	})
	if err != nil {
		release()
		s.putBack(key, p)
		return nil, err
	}
	if !ok {
		p.Attempts++ // the reserved failure counts
		if p.Attempts >= MaxTokenAttempts {
			return nil, &account.RetryError{
				Err: httpx.RateLimited("too many invalid verification codes, please sign in again"),
			}
		}
		if !s.putBack(key, p) {
			return nil, errExpired()
		}
		return nil, errInvalid()
	}
	s.clearFailures(u.ID)
	return u, nil
}

// putBack restores a claimed MFA token for the rest of its lifetime; false when it already expired.
func (s *Service) putBack(key string, p pendingLogin) bool {
	left := remaining(p.Expires)
	if left <= 0 {
		return false
	}
	s.a.KV.Set(key, p, left)
	return true
}
