package formats

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// A tiny YAML emitter for the block-style subset Clash configs need: ordered maps, lists and scalars.
// Writing YAML by hand keeps the output deterministic, readable and allocation-light; the tests parse every
// golden file with gopkg.in/yaml.v3.

type kv struct {
	k string
	v any // string, int, int64, bool, ymap, ylist, []string; nil values are skipped
}

// ymap is an ordered YAML mapping.
type ymap []kv

// ylist is a YAML sequence.
type ylist []any

func (m *ymap) set(k string, v any) { *m = append(*m, kv{k, v}) }

// setStr adds k only when v is not empty.
func (m *ymap) setStr(k, v string) {
	if v != "" {
		*m = append(*m, kv{k, v})
	}
}

// setTrue adds k: true only when v is true.
func (m *ymap) setTrue(k string, v bool) {
	if v {
		*m = append(*m, kv{k, true})
	}
}

func writeYAML(b *strings.Builder, m ymap) { m.write(b, 0, false) }

func pad(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}

// write emits the mapping at indent; when inline is true the first key continues the current line (list item).
func (m ymap) write(b *strings.Builder, indent int, inline bool) {
	first := true
	for _, e := range m {
		if e.v == nil {
			continue
		}
		if !(first && inline) {
			pad(b, indent)
		}
		first = false
		b.WriteString(yamlScalar(e.k))
		b.WriteByte(':')
		writeAfterKey(b, e.v, indent)
	}
	if first && inline {
		b.WriteString("{}\n")
	}
}

func writeAfterKey(b *strings.Builder, v any, indent int) {
	switch x := v.(type) {
	case ymap:
		if len(x) == 0 {
			b.WriteString(" {}\n")
			return
		}
		b.WriteByte('\n')
		x.write(b, indent+2, false)
	case ylist:
		if len(x) == 0 {
			b.WriteString(" []\n")
			return
		}
		b.WriteByte('\n')
		x.write(b, indent+2)
	case []string:
		if len(x) == 0 {
			b.WriteString(" []\n")
			return
		}
		b.WriteByte('\n')
		for _, s := range x {
			pad(b, indent+2)
			b.WriteString("- ")
			b.WriteString(yamlScalar(s))
			b.WriteByte('\n')
		}
	default:
		b.WriteByte(' ')
		writeScalar(b, v)
		b.WriteByte('\n')
	}
}

func (l ylist) write(b *strings.Builder, indent int) {
	for _, it := range l {
		pad(b, indent)
		b.WriteString("- ")
		switch x := it.(type) {
		case ymap:
			x.write(b, indent+2, true)
		default:
			writeScalar(b, it)
			b.WriteByte('\n')
		}
	}
}

func writeScalar(b *strings.Builder, v any) {
	switch x := v.(type) {
	case string:
		b.WriteString(yamlScalar(x))
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case bool:
		b.WriteString(strconv.FormatBool(x))
	default:
		b.WriteString(`""`)
	}
}

// yamlScalar returns s as a plain scalar when that is unambiguous in block context, otherwise double-quoted.
func yamlScalar(s string) string {
	if yamlPlainSafe(s) {
		return s
	}
	return dquote(s)
}

func yamlPlainSafe(s string) bool {
	if s == "" {
		return false
	}
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off", "null", "y", "n", "~":
		return false
	}
	c := s[0]
	if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '/') {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '_', c == '.', c == '/', c == '-', c == ',', c == '+':
		default:
			return false
		}
	}
	return true
}

// dquote returns s as a double-quoted YAML (and JSON-compatible) string.
func dquote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || r == 0x2028 || r == 0x2029 || r == 0xfeff || r == utf8.RuneError:
			b.WriteString(`\u`)
			h := strconv.FormatInt(int64(r), 16)
			b.WriteString(strings.Repeat("0", 4-len(h)))
			b.WriteString(h)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
