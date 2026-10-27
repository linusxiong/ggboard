package mail

import (
	"bytes"
	"fmt"
	"html"
	htmltemplate "html/template"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Custom templates (docs/API.md "Mail templates"): an administrator replaces a built-in e-mail for one locale with a
// one-line subject and a Markdown body containing {{name}} placeholders. Renderer.Render uses the override when
// there is one; the caller's Data does not change.

// TemplateLocales are the locales a template can be customised for.
var TemplateLocales = []string{"zh-CN", "en"}

// IsTemplateLocale reports whether l is one of TemplateLocales (exact match).
func IsTemplateLocale(l string) bool { return slices.Contains(TemplateLocales, l) }

// Size limits of a custom template.
const (
	MaxCustomSubject = 255   // characters, after trimming
	MaxCustomBody    = 65535 // bytes
)

// Custom is an administrator's replacement of a built-in template for one locale.
type Custom struct {
	Subject string // one line, {{name}} placeholders; empty renders the built-in subject
	Body    string // Markdown (GFM) with {{name}} placeholders
}

// Variable is one placeholder of a template's catalogue.
type Variable struct {
	Name     string `json:"name"`
	Required bool   `json:"required"` // must appear in the body
}

// templateVar maps a placeholder to the Data key the callers pass (BACKEND.md §6).
type templateVar struct {
	Variable
	key       string
	multiline bool // line breaks are kept: <br> in HTML
}

var templateVars = map[string][]templateVar{
	TplVerifyCode: {
		{Variable: Variable{Name: "code", Required: true}, key: "Code"},
		{Variable: Variable{Name: "minutes"}, key: "Minutes"},
	},
	TplRemindExpire: {
		{Variable: Variable{Name: "expiredAt", Required: true}, key: "ExpiredAt"},
		{Variable: Variable{Name: "url"}, key: "URL"},
	},
	TplRemindTraffic: {
		{Variable: Variable{Name: "percent", Required: true}, key: "Percent"},
		{Variable: Variable{Name: "used"}, key: "Used"},
		{Variable: Variable{Name: "total"}, key: "Total"},
		{Variable: Variable{Name: "url"}, key: "URL"},
	},
	TplOrderPaid: {
		{Variable: Variable{Name: "tradeNo", Required: true}, key: "TradeNo"},
		{Variable: Variable{Name: "amount", Required: true}, key: "Amount"},
		{Variable: Variable{Name: "planName"}, key: "PlanName"},
		{Variable: Variable{Name: "period"}, key: "Period"},
		{Variable: Variable{Name: "url"}, key: "URL"},
	},
	TplTicketReply: {
		{Variable: Variable{Name: "ticketSubject", Required: true}, key: "Subject"},
		{Variable: Variable{Name: "message"}, key: "Message", multiline: true},
		{Variable: Variable{Name: "url"}, key: "URL"},
	},
	// The flags TokenReset / UUIDReset / SignedOut choose the built-in sentences; a custom body is static text.
	TplSubscriptionReset: {
		{Variable: Variable{Name: "url"}, key: "URL"},
	},
	TplNotify: {
		{Variable: Variable{Name: "content", Required: true}, key: "Content", multiline: true},
		{Variable: Variable{Name: "title"}, key: "Title"},
		{Variable: Variable{Name: "url"}, key: "URL"},
	},
}

// commonVars are available in every template: the site name, app_url (may be empty) and the current year.
var commonVars = []Variable{{Name: "siteName"}, {Name: "siteUrl"}, {Name: "year"}}

// Variables returns the placeholder catalogue of template name (its own variables first, then the common ones), or
// nil for an unknown template.
func Variables(name string) []Variable {
	tv, ok := templateVars[name]
	if !ok {
		return nil
	}
	out := make([]Variable, 0, len(tv)+len(commonVars))
	for _, v := range tv {
		out = append(out, v.Variable)
	}
	return append(out, commonVars...)
}

var (
	// placeholderRe matches a placeholder exactly as substituted: no spaces, case-sensitive.
	placeholderRe = regexp.MustCompile(`\{\{([A-Za-z][A-Za-z0-9]*)\}\}`)
	// looseRe also matches near misses ({{ code }}, {{Code.x}}) so that Validate can report them.
	looseRe = regexp.MustCompile(`\{\{[^{}\r\n]{0,64}\}\}`)
)

// Validation is the outcome of Validate.
type Validation struct {
	// Limit holds size violations per field ("subject", "body"): they fail every request, a preview included.
	Limit map[string][]string
	// Strict holds the other field errors (empty or multi-line subject, empty body, unknown and missing
	// placeholders): they fail PUT and the test mail, while a preview lists Unknown and Missing instead.
	Strict map[string][]string
	// Unknown lists the unknown placeholders as written ("{{foo}}"), subject first, without duplicates.
	Unknown []string
	// Missing lists the required placeholders absent from the body ("{{code}}").
	Missing []string
}

// Errors returns every field error of a strict check (Limit and Strict merged); empty when the draft is valid.
func (v Validation) Errors() map[string][]string {
	out := make(map[string][]string, len(v.Limit)+len(v.Strict))
	for _, m := range []map[string][]string{v.Limit, v.Strict} {
		for k, msgs := range m {
			out[k] = append(out[k], msgs...)
		}
	}
	return out
}

const errNUL = "must not contain NUL characters"

func addField(m *map[string][]string, field, msg string) {
	if *m == nil {
		*m = map[string][]string{}
	}
	(*m)[field] = append((*m)[field], msg)
}

// Validate checks a draft of template name: the size limits, a one-line non-empty subject, a non-empty body, that
// every placeholder is in Variables(name) and that the body contains every required one. Fields over their size
// limit are not scanned further. Shared by PUT (strict), the test mail (strict) and the preview (lenient).
func Validate(name, subject, body string) Validation {
	var v Validation
	known := map[string]bool{}
	for _, x := range Variables(name) {
		known[x.Name] = true
	}
	seen := map[string]bool{}
	unknownIn := func(field, s string) {
		var found []string
		for _, m := range looseRe.FindAllString(s, -1) {
			if known[m[2:len(m)-2]] || seen[m] {
				continue
			}
			seen[m] = true
			found = append(found, m)
		}
		if len(found) > 0 {
			v.Unknown = append(v.Unknown, found...)
			addField(&v.Strict, field, "unknown placeholders: "+strings.Join(found, ", "))
		}
	}

	subject = strings.TrimSpace(subject)
	switch {
	case utf8.RuneCountInString(subject) > MaxCustomSubject:
		addField(&v.Limit, "subject", fmt.Sprintf("must be at most %d characters", MaxCustomSubject))
	case subject == "":
		addField(&v.Strict, "subject", "is required")
	default:
		if strings.ContainsAny(subject, "\r\n") {
			addField(&v.Strict, "subject", "must be a single line")
		}
		if strings.ContainsRune(subject, 0) {
			addField(&v.Strict, "subject", errNUL) // PostgreSQL text cannot store it
		}
		unknownIn("subject", subject)
	}

	switch {
	case len(body) > MaxCustomBody:
		addField(&v.Limit, "body", fmt.Sprintf("must be at most %d bytes", MaxCustomBody))
	case strings.TrimSpace(body) == "":
		addField(&v.Strict, "body", "is required")
		v.Missing = missingRequired(name, body) // listed by the preview
	default:
		if strings.ContainsRune(body, 0) {
			addField(&v.Strict, "body", errNUL)
		}
		unknownIn("body", body)
		if v.Missing = missingRequired(name, body); len(v.Missing) > 0 {
			addField(&v.Strict, "body", "missing required placeholders: "+strings.Join(v.Missing, ", "))
		}
	}
	if v.Unknown == nil {
		v.Unknown = []string{}
	}
	if v.Missing == nil {
		v.Missing = []string{}
	}
	return v
}

func missingRequired(name, body string) []string {
	var out []string
	for _, x := range templateVars[name] {
		if p := "{{" + x.Name + "}}"; x.Required && !strings.Contains(body, p) {
			out = append(out, p)
		}
	}
	return out
}

// SampleData returns realistic data for template name in locale (example.com addresses, fake order numbers) for
// the admin preview and test mail. Keys are the ones the real callers pass.
func SampleData(name, locale string) Data {
	zh := NormalizeLocale(locale) == "zh-CN"
	pick := func(zhValue, enValue string) string {
		if zh {
			return zhValue
		}
		return enValue
	}
	switch name {
	case TplVerifyCode:
		return Data{"Code": "482913", "Minutes": 10}
	case TplRemindExpire:
		return Data{"ExpiredAt": "2026-11-01 20:00"}
	case TplRemindTraffic:
		return Data{"Percent": 86, "Used": "86.00 GB", "Total": "100.00 GB"}
	case TplOrderPaid:
		return Data{
			"TradeNo":  "2026102314302291834",
			"PlanName": pick("专业版", "Pro"),
			"Period":   pick("月付", "Monthly"),
			"Amount":   "¥25.00",
			"URL":      "https://panel.example.com/orders/2026102314302291834",
		}
	case TplTicketReply:
		return Data{
			"Subject": pick("无法连接香港节点", "Cannot connect to the Hong Kong node"),
			"Message": pick("您好，节点已经恢复。\n请在客户端更新订阅后重试。",
				"Hello, the node is back online.\nPlease update your subscription in the client and try again."),
			"URL": "https://panel.example.com/tickets/1024",
		}
	case TplSubscriptionReset:
		return Data{"URL": "https://panel.example.com/subscription"}
	case TplNotify:
		return Data{
			"Subject": pick("系统维护通知", "Scheduled maintenance"),
			"Content": pick("我们将于 10 月 25 日 02:00–04:00 进行系统维护。\n\n维护期间服务可能短暂中断，感谢您的理解。",
				"We will perform scheduled maintenance on October 25, 02:00–04:00.\n\nThe service may be briefly unavailable during this window."),
		}
	}
	return Data{}
}

// DefaultCustom returns the built-in template name in locale written as a custom template: the starting point of
// the editor (GET /admin/mail-templates/:name "default"). It reuses the i18n strings of the built-in files; the
// verification code box and the traffic bar are raw HTML blocks with the built-in styles. Unknown names give "".
func DefaultCustom(name, locale string) (subject, body string) {
	if _, ok := templateVars[name]; !ok {
		return "", ""
	}
	locale = NormalizeLocale(locale)
	t := translator(locale)
	sep := ": "
	if locale == "zh-CN" {
		sep = "："
	}
	subject = strings.ReplaceAll(lookup(locale, name+".subject"), "{{.AppName}}", "{{siteName}}")
	var parts []string
	switch name {
	case TplVerifyCode:
		parts = []string{
			"# " + t("verify_code.title"),
			t("verify_code.body", "{{minutes}}"),
			`<div style="margin:8px 0 8px;padding:18px 0;background:#F2F2F7;border-radius:12px;text-align:center;font-size:34px;line-height:40px;font-weight:700;letter-spacing:8px;color:#1C1C1E;font-family:'SF Mono',ui-monospace,Menlo,Consolas,monospace;">{{code}}</div>`,
			t("verify_code.ignore"),
		}
	case TplRemindExpire:
		parts = []string{
			"# " + t("remind_expire.title"),
			t("remind_expire.body", "{{expiredAt}}"),
			"[" + t("button.renew") + "]({{url}})",
		}
	case TplRemindTraffic:
		parts = []string{
			"# " + t("remind_traffic.title"),
			t("remind_traffic.body", "{{percent}}", "{{used}}", "{{total}}"),
			`<div style="margin:8px 0 8px;height:8px;background:#E5E5EA;border-radius:4px;overflow:hidden;"><div style="width:{{percent}}%;height:8px;background:#FF9500;border-radius:4px;"></div></div>`,
			"[" + t("button.open") + "]({{url}})",
		}
	case TplOrderPaid:
		parts = []string{
			"# " + t("order_paid.title"),
			t("order_paid.body"),
			strings.Join([]string{
				"- " + t("order.trade_no") + sep + "{{tradeNo}}",
				"- " + t("order.plan") + sep + "{{planName}}",
				"- " + t("order.period") + sep + "{{period}}",
				"- " + t("order.amount") + sep + "**{{amount}}**",
			}, "\n"),
			"[" + t("button.view_order") + "]({{url}})",
		}
	case TplTicketReply:
		parts = []string{
			"# " + t("ticket_reply.title"),
			t("ticket_reply.body", "{{ticketSubject}}"),
			"> {{message}}",
			"[" + t("button.view_ticket") + "]({{url}})",
		}
	case TplSubscriptionReset: // the text of POST /admin/users/:id/reset-secret: token and uuid, sessions ended
		parts = []string{
			"# " + t("subscription_reset.title"),
			t("subscription_reset.link_uuid"),
			t("subscription_reset.signed_out"),
			t("subscription_reset.action"),
			"[" + t("button.subscription") + "]({{url}})",
		}
	case TplNotify:
		subject = "{{title}}" // the caller's subject (alerts, bulk mail) must survive the customisation
		parts = []string{"# {{title}}", "{{content}}", "[" + t("button.open") + "]({{url}})"}
	}
	return subject, strings.Join(parts, "\n\n") + "\n"
}

// SetOverrides replaces every custom template: m maps a template name to its locales. Unknown names and locales
// are ignored; nil removes every override. Safe for concurrent use with Render.
func (r *Renderer) SetOverrides(m map[string]map[string]Custom) {
	cp := make(map[string]map[string]Custom, len(m))
	for name, locales := range m {
		if _, ok := templateVars[name]; !ok {
			continue
		}
		for l, c := range locales {
			if !IsTemplateLocale(l) {
				continue
			}
			if cp[name] == nil {
				cp[name] = map[string]Custom{}
			}
			cp[name][l] = c
		}
	}
	r.overrides.Store(&cp)
}

// Override returns the custom template of name for locale, if any.
func (r *Renderer) Override(name, locale string) (Custom, bool) {
	m := r.overrides.Load()
	if m == nil {
		return Custom{}, false
	}
	c, ok := (*m)[name][locale]
	return c, ok
}

// RenderCustom renders c in place of the built-in template name for brand and data: the subject with raw values
// (one line; empty → the built-in subject), the Markdown body inside the standard layout and the plain-text part
// derived from the same Markdown (markdown.go).
func (r *Renderer) RenderCustom(name string, brand Brand, c Custom, data Data) (Message, error) {
	if _, ok := templateVars[name]; !ok {
		return Message{}, fmt.Errorf("mail: unknown template %q", name)
	}
	locale := NormalizeLocale(brand.Locale)
	tr := translator(locale)
	builtin, err := builtinSubject(name, locale, brand, data)
	if err != nil {
		return Message{}, err
	}
	vals := customValues(name, brand, data, builtin)
	subject := customSubject(c.Subject, vals)
	if subject == "" {
		subject = builtin
	}
	bodyHTML, bodyText, err := renderMarkdown(c.Body, vals)
	if err != nil {
		return Message{}, fmt.Errorf("mail: render custom %s: %w", name, err)
	}
	view := map[string]any{
		"AppName":      brand.AppName,
		"AppURL":       brand.AppURL,
		"Logo":         brand.Logo,
		"Locale":       locale,
		"Year":         time.Now().Year(),
		"MailSubject":  subject,
		"MailBodyHTML": htmltemplate.HTML(bodyHTML), //nolint:gosec // G203: built by renderMarkdown, every value escaped
	}
	hc, err := r.custom.Clone()
	if err != nil {
		return Message{}, err
	}
	var hb bytes.Buffer
	if err := hc.Funcs(htmltemplate.FuncMap{"t": tr}).ExecuteTemplate(&hb, "layout", view); err != nil {
		return Message{}, fmt.Errorf("mail: render custom %s layout: %w", name, err)
	}
	footer := tr("footer.auto")
	if brand.AppURL != "" {
		footer += "\n" + brand.AppURL
	}
	text := brand.AppName + "\n\n" + strings.TrimSpace(bodyText) + "\n\n--\n" + footer + "\n"
	return Message{Subject: subject, HTML: hb.String(), Text: text, Template: name}, nil
}

// builtinSubject renders the built-in subject of name, the fallback of an empty custom subject and of notify's
// title.
func builtinSubject(name, locale string, brand Brand, data Data) (string, error) {
	view := map[string]any{"AppName": brand.AppName, "Subject": ""}
	for k, v := range data {
		view[k] = v
	}
	return renderSubject(name, locale, view)
}

// customValue is one placeholder value: raw for the subject and the plain-text part, HTML-safe for the body.
type customValue struct {
	text    string
	html    string
	trusted bool // html is notify's ContentHTML, inserted as is
}

// customValues maps the caller's Data to the placeholders of template name (BACKEND.md §6): url falls back to the
// site URL, minutes to 10 like the built-in template, notify's title to the built-in subject (the caller's
// Subject) and its content to ContentHTML (template.HTML, sanitized by the caller) when given.
func customValues(name string, brand Brand, data Data, subject string) map[string]customValue {
	vals := make(map[string]customValue, len(templateVars[name])+len(commonVars))
	set := func(n, s string, multiline bool) {
		s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
		h := html.EscapeString(s)
		if multiline {
			h = strings.ReplaceAll(h, "\n", "<br>")
		}
		vals[n] = customValue{text: s, html: h}
	}
	for _, v := range templateVars[name] {
		set(v.Name, dataString(data[v.key]), v.multiline)
	}
	if vals["url"].text == "" {
		set("url", brand.AppURL, false)
	}
	switch name {
	case TplVerifyCode:
		if _, ok := data["Minutes"]; !ok {
			set("minutes", "10", false)
		}
	case TplNotify:
		if vals["title"].text == "" {
			set("title", subject, false)
		}
		if h, ok := data["ContentHTML"].(htmltemplate.HTML); ok && h != "" {
			text := vals["content"].text
			if text == "" {
				text = htmlToText(string(h))
			}
			vals["content"] = customValue{text: text, html: string(h), trusted: true}
		}
	}
	set("siteName", brand.AppName, false)
	set("siteUrl", brand.AppURL, false)
	set("year", strconv.Itoa(time.Now().Year()), false)
	return vals
}

// dataString formats a Data value for a placeholder (nil → "").
func dataString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case htmltemplate.HTML:
		return string(x)
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

// customSubject substitutes raw values into a custom subject and collapses it to one trimmed line. Unknown
// placeholders stay literal.
func customSubject(src string, vals map[string]customValue) string {
	s := placeholderRe.ReplaceAllStringFunc(src, func(m string) string {
		if v, ok := vals[m[2:len(m)-2]]; ok {
			return v.text
		}
		return m
	})
	return strings.Join(strings.Fields(s), " ")
}
