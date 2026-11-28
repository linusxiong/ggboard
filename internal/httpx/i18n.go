package httpx

import (
	"regexp"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
)

// Error messages are written in English in the code and the logs. Fail translates the message and the field
// messages of the response into the language of the request (Accept-Language; the panel sends its UI locale), so
// a Chinese UI never shows the English text. Messages without a translation stay English.
//
// A catalog key is either the exact English message or a pattern: "{}" stands for any text and "{n}" for a number,
// and the translation takes the matched pieces in the same order at its own "{}" marks. The coverage test
// (i18n_coverage_test.go) fails when a message written in the code has no translation.

// Locales of API error messages.
const (
	LocaleEN = "en"
	LocaleZH = "zh-CN"
)

var catalogs = map[string]*catalog{LocaleZH: newCatalog(messagesZH)}

// RequestLocale returns the language of c's error messages: zh-CN when the first language of Accept-Language is
// Chinese, otherwise en.
func RequestLocale(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return LocaleEN
	}
	return ParseLocale(c.GetHeader("Accept-Language"))
}

// ParseLocale maps an Accept-Language value to LocaleZH or LocaleEN by its first language.
func ParseLocale(header string) string {
	first, _, _ := strings.Cut(header, ",")
	tag, _, _ := strings.Cut(first, ";")
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "zh" || strings.HasPrefix(tag, "zh-") || strings.HasPrefix(tag, "zh_") {
		return LocaleZH
	}
	return LocaleEN
}

// Translate returns msg in locale, or msg itself when the locale or the message has no translation.
func Translate(locale, msg string) string {
	if cat := catalogs[locale]; cat != nil {
		return cat.translate(msg)
	}
	return msg
}

// Localized returns a copy of e with its message and field messages in locale. e itself is never changed: errors
// are often package-level values shared by every request.
func (e *Error) Localized(locale string) *Error {
	cat := catalogs[locale]
	if e == nil || cat == nil {
		return e
	}
	out := *e
	out.Message = cat.translate(e.Message)
	if e.Errors != nil {
		out.Errors = make(map[string][]string, len(e.Errors))
		for field, msgs := range e.Errors {
			tr := make([]string, len(msgs))
			for i, m := range msgs {
				tr[i] = cat.translate(m)
			}
			out.Errors[field] = tr
		}
	}
	return &out
}

type catalog struct {
	exact    map[string]string
	patterns []msgPattern
}

type msgPattern struct {
	key    string
	re     *regexp.Regexp
	pieces []string // the translation split at its "{}" marks: len = captures + 1
	text   int      // bytes of literal text in key (its specificity)
}

var placeholderRe = regexp.MustCompile(`\{n?\}`)

func newCatalog(m map[string]string) *catalog {
	c := &catalog{exact: map[string]string{}}
	for key, tr := range m {
		if !placeholderRe.MatchString(key) {
			c.exact[key] = tr
			continue
		}
		var re strings.Builder
		re.WriteString("^")
		last := 0
		for _, loc := range placeholderRe.FindAllStringIndex(key, -1) {
			re.WriteString(regexp.QuoteMeta(key[last:loc[0]]))
			if key[loc[0]:loc[1]] == "{n}" {
				re.WriteString(`(-?[0-9]+(?:\.[0-9]+)?)`)
			} else {
				re.WriteString(`(.+?)`)
			}
			last = loc[1]
		}
		re.WriteString(regexp.QuoteMeta(key[last:]) + "$")
		c.patterns = append(c.patterns, msgPattern{
			key: key, re: regexp.MustCompile("(?s)" + re.String()), pieces: strings.Split(tr, "{}"),
			text: len(placeholderRe.ReplaceAllString(key, "")),
		})
	}
	// The most specific pattern (the most literal text) wins; ties go by key for a stable order.
	slices.SortFunc(c.patterns, func(a, b msgPattern) int {
		if d := b.text - a.text; d != 0 {
			return d
		}
		return strings.Compare(a.key, b.key)
	})
	return c
}

func (c *catalog) translate(msg string) string {
	if msg == "" {
		return msg
	}
	if tr, ok := c.exact[msg]; ok {
		return tr
	}
	for _, p := range c.patterns {
		m := p.re.FindStringSubmatch(msg)
		if m == nil || len(m) != len(p.pieces) {
			continue
		}
		var b strings.Builder
		for i, piece := range p.pieces {
			b.WriteString(piece)
			if i+1 < len(m) {
				b.WriteString(c.inner(m[i+1]))
			}
		}
		return b.String()
	}
	return msg
}

// inner translates a captured piece that is itself a whole message ("invalid catalog: {}" around a field
// message); anything else, such as a number or a name, comes back unchanged.
func (c *catalog) inner(s string) string {
	if tr, ok := c.exact[s]; ok {
		return tr
	}
	return s
}
