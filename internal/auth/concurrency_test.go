package auth

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

// perf 2026-10-23 (docs/perf/README.md, finding 2): the default limit bounds in-flight argon2 memory to
// MaxDefaultConcurrency × 64 MiB however many cores there are, and never exceeds the cores Go may use.
func TestConcurrencyDefault(t *testing.T) {
	prev := runtime.GOMAXPROCS(0)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev); SetConcurrency(0) })
	for _, c := range []struct{ procs, want int }{{1, 1}, {2, 2}, {3, 3}, {4, 4}, {18, 4}, {64, 4}} {
		runtime.GOMAXPROCS(c.procs)
		if got := DefaultConcurrency(); got != c.want {
			t.Errorf("GOMAXPROCS=%d: DefaultConcurrency = %d, want %d", c.procs, got, c.want)
		}
		SetConcurrency(0)
		if got := Concurrency(); got != c.want {
			t.Errorf("GOMAXPROCS=%d: SetConcurrency(0) gave %d, want %d", c.procs, got, c.want)
		}
	}
	runtime.GOMAXPROCS(prev)
	SetConcurrency(-1)
	if Concurrency() != DefaultConcurrency() {
		t.Fatalf("negative must select the default, got %d", Concurrency())
	}
}

// An explicit limit (auth.argon2_concurrency) replaces the default and is enforced.
func TestConcurrencyOverride(t *testing.T) {
	SetParams(Params{Memory: 64, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16})
	t.Cleanup(func() { SetParams(DefaultParams); argon2Key = argon2.IDKey; SetConcurrency(0) })
	for _, n := range []int{1, 3, 7} {
		SetConcurrency(n)
		if Concurrency() != n {
			t.Fatalf("SetConcurrency(%d): Concurrency = %d", n, Concurrency())
		}
		var cur, peak atomic.Int32
		argon2Key = func(pw, salt []byte, time_, mem uint32, th uint8, kl uint32) []byte {
			c := cur.Add(1)
			for {
				p := peak.Load()
				if c <= p || peak.CompareAndSwap(p, c) {
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
		for range 4 * n {
			wg.Go(func() { VerifyPassword(hash, "wrong") })
		}
		wg.Wait()
		if p := peak.Load(); p > int32(n) || p < 1 {
			t.Fatalf("limit %d: peak concurrent derivations = %d", n, p)
		}
	}
}

// A derivation that took a slot before SetConcurrency gives it back to the channel it came from: the new limit
// starts with every slot free.
func TestSetConcurrencyWhileDeriving(t *testing.T) {
	SetParams(Params{Memory: 64, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16})
	t.Cleanup(func() { SetParams(DefaultParams); argon2Key = argon2.IDKey; SetConcurrency(0) })
	SetConcurrency(1)
	old := *hashSlots.Load()
	started, release := make(chan struct{}), make(chan struct{})
	var first atomic.Bool
	first.Store(true)
	argon2Key = func(pw, salt []byte, time_, mem uint32, th uint8, kl uint32) []byte {
		if first.CompareAndSwap(true, false) { // the first derivation holds its slot until released
			close(started)
			<-release
		}
		return argon2.IDKey(pw, salt, time_, mem, th, kl)
	}
	done := make(chan struct{})
	go func() { _, _ = HashPassword("password123"); close(done) }()
	<-started
	SetConcurrency(2)
	if _, err := HashPassword("other-password"); err != nil { // does not wait for the old slot
		t.Fatal(err)
	}
	close(release)
	<-done
	if len(old) != 0 || len(*hashSlots.Load()) != 0 || Concurrency() != 2 {
		t.Fatalf("slots leaked: old %d/%d, new %d/%d", len(old), cap(old), len(*hashSlots.Load()), Concurrency())
	}
}
