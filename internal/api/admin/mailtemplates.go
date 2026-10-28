package admin

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
)

// registerMailTemplates mounts /admin/mail-templates* (admin only): per-locale replacements of the built-in
// e-mails (docs/API.md "Mail templates").
// Owner: mail templates (BE). Implement in this file; do not add routes elsewhere.
func registerMailTemplates(g *gin.RouterGroup, a *app.App) {
	t := g.Group("/mail-templates", auth.RequireAdmin(a))
	t.GET("", mailTemplateList(a))
	t.GET("/:name", mailTemplateShow(a))
	t.PUT("/:name", mailTemplateSave(a))
	t.DELETE("/:name", mailTemplateReset(a))
	t.POST("/:name/preview", mailTemplatePreview(a))
	t.POST("/:name/test", mailTemplateTest(a))
}

// mailTemplateLocaleState is one locale of a list row.
type mailTemplateLocaleState struct {
	Custom    bool       `json:"custom"`
	UpdatedAt *time.Time `json:"updated_at"` // null while the built-in template is in use
}

// mailTemplateItem is a GET /admin/mail-templates row.
type mailTemplateItem struct {
	Name      string                             `json:"name"`
	Variables []mail.Variable                    `json:"variables"`
	Locales   map[string]mailTemplateLocaleState `json:"locales"`
}

// mailTemplateDraft is a subject and Markdown body (the built-in default of a MailTemplate).
type mailTemplateDraft struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// mailTemplateView is the MailTemplate object of docs/API.md.
type mailTemplateView struct {
	Name      string            `json:"name"`
	Locale    string            `json:"locale"`
	Custom    bool              `json:"custom"`
	Subject   string            `json:"subject"`
	Body      string            `json:"body"`
	UpdatedAt *time.Time        `json:"updated_at"`
	Default   mailTemplateDraft `json:"default"`
}

