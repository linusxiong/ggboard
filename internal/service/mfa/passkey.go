package mfasvc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/settings"
)

// MaxPasskeyName is the maximum length of a passkey name.
const MaxPasskeyName = 64

// RelyingParty is the WebAuthn relying party derived from settings.
type RelyingParty struct {
	ID      string   // RP id: a registrable domain, never with a port
	Name    string   // display name (app_name)
	Origins []string // allowed origins (scheme://host[:port])
}

// RPFromSettings derives the relying party: the id is passkey_rp_id or the host of app_url; the origins are the
// app_url origin plus passkey_origins (https://<id> when neither gives one); the name is app_name. ok is false
// when no id can be derived (or it is an IP address), i.e. passkeys are unavailable.
func RPFromSettings(st *settings.Service) (RelyingParty, bool) {
	origin, host := parseOrigin(st.String("app_url"))
	id := strings.ToLower(strings.TrimSpace(st.String("passkey_rp_id")))
	if id == "" {
		id = host
	}
	// WebAuthn RP ids are domains: an IP address (app_url http://127.0.0.1:8080) can never register or use a
	// passkey, so it means "passkeys unavailable" (400 + passkey_enabled=false) instead of a 500 from go-webauthn.
	if id == "" || net.ParseIP(strings.Trim(id, "[]")) != nil {
		return RelyingParty{}, false
	}
	var origins []string
	if origin != "" {
		origins = append(origins, origin)
	}
	for _, o := range st.Strings("passkey_origins") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" && !slices.Contains(origins, o) {
			origins = append(origins, o)
		}
	}
	if len(origins) == 0 {
		origins = []string{"https://" + id}
	}
	name := strings.TrimSpace(st.String("app_name"))
	if name == "" {
		name = "GG Bond"
	}
	return RelyingParty{ID: id, Name: name, Origins: origins}, true
}

// parseOrigin returns the origin (scheme://host[:port], lower case) and host (no port) of an http(s) URL; empty
// strings when raw is not one. A missing scheme means https.
func parseOrigin(raw string) (origin, host string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), strings.ToLower(u.Hostname())
}

// PasskeyEnabled reports whether passkeys are available (app_url or passkey_rp_id is set).
func PasskeyEnabled(st *settings.Service) bool {
	_, ok := RPFromSettings(st)
	return ok
}

// selection is the authenticator selection of every registration: discoverable credentials (required by
// password managers and usernameless login) with user verification preferred.
var selection = protocol.AuthenticatorSelection{
	ResidentKey:        protocol.ResidentKeyRequirementRequired,
	RequireResidentKey: protocol.ResidentKeyRequired(),
	UserVerification:   protocol.VerificationPreferred,
}

func (s *Service) webAuthn() (*webauthn.WebAuthn, RelyingParty, error) {
	rp, ok := RPFromSettings(s.a.Settings)
	if !ok {
		return nil, rp, httpx.BadRequest("passkeys are not configured on this site")
	}
	timeout := webauthn.TimeoutConfig{Enforce: true, Timeout: ChallengeTTL, TimeoutUVD: ChallengeTTL}
	w, err := webauthn.New(&webauthn.Config{
		RPID:                              rp.ID,
		RPDisplayName:                     rp.Name,
		RPOrigins:                         rp.Origins,
		AttestationPreference:             protocol.PreferNoAttestation,
		AuthenticatorSelection:            selection,
		Timeouts:                          webauthn.TimeoutsConfig{Login: timeout, Registration: timeout},
		ExtensionsUnsolicitedOutputPolicy: protocol.UnsolicitedOutputPolicyIgnore,
	})
	if err != nil {
		return nil, rp, httpx.Internal(fmt.Errorf("mfa: webauthn config for rp %q: %w", rp.ID, err))
	}
	return w, rp, nil
}

