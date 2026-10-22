package guest

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
)

// registerTelegram mounts POST /guest/telegram/webhook/:secret (the Telegram bot webhook).
// Owner: BE-T notifications. Implement in this file; do not add routes elsewhere.
func registerTelegram(g *gin.RouterGroup, a *app.App) {
	g.POST("/telegram/webhook/:secret",
		httpx.RateLimit("telegram_webhook", httpx.Per(600, time.Minute), 120, httpx.KeyByIP),
		telegramWebhook(a))
}

// telegramWebhook checks the path secret (and Telegram's X-Telegram-Bot-Api-Secret-Token header when present)
// in constant time, runs the bot command and answers with the reply as a webhook method call. Malformed or
// ignored updates get an empty 200 so Telegram does not redeliver them.
func telegramWebhook(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !telegram.ValidSecret(a, c.Param("secret")) {
			httpx.Fail(c, httpx.Forbidden("invalid webhook"))
			return
		}
		if h := c.GetHeader("X-Telegram-Bot-Api-Secret-Token"); h != "" && !telegram.ValidSecret(a, h) {
			httpx.Fail(c, httpx.Forbidden("invalid webhook"))
			return
		}
		var up telegram.Update
		if err := c.ShouldBindJSON(&up); err != nil {
			httpx.Logger(c).Debug("telegram: undecodable update ignored", "err", err)
			c.Status(http.StatusOK)
			return
		}
		if r := telegram.HandleUpdate(c.Request.Context(), a, up); r != nil {
			c.JSON(http.StatusOK, r)
			return
		}
		c.Status(http.StatusOK)
	}
}
