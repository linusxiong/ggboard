package auth

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

// Review 2026-10-22: argon2 derivations allocate 64 MiB each and were unbounded; they now queue for hashSlots.
func TestReviewArgon2Concurrency(t *testing.T) {
	SetParams(Params{Memory: 64, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16})
	t.Cleanup(func() { SetParams(DefaultParams); argon2Key = argon2.IDKey })
	var cur, peak atomic.Int32
	argon2Key = func(pw, salt []byte, time_, mem uint32, th uint8, kl uint32) []byte {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		cur.Add(-1)
		return argon2.IDKey(pw, salt, time_, mem, th, kl)
	}
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 * cap(hashSlots) {
		wg.Go(func() { VerifyPassword(hash, "wrong") })
	}
	wg.Wait()
	if p := peak.Load(); p > int32(cap(hashSlots)) {
		t.Fatalf("peak concurrent derivations = %d, limit %d", p, cap(hashSlots))
	}
}

// Review 2026-10-22: a stored hash cannot make the server allocate arbitrary memory.
func TestReviewArgon2RejectsHugeParams(t *testing.T) {
	if VerifyPassword("$argon2id$v=19$m=4194304,t=3,p=4$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g", "x") {
		t.Fatal("must not verify")
	}
	if _, _, _, err := decodeArgon2("$argon2id$v=19$m=4194304,t=3,p=4$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g"); err == nil {
		t.Fatal("4 GiB memory parameter accepted")
	}
}
