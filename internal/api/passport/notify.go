package passport

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	mfasvc "github.com/linusxiong/ggboard/internal/service/mfa"
)

// notifyStaffLogin emits admin_login when an administrator or staff member signs in (password, password +
// TOTP/backup code, or passkey). Other users are ignored.
func notifyStaffLogin(c *gin.Context, a *app.App, u *model.User, method string) {
	if !u.IsStaffOrAdmin() {
		return
	}
	notify.Emit(a, notify.AdminLogin(a, notify.Login{
		Email: u.Email, IsAdmin: u.IsAdmin, Method: method, IP: c.ClientIP(), UA: c.Request.UserAgent(), At: time.Now(),
	}))
}

// secondFactorMethod tells a TOTP code from a backup code (the same field of /login/mfa).
func secondFactorMethod(code string) string {
	if mfasvc.IsTOTPCode(code) {
		return notify.MethodTOTP
	}
	return notify.MethodBackup
}
