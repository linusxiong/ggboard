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
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var k *model.SubscriptionKey
		err = withLiveAdmin(c, a, func(tx *gorm.DB) error {
			if err := tx.Select("id").First(&model.User{}, id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return httpx.NotFound("user not found")
				}
				return err
			}
			var rows []model.SubscriptionKey
			if err := tx.Where("user_id = ?", id).Limit(1).Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) > 0 {
				k = &rows[0]
			}
			return nil
		})
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
		k, _, err := guard.GenerateKeyIf(c.Request.Context(), a, u.ID, true, nil, func(tx *gorm.DB) error {
			return settingsLiveAdmin(c, tx, true)
		})
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
		out, err := guard.ListAccessIf(c.Request.Context(), a, u.ID, guard.AdminLogLimit, false, func(tx *gorm.DB) error {
			return settingsLiveAdmin(c, tx, true)
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}
