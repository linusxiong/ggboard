package nodesvc

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // ETag content hash mandated by docs/NODE_PROTOCOLS.md §6.6, not a security primitive
	"encoding/hex"
	"encoding/json"
)

// Field is one key/value pair of an Object.
type Field struct {
	Key   string
	Value any
}

// Object is a JSON object that keeps its key order. encoding/json sorts map keys, but node bodies follow the
// key order of docs/NODE_PROTOCOLS.md so that the bytes (and therefore the ETags) match the worked examples.
type Object []Field

// MarshalJSON encodes the fields in order.
func (o Object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(f.Key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(f.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Get returns the value stored under key.
func (o Object) Get(key string) (any, bool) {
	for _, f := range o {
		if f.Key == key {
			return f.Value, true
		}
	}
	return nil, false
}

// Has reports whether key is present.
func (o Object) Has(key string) bool {
	_, ok := o.Get(key)
	return ok
}

// Add appends key/value (it does not check for duplicates).
func (o *Object) Add(key string, value any) { *o = append(*o, Field{Key: key, Value: value}) }

// Body is an encoded node response with its content hash.
type Body struct {
	JSON []byte
	ETag string // hex(sha1(JSON)), 40 lowercase characters, unquoted
}

// NewBody encodes v and computes its ETag.
func NewBody(v any) (*Body, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &Body{JSON: b, ETag: ETag(b)}, nil
}

// ETag returns hex(sha1(b)) (docs/NODE_PROTOCOLS.md §6.6).
func ETag(b []byte) string {
	sum := sha1.Sum(b) //nolint:gosec // content hash, see import comment
	return hex.EncodeToString(sum[:])
}
