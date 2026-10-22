package user

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
)

// registerTelegram mounts GET /user/telegram, POST /user/telegram/bind-code and DELETE /user/telegram.
// Owner: BE-T notifications. Implement in this file; do not add routes elsewhere.
func registerTelegram(g *gin.RouterGroup, a *app.App) {
	byUser := func(c *gin.Context) string { return strconv.FormatUint(auth.CurrentUser(c).ID, 10) }
	limit := httpx.Limit("telegram_bind", httpx.NewLimiter(httpx.Per(10, time.Minute), 5), byUser)
	g.GET("/telegram", telegramStatus(a))
	g.POST("/telegram/bind-code", limit, telegramBindCode(a))
	g.DELETE("/telegram", telegramUnbind(a))
}

// telegramFail maps telegram package errors: no bot token → 403, Bot API unreachable → 502.
func telegramFail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, telegram.ErrNotConfigured):
		httpx.Fail(c, httpx.Forbidden("Telegram notifications are not enabled"))
	case errors.Is(err, telegram.ErrUnavailable):
		httpx.Logger(c).Warn("telegram: Bot API call failed", "err", err)
		httpx.Fail(c, httpx.NewError(http.StatusBadGateway, telegram.CodeUnavailable, "Telegram is unreachable, try again later"))
	default:
		httpx.Fail(c, err)
	}
}

// telegramStatus is GET /user/telegram → telegram.Status (read from the database, not the 30 s user cache).
func telegramStatus(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, telegram.UserStatus(c.Request.Context(), a, u))
	}
}

// telegramBindCode is POST /user/telegram/bind-code → {code, bot_username, deep_link} (code valid 10 min).
func telegramBindCode(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		code, err := telegram.IssueBindCode(c.Request.Context(), a, auth.CurrentUser(c).ID)
		if err != nil {
			telegramFail(c, err)
			return
		}
		httpx.OK(c, code)
	}
}

// telegramUnbind is DELETE /user/telegram → true (idempotent).
func telegramUnbind(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := telegram.Unbind(c.Request.Context(), a, auth.CurrentUser(c).ID); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, true)
	}
}