// mailTemplateInput is the body of PUT, preview and test (test adds email).
type mailTemplateInput struct {
	Locale  string `json:"locale"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Email   string `json:"email"`
}

// mailTemplateName returns the :name parameter, or a 404 for anything but a built-in template.
func mailTemplateName(c *gin.Context) (string, error) {
	name := c.Param("name")
	if mail.Variables(name) == nil {
		return "", httpx.NotFound("mail template not found")
	}
	return name, nil
}

// mailTemplateLocaleError is the field message for an invalid locale ("" when valid).
func mailTemplateLocaleError(locale string) string {
	switch {
	case locale == "":
		return "is required"
	case !mail.IsTemplateLocale(locale):
		return "must be zh-CN or en"
	}
	return ""
}

// mailTemplateQueryLocale validates the locale query parameter of GET and DELETE.
func mailTemplateQueryLocale(c *gin.Context) (string, error) {
	locale := c.Query("locale")
	if msg := mailTemplateLocaleError(locale); msg != "" {
		return "", httpx.FieldError("locale", msg)
	}
	return locale, nil
}

// mailTemplateViewOf builds the MailTemplate of name/locale from its row (nil = built-in).
func mailTemplateViewOf(name, locale string, row *model.MailTemplate) mailTemplateView {
	subject, body := mail.DefaultCustom(name, locale)
	v := mailTemplateView{Name: name, Locale: locale, Subject: subject, Body: body,
		Default: mailTemplateDraft{Subject: subject, Body: body}}
	if row != nil {
		at := row.UpdatedAt
		v.Custom, v.Subject, v.Body, v.UpdatedAt = true, row.Subject, row.Body, &at
	}
	return v
}

// mailTemplateFind loads the row of name/locale (nil when the built-in template is in use).
func mailTemplateFind(db *gorm.DB, name, locale string) (*model.MailTemplate, error) {
	var row model.MailTemplate
	err := db.Where("name = ? AND locale = ?", name, locale).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// mailTemplateList: GET /admin/mail-templates → array of every built-in template in order with its variables and
// which locales are customised.
func mailTemplateList(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var rows []model.MailTemplate
		if err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			return tx.Select("name", "locale", "updated_at").Find(&rows).Error
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		saved := make(map[string]time.Time, len(rows))
		for _, r := range rows {
			saved[r.Name+"/"+r.Locale] = r.UpdatedAt
		}
		out := make([]mailTemplateItem, 0, len(mail.TemplateNames))
		for _, name := range mail.TemplateNames {
			item := mailTemplateItem{Name: name, Variables: mail.Variables(name),
				Locales: make(map[string]mailTemplateLocaleState, len(mail.TemplateLocales))}
			for _, l := range mail.TemplateLocales {
				st := mailTemplateLocaleState{}
				if at, ok := saved[name+"/"+l]; ok {
					st.Custom, st.UpdatedAt = true, &at
				}
				item.Locales[l] = st
			}
			out = append(out, item)
		}
		httpx.OK(c, out)
	}
}

// mailTemplateShow: GET /admin/mail-templates/:name?locale= → MailTemplate.
func mailTemplateShow(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		name, err := mailTemplateName(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		locale, err := mailTemplateQueryLocale(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var row *model.MailTemplate
		err = withLiveAdmin(c, a, func(tx *gorm.DB) error {
			var loadErr error
			row, loadErr = mailTemplateFind(tx, name, locale)
			return loadErr
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, mailTemplateViewOf(name, locale, row))
	}
}

// mailTemplateBind reads the JSON body and checks it strictly (PUT, test) or leniently (preview: only the
// locale and the size limits). It returns the input and the placeholder findings.
func mailTemplateBind(c *gin.Context, strict, withEmail bool) (mailTemplateInput, mail.Validation, error) {
	var in mailTemplateInput
	if err := httpx.Bind(c, &in); err != nil {
		return in, mail.Validation{}, err
	}
	e := httpx.BadRequest("validation failed")
	if msg := mailTemplateLocaleError(in.Locale); msg != "" {
		e = e.WithField("locale", msg)
	}
	if withEmail {
		addr := struct {
			Email string `json:"email" binding:"required,email,max=191"`
		}{in.Email}
		if err := httpx.Validate(&addr); err != nil {
			var he *httpx.Error
			if !errors.As(err, &he) {
				return in, mail.Validation{}, err
			}
			for _, msg := range he.Errors["email"] {
				e = e.WithField("email", msg)
			}
		}
	}
	v := mail.Validate(c.Param("name"), in.Subject, in.Body)
	fields := v.Limit
	if strict {
		fields = v.Errors()
	}
	for _, f := range []string{"subject", "body"} {
		for _, msg := range fields[f] {
			e = e.WithField(f, msg)
		}
	}
	if len(e.Errors) > 0 {
		return in, v, e
	}
	return in, v, nil
}

// mailTemplateSave: PUT /admin/mail-templates/:name {locale*, subject*, body*} → MailTemplate (creates or
// replaces the override; the mail queue reloads it at once).
func mailTemplateSave(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		name, err := mailTemplateName(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		in, _, err := mailTemplateBind(c, true, false)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		row := model.MailTemplate{Name: name, Locale: in.Locale, Subject: strings.TrimSpace(in.Subject), Body: in.Body}
		var saved *model.MailTemplate
		if err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "name"}, {Name: "locale"}},
				DoUpdates: clause.AssignmentColumns([]string{"subject", "body", "updated_at"}),
			}).Create(&row).Error; err != nil {
				return err
			}
			var err error
			saved, err = mailTemplateFind(tx, name, in.Locale)
			return err
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		if saved == nil {
			err = errors.New("admin: saved mail template not found")
		}
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := a.Mailer.ReloadTemplates(ctx); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("admin saved mail template", "admin_id", auth.CurrentUser(c).ID, "name", name, "locale", in.Locale)
		httpx.OK(c, mailTemplateViewOf(name, in.Locale, saved))
	}
}

// mailTemplateReset: DELETE /admin/mail-templates/:name?locale= → true (back to the built-in template;
// idempotent).
func mailTemplateReset(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		name, err := mailTemplateName(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		locale, err := mailTemplateQueryLocale(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		var deleted int64
		err = withLiveAdmin(c, a, func(tx *gorm.DB) error {
			res := tx.Where("name = ? AND locale = ?", name, locale).Delete(&model.MailTemplate{})
			deleted = res.RowsAffected
			return res.Error
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := a.Mailer.ReloadTemplates(ctx); err != nil {
			httpx.Fail(c, err)
			return
		}
		if deleted > 0 {
			httpx.Logger(c).Info("admin reset mail template", "admin_id", auth.CurrentUser(c).ID, "name", name, "locale", locale)
		}
		httpx.OK(c, true)
	}
}

// mailTemplatePreviewResult is the response of POST /admin/mail-templates/:name/preview.
type mailTemplatePreviewResult struct {
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	Text    string   `json:"text"`
	Unknown []string `json:"unknown"`
	Missing []string `json:"missing"`
}

// mailTemplatePreview: POST /admin/mail-templates/:name/preview {locale*, subject*, body*} → the draft rendered
// with the sample data. Lenient: unknown placeholders stay literal and are listed, required ones absent from the
// body are listed; only the locale and the size limits fail.
func mailTemplatePreview(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		name, err := mailTemplateName(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		in, v, err := mailTemplateBind(c, false, false)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		msg, err := a.Mailer.RenderCustom("", name, in.Locale, mail.Custom{Subject: in.Subject, Body: in.Body},
			mail.SampleData(name, in.Locale))
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if !requireLiveAdmin(c, a) {
			return
		}
		httpx.OK(c, mailTemplatePreviewResult{Subject: msg.Subject, HTML: msg.HTML, Text: msg.Text,
			Unknown: v.Unknown, Missing: v.Missing})
	}
}

// mailTemplateTest: POST /admin/mail-templates/:name/test {locale*, subject*, body*, email*} → true. Validated
// like PUT, rendered with the sample data and sent synchronously with the current mail settings; a delivery error
// is a 400 on email.
func mailTemplateTest(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		name, err := mailTemplateName(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		in, _, err := mailTemplateBind(c, true, true)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		msg, err := a.Mailer.RenderCustom(in.Email, name, in.Locale, mail.Custom{Subject: in.Subject, Body: in.Body},
			mail.SampleData(name, in.Locale))
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if !requireLiveAdmin(c, a) {
			return
		}
		if err := a.Mailer.SendNow(c.Request.Context(), msg); err != nil {
			httpx.Fail(c, httpx.BadRequest(err.Error()).WithField("email", "test mail failed").WithCause(err))
			return
		}
		httpx.Logger(c).Info("admin sent mail template test", "admin_id", auth.CurrentUser(c).ID, "name", name, "locale", in.Locale)
		httpx.OK(c, true)
	}
}
