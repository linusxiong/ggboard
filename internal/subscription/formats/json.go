package formats

import (
	"bytes"
	"encoding/json"
)

// jobj is an ordered JSON object (sing-box configs and vmess share links read better with a fixed key order).
// nil values are skipped.
type jobj []kv

func (o *jobj) set(k string, v any) { *o = append(*o, kv{k, v}) }

func (o *jobj) setStr(k, v string) {
	if v != "" {
		*o = append(*o, kv{k, v})
	}
}

func (o *jobj) setTrue(k string, v bool) {
	if v {
		*o = append(*o, kv{k, true})
	}
}

// MarshalJSON writes the object in insertion order without HTML escaping.
func (o jobj) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	if err := o.encode(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (o jobj) encode(buf *bytes.Buffer) error {
	buf.WriteByte('{')
	first := true
	for _, e := range o {
		if e.v == nil {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		if err := encodeJSON(buf, e.k); err != nil {
			return err
		}
		buf.WriteByte(':')
		if err := encodeValue(buf, e.v); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

func encodeValue(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case jobj:
		return x.encode(buf)
	case []jobj:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := e.encode(buf); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeValue(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	}
	return encodeJSON(buf, v)
}

func encodeJSON(buf *bytes.Buffer, v any) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	buf.Truncate(buf.Len() - 1) // Encode appends a newline
	return nil
}

// marshalIndent renders o with two-space indentation and a trailing newline.
func marshalIndent(o jobj) ([]byte, error) {
	var raw bytes.Buffer
	if err := o.encode(&raw); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Grow(raw.Len() * 2)
	if err := json.Indent(&out, raw.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
