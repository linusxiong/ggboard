// Package plugin provides the generic named registry used by every plugin point (payment gateways,
// subscription formats, captcha providers, ...). Built-ins register themselves from init().
package plugin

import (
	"fmt"
	"sort"
	"sync"
)

// Registry maps unique names to values of type T.
type Registry[T any] struct {
	kind  string
	mu    sync.RWMutex
	items map[string]T
}

// NewRegistry returns an empty registry; kind is used in panic messages ("payment gateway").
func NewRegistry[T any](kind string) *Registry[T] {
	return &Registry[T]{kind: kind, items: map[string]T{}}
}

// Register adds v under name. It panics on an empty or duplicate name (a programming error caught at init).
func (r *Registry[T]) Register(name string, v T) {
	if name == "" {
		panic(fmt.Sprintf("plugin: %s registered with empty name", r.kind))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.items[name]; dup {
		panic(fmt.Sprintf("plugin: duplicate %s %q", r.kind, name))
	}
	r.items[name] = v
}

// Get returns the value registered under name.
func (r *Registry[T]) Get(name string) (T, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.items[name]
	return v, ok
}

// Has reports whether name is registered.
func (r *Registry[T]) Has(name string) bool {
	_, ok := r.Get(name)
	return ok
}

// Names returns the registered names sorted alphabetically.
func (r *Registry[T]) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.items))
	for n := range r.items {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Each calls fn for every entry in name order.
func (r *Registry[T]) Each(fn func(name string, v T)) {
	for _, n := range r.Names() {
		if v, ok := r.Get(n); ok {
			fn(n, v)
		}
	}
}

// Len returns the number of registered entries.
func (r *Registry[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}
