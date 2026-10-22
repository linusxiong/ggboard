package user

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
)

// registerSubGuard mounts the subscription leak-protection routes (docs/API.md "Subscription protection"):
// the age key, the access log and the token-only rotation. Owner: BE-S.
func registerSubGuard(g *gin.RouterGroup, a *app.App) {
	limit := httpx.Limit("subscription_guard", httpx.NewLimiter(httpx.Per(30, time.Minute), 10), func(c *gin.Context) string {
		if u := auth.CurrentUser(c); u != nil {
			return "u" + strconv.FormatUint(u.ID, 10)
		}
		return httpx.KeyByIP(c)
	})
	g.GET("/subscription/age", getAgeKey(a))
	g.POST("/subscription/age", limit, createAgeKey(a))
	g.PATCH("/subscription/age", limit, patchAgeKey(a))
	g.DELETE("/subscription/age", limit, deleteAgeKey(a))
	g.GET("/subscription/access-log", subscriptionAccessLog(a))
	g.POST("/subscription/rotate-token", limit, rotateSubscriptionToken(a))
}

func getAgeKey(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		k, err := guard.LoadKey(c.Request.Context(), a, auth.CurrentUser(c).ID)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, guard.View(a, k, true))
	}
}

func createAgeKey(a *app.App) gin.HandlerFunc {
	type req struct {
		Regenerate bool  `json:"regenerate"`
		Enabled    *bool `json:"enabled"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		id := auth.CurrentUser(c).ID
		k, _, err := guard.GenerateKey(c.Request.Context(), a, id, in.Regenerate, in.Enabled)
		if errors.Is(err, guard.ErrKeyExists) {
			httpx.Fail(c, httpx.Conflict("an age key already exists; send {\"regenerate\": true} to replace it"))
			return
		}
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("subscription age key generated", "user_id", id, "regenerate", in.Regenerate, "enabled", k.Enabled)
		httpx.OK(c, guard.View(a, k, true))
	}
}

func patchAgeKey(a *app.App) gin.HandlerFunc {
	type req struct {
		Enabled *bool `json:"enabled" binding:"required"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		id := auth.CurrentUser(c).ID
		k, err := guard.SetKeyEnabled(c.Request.Context(), a, id, *in.Enabled)
		if errors.Is(err, guard.ErrNoKey) {
			httpx.Fail(c, httpx.NotFound("no age key; create one first"))
			return
		}
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("subscription age encryption switched", "user_id", id, "enabled", k.Enabled)
		httpx.OK(c, guard.View(a, k, true))
	}
}

func deleteAgeKey(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := auth.CurrentUser(c).ID
		if err := guard.DeleteKey(c.Request.Context(), a, id); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("subscription age key deleted", "user_id", id)
		httpx.OK(c, true)
	}
}

func subscriptionAccessLog(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := guard.ListAccess(c.Request.Context(), a, auth.CurrentUser(c).ID, guard.UserLogLimit, true)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}

// TokenRotation is the response of POST /user/subscription/rotate-token.
type TokenRotation struct {
	SubscribeURL string `json:"subscribe_url"`
	Token        string `json:"token"`
}

func rotateSubscriptionToken(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		users := usersvc.New(a)
		if err := users.RotateToken(c.Request.Context(), u); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("subscription token rotated", "user_id", u.ID)
		httpx.OK(c, TokenRotation{SubscribeURL: users.SubscribeURL(u), Token: u.Token})
	}
}
