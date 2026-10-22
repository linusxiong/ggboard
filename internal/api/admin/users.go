package admin

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

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
}

// usersRedact blanks credentials in views returned to staff.
func usersRedact(c *gin.Context, vs ...*adminuser.View) {
	if u := auth.CurrentUser(c); u != nil && u.IsAdmin {
		return
	}
	for _, v := range vs {
		v.Redact()
	}
}

// usersRespond renders u as an admin view with status.
func usersRespond(c *gin.Context, a *app.App, status int, u *model.User) {
	v, err := adminuser.New(a).View(c.Request.Context(), u)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(status, gin.H{"data": v})
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
		for i := range views {
			usersRedact(c, &views[i])
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
		usersRedact(c, &d.View)
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
		u, err := adminuser.New(a).Create(c.Request.Context(), in, time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("admin created user", "admin_id", auth.CurrentUser(c).ID, "user_id", u.ID)
		usersRespond(c, a, http.StatusCreated, u)
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
		u, err := adminuser.New(a).Update(c.Request.Context(), actor, id, in, time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("admin updated user", "admin_id", actor.ID, "user_id", id, "password_changed", in.Password != nil)
		usersRespond(c, a, http.StatusOK, u)
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
		if err := adminuser.New(a).Delete(c.Request.Context(), actor, id); err != nil {
			httpx.Fail(c, err)
			return
		}
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
		u, err := adminuser.New(a).ResetSecret(c.Request.Context(), id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("admin reset user secret", "admin_id", auth.CurrentUser(c).ID, "user_id", id)
		usersRespond(c, a, http.StatusOK, u)
	}
}

func usersLoginTokenHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		url, err := adminuser.New(a).LoginURL(c.Request.Context(), id)
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
		u, err := adminuser.New(a).ResetTraffic(c.Request.Context(), id, time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		usersRespond(c, a, http.StatusOK, u)
	}
}

func usersGenerateHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in adminuser.GenerateInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		out, err := adminuser.New(a).Generate(c.Request.Context(), in, time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
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
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="users-%s.csv"`, now.Format("20060102-150405")))
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusOK)
		if err := adminuser.New(a).Export(c.Request.Context(), c.Writer, f, now); err != nil {
			if !c.Writer.Written() { // nothing sent yet: answer with a JSON error instead of a CSV
				c.Writer.Header().Del("Content-Disposition")
				c.Writer.Header().Del("Content-Type")
			}
			httpx.Fail(c, err) // after the first byte this only logs and aborts
			return
		}
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
		n, err := adminuser.New(a).Mail(c.Request.Context(), in, time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, gin.H{"queued": n})
	}
}
