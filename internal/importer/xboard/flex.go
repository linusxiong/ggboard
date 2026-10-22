package xboardimport

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Flex scanners (docs/IMPORT_XBOARD.md §1.2). One Laravel column has a different Go type per source driver:
// boolean() is tinyint on MySQL/SQLite but bool on PostgreSQL, decimal and JSON columns arrive as strings or
// []byte, and MySQL's text protocol may return numbers as []byte. Every source value goes through these.

// flexInt converts int, bool (1/0), digit strings / []byte (trimmed) and floats (rounded). NULL and
// non-numeric values report ok=false.
func flexInt(v any) (int64, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case int64:
		return x, true
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int16:
		return int64(x), true
	case int8:
		return int64(x), true
	case uint64:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint8:
		return int64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		return int64(math.Round(x)), true
	case float32:
		return flexInt(float64(x))
	case []byte:
		return flexInt(string(x))
	case json.Number:
		return flexInt(string(x))
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return flexInt(f)
		}
		switch strings.ToLower(s) {
		case "true":
			return 1, true
		case "false":
			return 0, true
		}
	}
	return 0, false
}

// flexBool is true for 1, "1", "true", "on", "yes" and bool true (and any non-zero number); false for
// everything else, NULL included.
func flexBool(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case []byte:
		return flexBool(string(x))
	case json.Number:
		return flexBool(string(x))
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "on", "yes":
			return true
		case "", "0", "false", "off", "no":
			return false
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
			return f != 0
		}
		return false
	}
	n, ok := flexInt(v)
	return ok && n != 0
}

// flexFloat converts numbers and numeric strings (decimal columns).
func flexFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case float64:
		return x, !math.IsNaN(x) && !math.IsInf(x, 0)
	case float32:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case []byte:
		return flexFloat(string(x))
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case json.Number:
		return flexFloat(string(x))
	}
	if n, ok := flexInt(v); ok {
		return float64(n), true
	}
	return 0, false
}

// flexStr returns the text of a value with trailing spaces removed (PostgreSQL pads char(n)). NULL reports
// ok=false. Times are formatted as "2006-01-02 15:04:05" (their wall clock).
func flexStr(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", false
	case string:
		return strings.TrimRight(x, " "), true
	case []byte:
		return strings.TrimRight(string(x), " "), true
	case json.Number:
		return string(x), true
	case time.Time:
		return x.Format(time.DateTime), true
	case bool:
		if x {
			return "1", true
		}
		return "0", true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32), true
	}
	if n, ok := flexInt(v); ok {
		return strconv.FormatInt(n, 10), true
	}
	return fmt.Sprint(v), true
}

// str is flexStr without the ok flag (NULL → "").
func str(v any) string {
	s, _ := flexStr(v)
	return s
}

// cents converts currency units to integer cents: int64(round(v*100)).
func cents(v float64) int64 { return int64(math.Round(v * 100)) }

// nz maps NULL and 0 to nil ("0 = unlimited" columns).
func nz(v any) *int {
	n, ok := flexInt(v)
	if !ok || n == 0 {
		return nil
	}
	i := int(n)
	return &i
}

// msThreshold: a unix value above it is in milliseconds.
const msThreshold = 1e11

// unixTime converts a unix-seconds column. A value above 1e11 is treated as milliseconds (ms=true, the caller
// warns). ok=false for NULL, non-numeric and values <= 0.
func unixTime(v any) (t time.Time, ms bool, ok bool) {
	n, ok := flexInt(v)
	if !ok || n <= 0 {
		return time.Time{}, false, false
	}
	if n > msThreshold {
		n /= 1000
		ms = true
	}
	return time.Unix(n, 0).UTC(), ms, true
}

// serverKey is Xboard's Helper::getServerKey: std base64 (padded) of the first n hex characters of
// md5(decimal(createdAt)). Used for the shadowsocks-2022 PSK and the hysteria obfs password.
func serverKey(createdAt int64, n int) string {
	sum := md5.Sum([]byte(strconv.FormatInt(createdAt, 10)))
	h := hex.EncodeToString(sum[:])
	if n > len(h) {
		n = len(h)
	}
	return base64.StdEncoding.EncodeToString([]byte(h[:n]))
}

// jsonValue decodes a JSON text column. A value that already arrived decoded (pgx json) is re-encoded first.
// ok=false for NULL / empty text; err reports malformed JSON.
func jsonValue(v any) (out any, ok bool, err error) {
	var raw []byte
	switch x := v.(type) {
	case nil:
		return nil, false, nil
	case string:
		raw = []byte(x)
	case []byte:
		raw = x
	case map[string]any, []any:
		return x, true, nil
	default:
		s, _ := flexStr(v)
		raw = []byte(s)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, false, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, false, err
	}
	if dec.More() {
		return nil, false, fmt.Errorf("trailing data after JSON value")
	}
	return out, true, nil
}

// jsonObject decodes a JSON object column (NULL / "" / "null" / [] → nil without error). A JSON string that
// itself contains an object (double encoding) is unwrapped once.
func jsonObject(v any) (map[string]any, error) {
	val, ok, err := jsonValue(v)
	if err != nil || !ok {
		return nil, err
	}
	switch x := val.(type) {
	case map[string]any:
		return x, nil
	case nil:
		return nil, nil
	case []any:
		if len(x) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("expected a JSON object, got an array")
	case string:
		if strings.TrimSpace(x) == "" {
			return nil, nil
		}
		return jsonObject(x)
	}
	return nil, fmt.Errorf("expected a JSON object, got %T", val)
}

// isEmptyJSON reports whether a JSON-ish column is NULL, "", "null", "[]", "{}" or an empty array/object.
func isEmptyJSON(v any) bool {
	val, ok, err := jsonValue(v)
	if err != nil {
		s, _ := flexStr(v)
		return strings.TrimSpace(s) == ""
	}
	if !ok || val == nil {
		return true
	}
	switch x := val.(type) {
	case map[string]any:
		return len(x) == 0
	case []any:
		return len(x) == 0
	case string:
		return strings.TrimSpace(x) == ""
	}
	return false
}
