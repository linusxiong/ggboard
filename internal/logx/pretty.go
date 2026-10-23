package logx

import (
	"encoding"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lmittmann/tint"
)

// PrettyTimeFormat is the time column of the pretty text format: fixed width, unambiguous month, milliseconds.
const PrettyTimeFormat = "Jan 02 15:04:05.000"

// ANSI colour numbers for tint.Attr, matching tint's level colours (WRN bright yellow, ERR bright red).
const (
	colorYellow = 11
	colorRed    = 9
)

func newPrettyHandler(w io.Writer, level slog.Level, color bool) slog.Handler {
	return tint.NewTextHandler(w, &tint.Options{
		Level:       level,
		TimeFormat:  PrettyTimeFormat,
		NoColor:     !color,
		ReplaceAttr: prettyAttr,
	})
}

// prettyAttr adapts attributes for tint.
//
// Escape sequences: tint writes the message unquoted, and in colour mode it turns every `\x1b` in a quoted value
// back into a raw ESC (to keep colours inside values). Log values carry request data (errors that quote input,
// mail subjects, …), so a message with control characters is quoted like slog.TextHandler does, and ESC and the
// literal text `\x1b` are defused in every other value: nothing from a request can emit an escape sequence.
//
// Emphasis: error values are red, HTTP status codes 4xx yellow and 5xx red.
//
// It runs for every attribute of every pretty record, so the common cases (no control characters) only scan.
func prettyAttr(groups []string, a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindString:
		s := a.Value.String()
		if a.Key == slog.MessageKey && len(groups) == 0 {
			if hasControl(s) {
				a.Value = slog.StringValue(strconv.Quote(s))
			}
			return a
		}
		if needsDefuse(s) {
			a.Value = slog.StringValue(defuse(s))
		}
	case slog.KindInt64:
		if a.Key == "status" {
			switch n := a.Value.Int64(); {
			case n >= 500 && n < 600:
				return tint.Attr(colorRed, a)
			case n >= 400 && n < 500:
				return tint.Attr(colorYellow, a)
			}
		}
	case slog.KindAny:
		switch v := a.Value.Any().(type) {
		case slog.Level: // the level column
		case error:
			return tint.Attr(colorRed, slog.String(a.Key, defuse(safeString(v.Error))))
		case encoding.TextMarshaler:
			return slog.String(a.Key, defuse(safeString(func() string {
				b, err := v.MarshalText()
				if err != nil {
					return "!ERROR: " + err.Error()
				}
				return string(b)
			})))
		default:
			return slog.String(a.Key, defuse(safeString(func() string {
				if b, ok := v.([]byte); ok {
					return string(b)
				}
				return fmt.Sprintf("%+v", v)
			})))
		}
	}
	return a
}

// safeString calls fn, turning a panic (typically a nil pointer receiver) into "<nil>" like slog and tint do.
func safeString(fn func() string) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprintf("!PANIC: %v", r)
			if v := reflect.ValueOf(r); v.Kind() == reflect.Pointer && v.IsNil() {
				s = "<nil>"
			}
		}
	}()
	return fn()
}

const escapeText = `\x1b`

func needsDefuse(s string) bool {
	return strings.IndexByte(s, 0x1b) >= 0 || strings.Contains(s, escapeText)
}

// defuse replaces ESC with the visible ␛ and upper-cases the hex digits of the literal text `\x1b` (strconv.Quote
// only writes lower-case), so tint's colour mode finds nothing to turn back into an escape.
func defuse(s string) string {
	if !needsDefuse(s) {
		return s
	}
	s = strings.ReplaceAll(s, "\x1b", "␛")
	return strings.ReplaceAll(s, escapeText, `\x1B`)
}

// hasControl reports whether s contains a C0 or C1 control character, DEL or invalid UTF-8.
func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c == 0x7f {
			return true
		} else if c >= utf8.RuneSelf {
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError || r < 0xa0 {
				return true
			}
			i += size - 1
		}
	}
	return false
}
