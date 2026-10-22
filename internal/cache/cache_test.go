package cache

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newCache(t *testing.T) *Cache {
	t.Helper()
	c, err := New(1000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestCacheBasics(t *testing.T) {
	c := newCache(t)
	c.Set("a", 1, time.Minute)
	if v, ok := c.Get("a"); !ok || v.(int) != 1 {
		t.Fatalf("get = %v %v", v, ok)
	}
	if v, ok := GetAs[int](c, "a"); !ok || v != 1 {
		t.Fatal("GetAs")
	}
	if _, ok := GetAs[string](c, "a"); ok {
		t.Fatal("GetAs wrong type must miss")
	}
	c.Delete("a")
	if _, ok := c.Get("a"); ok {
		t.Fatal("deleted key still present")
	}
	c.Set("ttl", "x", 20*time.Millisecond)
	time.Sleep(40 * time.Millisecond)
	if _, ok := c.Get("ttl"); ok {
		t.Fatal("expired key still present")
	}
	c.Set("b", 2, 0)
	c.Clear()
	if _, ok := c.Get("b"); ok {
		t.Fatal("Clear")
	}
	if c.Stats().MaxItems != 1000 {
		t.Fatal("stats")
	}
}

func TestGetOrLoadSingleflight(t *testing.T) {
	c := newCache(t)
	var calls atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := GetOrLoad(c, "k", time.Minute, func() (string, error) {
				calls.Add(1)
				time.Sleep(10 * time.Millisecond)
				return "v", nil
			})
			if err != nil || v != "v" {
				t.Errorf("GetOrLoad = %v %v", v, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("loader called %d times", calls.Load())
	}
	if _, err := GetOrLoad(c, "err", time.Minute, func() (int, error) { return 0, errors.New("boom") }); err == nil {
		t.Fatal("error must propagate")
	}
	if _, ok := c.Get("err"); ok {
		t.Fatal("errors must not be cached")
	}
}

func TestNamespaces(t *testing.T) {
	c := newCache(t)
	k1 := c.Key("users", "1,2")
	c.Set(k1, "old", time.Minute)
	c.InvalidateNamespace("users")
	k2 := c.Key("users", "1,2")
	if k1 == k2 {
		t.Fatal("key must change after invalidation")
	}
	if _, ok := c.Get(k2); ok {
		t.Fatal("new key must miss")
	}
	if c.Key("other", "x") != "other@0:x" {
		t.Fatal("unrelated namespace affected")
	}
}

func TestTTLMap(t *testing.T) {
	now := time.Unix(1000, 0)
	m := NewTTLMap[string, int](time.Minute)
	m.now = func() time.Time { return now }

	m.Set("a", 1, 0)
	m.Set("forever", 2, -1)
	m.Set("short", 3, time.Second)
	if v, ok := m.Get("a"); !ok || v != 1 {
		t.Fatal("get")
	}
	if d, ok := m.TTL("a"); !ok || d != time.Minute {
		t.Fatalf("ttl = %v", d)
	}
	if d, ok := m.TTL("forever"); !ok || d != -1 {
		t.Fatalf("forever ttl = %v", d)
	}
	now = now.Add(2 * time.Second)
	if _, ok := m.Get("short"); ok {
		t.Fatal("short must be expired")
	}
	if m.Len() != 2 {
		t.Fatalf("len = %d", m.Len())
	}
	if n := m.Purge(); n != 1 {
		t.Fatalf("purged %d", n)
	}

	// Update keeps the expiry when asked and counts.
	for range 3 {
		m.Update("count", 10*time.Second, true, func(old int, ok bool) int { return old + 1 })
	}
	if v, _ := m.Get("count"); v != 3 {
		t.Fatalf("count = %d", v)
	}
	now = now.Add(11 * time.Second)
	if _, ok := m.Get("count"); ok {
		t.Fatal("keepTTL must not extend expiry")
	}
	if v := m.Update("count", 0, true, func(old int, ok bool) int {
		if ok {
			t.Error("expired entry must be reported as absent")
		}
		return old + 1
	}); v != 1 {
		t.Fatalf("restart = %d", v)
	}

	if v, ok := m.Take("a"); !ok || v != 1 {
		t.Fatal("take")
	}
	if _, ok := m.Take("a"); ok {
		t.Fatal("take twice")
	}
	seen := 0
	m.Range(func(string, int) bool { seen++; return true })
	if seen != m.Len() {
		t.Fatalf("range saw %d of %d", seen, m.Len())
	}
	m.Delete("forever")
	if _, ok := m.Get("forever"); ok {
		t.Fatal("delete")
	}

	// Opportunistic sweep on writes after the sweep interval.
	m.Set("old", 1, time.Second)
	now = now.Add(2 * time.Minute)
	m.Set("trigger", 1, 0)
	m.mu.Lock()
	_, stillThere := m.items["old"]
	m.mu.Unlock()
	if stillThere {
		t.Fatal("expired entry not swept")
	}
}
