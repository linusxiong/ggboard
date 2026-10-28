package admin

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/adminuser"
)

// usersSort maps the sortable API fields of GET /admin/users to columns.
var usersSort = map[string]string{
	"id": "id", "email": "email", "expired_at": "expired_at", "created_at": "created_at", "u": "u", "d": "d",
	"transfer_enable": "transfer_enable", "balance": "balance", "last_login_at": "last_login_at",
}

// registerUsers mounts /admin/users* (GET list/detail allowed for staff, everything else admin only).
// Owner: BE-E1 admin ops. Implement in this file; do not add routes elsewhere.
func registerUsers(g *gin.RouterGroup, a *app.App) {
	read := g.Group("/users") // staff: the parent group already requires staff
	read.GET("", usersListHandler(a))
	read.GET("/:id", usersDetailHandler(a))

	w := g.Group("/users", auth.RequireAdmin(a))
	w.POST("", usersCreateHandler(a))
	w.POST("/generate", usersGenerateHandler(a))
	w.POST("/export", usersExportHandler(a))
	w.POST("/mail", usersMailHandler(a))
	w.PATCH("/:id", usersUpdateHandler(a))
	w.DELETE("/:id", usersDeleteHandler(a))
	w.POST("/:id/reset-secret", usersResetSecretHandler(a))
	w.POST("/:id/login-token", usersLoginTokenHandler(a))
	w.POST("/:id/traffic-reset", usersTrafficResetHandler(a))
	// Subscription leak protection (admin/subguard.go, BE-S).
	w.GET("/:id/subscription/age", subguardAgeHandler(a))
	w.POST("/:id/subscription/age/reset", subguardAgeResetHandler(a))
	w.GET("/:id/subscription/access-log", subguardAccessLogHandler(a))
}

func usersListHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var f adminuser.Filters
		if err := httpx.BindQuery(c, &f); err != nil {
			httpx.Fail(c, err)
			return
		}
		q := f.Apply(a.DB.WithContext(c.Request.Context()).Model(&model.User{}), time.Now())
		users, total, err := httpx.Paginate[model.User](c, q, usersSort, "id DESC")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		views, err := adminuser.New(a).Views(c.Request.Context(), users)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		err = withLiveStaff(c, a, func(admin bool) error {
			if !admin {
				for i := range views {
					views[i].Redact()
				}
			}
			return nil
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, views, total)
	}
}

func usersDetailHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		d, err := adminuser.New(a).Detail(c.Request.Context(), id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := withLiveStaff(c, a, func(admin bool) error {
			if !admin {
				d.View.Redact()
			}
			return nil
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, d)
	}
}

func usersCreateHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in adminuser.CreateInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		var u *model.User
		var view adminuser.View
		err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			var err error
			u, err = adminuser.New(a).CreateWithDB(tx, in, time.Now())
			if err != nil {
				return err
			}
			view, err = adminuser.New(a).ViewWithDB(tx, u)
			return err
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		adminuser.New(a).InvalidateNodeUsers()
		httpx.Logger(c).Info("admin created user", "admin_id", auth.CurrentUser(c).ID, "user_id", u.ID)
		c.JSON(http.StatusCreated, gin.H{"data": view})
	}
}

func usersUpdateHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var in adminuser.UpdateInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		actor := auth.CurrentUser(c)
		var u *model.User
		var view adminuser.View
		err = withLiveAdmin(c, a, func(tx *gorm.DB) error {
			var err error
			u, err = adminuser.New(a).UpdateWithDB(tx, actor, id, in, time.Now())
			if err != nil {
				return err
			}
			view, err = adminuser.New(a).ViewWithDB(tx, u)
			return err
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		adminuser.New(a).InvalidateUser(id)
		httpx.Logger(c).Info("admin updated user", "admin_id", actor.ID, "user_id", id, "password_changed", in.Password != nil)
		c.JSON(http.StatusOK, gin.H{"data": view})
	}
}

func usersDeleteHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		actor := auth.CurrentUser(c)
		if err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			return adminuser.New(a).DeleteWithDB(tx, actor, id)
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		adminuser.New(a).InvalidateUser(id)
		httpx.Logger(c).Info("admin deleted user", "admin_id", actor.ID, "user_id", id)
		httpx.OK(c, true)
	}
}

func usersResetSecretHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var before, u *model.User
		var view adminuser.View
		err = withLiveAdmin(c, a, func(tx *gorm.DB) error {
			var err error
			before, u, err = adminuser.New(a).ResetSecretWithDB(tx, id)
			if err != nil {
				return err
			}
			view, err = adminuser.New(a).ViewWithDB(tx, u)
			return err
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		adminuser.New(a).FinishResetSecret(before, u)
		httpx.Logger(c).Info("admin reset user secret", "admin_id", auth.CurrentUser(c).ID, "user_id", id)
		c.JSON(http.StatusOK, gin.H{"data": view})
	}
}

func usersLoginTokenHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var url string
		err = withLiveAdmin(c, a, func(tx *gorm.DB) error {
			var err error
			url, err = adminuser.New(a).LoginURLWithDB(tx, id)
			return err
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("admin issued login token", "admin_id", auth.CurrentUser(c).ID, "user_id", id)
		httpx.OK(c, gin.H{"url": url})
	}
}

func usersTrafficResetHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var u *model.User
		var view adminuser.View
		err = withLiveAdmin(c, a, func(tx *gorm.DB) error {
			var err error
			u, err = adminuser.New(a).ResetTrafficWithDB(tx, id, time.Now())
			if err != nil {
				return err
			}
			view, err = adminuser.New(a).ViewWithDB(tx, u)
			return err
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		adminuser.New(a).InvalidateUser(id)
		c.JSON(http.StatusOK, gin.H{"data": view})
	}
}

func usersGenerateHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in adminuser.GenerateInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		var out *adminuser.GenerateResult
		err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			var err error
			out, err = adminuser.New(a).GenerateWithDB(tx, in, time.Now())
			return err
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		adminuser.New(a).InvalidateNodeUsers()
		httpx.OK(c, out)
	}
}

func usersExportHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var f adminuser.Filters
		if err := httpx.Bind(c, &f); err != nil {
			httpx.Fail(c, err)
			return
		}
		now := time.Now()
		file, err := os.CreateTemp("", "ggboard-users-*.csv")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		defer os.Remove(file.Name())
		defer file.Close()
		if err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			return adminuser.New(a).ExportWithDB(tx, file, f, now)
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		info, err := file.Stat()
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if _, err := file.Seek(0, 0); err != nil {
			httpx.Fail(c, err)
			return
		}
		c.DataFromReader(http.StatusOK, info.Size(), "text/csv; charset=utf-8", file, map[string]string{
			"Content-Disposition": fmt.Sprintf(`attachment; filename="users-%s.csv"`, now.Format("20060102-150405")),
			"Cache-Control":       "no-store",
		})
		httpx.Logger(c).Info("admin exported users", "admin_id", auth.CurrentUser(c).ID)
	}
}

func usersMailHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in adminuser.MailInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		var n int
		err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			emails, err := adminuser.New(a).MailRecipientsWithDB(tx, in, time.Now())
			if err != nil {
				return err
			}
			n, err = adminuser.New(a).MailToRecipients(c.Request.Context(), emails, in)
			return err
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, gin.H{"queued": n})
	}
}
