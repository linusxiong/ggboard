package mfasvc

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
)

// TOTP parameters (Google Authenticator compatible): SHA-1, 6 digits, 30 s period, ±1 step accepted.
const (
	totpPeriod = 30
	totpSkew   = 1
)

var totpOpts = totp.ValidateOpts{Period: totpPeriod, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}

// TOTPSetup is the response of POST /user/security/totp/setup.
type TOTPSetup struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
}

// BackupCodes is the response of /totp/enable and /backup-codes: the plain codes, shown once.
type BackupCodes struct {
	BackupCodes []string `json:"backup_codes"`
}

type pendingSetup struct {
	Secret   string
	Attempts int
	Expires  time.Time
}

// Issuer returns the issuer shown by authenticator apps: mfa_issuer, else app_name.
func (s *Service) Issuer() string {
	if v := strings.TrimSpace(s.a.Settings.String("mfa_issuer")); v != "" {
		return v
	}
	if v := strings.TrimSpace(s.a.Settings.String("app_name")); v != "" {
		return v
	}
	return "GG Bond"
}

// normalizeTOTP strips spaces; ok is true for exactly six ASCII digits.
func normalizeTOTP(code string) (string, bool) {
	c := strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(c) != 6 {
		return "", false
	}
	for i := range len(c) {
		if c[i] < '0' || c[i] > '9' {
			return "", false
		}
	}
	return c, true
}

// matchTOTP returns the time step whose code equals code (current step first, then ±1).
func matchTOTP(secret, code string, now time.Time) (int64, bool) {
	step := now.Unix() / totpPeriod
	for _, d := range []int64{0, -1, 1} {
		at := time.Unix((step+d)*totpPeriod, 0)
		want, err := totp.GenerateCodeCustom(secret, at, totpOpts)
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step + d, true
		}
	}
	return 0, false
}

// SetupTOTP generates a new secret for u, kept pending (SetupTTL) until EnableTOTP confirms a code.
// 409 when TOTP is already enabled.
func (s *Service) SetupTOTP(ctx context.Context, u *model.User) (TOTPSetup, error) {
	on, err := s.TOTPEnabled(ctx, u.ID)
	if err != nil {
		return TOTPSetup{}, err
	}
	if on {
		return TOTPSetup{}, httpx.Conflict("two-factor authentication is already enabled")
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer: s.Issuer(), AccountName: u.Email, Period: totpPeriod, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return TOTPSetup{}, fmt.Errorf("mfa: generate totp secret: %w", err)
	}
	s.a.KV.Set(idKey(kvSetup, u.ID), pendingSetup{Secret: key.Secret(), Expires: time.Now().Add(SetupTTL)}, SetupTTL)
	return TOTPSetup{Secret: key.Secret(), OTPAuthURL: key.URL()}, nil
}

// EnableTOTP confirms the pending secret of u with a code, stores it encrypted and returns fresh backup codes.
// Wrong codes are 400 on "code" (the pending secret is discarded after MaxSetupAttempts); no pending secret is
// 400 too; an already enabled TOTP is 409.
func (s *Service) EnableTOTP(ctx context.Context, u *model.User, code string) ([]string, error) {
	on, err := s.TOTPEnabled(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if on {
		return nil, httpx.Conflict("two-factor authentication is already enabled")
	}
	key := idKey(kvSetup, u.ID)
	v, ok := s.a.KV.Take(key)
	p, _ := v.(pendingSetup)
	if !ok || p.Secret == "" {
		return nil, httpx.FieldError("code", "no pending setup, start again")
	}
	c, okFmt := normalizeTOTP(code)
	step, match := matchTOTP(p.Secret, c, TOTPNow())
	if !okFmt || !match {
		p.Attempts++
		if left := remaining(p.Expires); p.Attempts < MaxSetupAttempts && left > 0 {
			s.a.KV.Set(key, p, left)
		}
		return nil, errCodeField()
	}
	enc, err := s.encrypt(u.ID, p.Secret)
	if err != nil {
		return nil, err
	}
	codes, hashes := newBackupCodes()
	now := time.Now()
	err = s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureRow(ctx, tx, u.ID); err != nil {
			return err
		}
		return tx.Model(&model.UserMFA{}).Where("user_id = ?", u.ID).Updates(map[string]any{
			"totp_secret":     enc,
			"totp_enabled":    true,
			"totp_enabled_at": now,
			"totp_last_step":  step,
			"backup_codes":    model.JSONSlice[string](hashes),
		}).Error
	})
	if err != nil {
		return nil, fmt.Errorf("mfa: enable totp: %w", err)
	}
	s.changed(u.ID)
	return codes, nil
}

