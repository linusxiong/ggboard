package mail

import (
	"context"
	htmltemplate "html/template"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/db"
	"github.com/linusxiong/ggboard/internal/logx"
	"github.com/linusxiong/ggboard/internal/model"
)

var customBrand = Brand{AppName: "Example Panel", AppURL: "https://panel.example.com", Locale: "zh-CN"}

func customRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func brandIn(locale string) Brand {
	b := customBrand
	b.Locale = locale
	return b
}

func TestCustomVariablesCatalogue(t *testing.T) {
	want := map[string]string{ // ★ = required
		TplVerifyCode:    "code★ minutes",
		TplRemindExpire:  "expiredAt★ url",
		TplRemindTraffic: "percent★ used total url",
		TplOrderPaid:     "tradeNo★ amount★ planName period url",
		TplTicketReply:   "ticketSubject★ message url",
		TplNotify:        "content★ title url",
	}
	for _, name := range TemplateNames {
		var got []string
		for _, v := range Variables(name) {
			s := v.Name
			if v.Required {
				s += "★"
			}
			got = append(got, s)
		}
		if g := strings.Join(got, " "); g != want[name]+" siteName siteUrl year" {
			t.Errorf("%s variables = %q", name, g)
		}
	}
	if Variables("nope") != nil || Variables("layout") != nil {
		t.Fatal("unknown templates have no catalogue")
	}
	if !IsTemplateLocale("zh-CN") || !IsTemplateLocale("en") || IsTemplateLocale("zh") || IsTemplateLocale("EN") {
		t.Fatal("IsTemplateLocale")
	}
}

func TestCustomValidate(t *testing.T) {
	for _, name := range TemplateNames {
		for _, l := range TemplateLocales {
			s, b := DefaultCustom(name, l)
			if s == "" || b == "" {
				t.Fatalf("%s/%s: empty default", name, l)
			}
			if v := Validate(name, s, b); len(v.Errors()) != 0 || len(v.Unknown) != 0 || len(v.Missing) != 0 {
				t.Errorf("%s/%s: the default must validate: %+v", name, l, v)
			}
		}
	}
	if s, b := DefaultCustom("nope", "en"); s != "" || b != "" {
		t.Fatal("unknown template default")
	}

	v := Validate(TplVerifyCode, "{{foo}} {{siteName}} {{foo}}", "Code {{code}} {{ code }} {{Code}} {{bar}} {{foo}} {{}}")
	if got := v.Strict["subject"]; !slices.Equal(got, []string{"unknown placeholders: {{foo}}"}) {
		t.Errorf("subject errors = %q", got)
	}
	if got := v.Strict["body"]; !slices.Equal(got, []string{"unknown placeholders: {{ code }}, {{Code}}, {{bar}}, {{}}"}) {
		t.Errorf("body errors = %q", got)
	}
	if !slices.Equal(v.Unknown, []string{"{{foo}}", "{{ code }}", "{{Code}}", "{{bar}}", "{{}}"}) || len(v.Missing) != 0 || len(v.Limit) != 0 {
		t.Errorf("unknown = %q, missing = %q, limit = %v", v.Unknown, v.Missing, v.Limit)
	}

	v = Validate(TplOrderPaid, "Paid", "Order {{planName}} — {{url}}")
	if !slices.Equal(v.Missing, []string{"{{tradeNo}}", "{{amount}}"}) ||
		!slices.Equal(v.Strict["body"], []string{"missing required placeholders: {{tradeNo}}, {{amount}}"}) {
		t.Errorf("missing = %q, body = %q", v.Missing, v.Strict["body"])
	}
	// A required placeholder in the subject does not count.
	if v = Validate(TplVerifyCode, "{{code}}", "no code"); !slices.Equal(v.Missing, []string{"{{code}}"}) {
		t.Errorf("subject must not satisfy a required placeholder: %+v", v)
	}

	v = Validate(TplVerifyCode, "  \n ", "  ")
	if !slices.Equal(v.Strict["subject"], []string{"is required"}) || !slices.Equal(v.Strict["body"], []string{"is required"}) ||
		!slices.Equal(v.Missing, []string{"{{code}}"}) || len(v.Limit) != 0 {
		t.Errorf("empty draft = %+v", v)
	}
	if v = Validate(TplVerifyCode, "line one\nline two", "{{code}}"); !slices.Equal(v.Strict["subject"], []string{"must be a single line"}) {
		t.Errorf("multi-line subject = %+v", v)
	}
	if v = Validate(TplVerifyCode, "a\x00b", "{{code}}\x00"); !slices.Equal(v.Strict["subject"], []string{errNUL}) || !slices.Equal(v.Strict["body"], []string{errNUL}) {
		t.Errorf("NUL = %+v", v)
	}
	if v = Validate(TplVerifyCode, "  padded subject  ", "{{code}}"); len(v.Errors()) != 0 {
		t.Errorf("surrounding spaces are trimmed: %+v", v)
	}

	// Limits: characters for the subject (CJK counts once), bytes for the body; oversized fields are not scanned.
	if v = Validate(TplVerifyCode, strings.Repeat("验", MaxCustomSubject), "{{code}}"); len(v.Errors()) != 0 {
		t.Errorf("255 characters must be accepted: %+v", v)
	}
	v = Validate(TplVerifyCode, strings.Repeat("验", MaxCustomSubject+1)+"{{foo}}", "{{code}}"+strings.Repeat("x", MaxCustomBody))
	if !slices.Equal(v.Limit["subject"], []string{"must be at most 255 characters"}) ||
		!slices.Equal(v.Limit["body"], []string{"must be at most 65535 bytes"}) || len(v.Strict) != 0 || len(v.Unknown) != 0 {
		t.Errorf("limits = %+v", v)
	}
	if e := v.Errors(); len(e["subject"]) != 1 || len(e["body"]) != 1 {
		t.Errorf("Errors merges the limits: %v", e)
	}
	if v = Validate(TplVerifyCode, "s", "{{code}}"+strings.Repeat("x", MaxCustomBody-8)); len(v.Errors()) != 0 {
		t.Errorf("65535 bytes must be accepted: %+v", v)
	}
}

