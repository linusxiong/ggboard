package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func cheap(t *testing.T) {
	t.Helper()
	old := CurrentParams()
	SetParams(Params{Memory: 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16})
	t.Cleanup(func() { SetParams(old) })
}

func TestArgon2HashVerify(t *testing.T) {
	cheap(t)
	h, err := HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=1024,t=1,p=1$") {
		t.Fatalf("hash = %s", h)
	}
	if !VerifyPassword(h, "password123") || VerifyPassword(h, "password124") {
		t.Fatal("verify")
	}
	h2, _ := HashPassword("password123")
	if h == h2 {
		t.Fatal("salts must differ")
	}
	if NeedsRehash(h) {
		t.Fatal("current params must not need rehash")
	}
	SetParams(Params{Memory: 2048, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16})
	if !NeedsRehash(h) || !VerifyPassword(h, "password123") {
		t.Fatal("old params: verify still works, rehash needed")
	}
	for _, bad := range []string{"$argon2id$v=19$m=1,t=1$x$y", "$argon2id$v=18$m=1024,t=1,p=1$c2FsdA$a2V5", "$argon2id$v=19$m=1024,t=1,p=1$!!$a2V5"} {
		if VerifyPassword(bad, "x") || !NeedsRehash(bad) {
			t.Errorf("malformed %q must not verify", bad)
		}
	}
}

func TestDefaultParams(t *testing.T) {
	if DefaultParams.Memory != 64*1024 || DefaultParams.Time != 3 || DefaultParams.Threads != 4 {
		t.Fatalf("default params = %+v", DefaultParams)
	}
}

func TestBcryptVariants(t *testing.T) {
	cheap(t)
	b, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	h := string(b) // $2a$
	for _, prefix := range []string{"$2a$", "$2b$", "$2y$"} {
		v := prefix + h[4:]
		if !VerifyPassword(v, "password123") {
			t.Errorf("%s must verify", prefix)
		}
		if VerifyPassword(v, "wrong") {
			t.Errorf("%s wrong password verified", prefix)
		}
		if !NeedsRehash(v) {
			t.Errorf("%s must need rehash", prefix)
		}
	}
}

func TestXboardLegacyHashes(t *testing.T) {
	cases := []struct {
		name, hash string
		want       bool
	}{
		{"md5", "xb$md5$$482c811da5d5b4bc6d497ffa98491e38", true},
		{"md5 uppercase hex", "xb$md5$$482C811DA5D5B4BC6D497FFA98491E38", true},
		{"sha256", "xb$sha256$$ef92b778bafe771e89245b89ecbc08a44a4e166c06659911881f383d4473e94f", true},
		// md5("password123abc$12"); the briefing's 3d6431cb… is md5("password123abc") (zsh expanded "$12").
		{"md5salt with $ in salt", "xb$md5salt$abc$12$1b84a6e599feeab25ace9f402aaa4858", true},
		{"md5salt", "xb$md5salt$abc$3d6431cb02ae9833ac1e21a688bf2e81", true},
		{"sha256salt", "xb$sha256salt$s4lt$" + sha256hex("password123s4lt"), true},
		{"wrong md5", "xb$md5$$482c811da5d5b4bc6d497ffa98491e39", false},
		{"wrong salt", "xb$md5salt$abc$13$3d6431cb02ae9833ac1e21a688bf2e81", false},
		{"unknown algo", "xb$sha1$$cbfdac6008f9cab4083784cbd1874f76618d2a97", false},
		{"truncated", "xb$md5", false},
		{"no hash separator", "xb$md5salt", false},
		{"unusable", "!xb-unusable", false},
		{"unusable bcrypt", "!$2y$10$abcdefghijklmnopqrstuu", false},
		{"empty", "", false},
		{"garbage", "plaintext", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifyPassword(tc.hash, "password123"); got != tc.want {
				t.Fatalf("VerifyPassword(%q) = %v, want %v", tc.hash, got, tc.want)
			}
			if !NeedsRehash(tc.hash) {
				t.Fatal("legacy hashes always need a rehash")
			}
		})
	}
}
