// Package auth implements password hashing (argon2id; bcrypt and Xboard legacy hashes verified for
// imported users), JWT sessions, gin authentication middleware and one-time login tokens.
package auth

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Params are the argon2id cost parameters.
type Params struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
	KeyLen  uint32
	SaltLen uint32
}

// DefaultParams: 64 MiB, 3 passes, 4 lanes, 32-byte key, 16-byte salt.
var DefaultParams = Params{Memory: 64 * 1024, Time: 3, Threads: 4, KeyLen: 32, SaltLen: 16}

var params atomic.Pointer[Params]

func init() { p := DefaultParams; params.Store(&p) }

// SetParams replaces the hashing parameters. Only tests should call it (testutil uses cheap parameters).
func SetParams(p Params) { params.Store(&p) }

// CurrentParams returns the parameters used for new hashes.
func CurrentParams() Params { return *params.Load() }

// argon2 allocates Params.Memory (64 MiB by default) per call: without a bound, a burst of logins (30 per IP
// are allowed) needs gigabytes. hashSlots caps concurrent derivations (SetConcurrency, config
// auth.argon2_concurrency); callers queue for a slot, so a burst waits instead of taking memory.
var (
	hashSlots atomic.Pointer[chan struct{}]
	argon2Key = argon2.IDKey // replaced in tests
)

func init() { SetConcurrency(0) }

// MaxDefaultConcurrency caps the default number of concurrent argon2 derivations: 4 × 64 MiB = 256 MiB in flight.
// One derivation already runs DefaultParams.Threads (4) lanes in parallel, so 4 slots can keep up to 16 cores busy.
const MaxDefaultConcurrency = 4

// DefaultConcurrency is min(GOMAXPROCS, MaxDefaultConcurrency): GOMAXPROCS follows a container's CPU limit, and
// more slots than cores would only add memory, not throughput.
func DefaultConcurrency() int { return max(1, min(runtime.GOMAXPROCS(0), MaxDefaultConcurrency)) }

// SetConcurrency sets how many argon2 derivations (password verification and hashing) may run at once; n <= 0
// selects DefaultConcurrency. Peak argon2 memory is about n × Params.Memory. Derivations already holding a slot
// finish under the previous limit.
func SetConcurrency(n int) {
	if n <= 0 {
		n = DefaultConcurrency()
	}
	ch := make(chan struct{}, n)
	hashSlots.Store(&ch)
}

// Concurrency returns the current limit on concurrent argon2 derivations.
func Concurrency() int { return cap(*hashSlots.Load()) }

// Bounds on the parameters of stored argon2id hashes: a hash asking for more is rejected, not computed.
const (
	maxMemory  = 1 << 20 // KiB (1 GiB)
	maxPasses  = 64
	maxThreads = 64
	maxKeyLen  = 1024
)

func deriveKey(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte {
	slots := *hashSlots.Load() // release the slot taken, even if SetConcurrency swaps the channel meanwhile
	slots <- struct{}{}
	defer func() { <-slots }()
	return argon2Key(password, salt, time, memory, threads, keyLen)
}

// ErrUnsupportedHash reports a malformed or unknown password hash.
var ErrUnsupportedHash = errors.New("auth: unsupported password hash")

// HashPassword returns an argon2id PHC string: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>.
func HashPassword(password string) (string, error) {
	p := CurrentParams()
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key := deriveKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.Memory, p.Time, p.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword checks password against an argon2id, bcrypt ($2y$/$2a$/$2b$) or Xboard legacy
// (xb$<algo>$<salt>$<hex>) hash. Hashes starting with "!" never verify.
func VerifyPassword(hash, password string) bool {
	switch {
	case hash == "" || strings.HasPrefix(hash, "!"):
		return false
	case strings.HasPrefix(hash, "$argon2id$"):
		ok, _ := verifyArgon2(hash, password)
		return ok
	case isBcrypt(hash):
		h := hash
		if strings.HasPrefix(h, "$2y$") {
			h = "$2a$" + h[4:] // identical algorithm; PHP uses the 2y prefix
		}
		return bcrypt.CompareHashAndPassword([]byte(h), []byte(password)) == nil
	case strings.HasPrefix(hash, "xb$"):
		return verifyXboard(hash, password)
	}
	return false
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// VerifyDummy spends the time of a real VerifyPassword without any account: login calls it for unknown emails
// so response timing does not reveal which emails are registered.
func VerifyDummy(password string) {
	dummyOnce.Do(func() { dummyHash, _ = HashPassword("ggboard: no such account") })
	VerifyPassword(dummyHash, password)
}

// NeedsRehash reports whether hash should be replaced by HashPassword after a successful login
// (bcrypt, Xboard legacy, or argon2id with different parameters). Rehashing must not bump password_version.
func NeedsRehash(hash string) bool {
	if !strings.HasPrefix(hash, "$argon2id$") {
		return true
	}
	p, _, _, err := decodeArgon2(hash)
	if err != nil {
		return true
	}
	cur := CurrentParams()
	return p.Memory != cur.Memory || p.Time != cur.Time || p.Threads != cur.Threads || p.KeyLen != cur.KeyLen
}

func isBcrypt(h string) bool {
	return strings.HasPrefix(h, "$2y$") || strings.HasPrefix(h, "$2a$") || strings.HasPrefix(h, "$2b$")
}

func decodeArgon2(hash string) (p Params, salt, key []byte, err error) {
	parts := strings.Split(hash, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[1] != "argon2id" {
		return p, nil, nil, ErrUnsupportedHash
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return p, nil, nil, ErrUnsupportedHash
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return p, nil, nil, ErrUnsupportedHash
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return p, nil, nil, ErrUnsupportedHash
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return p, nil, nil, ErrUnsupportedHash
	}
	p.SaltLen, p.KeyLen = uint32(len(salt)), uint32(len(key))
	if p.Threads == 0 || p.Time == 0 || p.KeyLen == 0 || p.KeyLen > maxKeyLen ||
		p.Memory > maxMemory || p.Time > maxPasses || p.Threads > maxThreads {
		return p, nil, nil, ErrUnsupportedHash
	}
	return p, salt, key, nil
}

func verifyArgon2(hash, password string) (bool, error) {
	p, salt, key, err := decodeArgon2(hash)
	if err != nil {
		return false, err
	}
	got := deriveKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return subtle.ConstantTimeCompare(got, key) == 1, nil
}

// verifyXboard checks "xb$<algo>$<salt>$<hexhash>" produced by the Xboard importer. The salt is everything
// between the algo and the last "$" (may be empty or contain "$").
func verifyXboard(hash, password string) bool {
	rest := strings.TrimPrefix(hash, "xb$")
	algo, rest, ok := strings.Cut(rest, "$")
	if !ok {
		return false
	}
	i := strings.LastIndex(rest, "$")
	if i < 0 {
		return false
	}
	salt, want := rest[:i], strings.ToLower(rest[i+1:])
	var got string
	switch algo {
	case "md5":
		s := md5.Sum([]byte(password))
		got = hex.EncodeToString(s[:])
	case "sha256":
		s := sha256.Sum256([]byte(password))
		got = hex.EncodeToString(s[:])
	case "md5salt":
		s := md5.Sum([]byte(password + salt))
		got = hex.EncodeToString(s[:])
	case "sha256salt":
		s := sha256.Sum256([]byte(password + salt))
		got = hex.EncodeToString(s[:])
	default:
		return false
	}
	return len(want) == len(got) && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
