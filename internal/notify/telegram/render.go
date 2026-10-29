package telegram

import (
	"html"
	"strings"
	"unicode/utf8"

	"github.com/linusxiong/ggboard/internal/notify"
)

// Message limits, counted in characters of the HTML actually sent (escaped text plus markup). Telegram accepts 4096;
// measuring the escaped form keeps entity-heavy text within it ("&" becomes "&amp;", so a 1500-character body of
// them was 7500 characters on the wire and the API refused the message as too long, a permanent failure).
const (
	maxMessage = 3800
	maxTitle   = 300
	maxBody    = 1500
	maxFields  = 20
)

// codeFields are shown in <code> (tap to copy in Telegram).
var codeFields = map[string]bool{"trade_no": true, "callback_no": true, "ip": true}

// Render formats ev as a Telegram HTML message: bold title, body, then "label: value" lines. Every piece of
// event text is HTML-escaped; title and body are cut to their budgets and fields that do not fit the message
// limit are dropped.
func Render(ev notify.Event, locale string) string {
	var b strings.Builder
	b.WriteString("<b>" + escTruncate(ev.Title, maxTitle) + "</b>")
	if body := escTruncate(strings.TrimSpace(ev.Body), maxBody); body != "" {
		b.WriteString("\n" + body)
	}
	sep := ": "
	if notify.NormalizeLocale(locale) == notify.LocaleZH {
		sep = "："
	}
	budget := maxMessage - utf8.RuneCountInString(b.String())
	for i, f := range ev.Fields {
		if i >= maxFields {
			break
		}
		value := esc(f.Value)
		if codeFields[f.Key] {
			value = "<code>" + value + "</code>"
		}
		line := "\n" + esc(notify.Label(locale, f.Key)) + sep + value
		if i == 0 {
			line = "\n" + line
		}
		if budget -= utf8.RuneCountInString(line); budget < 0 {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func esc(s string) string { return html.EscapeString(s) }

// escTruncate escapes s and cuts it (ending in "…") so that the escaped form has at most n characters; an entity is
// never split.
func escTruncate(s string, n int) string {
	e := esc(s)
	if utf8.RuneCountInString(e) <= n {
		return e
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		piece := esc(string(r))
		k := utf8.RuneCountInString(piece)
		if used+k > n-1 {
			break
		}
		b.WriteString(piece)
		used += k
	}
	return b.String() + "…"
}
