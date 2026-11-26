package admin

import (
	"maps"
	"slices"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/settings"
)

// registerSettings mounts /admin/settings* (admin only).
// Owner: BE-E1 admin ops. Implement in this file; do not add routes elsewhere.
func registerSettings(g *gin.RouterGroup, a *app.App) {
	s := g.Group("/settings", auth.RequireAdmin(a), func(c *gin.Context) {
		if err := settingsLiveAdmin(c, a.DB.WithContext(c.Request.Context()), false); err != nil {
			httpx.Fail(c, err)
			return
		}
		c.Next()
	})
	s.GET("", settingsGetHandler(a))
	s.PUT("", settingsPutHandler(a))
	s.POST("/mail/test", settingsMailTestHandler(a))
	s.POST("/telegram/webhook", settingsTelegramWebhookHandler(a)) // admin/telegram.go (BE-T)
	s.POST("/telegram/test", settingsTelegramTestHandler(a))       // admin/telegram.go (BE-T)
}

// settingsGetHandler returns {"<group>": {"<key>": value}, ..., "groups": [settings.Group]} (secrets included).
func settingsGetHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		out := make(gin.H, 16)
		err := withLiveAdmin(c, a, func(_ *gorm.DB) error {
			for g, kv := range a.Settings.AllGrouped() {
				out[g] = kv
			}
			return nil
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out["groups"] = settings.Groups()
		httpx.OK(c, out)
	}
}

// settingsPutHandler saves a flat {key: value} map (null resets a key). Keys managed by dedicated endpoints
// (hidden keys such as rulesets_custom) are rejected.
func settingsPutHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in map[string]any
		if err := c.ShouldBindJSON(&in); err != nil {
			httpx.Fail(c, httpx.Validation(err))
			return
		}
		nodeVisible := false
		e := httpx.BadRequest("validation failed")
		for _, k := range slices.Sorted(maps.Keys(in)) {
			d, ok := settings.Lookup(k)
			if ok && d.Hidden {
				e = e.WithField(k, "is managed by a dedicated endpoint")
			}
			if ok && (d.Group == settings.GroupServer || d.Group == settings.GroupSubscribe) {
				nodeVisible = true
			}
		}
		if len(e.Errors) > 0 {
			httpx.Fail(c, e)
			return
		}
		if err := a.Settings.SetContextIf(c.Request.Context(), in, func(tx *gorm.DB) error {
			if err := settingsLiveAdmin(c, tx, true); err != nil {
				return err
			}
			return serverTokenNotNodeToken(tx, in["server_token"])
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		if nodeVisible {
			usersvc.New(a).InvalidateNodeUsers()
		}
		httpx.Logger(c).Info("admin saved settings", "admin_id", auth.CurrentUser(c).ID, "keys", slices.Sorted(maps.Keys(in)))
		httpx.OK(c, true)
	}
}

// serverTokenNotNodeToken refuses a new server_token that is some node's own token: every holder of the shared
// token would then pass that node's own-token check (docs/NODE_PROTOCOLS.md §1.1; nodesvc.CheckNode refuses the
// same pair from the node side).
func serverTokenNotNodeToken(tx *gorm.DB, v any) error {
	token, _ := v.(string)
	if token == "" {
		return nil
	}
	var n int64
	if err := tx.Model(&model.Node{}).Where("token = ?", token).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return httpx.FieldError("server_token", "must differ from every node's own token")
	}
	return nil
}

// settingsMailTestHandler sends the notify template synchronously with the current mail settings and returns
// the delivery error as a 400.
func settingsMailTestHandler(a *app.App) gin.HandlerFunc {
	type req struct {
		Email string `json:"email" binding:"required,email,max=191"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		data := mail.Data{
			"Subject": "Test email from " + a.Settings.String("app_name"),
			"Content": "Your mail settings work: this message was sent by the administrator's test.",
		}
		if mail.NormalizeLocale(a.Settings.String("default_locale")) == "zh-CN" {
			data = mail.Data{
				"Subject": "【" + a.Settings.String("app_name") + "】测试邮件",
				"Content": "邮件设置正常：这是一封由管理员发送的测试邮件。",
			}
		}
		msg, err := a.Mailer.Render(in.Email, mail.TplNotify, data)
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
		httpx.OK(c, true)
	}
}
