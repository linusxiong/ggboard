package cache

import (
	"sync"
	"time"
)

// TTLMap is a mutex-protected map whose entries expire. Expired entries are invisible immediately and are
// swept opportunistically on writes (at most once per sweep interval) or explicitly with Purge.
type TTLMap[K comparable, V any] struct {
	mu        sync.Mutex
	items     map[K]ttlEntry[V]
	ttl       time.Duration
	lastSweep time.Time
	now       func() time.Time
}

type ttlEntry[V any] struct {
	val     V
	expires time.Time // zero = never
}

// NewTTLMap returns a map whose Set uses defaultTTL when ttl == 0 (defaultTTL 0 = no expiry).
func NewTTLMap[K comparable, V any](defaultTTL time.Duration) *TTLMap[K, V] {
	return &TTLMap[K, V]{items: map[K]ttlEntry[V]{}, ttl: defaultTTL, now: time.Now}
}

func (m *TTLMap[K, V]) expiry(ttl time.Duration, now time.Time) time.Time {
	if ttl == 0 {
		ttl = m.ttl
	}
	if ttl <= 0 {
		return time.Time{}
	}
	return now.Add(ttl)
}

func (e ttlEntry[V]) alive(now time.Time) bool { return e.expires.IsZero() || now.Before(e.expires) }

// Set stores v under k for ttl (0 = default TTL, negative = no expiry).
func (m *TTLMap[K, V]) Set(k K, v V, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	m.items[k] = ttlEntry[V]{val: v, expires: m.expiry(ttl, now)}
	m.maybeSweep(now)
}

// Get returns the live value for k.
func (m *TTLMap[K, V]) Get(k K) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[k]
	if !ok || !e.alive(m.now()) {
		var zero V
		return zero, false
	}
	return e.val, true
}

// TTL returns the remaining lifetime of k (0 and false when absent; -1 when it never expires).
func (m *TTLMap[K, V]) TTL(k K) (time.Duration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	e, ok := m.items[k]
	if !ok || !e.alive(now) {
		return 0, false
	}
	if e.expires.IsZero() {
		return -1, true
	}
	return e.expires.Sub(now), true
}

// Take returns and deletes the live value for k (one-time tokens).
func (m *TTLMap[K, V]) Take(k K) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[k]
	delete(m.items, k)
	if !ok || !e.alive(m.now()) {
		var zero V
		return zero, false
	}
	return e.val, true
}

// Update atomically replaces the value of k with fn(old, exists). When keepTTL is true and k is live, the
// existing expiry is kept; otherwise ttl applies (0 = default). Returns the new value.
func (m *TTLMap[K, V]) Update(k K, ttl time.Duration, keepTTL bool, fn func(old V, ok bool) V) V {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	e, ok := m.items[k]
	if ok && !e.alive(now) {
		var zero V
		e, ok = ttlEntry[V]{val: zero}, false
	}
	nv := fn(e.val, ok)
	exp := m.expiry(ttl, now)
	if keepTTL && ok {
		exp = e.expires
	}
	m.items[k] = ttlEntry[V]{val: nv, expires: exp}
	m.maybeSweep(now)
	return nv
}

// Delete removes k.
func (m *TTLMap[K, V]) Delete(k K) {
	m.mu.Lock()
	delete(m.items, k)
	m.mu.Unlock()
}

// Len returns the number of live entries.
func (m *TTLMap[K, V]) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	now, n := m.now(), 0
	for _, e := range m.items {
		if e.alive(now) {
			n++
		}
	}
	return n
}

// Range calls fn for every live entry until fn returns false. fn must not call back into the map.
func (m *TTLMap[K, V]) Range(fn func(k K, v V) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for k, e := range m.items {
		if e.alive(now) && !fn(k, e.val) {
			return
		}
	}
}

// Purge removes expired entries and returns how many were removed.
func (m *TTLMap[K, V]) Purge() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sweep(m.now())
}

func (m *TTLMap[K, V]) sweep(now time.Time) int {
	n := 0
	for k, e := range m.items {
		if !e.alive(now) {
			delete(m.items, k)
			n++
		}
	}
	m.lastSweep = now
	return n
}

const sweepInterval = time.Minute

func (m *TTLMap[K, V]) maybeSweep(now time.Time) {
	if now.Sub(m.lastSweep) >= sweepInterval {
		m.sweep(now)
	}
}
