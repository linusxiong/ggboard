// Package rulesets loads the embedded rule-set catalog (catalog.json, docs/RULESETS.md), merges the admin's
// rulesets_custom overrides, resolves and validates user rule profiles and turns a profile into the
// per-format rule plan consumed by subscription formats.
package rulesets

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/linusxiong/ggboard/internal/settings"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// SettingCustom is the hidden JSON setting holding the admin's catalog overrides.
const SettingCustom = "rulesets_custom"

//go:embed catalog.json
var builtinJSON []byte

var builtin = sync.OnceValues(func() (*subscription.Catalog, error) { return Parse(builtinJSON) })

// Builtin returns the embedded catalog. It is shared: never modify it.
func Builtin() *subscription.Catalog {
	c, err := builtin()
	if err != nil {
		panic(fmt.Sprintf("rulesets: embedded catalog.json is invalid: %v", err)) // caught by tests
	}
	return c
}

// BuiltinJSON returns the embedded catalog file.
func BuiltinJSON() []byte { return slices.Clone(builtinJSON) }

// Parse decodes a catalog document (the built-in file or a custom override, which may omit sections).
func Parse(b []byte) (*subscription.Catalog, error) {
	var c subscription.Catalog
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ParseCustom decodes the rulesets_custom setting value (any JSON-compatible value; nil or {} = no overrides).
func ParseCustom(v any) (*subscription.Catalog, error) {
	if v == nil {
		return &subscription.Catalog{}, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return &subscription.Catalog{}, nil
	}
	return Parse(b)
}

// Merge overlays custom on base by id: a source, category, template or mirror whose id exists replaces the
// built-in entry, a new id is appended. Categories are re-sorted by order (then id). Neither input is modified.
func Merge(base, custom *subscription.Catalog) *subscription.Catalog {
	out := &subscription.Catalog{Sources: maps.Clone(base.Sources), Meta: base.Meta}
	if out.Sources == nil {
		out.Sources = map[string]subscription.Source{}
	}
	if custom == nil {
		custom = &subscription.Catalog{}
	}
	for id, s := range custom.Sources {
		out.Sources[id] = s
	}
	out.Categories = mergeByID(base.Categories, custom.Categories, func(c subscription.Category) string { return c.ID })
	sort.SliceStable(out.Categories, func(i, j int) bool {
		a, b := out.Categories[i], out.Categories[j]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.ID < b.ID
	})
	out.Templates = mergeByID(base.Templates, custom.Templates, func(t subscription.Template) string { return t.ID })
	out.Mirrors = mergeByID(base.Mirrors, custom.Mirrors, func(m subscription.Mirror) string { return m.ID })
	return out
}

func mergeByID[T any](base, custom []T, id func(T) string) []T {
	out := slices.Clone(base)
	pos := make(map[string]int, len(out))
	for i, v := range out {
		pos[id(v)] = i
	}
	for _, v := range custom {
		if i, ok := pos[id(v)]; ok {
			out[i] = v
			continue
		}
		pos[id(v)] = len(out)
		out = append(out, v)
	}
	return out
}

type currentEntry struct {
	key any // identity of val
	val any // the setting value itself: keeps it reachable so its address cannot be reused while cached
	cat *subscription.Catalog
}

var current atomic.Pointer[currentEntry]

// Current returns the built-in catalog merged with the rulesets_custom setting. The merge is cached until the
// setting changes. An invalid stored override is ignored (the built-in catalog is returned) so subscriptions
// never break; PUT /admin/rulesets validates before storing.
func Current(s *settings.Service) *subscription.Catalog {
	v := s.Get(SettingCustom)
	key := identity(v)
	if e := current.Load(); e != nil && key != nil && e.key == key {
		return e.cat
	}
	cat := Builtin()
	if custom, err := ParseCustom(v); err == nil && !isEmpty(custom) {
		cat = Merge(cat, custom)
	}
	if key != nil {
		current.Store(&currentEntry{key: key, val: v, cat: cat})
	}
	return cat
}

// identity returns a comparable identity of a settings value: settings snapshots never mutate values in place,
// so a map's pointer changes exactly when the setting is written.
func identity(v any) any {
	if v == nil {
		return "nil"
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Map || rv.Kind() == reflect.Slice {
		return rv.Pointer()
	}
	return nil
}

func isEmpty(c *subscription.Catalog) bool {
	return len(c.Sources) == 0 && len(c.Categories) == 0 && len(c.Templates) == 0 && len(c.Mirrors) == 0
}

// FieldErrors maps fields to validation messages; httpx renders it as 400 validation_failed.
type FieldErrors map[string][]string

// Error implements error.
func (e FieldErrors) Error() string {
	keys := slices.Sorted(maps.Keys(e))
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+strings.Join(e[k], ", "))
	}
	return "rulesets: invalid values: " + strings.Join(parts, "; ")
}

// FieldErrors returns the per-field messages.
func (e FieldErrors) FieldErrors() map[string][]string { return e }

func (e FieldErrors) add(field, msg string) { e[field] = append(e[field], msg) }

func (e FieldErrors) err() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

// AsFieldErrors extracts FieldErrors from err.
func AsFieldErrors(err error) (FieldErrors, bool) {
	var fe FieldErrors
	ok := errors.As(err, &fe)
	return fe, ok
}
