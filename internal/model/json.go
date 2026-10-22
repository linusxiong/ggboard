package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
)

// All JSON column types are stored as TEXT on every driver (portable across SQLite, PostgreSQL and MySQL)
// and never as SQL NULL: empty slices serialize to "[]" and empty maps to "{}".

func jsonValue(v any) (driver.Value, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func jsonScan(src any, dst any) error {
	switch s := src.(type) {
	case nil:
		return nil
	case []byte:
		if len(s) == 0 {
			return nil
		}
		return json.Unmarshal(s, dst)
	case string:
		if s == "" {
			return nil
		}
		return json.Unmarshal([]byte(s), dst)
	default:
		return fmt.Errorf("model: cannot scan %T into a JSON column", src)
	}
}

// JSONSlice is a []T stored as a JSON array.
type JSONSlice[T any] []T

// Value implements driver.Valuer.
func (s JSONSlice[T]) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	return jsonValue([]T(s))
}

// Scan implements sql.Scanner.
func (s *JSONSlice[T]) Scan(src any) error {
	var out []T
	if err := jsonScan(src, &out); err != nil {
		return err
	}
	*s = out
	return nil
}

// GormDataType makes GORM create a TEXT column.
func (JSONSlice[T]) GormDataType() string { return "text" }

// MarshalJSON encodes nil as [] so API consumers never see null for list fields.
func (s JSONSlice[T]) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]T(s))
}

// SliceContains reports whether v is an element of s.
func SliceContains[T comparable](s JSONSlice[T], v T) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// JSONObject is a map[string]V stored as a JSON object (e.g. plan prices: JSONObject[int64]).
type JSONObject[V any] map[string]V

// Value implements driver.Valuer.
func (m JSONObject[V]) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	return jsonValue(map[string]V(m))
}

// Scan implements sql.Scanner.
func (m *JSONObject[V]) Scan(src any) error {
	var out map[string]V
	if err := jsonScan(src, &out); err != nil {
		return err
	}
	*m = out
	return nil
}

// GormDataType makes GORM create a TEXT column.
func (JSONObject[V]) GormDataType() string { return "text" }

// MarshalJSON encodes nil as {}.
func (m JSONObject[V]) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(map[string]V(m))
}

// JSONMap is a free-form JSON object (map[string]any), e.g. node protocol_settings.
type JSONMap map[string]any

// Value implements driver.Valuer.
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	return jsonValue(map[string]any(m))
}

// Scan implements sql.Scanner.
func (m *JSONMap) Scan(src any) error {
	var out map[string]any
	if err := jsonScan(src, &out); err != nil {
		return err
	}
	*m = out
	return nil
}

// GormDataType makes GORM create a TEXT column.
func (JSONMap) GormDataType() string { return "text" }

// MarshalJSON encodes nil as {}.
func (m JSONMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(map[string]any(m))
}

// Decode converts the map into dst (a pointer) through a JSON round trip.
func (m JSONMap) Decode(dst any) error {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(map[string]any(m))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// String returns m[key] as a string ("" when absent; numbers and bools are formatted).
func (m JSONMap) String(key string) string {
	switch v := m[key].(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

// Int returns m[key] as an int (0 when absent or not numeric; numeric strings are parsed).
func (m JSONMap) Int(key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(v)
		return i
	case bool:
		if v {
			return 1
		}
	}
	return 0
}

// Bool returns m[key] as a bool (true for true, non-zero numbers and "1"/"true").
func (m JSONMap) Bool(key string) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case int:
		return v != 0
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	}
	return false
}

// Map returns m[key] as a JSONMap (nil when absent or not an object).
func (m JSONMap) Map(key string) JSONMap {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	if v, ok := m[key].(JSONMap); ok {
		return v
	}
	return nil
}

// RawJSON is an opaque JSON document stored as TEXT (e.g. rule_profiles.data).
type RawJSON []byte

// Value implements driver.Valuer.
func (r RawJSON) Value() (driver.Value, error) {
	if len(r) == 0 {
		return "null", nil
	}
	if !json.Valid(r) {
		return nil, fmt.Errorf("model: invalid JSON in RawJSON")
	}
	return string(r), nil
}

// Scan implements sql.Scanner.
func (r *RawJSON) Scan(src any) error {
	switch s := src.(type) {
	case nil:
		*r = nil
	case []byte:
		*r = append(RawJSON(nil), s...)
	case string:
		*r = RawJSON(s)
	default:
		return fmt.Errorf("model: cannot scan %T into RawJSON", src)
	}
	return nil
}

// GormDataType makes GORM create a TEXT column.
func (RawJSON) GormDataType() string { return "text" }

// MarshalJSON returns the raw document (null when empty).
func (r RawJSON) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return r, nil
}

// UnmarshalJSON stores a copy of data.
func (r *RawJSON) UnmarshalJSON(data []byte) error {
	*r = append((*r)[:0], data...)
	return nil
}
