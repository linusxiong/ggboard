// Package cache wraps ristretto (a bounded, concurrent, TTL-aware in-process cache) and provides TTLMap for
// small ephemeral state that must never be evicted early (codes, one-time tokens, counters, online users).
package cache

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"golang.org/x/sync/singleflight"
)

// DefaultMaxItems is the default capacity of New(0).
const DefaultMaxItems = 100_000

// Cache is a best-effort cache: entries may be evicted (or, rarely, not admitted) at any time, so never use
// it as the only copy of data. Every entry costs 1, so capacity is counted in items.
type Cache struct {
	r     *ristretto.Cache[string, any]
	group singleflight.Group
	nsMu  sync.Mutex
	ns    map[string]uint64
}

// Stats is a snapshot of cache metrics (for /admin/system/status).
type Stats struct {
	Items    int64   `json:"items"`
	MaxItems int64   `json:"max_items"`
	Hits     uint64  `json:"hits"`
	Misses   uint64  `json:"misses"`
	HitRatio float64 `json:"hit_ratio"`
}

// New creates a cache holding up to maxItems entries (DefaultMaxItems when <= 0).
func New(maxItems int64) (*Cache, error) {
	if maxItems <= 0 {
		maxItems = DefaultMaxItems
	}
	r, err := ristretto.NewCache(&ristretto.Config[string, any]{
		NumCounters:        maxItems * 10,
		MaxCost:            maxItems,
		BufferItems:        64,
		Metrics:            true,
		IgnoreInternalCost: true,
	})
	if err != nil {
		return nil, fmt.Errorf("cache: %w", err)
	}
	return &Cache{r: r, ns: map[string]uint64{}}, nil
}

// Get returns the cached value for key.
func (c *Cache) Get(key string) (any, bool) { return c.r.Get(key) }

// Set stores val for ttl (ttl <= 0 means no expiry). The write is visible to subsequent Gets on return.
func (c *Cache) Set(key string, val any, ttl time.Duration) {
	if ttl < 0 {
		ttl = 0
	}
	c.r.SetWithTTL(key, val, 1, ttl)
	c.r.Wait()
}

// Delete removes key.
func (c *Cache) Delete(key string) { c.r.Del(key) }

// Clear removes every entry.
func (c *Cache) Clear() { c.r.Clear() }

// Close stops the cache's background goroutines.
func (c *Cache) Close() { c.r.Close() }

// Stats returns current metrics.
func (c *Cache) Stats() Stats {
	m := c.r.Metrics
	s := Stats{MaxItems: c.r.MaxCost()}
	if m != nil {
		s.Items = int64(m.KeysAdded() - m.KeysEvicted())
		s.Hits, s.Misses, s.HitRatio = m.Hits(), m.Misses(), m.Ratio()
	}
	return s
}

// Key builds a cache key inside namespace ns. Calling InvalidateNamespace(ns) makes every key previously built
// for ns unreachable (they age out by TTL), which is how prefix invalidation is done on top of ristretto.
func (c *Cache) Key(ns, key string) string {
	c.nsMu.Lock()
	v := c.ns[ns]
	c.nsMu.Unlock()
	return ns + "@" + strconv.FormatUint(v, 10) + ":" + key
}

// InvalidateNamespace invalidates all keys built with Key(ns, ...).
func (c *Cache) InvalidateNamespace(ns string) {
	c.nsMu.Lock()
	c.ns[ns]++
	c.nsMu.Unlock()
}

// GetAs returns the cached value for key if it exists and has type T.
func GetAs[T any](c *Cache, key string) (T, bool) {
	v, ok := c.Get(key)
	if !ok {
		var zero T
		return zero, false
	}
	t, ok := v.(T)
	return t, ok
}

// GetOrLoad returns the cached value for key or calls load (once per key across concurrent callers),
// caching a successful result for ttl. Errors are not cached.
func GetOrLoad[T any](c *Cache, key string, ttl time.Duration, load func() (T, error)) (T, error) {
	if v, ok := GetAs[T](c, key); ok {
		return v, nil
	}
	v, err, _ := c.group.Do(key, func() (any, error) {
		if v, ok := GetAs[T](c, key); ok {
			return v, nil
		}
		v, err := load()
		if err != nil {
			return nil, err
		}
		c.Set(key, v, ttl)
		return v, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return v.(T), nil
}
