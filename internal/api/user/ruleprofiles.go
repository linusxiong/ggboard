package user

import (
	"errors"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
)

// registerRuleProfiles mounts the multiple rule profile routes (docs/API.md "Rule profile"). GET/PUT
// /user/rule-profile stay as the alias of the default profile. Owner: BE-S.
func registerRuleProfiles(g *gin.RouterGroup, a *app.App, limit gin.HandlerFunc) {
	g.GET("/rule-profiles", listRuleProfiles(a))
	g.POST("/rule-profiles", limit, createRuleProfile(a))
	g.GET("/rule-profiles/:slug", getRuleProfile(a, ""))
	g.PUT("/rule-profiles/:slug", limit, putRuleProfile(a, ""))
	g.PATCH("/rule-profiles/:slug", limit, patchRuleProfile(a))
	g.DELETE("/rule-profiles/:slug", limit, deleteRuleProfile(a))
	g.POST("/rule-profiles/:slug/default", limit, defaultRuleProfile(a))
}

// RuleProfileSummary is one entry of GET /user/rule-profiles.
type RuleProfileSummary struct {
	ID         uint64    `json:"id"`
	Slug       string    `json:"slug"`
	Name       string    `json:"name"`
	IsDefault  bool      `json:"is_default"`
	Customized bool      `json:"customized"` // false: follows the admin's default template
	UpdatedAt  time.Time `json:"updated_at"`
}

func ruleProfileSummary(r *model.RuleProfile, isDefault bool) RuleProfileSummary {
	return RuleProfileSummary{ID: r.ID, Slug: r.Slug, Name: r.Name, IsDefault: isDefault,
		Customized: len(r.Data) > 0 && string(r.Data) != "null", UpdatedAt: r.UpdatedAt.UTC()}
}

// ruleProfileSummaries lists the user's profiles (the default row is created for users without one).
func ruleProfileSummaries(c *gin.Context, a *app.App) ([]RuleProfileSummary, error) {
	u := auth.CurrentUser(c)
	if err := engine.EnsureDefault(c.Request.Context(), a, u.ID); err != nil {
		return nil, err
	}
	rows, err := engine.LoadProfiles(c.Request.Context(), a, u.ID)
	if err != nil {
		return nil, err
	}
	def := engine.DefaultOf(rows)
	out := make([]RuleProfileSummary, 0, len(rows))
	for i := range rows {
		out = append(out, ruleProfileSummary(&rows[i], def != nil && def.ID == rows[i].ID))
	}
	return out, nil
}

func listRuleProfiles(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := ruleProfileSummaries(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}

// summaryOf returns the summary of the profile with slug.
func summaryOf(c *gin.Context, a *app.App, slug string) (RuleProfileSummary, error) {
	all, err := ruleProfileSummaries(c, a)
	if err != nil {
		return RuleProfileSummary{}, err
	}
	for _, s := range all {
		if s.Slug == slug {
			return s, nil
		}
	}
	return RuleProfileSummary{}, httpx.NotFound("rule profile not found")
}

// requireEditable answers 403 when users may not customize rule profiles.
func requireEditable(c *gin.Context, a *app.App) bool {
	if !a.Settings.Bool("allow_user_rule_profile") {
		httpx.Fail(c, httpx.Forbidden("rule profile customization is disabled"))
		return false
	}
	return true
}

func validateProfileMeta(name, slug *string) error {
	errs := map[string][]string{}
	if name != nil {
		if msg := engine.ValidProfileName(*name); msg != "" {
			errs["name"] = []string{msg}
		}
	}
	if slug != nil {
		if msg := engine.ValidSlug(*slug); msg != "" {
			errs["slug"] = []string{msg}
		}
	}
	if len(errs) > 0 {
		e := httpx.BadRequest("validation failed")
		e.Errors = errs
		return e
	}
	return nil
}

func createRuleProfile(a *app.App) gin.HandlerFunc {
	type req struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		CopyFrom string `json:"copy_from"`
	}
	return func(c *gin.Context) {
		if !requireEditable(c, a) {
			return
		}
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		var slug *string
		if in.Slug != "" {
			slug = &in.Slug
		}
		if err := validateProfileMeta(&in.Name, slug); err != nil {
			httpx.Fail(c, err)
			return
		}
		u := auth.CurrentUser(c)
		row, err := engine.CreateProfile(c.Request.Context(), a, u.ID, in.Name, in.Slug, in.CopyFrom)
		if err != nil {
			if in.CopyFrom != "" && errors.Is(err, engine.ErrProfileNotFound) {
				httpx.Fail(c, httpx.FieldError("copy_from", "unknown profile"))
				return
			}
			httpx.Fail(c, profileErr(err))
			return
		}
		httpx.Logger(c).Info("rule profile created", "user_id", u.ID, "slug", row.Slug)
		httpx.Created(c, ruleProfileSummary(row, false))
	}
}

func patchRuleProfile(a *app.App) gin.HandlerFunc {
	type req struct {
		Name *string `json:"name"`
		Slug *string `json:"slug"`
	}
	return func(c *gin.Context) {
		if !requireEditable(c, a) {
			return
		}
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := validateProfileMeta(in.Name, in.Slug); err != nil {
			httpx.Fail(c, err)
			return
		}
		u := auth.CurrentUser(c)
		row, err := engine.UpdateProfileMeta(c.Request.Context(), a, u.ID, c.Param("slug"), in.Name, in.Slug)
		if err != nil {
			httpx.Fail(c, profileErr(err))
			return
		}
		s, err := summaryOf(c, a, row.Slug)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, s)
	}
}

func deleteRuleProfile(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireEditable(c, a) {
			return
		}
		u := auth.CurrentUser(c)
		if err := engine.DeleteProfile(c.Request.Context(), a, u.ID, c.Param("slug")); err != nil {
			httpx.Fail(c, profileErr(err))
			return
		}
		httpx.Logger(c).Info("rule profile deleted", "user_id", u.ID, "slug", c.Param("slug"))
		httpx.OK(c, true)
	}
}

func defaultRuleProfile(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireEditable(c, a) {
			return
		}
		u := auth.CurrentUser(c)
		row, err := engine.SetDefaultProfile(c.Request.Context(), a, u.ID, c.Param("slug"))
		if err != nil {
			httpx.Fail(c, profileErr(err))
			return
		}
		s, err := summaryOf(c, a, row.Slug)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, s)
	}
}

// SubscriptionProfile is one entry of GET /user/subscription "profiles".
type SubscriptionProfile struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	IsDefault    bool   `json:"is_default"`
	SubscribeURL string `json:"subscribe_url"`
}

// subscriptionProfiles lists the user's profiles with their links (read-only: a user without rows gets the
// virtual default profile).
func subscriptionProfiles(c *gin.Context, a *app.App, userID uint64, subscribeURL string) ([]SubscriptionProfile, error) {
	rows, err := engine.LoadProfiles(c.Request.Context(), a, userID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []SubscriptionProfile{{Slug: model.RuleProfileDefaultSlug, Name: model.RuleProfileDefaultName,
			IsDefault: true, SubscribeURL: subscribeURL}}, nil
	}
	def := engine.DefaultOf(rows)
	out := make([]SubscriptionProfile, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		p := SubscriptionProfile{Slug: r.Slug, Name: r.Name, IsDefault: def.ID == r.ID, SubscribeURL: subscribeURL}
		if !p.IsDefault {
			p.SubscribeURL += "&profile=" + url.QueryEscape(r.Slug)
		}
		out = append(out, p)
	}
	return out, nil
}
