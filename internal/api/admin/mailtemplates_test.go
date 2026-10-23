package admin_test

import (
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	accountsvc "github.com/linusxiong/ggboard/internal/service/account"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const mailTplBase = "/api/v1/admin/mail-templates"

type mailTplLocale struct {
	Custom    bool       `json:"custom"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type mailTplItem struct {
	Name      string                   `json:"name"`
	Variables []mail.Variable          `json:"variables"`
	Locales   map[string]mailTplLocale `json:"locales"`
}

type mailTplView struct {
	Name      string     `json:"name"`
	Locale    string     `json:"locale"`
	Custom    bool       `json:"custom"`
	Subject   string     `json:"subject"`
	Body      string     `json:"body"`
	UpdatedAt *time.Time `json:"updated_at"`
	Default   struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	} `json:"default"`
}

type mailTplPreview struct {
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	Text    string   `json:"text"`
	Unknown []string `json:"unknown"`
	Missing []string `json:"missing"`
}

func mailTplCount(t *testing.T, e *opsEnv) int64 {
	t.Helper()
	var n int64
	if err := e.a.DB.Model(&model.MailTemplate{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func mailTplFieldErrors(t *testing.T, e *opsEnv, method, path string, body any, status int) map[string][]string {
	t.Helper()
	rec := testutil.Do(t, e.r, method, path, body, e.adminTok)
	testutil.ExpectStatus(t, rec, status)
	return testutil.DecodeError(t, rec).Errors
}

func TestMailTemplatesListAndDefaults(t *testing.T) {
	e := newOpsEnv(t)
	rec := testutil.Do(t, e.r, http.MethodGet, mailTplBase, nil, e.adminTok)
	items := testutil.Decode[[]mailTplItem](t, rec)
	if !strings.HasPrefix(rec.Body.String(), `{"data":[`) || strings.Contains(rec.Body.String(), `"total":`) {
		t.Fatal("the list is a bare array")
	}
	var names []string
	for _, it := range items {
		names = append(names, it.Name)
		if !slices.Equal(it.Variables, mail.Variables(it.Name)) || len(it.Locales) != 2 {
			t.Errorf("%s: %+v", it.Name, it)
		}
		for l, st := range it.Locales {
			if !mail.IsTemplateLocale(l) || st.Custom || st.UpdatedAt != nil {
				t.Errorf("%s/%s: %+v", it.Name, l, st)
			}
		}
	}
	if !slices.Equal(names, mail.TemplateNames) {
		t.Fatalf("names = %v", names)
	}
	if !strings.Contains(rec.Body.String(), `{"name":"verify_code","variables":[{"name":"code","required":true},{"name":"minutes","required":false},{"name":"siteName","required":false},{"name":"siteUrl","required":false},{"name":"year","required":false}],"locales":{"en":{"custom":false,"updated_at":null},"zh-CN":{"custom":false,"updated_at":null}}}`) {
		t.Fatalf("list shape: %s", rec.Body.String())
	}
	for _, name := range mail.TemplateNames {
		for _, l := range mail.TemplateLocales {
			rec := testutil.Do(t, e.r, http.MethodGet, mailTplBase+"/"+name+"?locale="+l, nil, e.adminTok)
			v := testutil.Decode[mailTplView](t, rec)
			s, b := mail.DefaultCustom(name, l)
			if v.Name != name || v.Locale != l || v.Custom || v.UpdatedAt != nil || v.Subject != s || v.Body != b ||
				v.Default.Subject != s || v.Default.Body != b || !strings.Contains(rec.Body.String(), `"updated_at":null`) {
				t.Errorf("%s/%s: %+v", name, l, v)
			}
		}
	}
}

func TestMailTemplatesSaveAndReset(t *testing.T) {
	e := newOpsEnv(t)
	put := map[string]any{"locale": "zh-CN", "subject": "  【{{siteName}}】您的验证码 {{code}}  ", "body": "# 验证\n\n验证码：**{{code}}**"}
	rec := testutil.Do(t, e.r, http.MethodPut, mailTplBase+"/verify_code", put, e.adminTok)
	v := testutil.Decode[mailTplView](t, rec)
	if !v.Custom || v.Subject != "【{{siteName}}】您的验证码 {{code}}" || v.Body != put["body"] || v.UpdatedAt == nil || v.Locale != "zh-CN" {
		t.Fatalf("PUT = %+v", v)
	}
	if s, _ := mail.DefaultCustom(mail.TplVerifyCode, "zh-CN"); v.Default.Subject != s {
		t.Fatalf("default = %+v", v.Default)
	}
	got := testutil.Decode[mailTplView](t, testutil.Do(t, e.r, http.MethodGet, mailTplBase+"/verify_code?locale=zh-CN", nil, e.adminTok))
	if !got.Custom || got.Subject != v.Subject || got.Body != v.Body || got.UpdatedAt == nil {
		t.Fatalf("GET = %+v", got)
	}
	if other := testutil.Decode[mailTplView](t, testutil.Do(t, e.r, http.MethodGet, mailTplBase+"/verify_code?locale=en", nil, e.adminTok)); other.Custom {
		t.Fatal("the other locale is not customised")
	}
	items := testutil.Decode[[]mailTplItem](t, testutil.Do(t, e.r, http.MethodGet, mailTplBase, nil, e.adminTok))
	if st := items[0].Locales["zh-CN"]; !st.Custom || st.UpdatedAt == nil || items[0].Locales["en"].Custom || items[1].Locales["zh-CN"].Custom {
		t.Fatalf("list after PUT = %+v", items)
	}

	// Replacing keeps one row; the queue renders the new version at once.
	put["body"] = "新版 {{code}}"
	if v = testutil.Decode[mailTplView](t, testutil.Do(t, e.r, http.MethodPut, mailTplBase+"/verify_code", put, e.adminTok)); v.Body != "新版 {{code}}" {
		t.Fatalf("replace = %+v", v)
	}
	if n := mailTplCount(t, e); n != 1 {
		t.Fatalf("rows = %d", n)
	}
	msg, err := e.a.Mailer.Render("a@example.com", mail.TplVerifyCode, mail.Data{"Code": "112233"})
	if err != nil || msg.Subject != "【GG Bond】您的验证码 112233" || !strings.Contains(msg.Text, "新版 112233") {
		t.Fatalf("render after PUT = %+v %v", msg, err)
	}
	var audited bool
	for _, en := range e.a.Logs.Entries(slog.LevelInfo, 0) {
		if en.Msg == "admin saved mail template" && en.Attrs["name"] == "verify_code" && en.Attrs["locale"] == "zh-CN" {
			audited = true
		}
		if strings.Contains(en.Msg+testutil.MustJSON(t, en.Attrs), "新版") {
			t.Fatalf("a template body was logged: %+v", en)
		}
	}
	if !audited {
		t.Fatal("no audit log line")
	}

	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodDelete, mailTplBase+"/verify_code?locale=zh-CN", nil, e.adminTok), http.StatusOK)
	if got = testutil.Decode[mailTplView](t, testutil.Do(t, e.r, http.MethodGet, mailTplBase+"/verify_code?locale=zh-CN", nil, e.adminTok)); got.Custom || got.Subject != got.Default.Subject {
		t.Fatalf("after DELETE = %+v", got)
	}
	if n := mailTplCount(t, e); n != 0 {
		t.Fatalf("rows = %d", n)
	}
	if msg, _ = e.a.Mailer.Render("a@example.com", mail.TplVerifyCode, mail.Data{"Code": "1"}); msg.Subject != "【GG Bond】邮箱验证码" {
		t.Fatalf("built-in after DELETE: %q", msg.Subject)
	}
	// Idempotent.
	if ok := testutil.Decode[bool](t, testutil.Do(t, e.r, http.MethodDelete, mailTplBase+"/verify_code?locale=zh-CN", nil, e.adminTok)); !ok {
		t.Fatal("second DELETE")
	}
}

func TestMailTemplatesValidation(t *testing.T) {
	e := newOpsEnv(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/nope?locale=en"}, {http.MethodPut, "/nope"}, {http.MethodDelete, "/nope?locale=en"},
		{http.MethodPost, "/nope/preview"}, {http.MethodPost, "/nope/test"}, {http.MethodGet, "/layout?locale=en"},
	} {
		rec := testutil.Do(t, e.r, c.method, mailTplBase+c.path, map[string]any{"locale": "en", "subject": "s", "body": "b"}, e.adminTok)
		testutil.ExpectStatus(t, rec, http.StatusNotFound)
	}
	for _, q := range []string{"", "?locale=zh", "?locale=EN", "?locale=fr"} {
		for _, m := range []string{http.MethodGet, http.MethodDelete} {
			if errs := mailTplFieldErrors(t, e, m, mailTplBase+"/verify_code"+q, nil, http.StatusBadRequest); len(errs["locale"]) == 0 {
				t.Errorf("%s %q: %v", m, q, errs)
			}
		}
	}

	ok := map[string]any{"locale": "en", "subject": "Code {{code}}", "body": "Your code: {{code}}"}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range ok {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	for _, c := range []struct {
		body  map[string]any
		field string
		msg   string
	}{
		{with("locale", ""), "locale", "is required"},
		{with("locale", "zh"), "locale", "must be zh-CN or en"},
		{with("subject", "Hi {{foo}} {{ code }}"), "subject", "unknown placeholders: {{foo}}, {{ code }}"},
		{with("body", "{{code}} {{tradeNo}}"), "body", "unknown placeholders: {{tradeNo}}"},
		{with("body", "no code here"), "body", "missing required placeholders: {{code}}"},
		{with("subject", "   "), "subject", "is required"},
		{with("subject", "two\nlines"), "subject", "must be a single line"},
		{with("subject", strings.Repeat("a", 256)), "subject", "must be at most 255 characters"},
		{with("body", "{{code}}"+strings.Repeat("x", 65535)), "body", "must be at most 65535 bytes"},
		{with("body", ""), "body", "is required"},
	} {
		errs := mailTplFieldErrors(t, e, http.MethodPut, mailTplBase+"/verify_code", c.body, http.StatusBadRequest)
		if !slices.Contains(errs[c.field], c.msg) {
			t.Errorf("%v: errors = %v, want %s: %s", c.body, errs, c.field, c.msg)
		}
	}
	// Several fields at once.
	errs := mailTplFieldErrors(t, e, http.MethodPut, mailTplBase+"/verify_code", map[string]any{"locale": "x", "subject": "{{foo}}", "body": "{{bar}}"}, http.StatusBadRequest)
	if len(errs["locale"]) != 1 || len(errs["subject"]) != 1 || len(errs["body"]) != 2 {
		t.Errorf("all fields: %v", errs)
	}
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPut, mailTplBase+"/verify_code", `{"locale": `, e.adminTok), http.StatusBadRequest)
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPut, mailTplBase+"/verify_code", `{"locale": 1}`, e.adminTok), http.StatusBadRequest)
	if n := mailTplCount(t, e); n != 0 {
		t.Fatalf("an invalid request saved %d rows", n)
	}
	// 255 characters (not bytes) are accepted.
	testutil.ExpectStatus(t, testutil.Do(t, e.r, http.MethodPut, mailTplBase+"/verify_code", with("subject", strings.Repeat("验", 255)), e.adminTok), http.StatusOK)
}

func TestMailTemplatesAdminOnly(t *testing.T) {
	e := newOpsEnv(t)
	body := map[string]any{"locale": "en", "subject": "s", "body": "{{code}}", "email": "ops@example.com"}
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, ""}, {http.MethodGet, "/verify_code?locale=en"}, {http.MethodPut, "/verify_code"},
		{http.MethodDelete, "/verify_code?locale=en"}, {http.MethodPost, "/verify_code/preview"}, {http.MethodPost, "/verify_code/test"},
	} {
		testutil.ExpectStatus(t, testutil.Do(t, e.r, c.method, mailTplBase+c.path, body, e.staffTok), http.StatusForbidden)
		testutil.ExpectStatus(t, testutil.Do(t, e.r, c.method, mailTplBase+c.path, body, e.userTok), http.StatusForbidden)
		testutil.ExpectStatus(t, testutil.Do(t, e.r, c.method, mailTplBase+c.path, body, ""), http.StatusUnauthorized)
	}
	if n := mailTplCount(t, e); n != 0 || len(testutil.Mailbox(t, e.a).Messages()) != 0 {
		t.Fatal("a refused request had an effect")
	}
}

func TestMailTemplatesPreview(t *testing.T) {
	e := newOpsEnv(t, testutil.WithSettings(map[string]any{"app_name": "Preview Panel", "app_url": "https://panel.example.com"}))
	rec := testutil.Do(t, e.r, http.MethodPost, mailTplBase+"/order_paid/preview", map[string]any{
		"locale": "en", "subject": "Paid {{tradeNo}}\n{{foo}}", "body": "Order {{tradeNo}} {{foo}} {{ planName }}\n\n[View]({{url}})",
	}, e.adminTok)
	p := testutil.Decode[mailTplPreview](t, rec)
	if p.Subject != "Paid 2026102314302291834 {{foo}}" || !slices.Equal(p.Unknown, []string{"{{foo}}", "{{ planName }}"}) ||
		!slices.Equal(p.Missing, []string{"{{amount}}"}) {
		t.Fatalf("preview = %+v", p)
	}
	for _, want := range []string{"Order 2026102314302291834 {{foo}} {{ planName }}", `<html lang="en">`, "Preview Panel",
		`href="https://panel.example.com/orders/2026102314302291834"`, "This email was sent automatically."} {
		if !strings.Contains(p.HTML, want) {
			t.Errorf("preview html lacks %q:\n%s", want, p.HTML)
		}
	}
	if !strings.Contains(p.Text, "View (https://panel.example.com/orders/2026102314302291834)") || !strings.HasPrefix(p.Text, "Preview Panel\n\n") {
		t.Errorf("preview text:\n%s", p.Text)
	}
	if n := mailTplCount(t, e); n != 0 || len(testutil.Mailbox(t, e.a).Messages()) != 0 {
		t.Fatal("a preview must not save or send")
	}

	// Empty drafts are fine (built-in subject, every required placeholder missing); the JSON keeps [] not null.
	rec = testutil.Do(t, e.r, http.MethodPost, mailTplBase+"/verify_code/preview", map[string]any{"locale": "zh-CN", "subject": "", "body": ""}, e.adminTok)
	if p = testutil.Decode[mailTplPreview](t, rec); p.Subject != "【Preview Panel】邮箱验证码" || !slices.Equal(p.Missing, []string{"{{code}}"}) ||
		!strings.Contains(rec.Body.String(), `"unknown":[]`) {
		t.Fatalf("empty preview = %s", rec.Body.String())
	}
	// The default renders like the built-in mail.
	s, b := mail.DefaultCustom(mail.TplVerifyCode, "zh-CN")
	if p = testutil.Decode[mailTplPreview](t, testutil.Do(t, e.r, http.MethodPost, mailTplBase+"/verify_code/preview",
		map[string]any{"locale": "zh-CN", "subject": s, "body": b}, e.adminTok)); !strings.Contains(p.HTML, ">482913</div>") ||
		len(p.Unknown) != 0 || len(p.Missing) != 0 {
		t.Fatalf("default preview = %+v", p)
	}

	// Only the locale and the size limits fail.
	errs := mailTplFieldErrors(t, e, http.MethodPost, mailTplBase+"/verify_code/preview", map[string]any{"locale": "de", "subject": "", "body": ""}, http.StatusBadRequest)
	if len(errs["locale"]) != 1 || len(errs) != 1 {
		t.Errorf("preview locale: %v", errs)
	}
	errs = mailTplFieldErrors(t, e, http.MethodPost, mailTplBase+"/verify_code/preview",
		map[string]any{"locale": "en", "subject": strings.Repeat("s", 256), "body": strings.Repeat("b", 65536)}, http.StatusBadRequest)
	if len(errs["subject"]) != 1 || len(errs["body"]) != 1 {
		t.Errorf("preview limits: %v", errs)
	}
}

func TestMailTemplatesTestMail(t *testing.T) {
	e := newOpsEnv(t, testutil.WithSettings(map[string]any{"app_name": "Test Panel"}))
	body := map[string]any{"locale": "en", "subject": "{{siteName}}: {{ticketSubject}}", "body": "Re **{{ticketSubject}}**\n\n> {{message}}", "email": "ops@example.com"}
	e.expect(t, http.MethodPost, mailTplBase+"/ticket_reply/test", body, e.adminTok, http.StatusOK)
	msgs := testutil.Mailbox(t, e.a).Messages()
	if len(msgs) != 1 {
		t.Fatalf("mails = %+v", msgs)
	}
	m := msgs[0]
	if m.To != "ops@example.com" || m.Template != mail.TplTicketReply || m.Subject != "Test Panel: Cannot connect to the Hong Kong node" ||
		!strings.Contains(m.HTML, "Re <strong>Cannot connect to the Hong Kong node</strong>") ||
		!strings.Contains(m.HTML, "Hello, the node is back online.<br>Please update") ||
		!strings.Contains(m.Text, "Hello, the node is back online.\nPlease update") || !strings.Contains(m.HTML, `<html lang="en">`) {
		t.Fatalf("test mail = %+v", m)
	}
	var logged int64
	if err := e.a.DB.Model(&model.MailLog{}).Where("email = ? AND template = ? AND error IS NULL", "ops@example.com", mail.TplTicketReply).Count(&logged).Error; err != nil || logged != 1 {
		t.Fatalf("mail_logs = %d, %v", logged, err)
	}
	if n := mailTplCount(t, e); n != 0 {
		t.Fatal("a test mail must not save the draft")
	}

	// Strict like PUT: nothing is sent for an invalid draft or address.
	for _, c := range []struct {
		k     string
		v     any
		field string
	}{{"body", "no subject placeholder", "body"}, {"subject", "{{foo}}", "subject"}, {"email", "not-an-email", "email"}, {"email", "", "email"}, {"locale", "", "locale"}} {
		in := map[string]any{}
		for k, v := range body {
			in[k] = v
		}
		in[c.k] = c.v
		if errs := mailTplFieldErrors(t, e, http.MethodPost, mailTplBase+"/ticket_reply/test", in, http.StatusBadRequest); len(errs[c.field]) == 0 {
			t.Errorf("%s=%v: %v", c.k, c.v, errs)
		}
	}
	if n := len(testutil.Mailbox(t, e.a).Messages()); n != 1 {
		t.Fatalf("an invalid test request sent mail (%d)", n)
	}

	e.a.Mailer.SetMailer(opsFailingMailer{})
	rec := testutil.Do(t, e.r, http.MethodPost, mailTplBase+"/ticket_reply/test", body, e.adminTok)
	testutil.ExpectStatus(t, rec, http.StatusBadRequest)
	if he := testutil.DecodeError(t, rec); !strings.Contains(he.Message, "connection refused") || len(he.Errors["email"]) == 0 {
		t.Fatalf("delivery error = %+v", he)
	}
}

// End to end: a saved verify_code template is used by the real flow (account.SendCode → queue → mailbox) with the
// real code; after DELETE the built-in mail is back. The other locale keeps the built-in template.
func TestMailTemplatesVerifyCodeEndToEnd(t *testing.T) {
	e := newOpsEnv(t, testutil.WithSettings(map[string]any{"app_name": "E2E Panel"}))
	e.expect(t, http.MethodPut, mailTplBase+"/verify_code", map[string]any{
		"locale": "zh-CN", "subject": "【{{siteName}}】自定义验证码", "body": "# 自定义标题\n\n您的验证码是 **{{code}}**，{{minutes}} 分钟内有效。",
	}, e.adminTok, http.StatusOK)

	codeRe := regexp.MustCompile(`\d{6}`)
	send := func(email string) mail.Message {
		t.Helper()
		before := len(testutil.Mails(t, e.a))
		if err := accountsvc.New(e.a).SendCode(email); err != nil {
			t.Fatal(err)
		}
		msgs := testutil.Mails(t, e.a)
		if len(msgs) != before+1 || msgs[len(msgs)-1].To != email {
			t.Fatalf("mails = %+v", msgs)
		}
		return msgs[len(msgs)-1]
	}
	m := send(e.user.Email)
	code := codeRe.FindString(m.Text)
	if m.Subject != "【E2E Panel】自定义验证码" || !strings.Contains(m.HTML, ">自定义标题</h1>") ||
		!strings.Contains(m.HTML, "您的验证码是 <strong>"+code+"</strong>，10 分钟内有效。") ||
		!strings.Contains(m.Text, "您的验证码是 "+code+"，10 分钟内有效。") || strings.Contains(m.HTML, "您正在进行邮箱验证") {
		t.Fatalf("custom mail = %+v", m)
	}
	if err := accountsvc.New(e.a).ConsumeCode(accountsvc.PurposeForgot, e.user.Email, code); err != nil {
		t.Fatalf("the mailed code is not the real one: %v", err)
	}

	e.expect(t, http.MethodDelete, mailTplBase+"/verify_code?locale=zh-CN", nil, e.adminTok, http.StatusOK)
	m = send(e.staff.Email) // another address: the per-email cooldown
	code = codeRe.FindString(m.Text)
	if m.Subject != "【E2E Panel】邮箱验证码" || !strings.Contains(m.HTML, "您正在进行邮箱验证") || !strings.Contains(m.HTML, ">"+code+"</div>") ||
		strings.Contains(m.HTML, "自定义") {
		t.Fatalf("built-in mail after DELETE = %+v", m)
	}

	// A zh-CN override does not touch English mail.
	e.expect(t, http.MethodPut, mailTplBase+"/verify_code", map[string]any{"locale": "zh-CN", "subject": "x", "body": "x {{code}}"}, e.adminTok, http.StatusOK)
	if err := e.a.Settings.Set(map[string]any{"default_locale": "en"}); err != nil {
		t.Fatal(err)
	}
	if m = send(e.admin.Email); m.Subject != "E2E Panel verification code" || !strings.Contains(m.Text, "Use the code below") {
		t.Fatalf("en mail = %+v", m)
	}
}