func (s *Service) encrypt(userID uint64, secret string) (string, error) {
	key, err := s.secretKey()
	if err != nil {
		return "", err
	}
	return encryptSecret(key, userID, secret)
}

// enabledRow returns the user_mfa row of userID, or 409 when TOTP is not enabled.
func (s *Service) enabledRow(ctx context.Context, userID uint64) (*model.UserMFA, error) {
	row, err := s.load(ctx, s.a.DB, userID)
	if err != nil {
		return nil, err
	}
	if row == nil || !row.TOTPEnabled {
		return nil, errNotEnabled()
	}
	return row, nil
}

// verifyCode checks a TOTP code (a step newer than the last accepted one) or an unused backup code against
// row and consumes it. ok is false for wrong, replayed or malformed codes.
func (s *Service) verifyCode(ctx context.Context, row *model.UserMFA, code string) (bool, error) {
	if c, ok := normalizeTOTP(code); ok {
		key, err := s.secretKey()
		if err != nil {
			return false, err
		}
		secret, err := decryptSecret(key, row.UserID, row.TOTPSecret)
		if err != nil {
			return false, err
		}
		step, match := matchTOTP(secret, c, TOTPNow())
		if !match || step <= row.TOTPLastStep {
			return false, nil
		}
		res := s.a.DB.WithContext(ctx).Model(&model.UserMFA{}).
			Where("user_id = ? AND totp_last_step < ?", row.UserID, step).
			Update("totp_last_step", step)
		if res.Error != nil {
			return false, fmt.Errorf("mfa: record totp step: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return false, nil
		}
		row.TOTPLastStep = step
		return true, nil
	}
	if c, ok := normalizeBackup(code); ok {
		return s.consumeBackup(ctx, row, c)
	}
	return false, nil
}

// checkCode verifies a second-factor code for an authenticated user (user lockout applies): 400 on "code"
// when wrong, 429 after UserFailureLimit failures.
func (s *Service) checkCode(ctx context.Context, row *model.UserMFA, code string) error {
	release, err := s.reserveFailure(row.UserID)
	if err != nil {
		return err
	}
	ok, err := s.verifyCode(ctx, row, code)
	if err != nil {
		release()
		return err
	}
	if !ok {
		return errCodeField() // the reserved failure counts
	}
	s.clearFailures(row.UserID)
	return nil
}

// DisableTOTP turns TOTP off for u after verifying the password and a TOTP or backup code; the secret and the
// backup codes are deleted. 409 when TOTP is not enabled.
func (s *Service) DisableTOTP(ctx context.Context, u *model.User, password, code string) error {
	row, err := s.enabledRow(ctx, u.ID)
	if err != nil {
		return err
	}
	if err := s.checkPassword(u, password); err != nil {
		return err
	}
	if err := s.checkCode(ctx, row, code); err != nil {
		return err
	}
	err = s.a.DB.WithContext(ctx).Model(&model.UserMFA{}).Where("user_id = ?", u.ID).Updates(map[string]any{
		"totp_secret":     "",
		"totp_enabled":    false,
		"totp_enabled_at": nil,
		"totp_last_step":  0,
		"backup_codes":    model.JSONSlice[string]{},
	}).Error
	if err != nil {
		return fmt.Errorf("mfa: disable totp: %w", err)
	}
	s.changed(u.ID)
	return nil
}

// RegenerateBackupCodes replaces the backup codes of u after verifying a TOTP or backup code (409 when TOTP is
// not enabled).
func (s *Service) RegenerateBackupCodes(ctx context.Context, u *model.User, code string) ([]string, error) {
	row, err := s.enabledRow(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if err := s.checkCode(ctx, row, code); err != nil {
		return nil, err
	}
	codes, hashes := newBackupCodes()
	if err := s.a.DB.WithContext(ctx).Model(&model.UserMFA{}).Where("user_id = ?", u.ID).
		Update("backup_codes", model.JSONSlice[string](hashes)).Error; err != nil {
		return nil, fmt.Errorf("mfa: regenerate backup codes: %w", err)
	}
	return codes, nil
}
