package user_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
	"github.com/linusxiong/ggboard/internal/service/mfa/mfatest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const sec = base + "/security"

type ceremony struct {
	ChallengeID string          `json:"challenge_id"`
	Options     json.RawMessage `json:"options"`
}

func expectStatus(t *testing.T, rec interface{ Result() *http.Response }, want int, field string) {
	t.Helper()
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()
	var e struct {
		Code   string              `json:"code"`
		Errors map[string][]string `json:"errors"`
	}
	_ = json.NewDecoder(res.Body).Decode(&e)
	if res.StatusCode != want || (field != "" && len(e.Errors[field]) == 0) {
		t.Fatalf("status = %d (want %d), field %q missing in %+v", res.StatusCode, want, field, e)
	}
}

func TestSecurityTOTPEndpoints(t *testing.T) {
	a, r := setup(t, testutil.WithSettings(map[string]any{"app_name": "Example"}))
	now := mfatest.FixClock(t, &mfasvc.TOTPNow, time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC))
	u := testutil.CreateUser(t, a, "alice@example.com")
	tok := testutil.Login(t, a, u)

	ov := testutil.Decode[mfasvc.Overview](t, testutil.Do(t, r, http.MethodGet, sec, nil, tok))
	if ov.TOTPEnabled || ov.TOTPEnabledAt != nil || ov.BackupCodesRemaining != 0 || ov.Passkeys == nil || len(ov.Passkeys) != 0 {
		t.Fatalf("initial overview = %+v", ov)
	}
	raw := testutil.Decode[map[string]json.RawMessage](t, testutil.Do(t, r, http.MethodGet, sec, nil, tok))
	for _, k := range []string{"totp_enabled", "totp_enabled_at", "backup_codes_remaining", "passkeys"} {
		if raw[k] == nil {
			t.Fatalf("missing %s in %v", k, raw)
		}
	}

	setup := testutil.Decode[mfasvc.TOTPSetup](t, testutil.Do(t, r, http.MethodPost, sec+"/totp/setup", nil, tok))
	if setup.Secret == "" || !strings.HasPrefix(setup.OTPAuthURL, "otpauth://totp/Example:alice@example.com?") {
		t.Fatalf("setup = %+v", setup)
	}
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/totp/enable", map[string]any{}, tok), 400, "code")
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/totp/enable", map[string]any{"code": "000000"}, tok), 400, "code")
	en := testutil.Decode[mfasvc.BackupCodes](t, testutil.Do(t, r, http.MethodPost, sec+"/totp/enable",
		map[string]any{"code": mfatest.TOTP(t, setup.Secret, *now)}, tok))
	if len(en.BackupCodes) != 10 {
		t.Fatalf("backup codes = %v", en.BackupCodes)
	}
	// Sessions survive enabling (password_version unchanged).
	if reload(t, a, u.ID).PasswordVersion != u.PasswordVersion {
		t.Fatal("password_version must not change")
	}
	ov = testutil.Decode[mfasvc.Overview](t, testutil.Do(t, r, http.MethodGet, sec, nil, tok))
	if !ov.TOTPEnabled || ov.TOTPEnabledAt == nil || ov.BackupCodesRemaining != 10 {
		t.Fatalf("enabled overview = %+v", ov)
	}
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/totp/setup", nil, tok), 409, "")

	*now = now.Add(30 * time.Second)
	regen := testutil.Decode[mfasvc.BackupCodes](t, testutil.Do(t, r, http.MethodPost, sec+"/backup-codes",
		map[string]any{"code": en.BackupCodes[0]}, tok))
	if len(regen.BackupCodes) != 10 || slices.Equal(regen.BackupCodes, en.BackupCodes) {
		t.Fatalf("regenerated = %v", regen.BackupCodes)
	}
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/backup-codes", map[string]any{"code": en.BackupCodes[1]}, tok), 400, "code")

	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/totp/disable",
		map[string]any{"password": "wrong-password", "code": regen.BackupCodes[0]}, tok), 400, "password")
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/totp/disable",
		map[string]any{"password": testutil.Password}, tok), 400, "code")
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/totp/disable",
		map[string]any{"password": testutil.Password, "code": "999999"}, tok), 400, "code")
	rec := testutil.Do(t, r, http.MethodPost, sec+"/totp/disable",
		map[string]any{"password": testutil.Password, "code": mfatest.TOTP(t, setup.Secret, *now)}, tok)
	if ok := testutil.Decode[bool](t, rec); !ok {
		t.Fatal("disable")
	}
	ov = testutil.Decode[mfasvc.Overview](t, testutil.Do(t, r, http.MethodGet, sec, nil, tok))
	if ov.TOTPEnabled || ov.BackupCodesRemaining != 0 {
		t.Fatalf("disabled overview = %+v", ov)
	}
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/backup-codes", map[string]any{"code": "123456"}, tok), 409, "")
	expectStatus(t, testutil.Do(t, r, http.MethodGet, sec, nil, ""), 401, "")
}

