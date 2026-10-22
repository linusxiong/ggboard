// Package mfatest provides a software WebAuthn authenticator (ES256, "none" attestation) and TOTP helpers so
// tests can run passkey and two-factor ceremonies end to end without a browser. Test code only.
package mfatest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	"github.com/pquerna/otp/totp"
)

var b64 = base64.RawURLEncoding

// Authenticator is a software passkey holding one ES256 credential.
type Authenticator struct {
	RPID         string // RP id whose hash goes into authenticatorData
	Origin       string // origin written into clientDataJSON
	Key          *ecdsa.PrivateKey
	CredentialID []byte
	UserHandle   []byte                      // learned from the creation options (user.id) by Register
	SignCount    uint32                      // incremented before every assertion (set it back to test regressions)
	Flags        protocol.AuthenticatorFlags // extra flags, e.g. FlagBackupEligible|FlagBackupState
	Attachment   string                      // authenticatorAttachment: "platform" or "cross-platform"
	Transports   []string
}

// New returns an authenticator for rpID used from origin, with a fresh key pair and credential id.
func New(t testing.TB, rpID, origin string) *Authenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("mfatest: key: %v", err)
	}
	id := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		t.Fatalf("mfatest: credential id: %v", err)
	}
	return &Authenticator{
		RPID: rpID, Origin: origin, Key: key, CredentialID: id,
		Attachment: "platform", Transports: []string{"internal", "hybrid"},
	}
}

// ID returns the base64url credential id.
func (a *Authenticator) ID() string { return b64.EncodeToString(a.CredentialID) }

func (a *Authenticator) clientData(t testing.TB, typ, challenge string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": a.Origin, "crossOrigin": false})
	if err != nil {
		t.Fatalf("mfatest: client data: %v", err)
	}
	return b
}

func (a *Authenticator) authData(flags protocol.AuthenticatorFlags, count uint32, attested []byte) []byte {
	h := sha256.Sum256([]byte(a.RPID))
	out := make([]byte, 0, 37+len(attested))
	out = append(out, h[:]...)
	out = append(out, byte(flags|a.Flags))
	out = binary.BigEndian.AppendUint32(out, count)
	return append(out, attested...)
}

func (a *Authenticator) coseKey(t testing.TB) []byte {
	t.Helper()
	pub := a.Key.PublicKey
	x, y := make([]byte, 32), make([]byte, 32)
	raw, err := pub.Bytes() // uncompressed point 0x04 || X || Y
	if err != nil {
		t.Fatalf("mfatest: public key: %v", err)
	}
	copy(x, raw[1:33])
	copy(y, raw[33:65])
	b, err := webauthncbor.Marshal(map[int64]any{
		1:  int64(webauthncose.EllipticKey),
		3:  int64(webauthncose.AlgES256),
		-1: int64(webauthncose.P256),
		-2: x,
		-3: y,
	})
	if err != nil {
		t.Fatalf("mfatest: cose key: %v", err)
	}
	return b
}

// Register answers PublicKeyCredentialCreationOptions JSON (the "options" of /user/security/passkeys/begin)
// with a RegistrationResponseJSON like @simplewebauthn/browser startRegistration produces.
func (a *Authenticator) Register(t testing.TB, options json.RawMessage) json.RawMessage {
	t.Helper()
	var opts struct {
		Challenge string `json:"challenge"`
		User      struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(options, &opts); err != nil {
		t.Fatalf("mfatest: creation options: %v", err)
	}
	handle, err := b64.DecodeString(opts.User.ID)
	if err != nil {
		t.Fatalf("mfatest: user.id: %v", err)
	}
	a.UserHandle = handle
	attested := make([]byte, 16, 16+2+len(a.CredentialID)+80) // zero AAGUID
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(a.CredentialID)))
	attested = append(attested, a.CredentialID...)
	attested = append(attested, a.coseKey(t)...)
	authData := a.authData(protocol.FlagUserPresent|protocol.FlagUserVerified|protocol.FlagAttestedCredentialData, a.SignCount, attested)
	attObj, err := webauthncbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": authData})
	if err != nil {
		t.Fatalf("mfatest: attestation object: %v", err)
	}
	cdj := a.clientData(t, "webauthn.create", opts.Challenge)
	return mustJSON(t, map[string]any{
		"id":    a.ID(),
		"rawId": a.ID(),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(cdj),
			"attestationObject": b64.EncodeToString(attObj),
			"transports":        a.Transports,
		},
		"clientExtensionResults":  map[string]any{"credProps": map[string]any{"rk": true}},
		"authenticatorAttachment": a.Attachment,
	})
}

// Assert answers PublicKeyCredentialRequestOptions JSON (the "options" of /passport/passkey/login/begin) with
// an AuthenticationResponseJSON like startAuthentication produces. SignCount is incremented first.
func (a *Authenticator) Assert(t testing.TB, options json.RawMessage) json.RawMessage {
	t.Helper()
	var opts struct {
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(options, &opts); err != nil {
		t.Fatalf("mfatest: request options: %v", err)
	}
	a.SignCount++
	authData := a.authData(protocol.FlagUserPresent|protocol.FlagUserVerified, a.SignCount, nil)
	cdj := a.clientData(t, "webauthn.get", opts.Challenge)
	cdh := sha256.Sum256(cdj)
	digest := sha256.Sum256(append(append([]byte{}, authData...), cdh[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.Key, digest[:])
	if err != nil {
		t.Fatalf("mfatest: sign: %v", err)
	}
	return mustJSON(t, map[string]any{
		"id":    a.ID(),
		"rawId": a.ID(),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64.EncodeToString(cdj),
			"authenticatorData": b64.EncodeToString(authData),
			"signature":         b64.EncodeToString(sig),
			"userHandle":        b64.EncodeToString(a.UserHandle),
		},
		"clientExtensionResults":  map[string]any{},
		"authenticatorAttachment": a.Attachment,
	})
}

func mustJSON(t testing.TB, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mfatest: %v", err)
	}
	return b
}

// TOTP returns the code of secret (base32) at time at.
func TOTP(t testing.TB, secret string, at time.Time) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, at)
	if err != nil {
		t.Fatalf("mfatest: totp: %v", err)
	}
	return code
}

// FixClock points clock (e.g. &mfasvc.TOTPNow) at a controllable time starting at start and restores it when
// the test ends. Advance the returned pointer to move time.
func FixClock(t testing.TB, clock *func() time.Time, start time.Time) *time.Time {
	t.Helper()
	now, prev := start, *clock
	*clock = func() time.Time { return now }
	t.Cleanup(func() { *clock = prev })
	return &now
}
