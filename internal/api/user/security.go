package user

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/service/account"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
)

// securityPerMinute limits /user/security/* requests per user (ceremonies keep state in memory).
const securityPerMinute = 60

// registerSecurity mounts /user/security: two-factor authentication (TOTP + backup codes) and passkeys.
func registerSecurity(g *gin.RouterGroup, a *app.App) {
	s := g.Group("/security", httpx.RateLimit("user_security", httpx.Per(securityPerMinute, time.Minute),
		securityPerMinute, func(c *gin.Context) string { return strconv.FormatUint(auth.CurrentUser(c).ID, 10) }))
	s.GET("", getSecurity(a))
	s.POST("/totp/setup", totpSetup(a))
	s.POST("/totp/enable", totpEnable(a))
	s.POST("/totp/disable", totpDisable(a))
	s.POST("/backup-codes", regenerateBackupCodes(a))
	s.POST("/passkeys/begin", passkeyBegin(a))
	s.POST("/passkeys/finish", passkeyFinish(a))
	s.PATCH("/passkeys/:id", passkeyRename(a))
	s.DELETE("/passkeys/:id", passkeyDelete(a))
}

// bindBody binds a JSON request body for any method (httpx.Bind reads the query string for DELETE).
func bindBody(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return httpx.Validate(dst)
		}
		return httpx.Validation(err)
	}
	return nil
}

func getSecurity(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := mfasvc.New(a).OverviewForUser(c.Request.Context(), auth.CurrentUser(c))
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}

func totpSetup(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out, err := mfasvc.New(a).SetupTOTP(c.Request.Context(), u)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}

func totpEnable(a *app.App) gin.HandlerFunc {
	type req struct {
		Code string `json:"code" binding:"required,max=32"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		codes, err := mfasvc.New(a).EnableTOTP(c.Request.Context(), u, in.Code)
		if err != nil {
			account.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("two-factor authentication enabled", "user_id", u.ID)
		httpx.OK(c, mfasvc.BackupCodes{BackupCodes: codes})
	}
}

func totpDisable(a *app.App) gin.HandlerFunc {
	type req struct {
		Password string `json:"password" binding:"required,max=256"`
		Code     string `json:"code" binding:"required,max=32"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := mfasvc.New(a).DisableTOTP(c.Request.Context(), u, in.Password, in.Code); err != nil {
			if e := httpx.AsError(err); e.Status < 500 {
				httpx.Logger(c).Warn("two-factor disable rejected", "user_id", u.ID, "reason", e.Message)
			}
			account.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("two-factor authentication disabled", "user_id", u.ID)
		httpx.OK(c, true)
	}
}

func regenerateBackupCodes(a *app.App) gin.HandlerFunc {
	type req struct {
		Code string `json:"code" binding:"required,max=32"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		codes, err := mfasvc.New(a).RegenerateBackupCodes(c.Request.Context(), u, in.Code)
		if err != nil {
			account.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("backup codes regenerated", "user_id", u.ID)
		httpx.OK(c, mfasvc.BackupCodes{BackupCodes: codes})
	}
}

func passkeyBegin(a *app.App) gin.HandlerFunc {
	type req struct {
		Name string `json:"name" binding:"max=64"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out, err := mfasvc.New(a).BeginRegistration(c.Request.Context(), u, in.Name)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}

func passkeyFinish(a *app.App) gin.HandlerFunc {
	type req struct {
		ChallengeID string          `json:"challenge_id" binding:"required,max=128"`
		Credential  json.RawMessage `json:"credential" binding:"required"`
		Name        string          `json:"name" binding:"max=64"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		pk, err := mfasvc.New(a).FinishRegistration(c.Request.Context(), u, in.ChallengeID, in.Credential, in.Name)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("passkey added", "user_id", u.ID, "passkey_id", pk.ID)
		httpx.OK(c, pk)
	}
}

func passkeyRename(a *app.App) gin.HandlerFunc {
	type req struct {
		Name string `json:"name" binding:"required,max=64"`
	}
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		pk, err := mfasvc.New(a).RenamePasskey(c.Request.Context(), u, id, in.Name)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, pk)
	}
}

func passkeyDelete(a *app.App) gin.HandlerFunc {
	type req struct {
		Password string `json:"password" binding:"required,max=256"`
	}
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var in req
		if err := bindBody(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := mfasvc.New(a).DeletePasskey(c.Request.Context(), u, id, in.Password); err != nil {
			account.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("passkey removed", "user_id", u.ID, "passkey_id", id)
		httpx.OK(c, true)
	}
}
