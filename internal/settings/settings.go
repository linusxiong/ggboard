// Package settings is the typed, cached key/value settings service backed by the settings table.
// Values are JSON-encoded in the database; reads come from an in-memory snapshot that is replaced atomically
// after every successful Set, so reads never touch the database.
package settings

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/model"
)

// Service reads and writes settings.
type Service struct {
	db   *gorm.DB
	mu   sync.Mutex // serializes writers
	vals atomic.Pointer[map[string]any]
}

// FieldErrors maps setting keys to validation messages. It implements error and is rendered by httpx.Fail
// as a 400 validation_failed response.
type FieldErrors map[string][]string

func (e FieldErrors) Error() string {
	keys := slices.Sorted(maps.Keys(e))
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+strings.Join(e[k], ", "))
	}
	return "settings: invalid values: " + strings.Join(parts, "; ")
}

// FieldErrors returns the per-field messages.
func (e FieldErrors) FieldErrors() map[string][]string { return e }

// generated lists the secrets created on first boot (32 random characters) and regenerated when reset to null.
var generated = []string{"server_token", "telegram_webhook_secret"}

// New loads all settings and generates server_token and telegram_webhook_secret on first boot.
func New(db *gorm.DB) (*Service, error) {
	s := &Service{db: db}
	if err := s.Reload(context.Background()); err != nil {
		return nil, err
	}
	for _, key := range generated {
		if s.String(key) == "" {
			if err := s.Set(map[string]any{key: RandomToken(32)}); err != nil {
				return nil, fmt.Errorf("settings: generate %s: %w", key, err)
			}
		}
	}
	return s, nil
}

// Reload re-reads every row from the database.
func (s *Service) Reload(ctx context.Context) error {
	var rows []model.Setting
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return fmt.Errorf("settings: load: %w", err)
	}
	vals := maps.Clone(Defaults)
	for _, r := range rows {
		var raw any
		if err := json.Unmarshal([]byte(r.Value), &raw); err != nil {
			continue // corrupt row: keep default
		}
		if d, ok := defIndex[r.Key]; ok {
			v, err := normalize(d, raw)
			if err != nil {
				continue
			}
			vals[r.Key] = v
		} else {
			vals[r.Key] = raw
		}
	}
	s.vals.Store(&vals)
	return nil
}

func (s *Service) snapshot() map[string]any {
	if p := s.vals.Load(); p != nil {
		return *p
	}
	return Defaults
}

// Get returns the current value of key (its default when unset; nil for unknown keys).
// Slices and maps are shared: do not mutate them (String/Strings/JSON return copies).
func (s *Service) Get(key string) any { return s.snapshot()[key] }

// String returns a string setting ("" if missing or not a string).
func (s *Service) String(key string) string {
	v, _ := s.Get(key).(string)
	return v
}

