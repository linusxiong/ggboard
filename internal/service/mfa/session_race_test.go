package mfasvc

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/mfa/mfatest"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestRenamePasskeyRejectsRevokedAccountSnapshot(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "rename-passkey@example.com")
	p := model.Passkey{UserID: u.ID, RPID: "example.com", CredentialHash: "rename-passkey-hash", CredentialID: "x", Credential: "{}", Name: "Original"}
	if err := a.DB.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("uuid", "replacement-account-uuid").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := New(a).RenamePasskey(t.Context(), u, p.ID, "Attacker label"); status(err) != http.StatusUnauthorized {
		t.Fatalf("stale passkey rename = %v, want 401", err)
	}
	var stored model.Passkey
	if err := a.DB.First(&stored, p.ID).Error; err != nil || stored.Name != "Original" {
		t.Fatalf("passkey after stale rename = %+v, %v", stored, err)
	}
}

func TestSecurityOverviewRejectsReplacedAccount(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "security-overview@example.com")
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("uuid", "replacement-account-uuid").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Create(&model.Passkey{UserID: u.ID, RPID: "example.com", CredentialHash: "replacement-passkey-hash",
		CredentialID: "x", Credential: "{}", Name: "Replacement key"}).Error; err != nil {
		t.Fatal(err)
	}
	if out, err := New(a).OverviewForUser(t.Context(), u); status(err) != http.StatusUnauthorized || len(out.Passkeys) != 0 {
		t.Fatalf("stale security overview = %+v, %v", out, err)
	}
}

func TestBeginPasskeyCannotCreateHandleForReplacedAccount(t *testing.T) {
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	u := testutil.CreateUser(t, a, "passkey-handle@example.com")
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("uuid", "replacement-account-uuid").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := New(a).BeginRegistration(t.Context(), u, ""); status(err) != http.StatusUnauthorized {
		t.Fatalf("stale passkey begin = %v, want 401", err)
	}
	var count int64
	if err := a.DB.Model(&model.UserMFA{}).Where("user_id = ?", u.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("replacement account handle rows = %d, %v", count, err)
	}
}