// waUser adapts a user to webauthn.User.
type waUser struct {
	handle []byte
	name   string
	creds  []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { return u.handle }
func (u *waUser) WebAuthnName() string                       { return u.name }
func (u *waUser) WebAuthnDisplayName() string                { return u.name }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// Ceremony is the response of a passkey "begin" endpoint: options is the PublicKeyCredentialCreationOptions or
// PublicKeyCredentialRequestOptions JSON for @simplewebauthn/browser (optionsJSON).
type Ceremony struct {
	ChallengeID string `json:"challenge_id"`
	Options     any    `json:"options"`
}

type pendingRegistration struct {
	UserID  uint64
	RPID    string
	Name    string
	Session webauthn.SessionData
}

type pendingAssertion struct {
	UserID  uint64 // 0 for a discoverable (usernameless) login
	RPID    string
	Session webauthn.SessionData
}

// PasskeyView is the API shape of a passkey.
type PasskeyView struct {
	ID             uint64     `json:"id"`
	Name           string     `json:"name"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	BackupEligible bool       `json:"backup_eligible"`
	Transports     []string   `json:"transports"`
}

// ViewOf returns the API view of p.
func ViewOf(p *model.Passkey) PasskeyView {
	v := PasskeyView{
		ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt.UTC(), BackupEligible: p.BackupEligible,
		Transports: append([]string{}, p.Transports...),
	}
	if p.LastUsedAt != nil {
		t := p.LastUsedAt.UTC()
		v.LastUsedAt = &t
	}
	return v
}

func credentialHash(id []byte) string {
	sum := sha256.Sum256(id)
	return hex.EncodeToString(sum[:])
}

// handle returns the WebAuthn user handle of userID, creating it (16 random bytes, stable for the account) when
// create is true. nil without error when it does not exist and create is false.
func (s *Service) handle(ctx context.Context, userID uint64, create bool) ([]byte, error) {
	row, err := s.load(ctx, s.a.DB, userID)
	if err != nil {
		return nil, err
	}
	if row == nil || row.WebAuthnHandle == nil {
		if !create {
			return nil, nil
		}
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("mfa: user handle: %w", err)
		}
		if err := ensureRow(ctx, s.a.DB, userID); err != nil {
			return nil, err
		}
		h := hex.EncodeToString(b)
		if err := s.a.DB.WithContext(ctx).Model(&model.UserMFA{}).
			Where("user_id = ? AND webauthn_handle IS NULL", userID).Update("webauthn_handle", h).Error; err != nil {
			return nil, fmt.Errorf("mfa: store user handle: %w", err)
		}
		if row, err = s.load(ctx, s.a.DB, userID); err != nil { // a concurrent request may have won
			return nil, err
		}
		if row == nil || row.WebAuthnHandle == nil {
			return nil, errors.New("mfa: user handle was not stored")
		}
	}
	return hex.DecodeString(*row.WebAuthnHandle)
}

// passkeys returns the passkeys of userID registered for rpID and their webauthn credentials (same order).
func (s *Service) passkeys(ctx context.Context, userID uint64, rpID string) ([]model.Passkey, []webauthn.Credential, error) {
	var rows []model.Passkey
	if err := s.a.DB.WithContext(ctx).Where("user_id = ? AND rp_id = ?", userID, rpID).Order("id").Find(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("mfa: load passkeys: %w", err)
	}
	creds := make([]webauthn.Credential, len(rows))
	for i := range rows {
		if err := json.Unmarshal([]byte(rows[i].Credential), &creds[i]); err != nil {
			return nil, nil, fmt.Errorf("mfa: decode passkey %d: %w", rows[i].ID, err)
		}
		// The columns are the source of truth for the values that change at every login.
		creds[i].Authenticator.SignCount = uint32(rows[i].SignCount)
		creds[i].Flags.BackupState = rows[i].BackupState
	}
	return rows, creds, nil
}

// defaultPasskeyName labels a new passkey from its attachment and transports.
func defaultPasskeyName(c *webauthn.Credential) string {
	has := func(t protocol.AuthenticatorTransport) bool { return slices.Contains(c.Transport, t) }
	switch {
	case c.Authenticator.Attachment == protocol.Platform || has(protocol.Internal):
		if c.Flags.BackupEligible {
			return "Synced passkey"
		}
		return "Device passkey"
	case has(protocol.Hybrid):
		return "Phone passkey"
	case c.Authenticator.Attachment == protocol.CrossPlatform || has(protocol.USB) || has(protocol.NFC) || has(protocol.BLE):
		return "Security key"
	}
	return "Passkey"
}

func cleanName(name string) string {
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > MaxPasskeyName {
		name = string(r[:MaxPasskeyName])
	}
	return name
}

// BeginRegistration starts adding a passkey for u: a discoverable credential (resident key required, user
// verification preferred) that excludes the user's existing passkeys. name is the optional label to store.
func (s *Service) BeginRegistration(ctx context.Context, u *model.User, name string) (Ceremony, error) {
	w, rp, err := s.webAuthn()
	if err != nil {
		return Ceremony{}, err
	}
	h, err := s.handle(ctx, u.ID, true)
	if err != nil {
		return Ceremony{}, err
	}
	_, creds, err := s.passkeys(ctx, u.ID, rp.ID)
	if err != nil {
		return Ceremony{}, err
	}
	exclude := make([]protocol.CredentialDescriptor, len(creds))
	for i := range creds {
		exclude[i] = creds[i].Descriptor()
	}
	creation, session, err := w.BeginRegistration(&waUser{handle: h, name: u.Email, creds: creds},
		webauthn.WithExclusions(exclude),
		webauthn.WithAuthenticatorSelection(selection),
		webauthn.WithExtensions(webauthn.WithExtensionCredProps()),
	)
	if err != nil {
		return Ceremony{}, httpx.Internal(fmt.Errorf("mfa: begin registration: %w", err))
	}
	id := randomID(24)
	s.a.KV.Set(kvRegister+id, pendingRegistration{UserID: u.ID, RPID: rp.ID, Name: cleanName(name), Session: *session}, ChallengeTTL)
	return Ceremony{ChallengeID: id, Options: creation.Response}, nil
}

// FinishRegistration verifies the attestation (credential is the RegistrationResponseJSON of
// startRegistration) and stores the passkey. name overrides the name given to BeginRegistration; the default
// is derived from the authenticator. Bad challenges and credentials are 400; an already registered credential
// is 409.
func (s *Service) FinishRegistration(ctx context.Context, u *model.User, challengeID string, credential json.RawMessage, name string) (PasskeyView, error) {
	v, ok := s.a.KV.Take(kvRegister + challengeID)
	p, _ := v.(pendingRegistration)
	if !ok || challengeID == "" || p.UserID != u.ID {
		return PasskeyView{}, httpx.FieldError("challenge_id", "invalid or expired challenge")
	}
	w, rp, err := s.webAuthn()
	if err != nil {
		return PasskeyView{}, err
	}
	if rp.ID != p.RPID {
		return PasskeyView{}, httpx.FieldError("challenge_id", "invalid or expired challenge")
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(credential)
	if err != nil {
		return PasskeyView{}, httpx.FieldError("credential", "malformed passkey response").WithCause(err)
	}
	h, err := s.handle(ctx, u.ID, false)
	if err != nil {
		return PasskeyView{}, err
	}
	cred, err := w.CreateCredential(&waUser{handle: h, name: u.Email}, p.Session, parsed)
	if err != nil {
		s.a.Log.Warn("mfa: passkey registration rejected", "user_id", u.ID, "err", protocolDetail(err))
		return PasskeyView{}, httpx.FieldError("credential", "passkey verification failed").WithCause(err)
	}
	enc, err := json.Marshal(cred)
	if err != nil {
		return PasskeyView{}, fmt.Errorf("mfa: encode credential: %w", err)
	}
	label := cleanName(name)
	if label == "" {
		label = p.Name
	}
	if label == "" {
		label = defaultPasskeyName(cred)
	}
	transports := make(model.JSONSlice[string], 0, len(cred.Transport))
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	row := model.Passkey{
		UserID:         u.ID,
		RPID:           rp.ID,
		CredentialHash: credentialHash(cred.ID),
		CredentialID:   base64.RawURLEncoding.EncodeToString(cred.ID),
		Credential:     string(enc),
		Name:           label,
		SignCount:      int64(cred.Authenticator.SignCount),
		BackupEligible: cred.Flags.BackupEligible,
		BackupState:    cred.Flags.BackupState,
		Transports:     transports,
	}
	if err := s.a.DB.WithContext(ctx).Create(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return PasskeyView{}, httpx.Conflict("this passkey is already registered")
		}
		return PasskeyView{}, fmt.Errorf("mfa: store passkey: %w", err)
	}
	s.changed(u.ID)
	return ViewOf(&row), nil
}

// protocolDetail returns the developer detail of a go-webauthn error for logs.
func protocolDetail(err error) string {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return strings.TrimSpace(pe.Details + " " + pe.DevInfo)
	}
	return err.Error()
}

// BeginLogin starts a passkey login. Without email it is a discoverable (usernameless) login with an empty
// allowCredentials list. With email the user's passkeys are listed; an unknown email, or one without passkeys,
// gets the discoverable options so the response does not reveal whether the account exists.
func (s *Service) BeginLogin(ctx context.Context, email string) (Ceremony, error) {
	w, rp, err := s.webAuthn()
	if err != nil {
		return Ceremony{}, err
	}
	var (
		userID    uint64
		assertion *protocol.CredentialAssertion
		session   *webauthn.SessionData
	)
	if email = strings.TrimSpace(email); email != "" {
		user, id, err := s.loginUser(ctx, email, rp.ID)
		if err != nil {
			return Ceremony{}, err
		}
		if user != nil {
			if assertion, session, err = w.BeginLogin(user); err != nil {
				return Ceremony{}, httpx.Internal(fmt.Errorf("mfa: begin login: %w", err))
			}
			userID = id
		}
	}
	if assertion == nil {
		if assertion, session, err = w.BeginDiscoverableLogin(); err != nil {
			return Ceremony{}, httpx.Internal(fmt.Errorf("mfa: begin discoverable login: %w", err))
		}
	}
	id := randomID(24)
	s.a.KV.Set(kvAssertion+id, pendingAssertion{UserID: userID, RPID: rp.ID, Session: *session}, ChallengeTTL)
	return Ceremony{ChallengeID: id, Options: assertion.Response}, nil
}

// loginUser returns the webauthn user of email when it exists and has passkeys for rpID (nil otherwise).
func (s *Service) loginUser(ctx context.Context, email, rpID string) (*waUser, uint64, error) {
	u, err := s.users.FindByEmail(ctx, email)
	if errors.Is(err, usersvc.ErrNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	user, _, err := s.assertionUser(ctx, u.ID, rpID)
	if err != nil || user == nil {
		return nil, 0, err
	}
	user.name = u.Email
	return user, u.ID, nil
}

// assertionUser loads the handle and passkeys of userID for rpID; nil when it has no handle or no passkeys.
func (s *Service) assertionUser(ctx context.Context, userID uint64, rpID string) (*waUser, []model.Passkey, error) {
	h, err := s.handle(ctx, userID, false)
	if err != nil || h == nil {
		return nil, nil, err
	}
	rows, creds, err := s.passkeys(ctx, userID, rpID)
	if err != nil || len(rows) == 0 {
		return nil, nil, err
	}
	return &waUser{handle: h, creds: creds}, rows, nil
}

func errPasskeyFailed() *httpx.Error { return httpx.Unauthorized("passkey verification failed") }

// FinishLogin verifies a passkey assertion (credential is the AuthenticationResponseJSON of
// startAuthentication) and returns the user. The challenge is single use. Unknown or expired challenges and
// failed verifications (including a sign counter that did not increase) are 401; banned users are 403.
func (s *Service) FinishLogin(ctx context.Context, challengeID string, credential json.RawMessage) (*model.User, error) {
	v, ok := s.a.KV.Take(kvAssertion + challengeID)
	p, _ := v.(pendingAssertion)
	if !ok || challengeID == "" {
		return nil, httpx.Unauthorized("passkey challenge expired, please try again")
	}
	w, rp, err := s.webAuthn()
	if err != nil {
		return nil, err
	}
	if rp.ID != p.RPID {
		return nil, httpx.Unauthorized("passkey challenge expired, please try again")
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(credential)
	if err != nil {
		return nil, httpx.FieldError("credential", "malformed passkey response").WithCause(err)
	}
	var (
		ownerID uint64
		rows    []model.Passkey
		cred    *webauthn.Credential
	)
	if p.UserID != 0 {
		user, r, lerr := s.assertionUser(ctx, p.UserID, rp.ID)
		if lerr != nil {
			return nil, lerr
		}
		if user == nil {
			return nil, errPasskeyFailed()
		}
		ownerID, rows = p.UserID, r
		cred, err = w.ValidateLogin(user, p.Session, parsed)
	} else {
		handler := func(_, userHandle []byte) (webauthn.User, error) {
			var row model.UserMFA
			if err := s.a.DB.WithContext(ctx).Where("webauthn_handle = ?", hex.EncodeToString(userHandle)).
				First(&row).Error; err != nil {
				return nil, fmt.Errorf("unknown user handle: %w", err)
			}
			user, r, err := s.assertionUser(ctx, row.UserID, rp.ID)
			if err != nil {
				return nil, err
			}
			if user == nil {
				return nil, errors.New("user has no passkeys")
			}
			ownerID, rows = row.UserID, r
			return user, nil
		}
		_, cred, err = w.ValidatePasskeyLogin(handler, p.Session, parsed)
	}
	if err != nil {
		s.a.Log.Warn("mfa: passkey login rejected", "user_id", ownerID, "err", protocolDetail(err))
		return nil, errPasskeyFailed()
	}
	idx := slices.IndexFunc(rows, func(r model.Passkey) bool { return r.CredentialHash == credentialHash(cred.ID) })
	if idx < 0 {
		return nil, errPasskeyFailed()
	}
	row := rows[idx]
	if cred.Authenticator.CloneWarning {
		s.a.Log.Warn("mfa: passkey sign counter did not increase (possible clone)", "user_id", ownerID,
			"passkey_id", row.ID, "stored", row.SignCount)
		return nil, errPasskeyFailed()
	}
	if err := s.recordUse(ctx, &row, cred); err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, ownerID)
	if errors.Is(err, usersvc.ErrNotFound) {
		return nil, errPasskeyFailed()
	}
	if err != nil {
		return nil, err
	}
	if u.Banned {
		return nil, httpx.Forbidden("account is banned")
	}
	return u, nil
}

// recordUse stores the new sign counter, backup state and last use of row. The update is conditional on the
// stored counter, so two assertions carrying the same counter cannot both succeed.
func (s *Service) recordUse(ctx context.Context, row *model.Passkey, cred *webauthn.Credential) error {
	enc, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("mfa: encode credential: %w", err)
	}
	now := time.Now()
	res := s.a.DB.WithContext(ctx).Model(&model.Passkey{}).
		Where("id = ? AND sign_count = ?", row.ID, row.SignCount).
		Updates(map[string]any{
			"sign_count":   int64(cred.Authenticator.SignCount),
			"backup_state": cred.Flags.BackupState,
			"credential":   string(enc),
			"last_used_at": now,
		})
	if res.Error != nil {
		return fmt.Errorf("mfa: record passkey use: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return errPasskeyFailed() // deleted or used concurrently
	}
	return nil
}

// ListPasskeys returns the passkeys of userID (every RP id), oldest first.
func (s *Service) ListPasskeys(ctx context.Context, userID uint64) ([]PasskeyView, error) {
	var rows []model.Passkey
	if err := s.a.DB.WithContext(ctx).Where("user_id = ?", userID).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("mfa: list passkeys: %w", err)
	}
	out := make([]PasskeyView, len(rows))
	for i := range rows {
		out[i] = ViewOf(&rows[i])
	}
	return out, nil
}

// ownPasskey loads passkey id of userID (404 when missing or owned by someone else).
func (s *Service) ownPasskey(ctx context.Context, userID, id uint64) (*model.Passkey, error) {
	var row model.Passkey
	err := s.a.DB.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, httpx.NotFound("passkey not found")
	}
	if err != nil {
		return nil, fmt.Errorf("mfa: load passkey: %w", err)
	}
	return &row, nil
}

// RenamePasskey renames passkey id of userID.
func (s *Service) RenamePasskey(ctx context.Context, userID, id uint64, name string) (PasskeyView, error) {
	row, err := s.ownPasskey(ctx, userID, id)
	if err != nil {
		return PasskeyView{}, err
	}
	if name = cleanName(name); name == "" {
		return PasskeyView{}, httpx.FieldError("name", "is required")
	}
	if err := s.a.DB.WithContext(ctx).Model(&model.Passkey{}).Where("id = ?", row.ID).Update("name", name).Error; err != nil {
		return PasskeyView{}, fmt.Errorf("mfa: rename passkey: %w", err)
	}
	row.Name = name
	return ViewOf(row), nil
}

// DeletePasskey removes passkey id of u after verifying the password.
func (s *Service) DeletePasskey(ctx context.Context, u *model.User, id uint64, password string) error {
	if err := s.checkPassword(u, password); err != nil {
		return err
	}
	row, err := s.ownPasskey(ctx, u.ID, id)
	if err != nil {
		return err
	}
	if err := s.a.DB.WithContext(ctx).Where("id = ?", row.ID).Delete(&model.Passkey{}).Error; err != nil {
		return fmt.Errorf("mfa: delete passkey: %w", err)
	}
	s.changed(u.ID)
	return nil
}

// AppBaseURL returns app_url without a trailing slash when it is an absolute http(s) URL, else "".
func AppBaseURL(st *settings.Service) string {
	raw := strings.TrimRight(strings.TrimSpace(st.String("app_url")), "/")
	if origin, _ := parseOrigin(raw); origin == "" || !strings.Contains(raw, "://") {
		return ""
	}
	return raw
}
