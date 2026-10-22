package admin

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
)

// Subscription leak protection of one user (admin only; mounted by registerUsers). Owner: BE-S.

// subguardUser loads the :id user (404 when unknown).
func subguardUser(c *gin.Context, a *app.App) (*model.User, error) {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return nil, err
	}
	var u model.User
	if err := a.DB.WithContext(c.Request.Context()).First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, httpx.NotFound("user not found")
		}
		return nil, err
	}
	return &u, nil
}

// subguardAgeHandler: GET /admin/users/:id/subscription/age → the key without the private key.
func subguardAgeHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := subguardUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		k, err := guard.LoadKey(c.Request.Context(), a, u.ID)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, guard.View(a, k, false))
	}
}

// subguardAgeResetHandler: POST /admin/users/:id/subscription/age/reset → a new key pair (the enabled switch
// is kept; the user reads the new private key in the user center). 404 when the user has no key.
func subguardAgeResetHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := subguardUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		cur, err := guard.LoadKey(c.Request.Context(), a, u.ID)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if cur == nil {
			httpx.Fail(c, httpx.NotFound("the user has no age key"))
			return
		}
		k, _, err := guard.GenerateKey(c.Request.Context(), a, u.ID, true, nil)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("admin reset subscription age key", "admin_id", auth.CurrentUser(c).ID, "user_id", u.ID)
		httpx.OK(c, guard.View(a, k, false))
	}
}

// subguardAccessLogHandler: GET /admin/users/:id/subscription/access-log → the newest rows, unmasked.
func subguardAccessLogHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := subguardUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out, err := guard.ListAccess(c.Request.Context(), a, u.ID, guard.AdminLogLimit, false)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}
