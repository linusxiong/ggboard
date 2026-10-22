package admin

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
)

// registerRulesets mounts GET/PUT /admin/rulesets (admin only): the built-in catalog, the admin's overrides
// (setting rulesets_custom) and the merged result. Owner: BE-D subscription.
func registerRulesets(g *gin.RouterGroup, a *app.App) {
	rg := g.Group("/rulesets", auth.RequireAdmin(a))
	rg.GET("", getRulesets(a))
	rg.PUT("", putRulesets(a))
}

type rulesetsView struct {
	Builtin   *subscription.Catalog `json:"builtin"`
	Custom    any                   `json:"custom"`
	Effective *subscription.Catalog `json:"effective"`
}

func getRulesets(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		custom := a.Settings.Get(rulesets.SettingCustom)
		if custom == nil {
			custom = map[string]any{}
		}
		httpx.OK(c, rulesetsView{Builtin: rulesets.Builtin(), Custom: custom, Effective: rulesets.Current(a.Settings)})
	}
}

func putRulesets(a *app.App) gin.HandlerFunc {
	type request struct {
		Custom json.RawMessage `json:"custom"`
	}
	return func(c *gin.Context) {
		var in request
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		var value any // nil resets the setting
		if len(in.Custom) > 0 && string(in.Custom) != "null" {
			custom, err := rulesets.Parse(in.Custom)
			if err != nil {
				httpx.Fail(c, httpx.FieldError("custom", "invalid catalog: "+err.Error()))
				return
			}
			if err := rulesets.ValidateCatalog(rulesets.Merge(rulesets.Builtin(), custom)); err != nil {
				httpx.Fail(c, err)
				return
			}
			// Store the canonical form (unknown fields were rejected by Parse).
			b, err := json.Marshal(custom)
			if err != nil {
				httpx.Fail(c, err)
				return
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				httpx.Fail(c, err)
				return
			}
			value = m
		}
		if err := a.Settings.SetContext(c.Request.Context(), map[string]any{rulesets.SettingCustom: value}); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("rulesets custom catalog updated", "admin_id", auth.CurrentUser(c).ID, "reset", value == nil)
		httpx.OK(c, true)
	}
}
