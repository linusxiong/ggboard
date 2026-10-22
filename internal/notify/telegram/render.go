package telegram

import (
	"html"
	"strings"
	"unicode/utf8"

	"github.com/linusxiong/ggboard/internal/notify"
)

// Message limits: Telegram accepts 4096 characters after entity parsing; stay below with plain-text budgets.
const (
	maxMessage = 3800
	maxBody    = 1500
	maxFields  = 20
)

// codeFields are shown in <code> (tap to copy in Telegram).
var codeFields = map[string]bool{"trade_no": true, "callback_no": true, "ip": true}

// Render formats ev as a Telegram HTML message: bold title, body, then "label: value" lines. Every piece of
// event text is HTML-escaped; fields that do not fit the message limit are dropped.
func Render(ev notify.Event, locale string) string {
	var b strings.Builder
	b.WriteString("<b>" + esc(ev.Title) + "</b>")
	body := notify.Truncate(strings.TrimSpace(ev.Body), maxBody)
	if body != "" {
		b.WriteString("\n" + esc(body))
	}
	sep := ": "
	if notify.NormalizeLocale(locale) == notify.LocaleZH {
		sep = "："
	}
	budget := maxMessage - utf8.RuneCountInString(ev.Title) - utf8.RuneCountInString(body)
	for i, f := range ev.Fields {
		if i >= maxFields {
			break
		}
		label := notify.Label(locale, f.Key)
		budget -= utf8.RuneCountInString(label) + utf8.RuneCountInString(f.Value) + 2
		if budget < 0 {
			break
		}
		if i == 0 {
			b.WriteString("\n")
		}
		value := esc(f.Value)
		if codeFields[f.Key] {
			value = "<code>" + value + "</code>"
		}
		b.WriteString("\n" + esc(label) + sep + value)
	}
	return b.String()
}

func esc(s string) string { return html.EscapeString(s) }
