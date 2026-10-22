package passport

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/service/account"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
)

// loginMFA is POST /passport/login/mfa {mfa_token, code}: the second step of a password login for accounts
// with TOTP. code is a 6-digit TOTP or an unused backup code ("xxxxx-xxxxx").
func loginMFA(a *app.App) gin.HandlerFunc {
	type req struct {
		MFAToken string `json:"mfa_token" binding:"required,max=128"`
		Code     string `json:"code" binding:"required,max=32"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		u, err := mfasvc.New(a).CompleteLogin(ctx, in.MFAToken, in.Code)
		if err != nil {
			if e := httpx.AsError(err); e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden ||
				e.Status == http.StatusTooManyRequests {
				httpx.Logger(c).Warn("second factor rejected", "status", e.Status, "code", e.Code)
			}
			account.Fail(c, err)
			return
		}
		svc := account.New(a)
		svc.RecordLogin(ctx, u, c.ClientIP())
		httpx.Logger(c).Info("user logged in with second factor", "user_id", u.ID)
		respond(c, a, svc, u)
	}
}
