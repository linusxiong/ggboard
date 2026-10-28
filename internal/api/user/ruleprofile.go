package user

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
)

// registerRuleProfile mounts GET/PUT /user/rule-profile (alias of the default profile), the multi-profile
// routes /user/rule-profiles[/:slug[/default]], GET /user/rulesets/catalog and GET /user/subscription/preview.
// Owner: BE-D subscription (multi-profile routes: BE-S).
func registerRuleProfile(g *gin.RouterGroup, a *app.App) {
	// Saving and previewing render whole configs (rule-set plans, group resolution over every node): 60 per
	// minute per user, like the subscription endpoint's burst, keeps one account from monopolising the CPU.
	limit := httpx.Limit("rule_profile", httpx.NewLimiter(httpx.Per(60, time.Minute), 30), func(c *gin.Context) string {
		if u := auth.CurrentUser(c); u != nil {
			return "u" + strconv.FormatUint(u.ID, 10)
		}
		return httpx.KeyByIP(c)
	})
	g.GET("/rule-profile", getRuleProfile(a, ""))
	g.PUT("/rule-profile", limit, putRuleProfile(a, ""))
	g.GET("/rulesets/catalog", func(c *gin.Context) { httpx.OK(c, rulesets.PublicView(rulesets.Current(a.Settings), a.Settings)) })
	g.GET("/subscription/preview", limit, previewSubscription(a))
	registerRuleProfiles(g, a, limit) // user/ruleprofiles.go (BE-S)
}

// ruleProfileView is the RuleProfile (stored, or the default template when the profile follows the defaults)
// plus whether it is stored, whether users may edit it, the effective profile subscriptions use (the forced
// default template when customization is disabled) and which named profile it is.
type ruleProfileView struct {
	subscription.RuleProfile
	Customized bool                     `json:"customized"`
	Editable   bool                     `json:"editable"`
	Effective  subscription.RuleProfile `json:"effective"`
	ID         uint64                   `json:"id"`
	Slug       string                   `json:"slug"`
	Name       string                   `json:"name"`
	IsDefault  bool                     `json:"is_default"`
}

// profileView builds the view of the profile with slug ("" = the default profile; created when the user has
// none, so every view names a real profile).
func profileView(c *gin.Context, a *app.App, cat *subscription.Catalog, slug string) (ruleProfileView, error) {
	ctx := c.Request.Context()
	u := auth.CurrentUser(c)
	if err := engine.EnsureDefault(ctx, a, u.ID, u); err != nil {
		return ruleProfileView{}, profileErr(err)
	}
	row, stored, rows, err := engine.StoredProfileSlugBound(ctx, a, cat, u, slug)
	if err != nil {
		return ruleProfileView{}, profileErr(err)
	}
	if row == nil {
		return ruleProfileView{}, httpx.NotFound("rule profile not found")
	}
	def := engine.DefaultOf(rows)
	v := ruleProfileView{Editable: a.Settings.Bool("allow_user_rule_profile"), Customized: stored != nil,
		ID: row.ID, Slug: row.Slug, Name: row.Name, IsDefault: def != nil && def.ID == row.ID}
	if stored != nil {
		v.RuleProfile = *stored
	} else {
		v.RuleProfile = rulesets.DefaultProfile(cat, rulesets.DefaultsFrom(a.Settings))
	}
	if !v.Editable {
		v.Effective = rulesets.DefaultProfile(cat, rulesets.DefaultsFrom(a.Settings))
	} else {
		v.Effective, err = rulesets.ResolveProfile(cat, stored, rulesets.DefaultsFrom(a.Settings), rulesets.Overrides{})
	}
	return v, err
}

// profileErr maps profile store errors to API errors.
func profileErr(err error) error {
	switch {
	case errors.Is(err, engine.ErrProfileNotFound):
		return httpx.NotFound("rule profile not found")
	case errors.Is(err, engine.ErrProfileLimit):
		return httpx.Conflict(fmt.Sprintf("at most %d rule profiles", model.MaxRuleProfiles))
	case errors.Is(err, engine.ErrSlugTaken):
		e := httpx.Conflict("slug already used by another profile")
		e.Errors = map[string][]string{"slug": {"already used by another profile"}}
		return e
	case errors.Is(err, engine.ErrDefaultProfile):
		return httpx.Conflict("the default profile cannot be deleted")
	case errors.Is(err, engine.ErrProfileSessionExpired):
		return httpx.Unauthorized("session expired, please sign in again")
	}
	return err
}

func getRuleProfile(a *app.App, fixedSlug string) gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := fixedSlug
		if p := c.Param("slug"); p != "" {
			slug = p
		}
		v, err := profileView(c, a, rulesets.Current(a.Settings), slug)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, v)
	}
}

func putRuleProfile(a *app.App, fixedSlug string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.Settings.Bool("allow_user_rule_profile") {
			httpx.Fail(c, httpx.Forbidden("rule profile customization is disabled"))
			return
		}
		slug := fixedSlug
		if p := c.Param("slug"); p != "" {
			slug = p
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
		u := auth.CurrentUser(c)
		var present map[string]json.RawMessage
		if json.Unmarshal(raw, &present) == nil && present["nodes"] == nil {
			// Clients that do not know node operations (older SPA builds) must not wipe them.
			_, stored, _, err := engine.StoredProfileSlugBound(c.Request.Context(), a, cat, u, slug)
			if err != nil {
				httpx.Fail(c, profileErr(err))
				return
			}
			if stored != nil {
				p.Nodes = stored.Nodes
			}
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
		if err := engine.SaveProfileSlug(c.Request.Context(), a, u.ID, slug, data, u); err != nil {
			httpx.Fail(c, profileErr(err))
			return
		}
		httpx.Logger(c).Info("rule profile updated", "user_id", u.ID, "slug", slug, "template", p.Template)
		v, err := profileView(c, a, cat, slug)
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
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
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
