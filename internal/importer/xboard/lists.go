package xboardimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// List parsers (docs/IMPORT_XBOARD.md §2). Xboard stores id and string lists in many shapes:
//   - NULL or "" → empty
//   - a JSON array of numbers or numeric strings: ["1","2"], [1,2]
//   - a JSON object, produced by PHP array_filter + json_encode: {"0":"1","2":"3"} → values in document order
//   - a bare JSON scalar: 3, "3" (a JSON string is parsed again as plain text)
//   - plain text from pre-JSON v2board: id lists split on "," "\n" "|"; string lists on newlines when the text
//     has any, else on "," (a "|" is never a separator for strings: route rules contain regex alternations)

// listItems returns the raw elements of a list column in order. A value that looks like JSON (starts with
// '[' or '{') but does not parse is an error; the caller warns and uses an empty list.
func listItems(v any, splitText func(string) []string) ([]any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case []any:
		return x, nil
	case map[string]any:
		return mapValuesSorted(x), nil
	}
	text, _ := flexStr(v)
	raw := bytes.TrimSpace([]byte(text))
	if len(raw) == 0 {
		return nil, nil
	}
	items, err := orderedJSONItems(raw)
	if err == nil {
		out := make([]any, 0, len(items))
		for _, it := range items {
			if s, ok := it.(string); ok && len(items) == 1 && looksLikeList(s) {
				// a JSON string holding a list ("1,2", "[1,2]")
				nested, nerr := listItems(s, splitText)
				if nerr != nil {
					return nil, nerr
				}
				out = append(out, nested...)
				continue
			}
			out = append(out, it)
		}
		return out, nil
	}
	if raw[0] == '[' || raw[0] == '{' {
		return nil, fmt.Errorf("malformed JSON list: %w", err)
	}
	parts := splitText(string(raw))
	out := make([]any, 0, len(parts))
	for _, p := range parts {
		out = append(out, p)
	}
	return out, nil
}

func looksLikeList(s string) bool {
	s = strings.TrimSpace(s)
	return strings.ContainsAny(s, ",\n|") || strings.HasPrefix(s, "[") || strings.HasPrefix(s, "{")
}

// orderedJSONItems decodes raw as a single JSON value: arrays give their elements, objects their values in
// document order, scalars a one-element slice.
func orderedJSONItems(raw []byte) ([]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	var out []any
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '[':
			for dec.More() {
				var v any
				if err := dec.Decode(&v); err != nil {
					return nil, err
				}
				out = append(out, v)
			}
		case '{':
			for dec.More() {
				if _, err := dec.Token(); err != nil { // key
					return nil, err
				}
				var v any
				if err := dec.Decode(&v); err != nil {
					return nil, err
				}
				out = append(out, v)
			}
		default:
			return nil, fmt.Errorf("unexpected %v", t)
		}
		if _, err := dec.Token(); err != nil { // closing delimiter
			return nil, err
		}
	case nil:
		// JSON null
	default:
		out = append(out, t)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("trailing data after JSON value")
	}
	return out, nil
}

// mapValuesSorted returns the values of an already-decoded object ordered by numeric key (then string key).
func mapValuesSorted(m map[string]any) []any {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, aerr := strconv.Atoi(keys[i])
		b, berr := strconv.Atoi(keys[j])
		if aerr == nil && berr == nil {
			return a < b
		}
		return keys[i] < keys[j]
	})
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

func splitIDText(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '|' || r == '\r' })
}

func splitStringText(s string) []string {
	if strings.Contains(s, "\n") {
		return strings.Split(s, "\n")
	}
	return strings.Split(s, ",")
}

// parseIDList parses an id list. Non-numeric or <= 0 elements are dropped and returned in bad (the caller
// warns); duplicates are removed keeping the first occurrence. err reports malformed JSON (empty result).
func parseIDList(v any) (ids []uint64, bad []string, err error) {
	items, err := listItems(v, splitIDText)
	if err != nil {
		return nil, nil, err
	}
	seen := map[uint64]bool{}
	for _, it := range items {
		id, ok := toID(it)
		if !ok {
			if it == nil {
				bad = append(bad, "null")
			} else if s := strings.TrimSpace(fmt.Sprint(it)); s != "" {
				bad = append(bad, s)
			}
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, bad, nil
}

func toID(v any) (uint64, bool) {
	var s string
	switch x := v.(type) {
	case json.Number:
		s = x.String()
	case string:
		s = strings.TrimSpace(x)
	case float64:
		if x <= 0 || x != float64(uint64(x)) {
			return 0, false
		}
		return uint64(x), true
	case int64:
		if x <= 0 {
			return 0, false
		}
		return uint64(x), true
	default:
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err == nil && n > 0 {
		return n, true
	}
	// "2.0" style numbers
	if f, ferr := strconv.ParseFloat(s, 64); ferr == nil && f > 0 && f == float64(uint64(f)) {
		return uint64(f), true
	}
	return 0, false
}

// parseStringList parses a string list: items are trimmed; empty items and repeats are dropped (order kept).
// Non-scalar elements (nested arrays/objects) are dropped and returned in bad.
func parseStringList(v any) (out []string, bad []string, err error) {
	items, err := listItems(v, splitStringText)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	out = []string{}
	for _, it := range items {
		var s string
		switch x := it.(type) {
		case string:
			s = x
		case json.Number:
			s = x.String()
		case bool:
			s = strconv.FormatBool(x)
		case float64:
			s = strconv.FormatFloat(x, 'f', -1, 64)
		case nil:
			continue
		default:
			b, _ := json.Marshal(x)
			bad = append(bad, string(b))
			continue
		}
		s = strings.TrimSpace(strings.TrimRight(s, "\r"))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, bad, nil
}
