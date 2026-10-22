package user

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
)

// registerRuleProfile mounts GET/PUT /user/rule-profile, GET /user/rulesets/catalog, GET /user/subscription/preview.
// Owner: BE-D subscription.
func registerRuleProfile(g *gin.RouterGroup, a *app.App) {
	g.GET("/rule-profile", getRuleProfile(a))
	g.PUT("/rule-profile", putRuleProfile(a))
	g.GET("/rulesets/catalog", func(c *gin.Context) { httpx.OK(c, rulesets.PublicView(rulesets.Current(a.Settings), a.Settings)) })
	g.GET("/subscription/preview", previewSubscription(a))
}

// ruleProfileView is the RuleProfile (stored, or the default template when none) plus whether it is stored,
// whether users may edit it and the effective profile subscriptions use (the forced default template when
// customization is disabled).
type ruleProfileView struct {
	subscription.RuleProfile
	Customized bool                     `json:"customized"`
	Editable   bool                     `json:"editable"`
	Effective  subscription.RuleProfile `json:"effective"`
}

func profileView(c *gin.Context, a *app.App, cat *subscription.Catalog) (ruleProfileView, error) {
	u := auth.CurrentUser(c)
	def := rulesets.DefaultsFrom(a.Settings)
	stored, err := engine.StoredProfile(c.Request.Context(), a, cat, u.ID)
	if err != nil {
		return ruleProfileView{}, err
	}
	v := ruleProfileView{Editable: a.Settings.Bool("allow_user_rule_profile"), Customized: stored != nil}
	if stored != nil {
		v.RuleProfile = *stored
	} else {
		v.RuleProfile = rulesets.DefaultProfile(cat, def)
	}
	v.Effective, err = engine.EffectiveProfile(c.Request.Context(), a, cat, u, rulesets.Overrides{})
	return v, err
}

func getRuleProfile(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		v, err := profileView(c, a, rulesets.Current(a.Settings))
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, v)
	}
}

func putRuleProfile(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.Settings.Bool("allow_user_rule_profile") {
			httpx.Fail(c, httpx.Forbidden("rule profile customization is disabled"))
			return
		}
		raw, err := c.GetRawData()
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			httpx.Fail(c, httpx.BadRequest("request body is required"))
			return
		}
		cat := rulesets.Current(a.Settings)
		p, err := rulesets.DecodeProfile(cat, raw, rulesets.DefaultsFrom(a.Settings))
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := rulesets.ValidateProfile(cat, p); err != nil {
			httpx.Fail(c, err)
			return
		}
		data, err := json.Marshal(p)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		u := auth.CurrentUser(c)
		if err := engine.SaveProfile(c.Request.Context(), a, u.ID, data); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("rule profile updated", "user_id", u.ID, "template", p.Template)
		v, err := profileView(c, a, cat)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, v)
	}
}

// previewSubscription renders the subscription of the current user as text: ?client=<format or client name>
// (default clash-meta) plus every query parameter of GET /api/v1/client/subscribe. Base64 bodies (v2ray,
// shadowrocket, ssr) are shown decoded.
func previewSubscription(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := c.Request.URL.Query()
		client := strings.TrimSpace(q.Get("client"))
		if client == "" {
			client = subscription.FormatClashMeta
		}
		format := subscription.DetectFormat(client, "")
		if _, ok := subscription.GetFormat(format); !ok {
			httpx.Fail(c, httpx.FieldError("client", "unknown client"))
			return
		}
		u := auth.CurrentUser(c)
		res, err := engine.Generate(c.Request.Context(), a, u, engine.Request{
			Format: format, UserAgent: c.GetHeader("User-Agent"), Query: q, Host: hostOnly(c.Request.Host),
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		body := res.Body
		switch res.Format {
		case subscription.FormatV2ray, subscription.FormatShadowrocket, subscription.FormatSSR:
			if dec, err := base64.StdEncoding.DecodeString(string(body)); err == nil {
				body = dec // show the links, not the transport encoding
			}
		}
		c.Header("Cache-Control", "no-store")
		c.Header("X-Subscription-Format", res.Format)
		c.Data(http.StatusOK, subscription.ContentTypePlain, body)
	}
}

func hostOnly(h string) string {
	if i := strings.LastIndexByte(h, ':'); i >= 0 && !strings.HasSuffix(h, "]") {
		h = h[:i]
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}