// The built-in templates rewritten as Markdown keep the subjects and every value of the built-in mails.
func TestCustomDefaultsMatchBuiltin(t *testing.T) {
	r := customRenderer(t)
	for _, name := range TemplateNames {
		for _, l := range TemplateLocales {
			data := SampleData(name, l)
			builtin, err := r.Render(name, brandIn(l), data)
			if err != nil {
				t.Fatal(err)
			}
			s, b := DefaultCustom(name, l)
			custom, err := r.RenderCustom(name, brandIn(l), Custom{Subject: s, Body: b}, data)
			if err != nil {
				t.Fatal(err)
			}
			if custom.Subject != builtin.Subject || custom.Template != name {
				t.Errorf("%s/%s: subject %q, built-in %q", name, l, custom.Subject, builtin.Subject)
			}
			for k, v := range data {
				val := dataString(v)
				if k == "Content" || k == "Message" {
					val = strings.Split(val, "\n")[0]
				}
				if !strings.Contains(custom.Text, val) || !strings.Contains(custom.HTML, htmltemplate.HTMLEscapeString(val)) {
					t.Errorf("%s/%s: %s=%q missing:\n%s\n%s", name, l, k, val, custom.Text, custom.HTML)
				}
			}
			for _, want := range []string{`<html lang="` + l + `">`, "<title>" + htmltemplate.HTMLEscapeString(builtin.Subject) + "</title>",
				`border-radius:18px;padding:36px 32px;`, translator(l)("footer.auto")} {
				if !strings.Contains(custom.HTML, want) {
					t.Errorf("%s/%s: layout part %q missing", name, l, want)
				}
			}
			if !strings.HasPrefix(custom.Text, "Example Panel\n\n") || !strings.HasSuffix(custom.Text, "\n\n--\n"+translator(l)("footer.auto")+"\nhttps://panel.example.com\n") {
				t.Errorf("%s/%s: text header/footer:\n%s", name, l, custom.Text)
			}
			for _, bad := range []string{"{{", "<no value>", "%!"} {
				if strings.Contains(custom.HTML, bad) || strings.Contains(custom.Text, bad) {
					t.Errorf("%s/%s: output contains %q", name, l, bad)
				}
			}
			if tokenRe.MatchString(custom.HTML) || tokenRe.MatchString(custom.Text) {
				t.Errorf("%s/%s: a token leaked", name, l)
			}
		}
	}
	// The default verify_code keeps the code box and the order mail its button.
	msg, _ := r.RenderCustom(TplVerifyCode, customBrand, defaultCustom(TplVerifyCode, "zh-CN"), Data{"Code": "246810"})
	if !strings.Contains(msg.HTML, `letter-spacing:8px;color:#1C1C1E;font-family:'SF Mono',ui-monospace,Menlo,Consolas,monospace;">246810</div>`) ||
		!strings.Contains(msg.Text, "\n\n246810\n\n") || !strings.Contains(msg.Text, "10 分钟内有效") {
		t.Errorf("verify code box:\n%s\n%s", msg.HTML, msg.Text)
	}
	msg, _ = r.RenderCustom(TplOrderPaid, customBrand, defaultCustom(TplOrderPaid, "zh-CN"), SampleData(TplOrderPaid, "zh-CN"))
	if !strings.Contains(msg.HTML, buttonOpen+`<a href="https://panel.example.com/orders/2026102314302291834" style="`+styleButtonLink+`">查看订单</a>`+buttonClose) ||
		!strings.Contains(msg.Text, "查看订单 (https://panel.example.com/orders/2026102314302291834)") ||
		!strings.Contains(msg.HTML, "金额：<strong>¥25.00</strong>") {
		t.Errorf("order button:\n%s\n%s", msg.HTML, msg.Text)
	}
}

