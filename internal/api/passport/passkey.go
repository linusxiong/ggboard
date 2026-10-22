package passport

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/service/account"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
)

// passkeyLoginBegin is POST /passport/passkey/login/begin {email?} -> {challenge_id, options}. Without email
// the options are for a discoverable (usernameless) login with an empty allowCredentials list.
func passkeyLoginBegin(a *app.App) gin.HandlerFunc {
	type req struct {
		Email string `json:"email" binding:"omitempty,email,max=191"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		ceremony, err := mfasvc.New(a).BeginLogin(c.Request.Context(), in.Email)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, ceremony)
	}
}

// passkeyLoginFinish is POST /passport/passkey/login/finish {challenge_id, credential} -> {token, user}.
// credential is the AuthenticationResponseJSON produced by @simplewebauthn/browser startAuthentication.
func passkeyLoginFinish(a *app.App) gin.HandlerFunc {
	type req struct {
		ChallengeID string          `json:"challenge_id" binding:"required,max=128"`
		Credential  json.RawMessage `json:"credential" binding:"required"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		u, err := mfasvc.New(a).FinishLogin(ctx, in.ChallengeID, in.Credential)
		if err != nil {
			if e := httpx.AsError(err); e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden {
				httpx.Logger(c).Warn("passkey login rejected", "status", e.Status, "reason", e.Message)
			}
			account.Fail(c, err)
			return
		}
		svc := account.New(a)
		svc.RecordLogin(ctx, u, c.ClientIP())
		httpx.Logger(c).Info("user logged in with passkey", "user_id", u.ID)
		respond(c, a, svc, u)
	}
}

// SecurityPagePath is the SPA page where users manage their password, two-factor authentication and passkeys.
const SecurityPagePath = "/profile?tab=security"

// securityPageURL returns the absolute URL of the security page: app_url when it is set, else the origin of
// the request.
func securityPageURL(c *gin.Context, a *app.App) string {
	if base := mfasvc.AppBaseURL(a.Settings); base != "" {
		return base + SecurityPagePath
	}
	return httpx.RequestBase(c) + SecurityPagePath
}

// changePasswordURL is GET /.well-known/change-password: 302 to the security page
// (https://w3c.github.io/webappsec-change-password-url/), used by password managers.
func changePasswordURL(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Redirect(http.StatusFound, securityPageURL(c, a))
	}
}

// PasskeyEndpoints is the body of GET /.well-known/passkey-endpoints.
type PasskeyEndpoints struct {
	Enroll string `json:"enroll"`
	Manage string `json:"manage"`
}

// passkeyEndpoints is GET /.well-known/passkey-endpoints (Passkey Endpoints Well-Known URL, used by password
// managers such as 1Password to link to passkey enrolment and management).
func passkeyEndpoints(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := securityPageURL(c, a)
		body, err := json.Marshal(PasskeyEndpoints{Enroll: u, Manage: u})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		c.Header("Cache-Control", "public, max-age=3600")
		c.Data(http.StatusOK, "application/json", body)
	}
}
