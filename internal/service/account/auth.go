package account

import (
	"context"
	"errors"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// LoginInput is a password login request.
type LoginInput struct {
	Email    string
	Password string
	Captcha  string
	IP       string
}

// Login verifies the captcha, the password lockout and the credentials, then records last_login_at/ip.
// Wrong credentials are 401 (and count towards the lockout), banned accounts 403, locked emails 429.
func (s *Service) Login(ctx context.Context, in LoginInput) (*model.User, error) {
	u, err := s.VerifyLogin(ctx, in)
	if err != nil {
		return nil, err
	}
	s.RecordLogin(ctx, u, in.IP)
	return u, nil
}

// VerifyLogin is Login without recording the login: POST /passport/login uses it because accounts with
// two-factor authentication record the login only after the second step (internal/service/mfa).
func (s *Service) VerifyLogin(ctx context.Context, in LoginInput) (*model.User, error) {
	email := usersvc.NormalizeEmail(in.Email)
	release, err := s.ReservePasswordAttempt(email)
	if err != nil {
		return nil, err
	}
	if err := s.VerifyCaptcha(ctx, in.Captcha, in.IP); err != nil {
		release()
		return nil, err
	}
	u, err := s.users.Authenticate(ctx, email, in.Password)
	switch {
	case errors.Is(err, usersvc.ErrInvalidCredentials):
		return nil, httpx.Unauthorized("invalid email or password") // the reserved attempt counts
	case errors.Is(err, usersvc.ErrBanned):
		s.ClearPasswordFailures(email)
		return nil, httpx.Forbidden("account is banned")
	case err != nil:
		release()
		return nil, err
	}
	s.ClearPasswordFailures(email)
	return u, nil
}

// RecordLogin stores last_login_at/ip of u after a completed sign-in (failures are logged, not returned).
func (s *Service) RecordLogin(ctx context.Context, u *model.User, ip string) {
	if err := s.users.RecordLogin(ctx, u, ip); err != nil {
		s.a.Log.Warn("account: record login failed", "user_id", u.ID, "err", err)
	}
}

// LoginWithToken exchanges a one-time login token (admin "login as") for its user. Invalid, used or
// expired tokens are 401, and so are tokens of users promoted to admin/staff since the token was issued (they
// cannot be impersonated); banned users 403. last_login_* is not touched (the session is the admin's).
func (s *Service) LoginWithToken(ctx context.Context, token string) (*model.User, error) {
	claims, ok := auth.ConsumeLoginToken(s.a, token)
	if !ok {
		return nil, httpx.Unauthorized("invalid or expired login token")
	}
	id, err := claims.UserID()
	if err != nil {
		return nil, httpx.Unauthorized("invalid or expired login token")
	}
	u, err := s.users.FindByID(ctx, id)
	if errors.Is(err, usersvc.ErrNotFound) {
		return nil, httpx.Unauthorized("invalid or expired login token")
	}
	if err != nil {
		return nil, err
	}
	if !claims.BoundTo(u) || u.IsAdmin || u.IsStaff {
		return nil, httpx.Unauthorized("invalid or expired login token")
	}
	if u.Banned {
		return nil, httpx.Forbidden("account is banned")
	}
	return u, nil
}

// ResetPassword sets a new password after verifying a "forgot" email code. It bumps password_version
// (every existing session ends) and clears the password lockout of the email.
//
// The code proves control of the mailbox. When the account's email was never verified (registered while
// email_verify was off, so possibly by someone else with the owner's address), the reset is an ownership
// change (usersvc.ReclaimAccount): uuid and subscription token are rotated, passkeys, TOTP / backup codes, the
// Telegram binding and the hosted age key are removed, and email_verified_at is stamped; reclaimed is then true
// and the caller revokes a pending Telegram bind code.
func (s *Service) ResetPassword(ctx context.Context, email, code, password string) (u *model.User, reclaimed bool, err error) {
	email = usersvc.NormalizeEmail(email)
	if err := s.ConsumeCode(PurposeForgot, email, code); err != nil {
		return nil, false, err
	}
	u, err = s.users.FindByEmail(ctx, email)
	if errors.Is(err, usersvc.ErrNotFound) {
		// Only the owner of the mailbox gets this far, so naming the problem leaks nothing.
		return nil, false, httpx.FieldError("email", "no account is registered with this email")
	}
	if err != nil {
		return nil, false, err
	}
	if u.EmailVerifiedAt == nil {
		r, err := s.users.ReclaimAccount(ctx, u, password)
		if err != nil {
			return nil, false, err
		}
		if r != nil {
			s.a.Log.Info("account: unverified account reclaimed by the mailbox owner (credentials rotated, second factors removed)",
				"user_id", u.ID, "passkeys_removed", r.Passkeys, "mfa_removed", r.MFA, "telegram_unbound", r.Telegram, "age_key_removed", r.AgeKey)
			s.ClearPasswordFailures(email)
			return u, true, nil
		}
		// Verified meanwhile: an ordinary reset.
	}
	if err := s.users.SetPassword(ctx, u, password); err != nil {
		return nil, false, err
	}
	s.ClearPasswordFailures(email)
	return u, false, nil
}

// ChangePassword verifies oldPassword and stores newPassword against the authenticated snapshot u,
// bumping password_version. Wrong passwords count towards the password lockout.
func (s *Service) ChangePassword(ctx context.Context, u *model.User, oldPassword, newPassword string) (*model.User, error) {
	if _, err := s.ReservePasswordAttempt(u.Email); err != nil {
		return nil, err
	}
	if !auth.VerifyPassword(u.PasswordHash, oldPassword) {
		return nil, httpx.FieldError("old_password", "is incorrect") // the reserved attempt counts
	}
	if err := s.users.SetPassword(ctx, u, newPassword); err != nil {
		if errors.Is(err, usersvc.ErrNotFound) {
			return nil, httpx.Unauthorized("session expired, please sign in again")
		}
		return nil, err
	}
	s.ClearPasswordFailures(u.Email)
	auth.InvalidateUser(s.a, u.ID)
	return u, nil
}