func defaultCustom(name, locale string) Custom {
	s, b := DefaultCustom(name, locale)
	return Custom{Subject: s, Body: b}
}

var tokenRe = regexp.MustCompile(`mt[a-z2-7]{12}\d{3}z`)

// Placeholders survive goldmark everywhere: link destinations, raw HTML blocks and inline tags, code, tables,
// images, autolinks.
func TestCustomTokensSurvive(t *testing.T) {
	r := customRenderer(t)
	body := strings.Join([]string{
		"[查看订单]({{url}}) and [site]({{siteUrl}}/orders?no={{code}}&x=1 \"t {{code}}\")",
		"<div>{{code}}</div>",
		"inline <span title=\"{{code}}\">{{code}}</span> and `{{code}}` and **{{code}}**中文**{{code}}**中文",
		"```\nfenced {{code}}\n```",
		"| a | b |\n| --- | :-: |\n| {{code}} | {{minutes}} |",
		"![logo {{code}}]({{siteUrl}}/logo.png)",
		"https://example.com/check/{{code}}",
		"<!-- {{code}} --> © {{year}} {{siteName}}",
	}, "\n\n")
	msg, err := r.RenderCustom(TplVerifyCode, customBrand, Custom{Subject: "s", Body: body}, Data{"Code": "123456", "Minutes": 15})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<a href="https://panel.example.com" style="` + styleLink + `">查看订单</a>`,
		`<a href="https://panel.example.com/orders?no=123456&amp;x=1" title="t 123456" style="` + styleLink + `">site</a>`,
		"<div>123456</div>",
		`<span title="123456">123456</span>`,
		`<code style="` + styleCode + `">123456</code>`,
		"<strong>123456</strong>中文<strong>123456</strong>中文",
		`<code style="` + stylePreCode + `">fenced 123456`,
		`<td style="` + styleTD + `">123456</td>`,
		`<td style="` + styleTD + `;text-align:center">15</td>`,
		`<img src="https://panel.example.com/logo.png" alt="logo 123456" style="` + styleImage + `">`,
		`<a href="https://example.com/check/123456" style="` + styleLink + `">https://example.com/check/123456</a>`,
		"<!-- 123456 --> © 2026 Example Panel",
	} {
		if !strings.Contains(msg.HTML, strings.ReplaceAll(want, "2026", yearString())) {
			t.Errorf("HTML lacks %s\n%s", want, msg.HTML)
		}
	}
	if tokenRe.MatchString(msg.HTML) || tokenRe.MatchString(msg.Text) || strings.Contains(msg.HTML, "{{") {
		t.Fatalf("a token or placeholder leaked:\n%s\n%s", msg.HTML, msg.Text)
	}
	for _, want := range []string{
		"查看订单 (https://panel.example.com) and site (https://panel.example.com/orders?no=123456&x=1)",
		"\n\n123456\n\n",
		"inline 123456 and 123456 and 123456中文123456中文",
		"fenced 123456",
		"a | b\n123456 | 15",
		"logo 123456",
		"https://example.com/check/123456",
		"© " + yearString() + " Example Panel",
	} {
		if !strings.Contains(msg.Text, want) {
			t.Errorf("text lacks %q\n%s", want, msg.Text)
		}
	}
}