func TestSecurityPasskeyEndpoints(t *testing.T) {
	a, r := setup(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	u := testutil.CreateUser(t, a, "bob@example.com")
	other := testutil.CreateUser(t, a, "carol@example.com")
	tok, otherTok := testutil.Login(t, a, u), testutil.Login(t, a, other)

	cer := testutil.Decode[ceremony](t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/begin", map[string]any{"name": "Laptop"}, tok))
	var opts struct {
		RP struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"rp"`
		User struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"user"`
		Challenge   string `json:"challenge"`
		Timeout     int    `json:"timeout"`
		Attestation string `json:"attestation"`
		Selection   struct {
			ResidentKey        string `json:"residentKey"`
			RequireResidentKey bool   `json:"requireResidentKey"`
			UserVerification   string `json:"userVerification"`
		} `json:"authenticatorSelection"`
		Params []struct {
			Alg int `json:"alg"`
		} `json:"pubKeyCredParams"`
		Exclude []struct {
			ID string `json:"id"`
		} `json:"excludeCredentials"`
	}
	if err := json.Unmarshal(cer.Options, &opts); err != nil {
		t.Fatal(err)
	}
	if opts.RP.ID != "panel.example.com" || opts.RP.Name != "GG Bond" || opts.User.Name != u.Email || opts.User.ID == "" ||
		opts.Challenge == "" || opts.Timeout != 120000 || opts.Attestation != "none" || opts.Selection.ResidentKey != "required" ||
		!opts.Selection.RequireResidentKey || opts.Selection.UserVerification != "preferred" || len(opts.Params) == 0 ||
		len(opts.Exclude) != 0 {
		t.Fatalf("creation options = %s", cer.Options)
	}

	key := mfatest.New(t, "panel.example.com", "https://panel.example.com")
	credential := key.Register(t, cer.Options)
	// Another user cannot finish this ceremony.
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/finish",
		map[string]any{"challenge_id": cer.ChallengeID, "credential": credential}, otherTok), 400, "challenge_id")
	cer = testutil.Decode[ceremony](t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/begin", map[string]any{"name": "Laptop"}, tok))
	pk := testutil.Decode[mfasvc.PasskeyView](t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/finish",
		map[string]any{"challenge_id": cer.ChallengeID, "credential": key.Register(t, cer.Options)}, tok))
	if pk.ID == 0 || pk.Name != "Laptop" || pk.LastUsedAt != nil || pk.BackupEligible ||
		!slices.Equal(pk.Transports, []string{"internal", "hybrid"}) {
		t.Fatalf("passkey = %+v", pk)
	}
	// The ceremony is single use.
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/finish",
		map[string]any{"challenge_id": cer.ChallengeID, "credential": credential}, tok), 400, "challenge_id")

	// The second registration excludes the first credential, keeps the same user handle, and a synced
	// authenticator gets a default name; registering the same credential again is a conflict.
	cer2 := testutil.Decode[ceremony](t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/begin", nil, tok))
	if err := json.Unmarshal(cer2.Options, &opts); err != nil {
		t.Fatal(err)
	}
	if len(opts.Exclude) != 1 || opts.Exclude[0].ID != key.ID() || opts.User.ID != b64(key.UserHandle) {
		t.Fatalf("second options = %s", cer2.Options)
	}
	dup := testutil.Do(t, r, http.MethodPost, sec+"/passkeys/finish",
		map[string]any{"challenge_id": cer2.ChallengeID, "credential": key.Register(t, cer2.Options)}, tok)
	expectStatus(t, dup, 409, "")
	synced := mfatest.New(t, "panel.example.com", "https://panel.example.com")
	synced.Flags = 0x08 | 0x10 // BE | BS
	cer2 = testutil.Decode[ceremony](t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/begin", nil, tok))
	pk2 := testutil.Decode[mfasvc.PasskeyView](t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/finish",
		map[string]any{"challenge_id": cer2.ChallengeID, "credential": synced.Register(t, cer2.Options)}, tok))
	if pk2.Name != "Synced passkey" || !pk2.BackupEligible {
		t.Fatalf("synced passkey = %+v", pk2)
	}
	// A bad attestation (wrong origin) is rejected.
	evil := mfatest.New(t, "panel.example.com", "https://evil.example.org")
	cer2 = testutil.Decode[ceremony](t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/begin", nil, tok))
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/finish",
		map[string]any{"challenge_id": cer2.ChallengeID, "credential": evil.Register(t, cer2.Options)}, tok), 400, "credential")

	ov := testutil.Decode[mfasvc.Overview](t, testutil.Do(t, r, http.MethodGet, sec, nil, tok))
	if len(ov.Passkeys) != 2 || ov.Passkeys[0].ID != pk.ID || ov.Passkeys[1].ID != pk2.ID {
		t.Fatalf("overview passkeys = %+v", ov.Passkeys)
	}

	// Rename / delete enforce ownership.
	path := sec + "/passkeys/" + itoa(pk.ID)
	expectStatus(t, testutil.Do(t, r, http.MethodPatch, path, map[string]any{"name": "Mine now"}, otherTok), 404, "")
	expectStatus(t, testutil.Do(t, r, http.MethodPatch, path, map[string]any{"name": ""}, tok), 400, "name")
	renamed := testutil.Decode[mfasvc.PasskeyView](t, testutil.Do(t, r, http.MethodPatch, path, map[string]any{"name": " Work laptop "}, tok))
	if renamed.Name != "Work laptop" || renamed.ID != pk.ID {
		t.Fatalf("renamed = %+v", renamed)
	}
	expectStatus(t, testutil.Do(t, r, http.MethodDelete, path, map[string]any{"password": testutil.Password}, otherTok), 404, "")
	expectStatus(t, testutil.Do(t, r, http.MethodDelete, path, map[string]any{"password": "wrong-password"}, tok), 400, "password")
	expectStatus(t, testutil.Do(t, r, http.MethodDelete, path, nil, tok), 400, "password")
	expectStatus(t, testutil.Do(t, r, http.MethodDelete, sec+"/passkeys/999", map[string]any{"password": testutil.Password}, tok), 404, "")
	if ok := testutil.Decode[bool](t, testutil.Do(t, r, http.MethodDelete, path, map[string]any{"password": testutil.Password}, tok)); !ok {
		t.Fatal("delete")
	}
	var n int64
	a.DB.Model(&model.Passkey{}).Where("user_id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Fatalf("passkeys left = %d", n)
	}
	// Sessions stay valid after adding/removing passkeys.
	auth.InvalidateUser(a, u.ID)
	if testutil.Do(t, r, http.MethodGet, sec, nil, tok).Code != 200 {
		t.Fatal("session must survive passkey changes")
	}
}

func TestSecurityPasskeysUnavailable(t *testing.T) {
	a, r := setup(t)
	u := testutil.CreateUser(t, a, "dave@example.com")
	expectStatus(t, testutil.Do(t, r, http.MethodPost, sec+"/passkeys/begin", nil, testutil.Login(t, a, u)), 400, "")
}

func itoa(id uint64) string { return strconv.FormatUint(id, 10) }

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