// Int returns an int setting (0 if missing).
func (s *Service) Int(key string) int {
	switch v := s.Get(key).(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

// Int64 returns an int setting as int64.
func (s *Service) Int64(key string) int64 { return int64(s.Int(key)) }

// Bool returns a bool setting.
func (s *Service) Bool(key string) bool {
	v, _ := s.Get(key).(bool)
	return v
}

// Strings returns a copy of a []string setting (never nil).
func (s *Service) Strings(key string) []string {
	v, _ := s.Get(key).([]string)
	return append([]string{}, v...)
}

// JSON decodes a setting (typically TypeJSON) into dst.
func (s *Service) JSON(key string, dst any) error {
	b, err := json.Marshal(s.Get(key))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// Duration returns an int setting multiplied by unit (e.g. Duration("server_push_interval", time.Second)).
func (s *Service) Duration(key string, unit time.Duration) time.Duration {
	return time.Duration(s.Int(key)) * unit
}

// All returns a copy of every setting (including secrets and hidden keys).
func (s *Service) All() map[string]any { return maps.Clone(s.snapshot()) }

// AllGrouped returns {group: {key: value}} for every visible key, secrets included (admin only).
func (s *Service) AllGrouped() map[string]map[string]any {
	vals := s.snapshot()
	out := make(map[string]map[string]any, len(groupOrder))
	for _, d := range defs {
		if d.Hidden {
			continue
		}
		if out[d.Group] == nil {
			out[d.Group] = map[string]any{}
		}
		out[d.Group][d.Key] = vals[d.Key]
	}
	return out
}

// Set validates and persists values (key -> value). A nil value resets the key to its default. Unknown keys
// and invalid values are rejected as FieldErrors without writing anything.
func (s *Service) Set(values map[string]any) error {
	return s.SetContext(context.Background(), values)
}

// SetContext is Set with a context.
func (s *Service) SetContext(ctx context.Context, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	for _, key := range generated {
		if v, ok := values[key]; ok && v == nil {
			// "Reset to default" means a fresh token, as on first boot: the default ("") would disconnect every node
			// (an unset token rejects all requests) or the Telegram webhook until the next restart regenerated it.
			values = maps.Clone(values)
			values[key] = RandomToken(32)
		}
	}
	errs := FieldErrors{}
	type write struct {
		key   string
		value string
		reset bool
	}
	writes := make([]write, 0, len(values))
	keys := slices.Sorted(maps.Keys(values))
	for _, k := range keys {
		d, ok := defIndex[k]
		if !ok {
			errs[k] = append(errs[k], "unknown setting")
			continue
		}
		if values[k] == nil {
			writes = append(writes, write{key: k, reset: true})
			continue
		}
		v, err := normalize(d, values[k])
		if err != nil {
			errs[k] = append(errs[k], err.Error())
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			errs[k] = append(errs[k], "cannot encode value")
			continue
		}
		writes = append(writes, write{key: k, value: string(b)})
	}
	if len(errs) > 0 {
		return errs
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for _, w := range writes {
			if w.reset {
				if err := tx.Delete(&model.Setting{Key: w.key}).Error; err != nil {
					return err
				}
				continue
			}
			row := model.Setting{Key: w.key, Value: w.value, UpdatedAt: now}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("settings: save: %w", err)
	}
	return s.Reload(ctx)
}

var errType = errors.New("invalid type")

// normalize converts a decoded JSON (or Go) value to the canonical Go type of d and validates it.
func normalize(d Def, v any) (any, error) {
	switch d.Type {
	case TypeString:
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("must be a string")
		}
		if len(d.Options) > 0 && !slices.Contains(d.Options, str) {
			return nil, fmt.Errorf("must be one of: %s", strings.Join(d.Options, ", "))
		}
		if d.MinLen != nil && str != "" && len(str) < *d.MinLen {
			return nil, fmt.Errorf("must be at least %d characters", *d.MinLen)
		}
		return str, nil
	case TypeInt:
		n, err := toInt(v)
		if err != nil {
			return nil, fmt.Errorf("must be an integer")
		}
		if d.Min != nil && n < *d.Min {
			return nil, fmt.Errorf("must be at least %d", *d.Min)
		}
		if d.Max != nil && n > *d.Max {
			return nil, fmt.Errorf("must be at most %d", *d.Max)
		}
		return n, nil
	case TypeBool:
		switch x := v.(type) {
		case bool:
			return x, nil
		case float64:
			return x != 0, nil
		case int:
			return x != 0, nil
		case string:
			bv, err := strconv.ParseBool(strings.TrimSpace(x))
			if err != nil {
				return nil, fmt.Errorf("must be a boolean")
			}
			return bv, nil
		}
		return nil, fmt.Errorf("must be a boolean")
	case TypeStrings:
		out := []string{}
		switch x := v.(type) {
		case []string:
			for _, e := range x {
				if e = strings.TrimSpace(e); e != "" {
					out = append(out, e)
				}
			}
		case []any:
			for _, e := range x {
				str, ok := e.(string)
				if !ok {
					return nil, fmt.Errorf("must be a list of strings")
				}
				if str = strings.TrimSpace(str); str != "" {
					out = append(out, str)
				}
			}
		case string:
			for _, e := range strings.FieldsFunc(x, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' }) {
				if e = strings.TrimSpace(e); e != "" {
					out = append(out, e)
				}
			}
		default:
			return nil, fmt.Errorf("must be a list of strings")
		}
		if len(d.Options) > 0 {
			for _, e := range out {
				if !slices.Contains(d.Options, e) {
					return nil, fmt.Errorf("unknown value %q: must be among %s", e, strings.Join(d.Options, ", "))
				}
			}
		}
		return out, nil
	case TypeJSON:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("must be valid JSON")
		}
		var out any
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, fmt.Errorf("must be valid JSON")
		}
		return out, nil
	}
	return nil, errType
}

func toInt(v any) (int, error) {
	switch x := v.(type) {
	case int:
		return x, nil
	case int64:
		return int(x), nil
	case int32:
		return int(x), nil
	case float64:
		if x != math.Trunc(x) || math.IsInf(x, 0) || math.IsNaN(x) {
			return 0, errType
		}
		return int(x), nil
	case json.Number:
		n, err := x.Int64()
		return int(n), err
	case string:
		return strconv.Atoi(strings.TrimSpace(x))
	}
	return 0, errType
}

const alnum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// RandomToken returns n cryptographically random alphanumeric characters.
func RandomToken(n int) string {
	out := make([]byte, n)
	buf := make([]byte, n*2)
	for filled := 0; filled < n; {
		if _, err := rand.Read(buf); err != nil {
			panic("settings: crypto/rand failed: " + err.Error())
		}
		for _, c := range buf {
			if c >= 248 { // 248 = 62*4: reject to avoid modulo bias
				continue
			}
			out[filled] = alnum[int(c)%len(alnum)]
			filled++
			if filled == n {
				break
			}
		}
	}
	return string(out)
}

// SortedKeys returns every known key sorted (useful for docs/tests).
func SortedKeys() []string {
	keys := make([]string, 0, len(defs))
	for _, d := range defs {
		keys = append(keys, d.Key)
	}
	sort.Strings(keys)
	return keys
}
