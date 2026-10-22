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
	if err := s.CheckPasswordLock(email); err != nil {
		return nil, err
	}
	if err := s.VerifyCaptcha(ctx, in.Captcha, in.IP); err != nil {
		return nil, err
	}
	u, err := s.users.Authenticate(ctx, email, in.Password)
	switch {
	case errors.Is(err, usersvc.ErrInvalidCredentials):
		s.RecordPasswordFailure(email)
		return nil, httpx.Unauthorized("invalid email or password")
	case errors.Is(err, usersvc.ErrBanned):
		s.ClearPasswordFailures(email)
		return nil, httpx.Forbidden("account is banned")
	case err != nil:
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
// expired tokens are 401; banned users 403. last_login_* is not touched (the session is the admin's).
func (s *Service) LoginWithToken(ctx context.Context, token string) (*model.User, error) {
	id, ok := auth.ConsumeLoginToken(s.a, token)
	if !ok {
		return nil, httpx.Unauthorized("invalid or expired login token")
	}
	u, err := s.users.FindByID(ctx, id)
	if errors.Is(err, usersvc.ErrNotFound) {
		return nil, httpx.Unauthorized("invalid or expired login token")
	}
	if err != nil {
		return nil, err
	}
	if u.Banned {
		return nil, httpx.Forbidden("account is banned")
	}
	return u, nil
}

// ResetPassword sets a new password after verifying a "forgot" email code. It bumps password_version
// (every existing session ends) and clears the password lockout of the email.
func (s *Service) ResetPassword(ctx context.Context, email, code, password string) (*model.User, error) {
	email = usersvc.NormalizeEmail(email)
	if err := s.ConsumeCode(PurposeForgot, email, code); err != nil {
		return nil, err
	}
	u, err := s.users.FindByEmail(ctx, email)
	if errors.Is(err, usersvc.ErrNotFound) {
		// Only the owner of the mailbox gets this far, so naming the problem leaks nothing.
		return nil, httpx.FieldError("email", "no account is registered with this email")
	}
	if err != nil {
		return nil, err
	}
	if err := s.users.SetPassword(ctx, u, password); err != nil {
		return nil, err
	}
	s.ClearPasswordFailures(email)
	return u, nil
}

// ChangePassword verifies oldPassword (counting failures towards the password lockout) and stores
// newPassword, bumping password_version.
func (s *Service) ChangePassword(ctx context.Context, userID uint64, oldPassword, newPassword string) (*model.User, error) {
	u, err := s.users.FindByID(ctx, userID)
	if errors.Is(err, usersvc.ErrNotFound) {
		return nil, httpx.Unauthorized("invalid or expired token")
	}
	if err != nil {
		return nil, err
	}
	if err := s.CheckPasswordLock(u.Email); err != nil {
		return nil, err
	}
	if !auth.VerifyPassword(u.PasswordHash, oldPassword) {
		s.RecordPasswordFailure(u.Email)
		return nil, httpx.FieldError("old_password", "is incorrect")
	}
	if err := s.users.SetPassword(ctx, u, newPassword); err != nil {
		return nil, err
	}
	auth.InvalidateUser(s.a, u.ID)
	s.ClearPasswordFailures(u.Email)
	return u, nil
}
