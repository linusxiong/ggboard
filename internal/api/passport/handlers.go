package passport

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/telegram"
	"github.com/linusxiong/ggboard/internal/service/account"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// AuthResult is the {token, user} response of register, login and login/token.
type AuthResult struct {
	Token string           `json:"token"`
	User  account.UserView `json:"user"`
}

// respond issues a session token for u and writes {token, user}.
func respond(c *gin.Context, a *app.App, svc *account.Service, u *model.User) {
	tok, err := auth.Issue(a, u)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	view, err := svc.View(c.Request.Context(), u)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, AuthResult{Token: tok, User: view})
}

func register(a *app.App) gin.HandlerFunc {
	type req struct {
		Email      string `json:"email" binding:"required,email,max=191"`
		Password   string `json:"password" binding:"required,min=8,max=128"`
		InviteCode string `json:"invite_code" binding:"max=64"`
		EmailCode  string `json:"email_code" binding:"max=16"`
		Captcha    string `json:"captcha" binding:"max=8192"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		svc := account.New(a)
		u, err := svc.Register(c.Request.Context(), account.RegisterInput{
			Email: in.Email, Password: in.Password, InviteCode: in.InviteCode, EmailCode: in.EmailCode,
			Captcha: in.Captcha, IP: c.ClientIP(),
		})
		if err != nil {
			account.Fail(c, err)
			return
		}
		if err := usersvc.New(a).RecordLogin(c.Request.Context(), u, c.ClientIP()); err != nil {
			httpx.Logger(c).Warn("record login after register failed", "user_id", u.ID, "err", err)
		}
		var inviter, plan uint64
		if u.InviteUserID != nil {
			inviter = *u.InviteUserID
		}
		if u.PlanID != nil {
			plan = *u.PlanID
		}
		httpx.Logger(c).Info("user registered", "user_id", u.ID, "invite_user_id", inviter, "trial_plan_id", plan)
		respond(c, a, svc, u)
	}
}

func login(a *app.App) gin.HandlerFunc {
	type req struct {
		Email    string `json:"email" binding:"required,email,max=191"`
		Password string `json:"password" binding:"required,max=256"`
		Captcha  string `json:"captcha" binding:"max=8192"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		svc := account.New(a)
		u, err := svc.VerifyLogin(ctx, account.LoginInput{
			Email: in.Email, Password: in.Password, Captcha: in.Captcha, IP: c.ClientIP(),
		})
		if err != nil {
			if e := httpx.AsError(err); e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden || e.Status == http.StatusTooManyRequests {
				httpx.Logger(c).Warn("login rejected", "email_domain", account.EmailDomain(usersvc.NormalizeEmail(in.Email)),
					"status", e.Status, "reason", e.Message)
			}
			account.Fail(c, err)
			return
		}
		// Accounts with TOTP get an MFA token instead of a session (second step: POST /login/mfa).
		challenge, err := mfasvc.New(a).Challenge(ctx, u)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if challenge != nil {
			httpx.Logger(c).Info("password accepted, second factor required", "user_id", u.ID)
			httpx.OK(c, challenge)
			return
		}
		svc.RecordLogin(ctx, u, c.ClientIP())
		httpx.Logger(c).Info("user logged in", "user_id", u.ID)
		notifyStaffLogin(c, a, u, notify.MethodPassword)
		respond(c, a, svc, u)
	}
}

func loginToken(a *app.App) gin.HandlerFunc {
	type req struct {
		Token string `json:"token" binding:"required,max=128"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		svc := account.New(a)
		u, err := svc.LoginWithToken(c.Request.Context(), in.Token)
		if err != nil {
			httpx.Logger(c).Warn("one-time login rejected", "reason", httpx.AsError(err).Message)
			account.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("user logged in with one-time token", "user_id", u.ID)
		respond(c, a, svc, u)
	}
}

func forgot(a *app.App) gin.HandlerFunc {
	type req struct {
		Email     string `json:"email" binding:"required,email,max=191"`
		EmailCode string `json:"email_code" binding:"required,max=16"`
		Password  string `json:"password" binding:"required,min=8,max=128"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		u, reclaimed, err := account.New(a).ResetPassword(ctx, in.Email, in.EmailCode, in.Password)
		if err != nil {
			account.Fail(c, err)
			return
		}
		if reclaimed {
			// The account changed hands: a bind code the previous holder requested must not bind their chat now.
			if err := telegram.Unbind(ctx, a, u); err != nil {
				httpx.Logger(c).Warn("revoke telegram bind code after reclaim failed", "user_id", u.ID, "err", err)
			}
		}
		httpx.Logger(c).Info("password reset by email code", "user_id", u.ID, "reclaimed", reclaimed)
		httpx.OK(c, true)
	}
}

func sendCode(a *app.App) gin.HandlerFunc {
	type req struct {
		Email   string `json:"email" binding:"required,email,max=191"`
		Captcha string `json:"captcha" binding:"max=8192"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		svc := account.New(a)
		if err := svc.VerifyCaptcha(c.Request.Context(), in.Captcha, c.ClientIP()); err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := svc.SendCode(in.Email); err != nil {
			account.Fail(c, err)
			return
		}
		httpx.OK(c, true)
	}
}