func sessionRaceApp(t *testing.T) (*app.App, *Service, *model.User, *time.Time) {
	t.Helper()
	a := testutil.NewTestApp(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	now := mfatest.FixClock(t, &TOTPNow, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	return a, New(a), testutil.CreateUser(t, a, "ceremony@example.com"), now
}

func sessionRaceOptions(t *testing.T, ceremony Ceremony) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(ceremony.Options)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sessionRacePasskey(t *testing.T, s *Service, u *model.User) (*mfatest.Authenticator, PasskeyView) {
	t.Helper()
	key := mfatest.New(t, "panel.example.com", "https://panel.example.com")
	ceremony, err := s.BeginRegistration(ctx, u, "")
	if err != nil {
		t.Fatal(err)
	}
	pk, err := s.FinishRegistration(ctx, u, ceremony.ChallengeID, key.Register(t, sessionRaceOptions(t, ceremony)), "")
	if err != nil {
		t.Fatal(err)
	}
	return key, pk
}

func sessionRaceRevoke(t *testing.T, a *app.App, u *model.User) *model.User {
	t.Helper()
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("password_version", u.PasswordVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	var fresh model.User
	if err := a.DB.First(&fresh, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	return &fresh
}

func TestMFATokenCannotAdoptChangedEmail(t *testing.T) {
	a, s, u, now := sessionRaceApp(t)
	secret, _ := enable(t, a, u, now)
	challenge, err := s.Challenge(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("email", "new-owner@example.com").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteLogin(ctx, challenge.MFAToken, mfatest.TOTP(t, secret, *now)); code(err) != CodeExpired {
		t.Fatalf("MFA token accepted a different account email: %v", err)
	}
}

func TestPendingEnrollmentDoesNotSurvivePasswordReset(t *testing.T) {
	t.Run("totp", func(t *testing.T) {
		a, s, u, now := sessionRaceApp(t)
		setup, err := s.SetupTOTP(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		fresh := sessionRaceRevoke(t, a, u)
		if _, err := s.EnableTOTP(ctx, fresh, mfatest.TOTP(t, setup.Secret, *now)); status(err) != http.StatusBadRequest {
			t.Fatalf("old TOTP setup accepted by a new session: %v", err)
		}
		on, err := s.TOTPEnabled(ctx, u.ID)
		if err != nil || on {
			t.Fatalf("revoked setup enrolled TOTP: %v, %v", on, err)
		}
	})
	t.Run("passkey", func(t *testing.T) {
		a, s, u, _ := sessionRaceApp(t)
		ceremony, err := s.BeginRegistration(ctx, u, "")
		if err != nil {
			t.Fatal(err)
		}
		key := mfatest.New(t, "panel.example.com", "https://panel.example.com")
		credential := key.Register(t, sessionRaceOptions(t, ceremony))
		fresh := sessionRaceRevoke(t, a, u)
		if _, err := s.FinishRegistration(ctx, fresh, ceremony.ChallengeID, credential, ""); status(err) != http.StatusBadRequest {
			t.Fatalf("old passkey ceremony accepted by a new session: %v", err)
		}
		if n, err := s.PasskeyCount(ctx, u.ID); err != nil || n != 0 {
			t.Fatalf("revoked ceremony stored passkey: %d, %v", n, err)
		}
	})
}

func TestCredentialWritesRejectRevokedUserSnapshot(t *testing.T) {
	for _, operation := range []string{"enable", "disable", "backup", "register", "delete"} {
		t.Run(operation, func(t *testing.T) {
			a, s, u, now := sessionRaceApp(t)
			var write func() error
			switch operation {
			case "enable":
				setup, err := s.SetupTOTP(ctx, u)
				if err != nil {
					t.Fatal(err)
				}
				write = func() error {
					_, err := s.EnableTOTP(ctx, u, mfatest.TOTP(t, setup.Secret, *now))
					return err
				}
			case "disable", "backup":
				secret, _ := enable(t, a, u, now)
				write = func() error {
					if operation == "disable" {
						return s.DisableTOTP(ctx, u, testutil.Password, mfatest.TOTP(t, secret, *now))
					}
					_, err := s.RegenerateBackupCodes(ctx, u, mfatest.TOTP(t, secret, *now))
					return err
				}
			case "register":
				ceremony, err := s.BeginRegistration(ctx, u, "")
				if err != nil {
					t.Fatal(err)
				}
				key := mfatest.New(t, "panel.example.com", "https://panel.example.com")
				credential := key.Register(t, sessionRaceOptions(t, ceremony))
				write = func() error {
					_, err := s.FinishRegistration(ctx, u, ceremony.ChallengeID, credential, "")
					return err
				}
			case "delete":
				_, pk := sessionRacePasskey(t, s, u)
				write = func() error { return s.DeletePasskey(ctx, u, pk.ID, testutil.Password) }
			}
			sessionRaceRevoke(t, a, u) // The already authenticated request still carries u's old version.
			var before model.UserMFA
			if err := a.DB.Where("user_id = ?", u.ID).Find(&before).Error; err != nil {
				t.Fatal(err)
			}
			beforeKeys, err := s.PasskeyCount(ctx, u.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := write(); status(err) != http.StatusUnauthorized {
				t.Fatalf("revoked request changed credentials: %v", err)
			}
			var after model.UserMFA
			if err := a.DB.Where("user_id = ?", u.ID).Find(&after).Error; err != nil {
				t.Fatal(err)
			}
			beforeJSON, _ := json.Marshal(before)
			afterJSON, _ := json.Marshal(after)
			if string(beforeJSON) != string(afterJSON) {
				t.Fatal("rejected request changed second-factor state")
			}
			if n, err := s.PasskeyCount(ctx, u.ID); err != nil || n != beforeKeys {
				t.Fatalf("rejected request changed passkeys: %d, %v", n, err)
			}
		})
	}
}

func TestPasskeyLoginDoesNotAdoptVersionChangedDuringVerification(t *testing.T) {
	for _, discoverable := range []bool{false, true} {
		name := "email"
		if discoverable {
			name = "discoverable"
		}
		t.Run(name, func(t *testing.T) {
			a, s, u, _ := sessionRaceApp(t)
			key, _ := sessionRacePasskey(t, s, u)
			email := u.Email
			if discoverable {
				email = ""
			}
			ceremony, err := s.BeginLogin(ctx, email)
			if err != nil {
				t.Fatal(err)
			}
			credential := key.Assert(t, sessionRaceOptions(t, ceremony))
			var changed atomic.Bool
			if err := a.DB.Callback().Query().After("gorm:query").Register("test:revoke_after_passkey_read", func(tx *gorm.DB) {
				if tx.Statement.Table == "passkeys" && changed.CompareAndSwap(false, true) {
					if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("password_version", u.PasswordVersion+1).Error; err != nil {
						t.Error(err)
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.FinishLogin(ctx, ceremony.ChallengeID, credential); status(err) != http.StatusUnauthorized {
				t.Fatalf("assertion issued a session after password reset: %v", err)
			}
			if !changed.Load() {
				t.Fatal("verification did not reach the race barrier")
			}
		})
	}
}

func TestTOTPVerificationRejectsReplacedSecret(t *testing.T) {
	a, s, u, now := sessionRaceApp(t)
	secret, _ := enable(t, a, u, now)
	row, err := s.enabledRow(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	newSecret, err := s.encrypt(u.ID, "JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Model(&model.UserMFA{}).Where("user_id = ?", u.ID).Update("totp_secret", newSecret).Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := s.verifyCode(ctx, a.DB, row, mfatest.TOTP(t, secret, *now)); err != nil || ok {
		t.Fatalf("code for a replaced authenticator was accepted: %v, %v", ok, err)
	}
}

func TestConfiguredLoadCannotRestoreRemovedFactor(t *testing.T) {
	a, s, u, now := sessionRaceApp(t)
	secret, _ := enable(t, a, u, now)
	loaded, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(resume) }) }
	t.Cleanup(release)
	var paused atomic.Bool
	if err := a.DB.Callback().Query().After("gorm:query").Register("test:pause_configured_load", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_mfa" && paused.CompareAndSwap(false, true) {
			close(loaded)
			<-resume
		}
	}); err != nil {
		t.Fatal(err)
	}
	type result struct {
		configured bool
		err        error
	}
	done := make(chan result, 1)
	go func() {
		configured, err := s.Configured(ctx, u.ID)
		done <- result{configured, err}
	}()
	select {
	case <-loaded:
	case <-time.After(5 * time.Second):
		t.Fatal("factor load did not reach the barrier")
	}
	if err := s.DisableTOTP(ctx, u, testutil.Password, mfatest.TOTP(t, secret, *now)); err != nil {
		t.Fatal(err)
	}
	release()
	select {
	case got := <-done:
		if got.err != nil || got.configured {
			t.Fatalf("removed factor restored by an in-flight load: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("factor load did not finish")
	}
	if configured, err := s.Configured(ctx, u.ID); err != nil || configured {
		t.Fatalf("removed factor restored in cache: %v, %v", configured, err)
	}
}

func TestPendingCredentialsAreBoundToAccountUUID(t *testing.T) {
	for _, operation := range []string{"login", "totp", "passkey", "in-flight write"} {
		t.Run(operation, func(t *testing.T) {
			a, s, u, now := sessionRaceApp(t)
			var finish func(*model.User) error
			switch operation {
			case "login":
				secret, _ := enable(t, a, u, now)
				challenge, err := s.Challenge(ctx, u)
				if err != nil {
					t.Fatal(err)
				}
				finish = func(*model.User) error {
					_, err := s.CompleteLogin(ctx, challenge.MFAToken, mfatest.TOTP(t, secret, *now))
					return err
				}
			case "totp", "in-flight write":
				setup, err := s.SetupTOTP(ctx, u)
				if err != nil {
					t.Fatal(err)
				}
				finish = func(fresh *model.User) error {
					if operation == "in-flight write" {
						fresh = u
					}
					_, err := s.EnableTOTP(ctx, fresh, mfatest.TOTP(t, setup.Secret, *now))
					return err
				}
			case "passkey":
				ceremony, err := s.BeginRegistration(ctx, u, "")
				if err != nil {
					t.Fatal(err)
				}
				key := mfatest.New(t, "panel.example.com", "https://panel.example.com")
				credential := key.Register(t, sessionRaceOptions(t, ceremony))
				finish = func(fresh *model.User) error {
					_, err := s.FinishRegistration(ctx, fresh, ceremony.ChallengeID, credential, "")
					return err
				}
			}
			// Keep id, email and password_version identical; only the account incarnation changes.
			if err := a.DB.Delete(&model.User{}, u.ID).Error; err != nil {
				t.Fatal(err)
			}
			fresh := *u
			fresh.UUID = "10000000-0000-4000-8000-000000000003"
			if err := a.DB.Create(&fresh).Error; err != nil {
				t.Fatal(err)
			}
			if err := finish(&fresh); status(err) != http.StatusUnauthorized && status(err) != http.StatusBadRequest {
				t.Fatalf("old ceremony accepted a recreated account: %v", err)
			}
		})
	}
}

func TestPasskeyLoginCannotOutliveAccountReclaim(t *testing.T) {
	a, s, u, _ := sessionRaceApp(t)
	key, _ := sessionRacePasskey(t, s, u)
	ceremony, err := s.BeginLogin(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	credential := key.Assert(t, sessionRaceOptions(t, ceremony))
	var recorded, reclaimed atomic.Bool
	reclaim := func() error {
		if !reclaimed.CompareAndSwap(false, true) {
			return nil
		}
		owner := *u
		_, err := usersvc.New(a).ReclaimAccount(ctx, &owner, "mailbox-owner-password")
		return err
	}
	if err := a.DB.Callback().Update().After("gorm:commit_or_rollback_transaction").Register("test:passkey_use_recorded", func(tx *gorm.DB) {
		if tx.Statement.Table == "passkeys" {
			recorded.Store(true)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Callback().Query().Before("gorm:query").Register("test:reclaim_before_post_verification_reload", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && recorded.Load() {
			if err := reclaim(); err != nil {
				_ = tx.AddError(err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	loggedIn, err := s.FinishLogin(ctx, ceremony.ChallengeID, credential)
	if err != nil {
		t.Fatal(err)
	}
	// Reclaim before the handler issues its JWT, including when no post-verification reload occurs.
	if err := reclaim(); err != nil {
		t.Fatal(err)
	}
	var owner model.User
	if err := a.DB.First(&owner, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := s.PasskeyCount(ctx, u.ID); err != nil || n != 0 {
		t.Fatalf("reclaim did not remove the previous holder's passkey: %d, %v", n, err)
	}
	token, err := auth.Issue(a, loggedIn)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := auth.Parse(a, token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.BoundTo(&owner) {
		t.Fatal("the removed passkey issued a session for the reclaimed account")
	}
}
