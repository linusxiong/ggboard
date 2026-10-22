package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
)

// telegramFailAdmin renders telegram errors: incomplete settings are 400 field errors (already *httpx.Error),
// an unreachable Bot API is 502 telegram_unavailable with the (token-free) cause.
func telegramFailAdmin(c *gin.Context, err error) {
	if errors.Is(err, telegram.ErrUnavailable) {
		httpx.Logger(c).Warn("telegram: Bot API call failed", "err", err)
		httpx.Fail(c, httpx.NewError(http.StatusBadGateway, telegram.CodeUnavailable, err.Error()))
		return
	}
	httpx.Fail(c, err)
}

// settingsTelegramWebhookHandler is POST /admin/settings/telegram/webhook: setWebhook with
// <app_url>/api/v1/guest/telegram/webhook/<telegram_webhook_secret> → {ok, description, error_code?, url}.
func settingsTelegramWebhookHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := telegram.RegisterWebhook(c.Request.Context(), a)
		if err != nil {
			telegramFailAdmin(c, err)
			return
		}
		httpx.Logger(c).Info("admin set the telegram webhook", "admin_id", auth.CurrentUser(c).ID, "ok", res.OK)
		httpx.OK(c, res)
	}
}

// settingsTelegramTestHandler is POST /admin/settings/telegram/test: one test message per administrator chat,
// sent synchronously → {sent, failed, results:[{chat_id, ok, error?}]}.
func settingsTelegramTestHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		rep, err := telegram.SendTest(c.Request.Context(), a)
		if err != nil {
			telegramFailAdmin(c, err)
			return
		}
		httpx.OK(c, rep)
	}
}