func yearString() string { return customValues(TplNotify, customBrand, nil, "")["year"].text }

// Values are never read as Markdown, HTML or placeholders, in the body, the subject and the plain text.
func TestCustomValuesNeverInterpreted(t *testing.T) {
	r := customRenderer(t)
	data := Data{
		"Subject": `<img src=x onerror="alert(1)"> *subj*`,
		"Message": "*bold* _x_ # h\n<script>alert(1)</script>\n{{code}} {{ticketSubject}}\r\n[a](javascript:alert(1))",
		"URL":     `https://panel.example.com/tickets/1?a=1&b="2"`,
	}
	c := Custom{
		Subject: "Re: {{ticketSubject}} {{message}}",
		Body:    "# {{ticketSubject}}\n\n> {{message}}\n\n<p title=\"{{ticketSubject}}\">{{message}}</p>\n\n[open]({{url}})",
	}
	msg, err := r.RenderCustom(TplTicketReply, customBrand, c, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script>", "<em>", `<img src=x`, `<h1 style="` + styleH1 + `">h`, "javascript:alert(1)\"", `<a href="javascript`} {
		if strings.Contains(msg.HTML, bad) {
			t.Errorf("value interpreted (%s):\n%s", bad, msg.HTML)
		}
	}
	for _, want := range []string{
		"*bold* _x_ # h<br>&lt;script&gt;alert(1)&lt;/script&gt;<br>{{code}} {{ticketSubject}}<br>[a](javascript:alert(1))",
		`<h1 style="` + styleH1 + `">&lt;img src=x onerror=&#34;alert(1)&#34;&gt; *subj*</h1>`,
		`<p title="&lt;img src=x onerror=&#34;alert(1)&#34;&gt; *subj*">*bold*`,
		`href="https://panel.example.com/tickets/1?a=1&amp;b=&#34;2&#34;"`,
	} {
		if !strings.Contains(msg.HTML, want) {
			t.Errorf("HTML lacks %s\n%s", want, msg.HTML)
		}
	}
	wantSubject := `Re: <img src=x onerror="alert(1)"> *subj* *bold* _x_ # h <script>alert(1)</script> {{code}} {{ticketSubject}} [a](javascript:alert(1))`
	if msg.Subject != wantSubject {
		t.Errorf("subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.Text, "*bold* _x_ # h\n<script>alert(1)</script>\n{{code}} {{ticketSubject}}\n[a](javascript:alert(1))") ||
		!strings.Contains(msg.Text, `<img src=x onerror="alert(1)"> *subj*`) {
		t.Errorf("text must carry the raw values:\n%s", msg.Text)
	}
}

// A link whose address is empty once substituted is dropped: a button disappears, an inline link keeps its text.
func TestCustomEmptyLinks(t *testing.T) {
	r := customRenderer(t)
	noURL := Brand{AppName: "Example Panel", Locale: "en"}
	msg, err := r.RenderCustom(TplRemindExpire, noURL, defaultCustom(TplRemindExpire, "en"), Data{"ExpiredAt": "2026-11-01"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg.HTML, "Renew now</a>") || strings.Contains(msg.HTML, buttonOpen) || strings.Contains(msg.Text, "Renew now (") {
		t.Errorf("button without an address:\n%s\n%s", msg.HTML, msg.Text)
	}
	msg, _ = r.RenderCustom(TplRemindExpire, noURL, Custom{Body: "See [the panel]({{url}}) now: {{expiredAt}}\n\n> [quoted]({{siteUrl}})"}, Data{"ExpiredAt": "x"})
	if strings.Contains(msg.HTML, "<a ") || !strings.Contains(msg.HTML, "See the panel now: x") || !strings.Contains(msg.HTML, ">quoted</p>") {
		t.Errorf("inline empty link:\n%s", msg.HTML)
	}
	if !strings.Contains(msg.Text, "See the panel now: x") || strings.Contains(msg.Text, "()") {
		t.Errorf("inline empty link text:\n%s", msg.Text)
	}
	// The caller's URL wins over the site URL; without one the site URL is used.
	msg, _ = r.RenderCustom(TplRemindExpire, customBrand, Custom{Body: "[go]({{url}})\n\n{{expiredAt}}"}, Data{"ExpiredAt": "x"})
	if !strings.Contains(msg.HTML, buttonOpen) || !strings.Contains(msg.HTML, `href="https://panel.example.com"`) {
		t.Errorf("url must fall back to the site URL:\n%s", msg.HTML)
	}
}

func TestCustomNotify(t *testing.T) {
	r := customRenderer(t)
	c := defaultCustom(TplNotify, "zh-CN")
	msg, err := r.RenderCustom(TplNotify, customBrand, c, Data{"Subject": "订阅链接疑似泄露", "Content": "第一段。\n\n第二段 <b>x</b>"})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Subject != "订阅链接疑似泄露" || !strings.Contains(msg.HTML, ">订阅链接疑似泄露</h1>") ||
		!strings.Contains(msg.HTML, "第一段。<br><br>第二段 &lt;b&gt;x&lt;/b&gt;") || !strings.Contains(msg.Text, "第一段。\n\n第二段 <b>x</b>") {
		t.Errorf("notify:\n%s\n%s\n%s", msg.Subject, msg.HTML, msg.Text)
	}
	// Without a Subject the title and the subject fall back to the built-in subject; Title wins over it.
	msg, _ = r.RenderCustom(TplNotify, customBrand, c, Data{"Content": "x"})
	if msg.Subject != "【Example Panel】通知" || !strings.Contains(msg.HTML, ">【Example Panel】通知</h1>") {
		t.Errorf("notify fallback: %q", msg.Subject)
	}
	msg, _ = r.RenderCustom(TplNotify, customBrand, c, Data{"Subject": "S", "Title": "T", "Content": "x"})
	if msg.Subject != "T" || !strings.Contains(msg.HTML, ">T</h1>") {
		t.Errorf("notify title: %q", msg.Subject)
	}
	// Trusted ContentHTML is inserted as is; alone in a paragraph it becomes a <div> (block HTML inside a <p> is
	// invalid). The text part uses Content, else the HTML without tags.
	html := htmltemplate.HTML(`<p><b>bold</b> &amp; <a href="https://panel.example.com/subscription">link</a></p>`)
	msg, _ = r.RenderCustom(TplNotify, customBrand, c, Data{"Subject": "S", "ContentHTML": html})
	if !strings.Contains(msg.HTML, `<div style="`+styleTrusted+`">`+string(html)+"</div>") || strings.Contains(msg.HTML, "<p style=\""+stylePara+"\"><p>") {
		t.Errorf("trusted html:\n%s", msg.HTML)
	}
	if !strings.Contains(msg.Text, "bold & link (https://panel.example.com/subscription)") {
		t.Errorf("trusted html text:\n%s", msg.Text)
	}
	msg, _ = r.RenderCustom(TplNotify, customBrand, Custom{Body: "Note: {{content}}"}, Data{"ContentHTML": html, "Content": "plain"})
	if !strings.Contains(msg.HTML, "Note: "+string(html)) || !strings.Contains(msg.Text, "Note: plain") {
		t.Errorf("inline trusted html:\n%s\n%s", msg.HTML, msg.Text)
	}
	// A plain string ContentHTML is not trusted.
	msg, _ = r.RenderCustom(TplNotify, customBrand, c, Data{"Subject": "S", "ContentHTML": "<b>x</b>"})
	if strings.Contains(msg.HTML, "<b>x</b>") {
		t.Errorf("untrusted ContentHTML inserted:\n%s", msg.HTML)
	}
}

func TestCustomSubject(t *testing.T) {
	r := customRenderer(t)
	msg, _ := r.RenderCustom(TplVerifyCode, customBrand, Custom{Subject: "  {{siteName}}:\t{{code}}  {{nope}} ", Body: "{{code}}"}, Data{"Code": "1 2\n3"})
	if msg.Subject != "Example Panel: 1 2 3 {{nope}}" {
		t.Errorf("subject = %q", msg.Subject)
	}
	msg, _ = r.RenderCustom(TplVerifyCode, brandIn("en"), Custom{Subject: " \n ", Body: "{{code}}"}, Data{"Code": "1"})
	if msg.Subject != "Example Panel verification code" || !strings.Contains(msg.HTML, "<title>Example Panel verification code</title>") {
		t.Errorf("empty subject must fall back to the built-in one: %q", msg.Subject)
	}
	if _, err := r.RenderCustom("nope", customBrand, Custom{}, nil); err == nil {
		t.Fatal("unknown template must fail")
	}
}

func TestCustomPlainText(t *testing.T) {
	r := customRenderer(t)
	body := strings.Join([]string{
		"## Heading **bold**",
		"Line one\nline two  \nthree \\*not emphasis\\* &amp; &#169;",
		"3. third\n4. fourth\n   1. nested",
		"- [x] done\n- [ ] todo",
		"<table>\n  <tr><td>Plan</td><td>{{code}}</td></tr>\n</table>\n<style>p{color:red}</style>",
		"<p>a<br>b <a href=\"{{siteUrl}}/x\">here</a> <a href=\"{{siteUrl}}\">{{siteUrl}}</a> <img src=\"x.png\" alt=\"pic\"></p>",
		"[{{siteUrl}}]({{siteUrl}}) <https://example.com/a> [](https://example.com/empty)",
		"***",
		"    indented code",
	}, "\n\n")
	msg, err := r.RenderCustom(TplVerifyCode, brandIn("en"), Custom{Subject: "s", Body: body}, Data{"Code": "42"})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"Heading bold",
		"Line one\nline two\nthree *not emphasis* & ©",
		"3. third\n4. fourth\n   1. nested",
		"- [x] done\n- [ ] todo",
		"Plan 42",
		"a\nb here (https://panel.example.com/x) https://panel.example.com pic",
		"https://panel.example.com https://example.com/a https://example.com/empty",
		"---",
		"indented code",
	}, "\n\n")
	if got := strings.TrimSuffix(strings.TrimPrefix(msg.Text, "Example Panel\n\n"), "\n\n--\nThis email was sent automatically. Please do not reply.\nhttps://panel.example.com\n"); got != want {
		t.Errorf("text =\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(msg.HTML, `<h2 style="`+styleH2+`">Heading <strong>bold</strong></h2>`) ||
		!strings.Contains(msg.HTML, `<ol start="3" style="`+styleList+`">`) || !strings.Contains(msg.HTML, `<hr style="`+styleRule+`">`) ||
		!strings.Contains(msg.HTML, "Line one<br>\nline two<br>\nthree *not emphasis*") {
		t.Errorf("html:\n%s", msg.HTML)
	}
}

// Overrides apply to their locale only; built-in rendering is unchanged byte for byte otherwise.
func TestRendererOverrides(t *testing.T) {
	plain := customRenderer(t)
	r := customRenderer(t)
	r.SetOverrides(map[string]map[string]Custom{
		TplVerifyCode: {"zh-CN": {Subject: "自定义 {{code}}", Body: "自定义正文 **{{code}}**"}, "fr": {Subject: "x", Body: "x"}},
		"nope":        {"en": {Subject: "x", Body: "x"}},
	})
	if _, ok := r.Override(TplVerifyCode, "fr"); ok {
		t.Fatal("unknown locales are ignored")
	}
	if _, ok := r.Override("nope", "en"); ok {
		t.Fatal("unknown templates are ignored")
	}
	data := Data{"Code": "777111"}
	msg, err := r.Render(TplVerifyCode, brandIn("zh-TW"), data) // normalized to zh-CN
	if err != nil {
		t.Fatal(err)
	}
	if msg.Subject != "自定义 777111" || !strings.Contains(msg.HTML, "自定义正文 <strong>777111</strong>") ||
		!strings.Contains(msg.Text, "自定义正文 777111") || msg.Template != TplVerifyCode {
		t.Fatalf("override not used: %+v", msg)
	}
	for _, c := range []struct {
		name   string
		locale string
		data   Data
	}{{TplVerifyCode, "en", data}, {TplNotify, "zh-CN", Data{"Subject": "S", "Content": "C"}}} {
		got, _ := r.Render(c.name, brandIn(c.locale), c.data)
		want, _ := plain.Render(c.name, brandIn(c.locale), c.data)
		if got != want {
			t.Errorf("%s/%s changed:\n%s\nwant\n%s", c.name, c.locale, got.HTML, want.HTML)
		}
	}
	r.SetOverrides(nil)
	got, _ := r.Render(TplVerifyCode, brandIn("zh-CN"), data)
	want, _ := plain.Render(TplVerifyCode, brandIn("zh-CN"), data)
	if got != want {
		t.Fatal("SetOverrides(nil) must restore the built-in template")
	}
}

func TestQueueReloadTemplates(t *testing.T) {
	d, st := setup(t)
	if err := d.Create(&model.MailTemplate{Name: TplVerifyCode, Locale: "zh-CN", Subject: "库里的模板 {{code}}", Body: "DB {{code}}"}).Error; err != nil {
		t.Fatal(err)
	}
	q, err := NewQueue(d, st, logx.Discard(), QueueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := q.Render("a@example.com", TplVerifyCode, Data{"Code": "135790"})
	if err != nil || msg.Subject != "库里的模板 135790" || !strings.Contains(msg.Text, "DB 135790") || msg.To != "a@example.com" {
		t.Fatalf("loaded at NewQueue: %+v %v", msg, err)
	}
	if err := d.Model(&model.MailTemplate{}).Where("name = ?", TplVerifyCode).Update("subject", "改过 {{code}}").Error; err != nil {
		t.Fatal(err)
	}
	if msg, _ = q.Render("a@example.com", TplVerifyCode, Data{"Code": "1"}); msg.Subject != "库里的模板 1" {
		t.Fatal("rows are read only on reload")
	}
	if err := q.ReloadTemplates(context.Background()); err != nil {
		t.Fatal(err)
	}
	if msg, _ = q.Render("a@example.com", TplVerifyCode, Data{"Code": "1"}); msg.Subject != "改过 1" {
		t.Fatalf("after reload: %q", msg.Subject)
	}
	// The draft renderer ignores the stored override and uses the requested locale.
	if msg, _ = q.RenderCustom("b@example.com", TplVerifyCode, "en", Custom{Subject: "Draft {{code}}", Body: "{{code}}"}, Data{"Code": "9"}); msg.Subject != "Draft 9" || msg.To != "b@example.com" || !strings.Contains(msg.HTML, `<html lang="en">`) {
		t.Fatalf("RenderCustom: %+v", msg)
	}
	if err := d.Where("1 = 1").Delete(&model.MailTemplate{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := q.ReloadTemplates(context.Background()); err != nil {
		t.Fatal(err)
	}
	if msg, _ = q.Render("a@example.com", TplVerifyCode, Data{"Code": "1"}); msg.Subject != "【GG Bond】邮箱验证码" {
		t.Fatalf("built-in after delete: %q", msg.Subject)
	}

	// A database without the table (or any read error) does not stop the queue: the built-ins are used.
	raw, err := db.Open(config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(t.TempDir(), "raw.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(raw) })
	q2, err := NewQueue(raw, st, logx.Discard(), QueueOptions{})
	if err != nil {
		t.Fatalf("NewQueue must not fail on a load error: %v", err)
	}
	if err := q2.ReloadTemplates(context.Background()); err == nil {
		t.Fatal("ReloadTemplates must report the missing table")
	}
	if msg, err = q2.Render("a@example.com", TplVerifyCode, Data{"Code": "1"}); err != nil || msg.Subject != "【GG Bond】邮箱验证码" {
		t.Fatalf("built-in after a load error: %+v %v", msg, err)
	}
	q3, err := NewQueue(nil, st, logx.Discard(), QueueOptions{})
	if err != nil || q3.ReloadTemplates(context.Background()) != nil {
		t.Fatalf("no database: %v", err)
	}
}

// Concurrent renders while the overrides change (run with -race).
func TestCustomConcurrentRender(t *testing.T) {
	r := customRenderer(t)
	rich := Custom{Subject: "{{code}}", Body: "# T\n\n| a | b |\n| - | - |\n| {{code}} | x |\n\n- [x] {{code}}\n\n```\n{{code}}\n```\n\n" +
		"<div>{{code}}</div>\n\nhttps://example.com/{{code}} ~~old~~ [go]({{url}})\n\n[go]({{url}})"}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 50 {
				if i == 0 && j%5 == 0 {
					c := defaultCustom(TplVerifyCode, "zh-CN")
					if j%10 == 0 {
						c = rich
					}
					r.SetOverrides(map[string]map[string]Custom{TplVerifyCode: {"zh-CN": c}})
				}
				msg, err := r.Render(TplVerifyCode, customBrand, Data{"Code": "555000"})
				if err != nil || !strings.Contains(msg.HTML, "555000") {
					t.Errorf("render: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
