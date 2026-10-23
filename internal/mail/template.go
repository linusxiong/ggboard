package mail

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"strings"
	"sync/atomic"
	texttemplate "text/template"
	"time"
)

// Built-in template names.
const (
	TplVerifyCode    = "verify_code"
	TplRemindExpire  = "remind_expire"
	TplRemindTraffic = "remind_traffic"
	TplOrderPaid     = "order_paid"
	TplTicketReply   = "ticket_reply"
	TplNotify        = "notify"
)

// TemplateNames lists every built-in template.
var TemplateNames = []string{TplVerifyCode, TplRemindExpire, TplRemindTraffic, TplOrderPaid, TplTicketReply, TplNotify}

//go:embed templates/*
var templateFS embed.FS

// Brand is the site identity injected into every template (AppName, AppURL, Logo) plus the locale.
type Brand struct {
	AppName string
	AppURL  string
	Logo    string
	Locale  string // zh-CN | en
}

// Renderer renders the built-in templates, or an administrator's custom template (SetOverrides) in their place.
type Renderer struct {
	html map[string]*htmltemplate.Template
	text map[string]*texttemplate.Template

	custom    *htmltemplate.Template                       // layout around a custom body (MailBodyHTML)
	overrides atomic.Pointer[map[string]map[string]Custom] // name → locale → custom template
}

func placeholderFuncs() map[string]any {
	return map[string]any{
		"t":          func(string, ...any) string { return "" },
		"dict":       dict,
		"paragraphs": paragraphs,
	}
}

// NewRenderer parses the embedded templates.
func NewRenderer() (*Renderer, error) {
	r := &Renderer{html: map[string]*htmltemplate.Template{}, text: map[string]*texttemplate.Template{}}
	for _, name := range TemplateNames {
		h, err := htmltemplate.New(name).Funcs(placeholderFuncs()).ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("mail: parse %s.html: %w", name, err)
		}
		t, err := texttemplate.New(name+".txt").Funcs(placeholderFuncs()).ParseFS(templateFS, "templates/"+name+".txt")
		if err != nil {
			return nil, fmt.Errorf("mail: parse %s.txt: %w", name, err)
		}
		r.html[name], r.text[name] = h, t
	}
	c, err := htmltemplate.New("custom").Funcs(placeholderFuncs()).ParseFS(templateFS, "templates/layout.html")
	if err == nil {
		c, err = c.Parse(`{{define "content"}}{{.MailBodyHTML}}{{end}}`)
	}
	if err != nil {
		return nil, fmt.Errorf("mail: parse custom layout: %w", err)
	}
	r.custom = c
	return r, nil
}

// Render produces the subject, HTML and text bodies of template name for brand and data. A custom template for
// the brand's locale (SetOverrides) is rendered instead of the built-in one.
func (r *Renderer) Render(name string, brand Brand, data Data) (Message, error) {
	h, ok := r.html[name]
	if !ok {
		return Message{}, fmt.Errorf("mail: unknown template %q", name)
	}
	locale := NormalizeLocale(brand.Locale)
	if c, ok := r.Override(name, locale); ok {
		return r.RenderCustom(name, brand, c, data)
	}
	tr := translator(locale)
	view := map[string]any{
		"AppName": brand.AppName,
		"AppURL":  brand.AppURL,
		"Logo":    brand.Logo,
		"Locale":  locale,
		"Year":    time.Now().Year(),
		"URL":     brand.AppURL,
		"Minutes": 10,
	}
	// Pre-declare every field used by the templates so missing data renders empty (not "<no value>").
	for _, k := range []string{"Title", "Subject", "Content", "Message", "Code", "ExpiredAt", "Used", "Total", "TradeNo", "PlanName", "Period", "Amount"} {
		view[k] = ""
	}
	view["Percent"] = 0
	view["ContentHTML"] = htmltemplate.HTML("")
	for k, v := range data {
		view[k] = v
	}

	subject, err := renderSubject(name, locale, view)
	if err != nil {
		return Message{}, err
	}
	// MailSubject is the rendered e-mail subject (layout <title>). A caller-supplied Subject is template data
	// (e.g. the ticket subject of ticket_reply) and must not be replaced by the e-mail subject.
	view["MailSubject"] = subject
	if _, ok := data["Subject"]; !ok {
		view["Subject"] = subject
	}

	hc, err := h.Clone()
	if err != nil {
		return Message{}, err
	}
	var hb bytes.Buffer
	if err := hc.Funcs(htmltemplate.FuncMap{"t": tr}).ExecuteTemplate(&hb, "layout", view); err != nil {
		return Message{}, fmt.Errorf("mail: render %s.html: %w", name, err)
	}
	tc, err := r.text[name].Clone()
	if err != nil {
		return Message{}, err
	}
	var tb bytes.Buffer
	if err := tc.Funcs(texttemplate.FuncMap{"t": tr}).Execute(&tb, view); err != nil {
		return Message{}, fmt.Errorf("mail: render %s.txt: %w", name, err)
	}
	text := strings.TrimSpace(tb.String())
	footer := tr("footer.auto")
	if brand.AppURL != "" {
		footer += "\n" + brand.AppURL
	}
	text = brand.AppName + "\n\n" + text + "\n\n--\n" + footer + "\n"
	return Message{Subject: subject, HTML: hb.String(), Text: text, Template: name}, nil
}

func renderSubject(name, locale string, view map[string]any) (string, error) {
	src := lookup(locale, name+".subject")
	t, err := texttemplate.New("subject").Parse(src)
	if err != nil {
		return "", fmt.Errorf("mail: subject %s: %w", name, err)
	}
	var b bytes.Buffer
	if err := t.Execute(&b, view); err != nil {
		return "", fmt.Errorf("mail: subject %s: %w", name, err)
	}
	return strings.TrimSpace(b.String()), nil
}

func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}

func paragraphs(s any) []string {
	str, _ := s.(string)
	var out []string
	for _, p := range strings.Split(strings.ReplaceAll(str, "\r\n", "\n"), "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
