package adminuser

import (
	"bytes"
	"encoding/json"
)

// Nullable is a JSON field that distinguishes "absent" (Set false), "null" (Set true, Valid false) and a
// value (Set and Valid true). Use it for PATCH fields that may be cleared.
type Nullable[T any] struct {
	Set   bool
	Valid bool
	Value T
}

// UnmarshalJSON implements json.Unmarshaler (only called when the key is present).
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		var zero T
		n.Valid, n.Value = false, zero
		return nil
	}
	if err := json.Unmarshal(b, &n.Value); err != nil {
		return err
	}
	n.Valid = true
	return nil
}

// MarshalJSON renders null or the value.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

// IsZero reports an absent field (so `json:",omitzero"` omits it when encoding).
func (n Nullable[T]) IsZero() bool { return !n.Set }

// Ptr returns nil (null) or a pointer to a copy of the value.
func (n Nullable[T]) Ptr() *T {
	if !n.Valid {
		return nil
	}
	v := n.Value
	return &v
}

// Of returns a set, valid Nullable holding v.
func Of[T any](v T) Nullable[T] { return Nullable[T]{Set: true, Valid: true, Value: v} }

// Null returns a set Nullable holding null.
func Null[T any]() Nullable[T] { return Nullable[T]{Set: true} }
