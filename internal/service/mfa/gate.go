package mfasvc

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
)

// Overview is the response of GET /user/security.
type Overview struct {
	TOTPEnabled          bool          `json:"totp_enabled"`
	TOTPEnabledAt        *time.Time    `json:"totp_enabled_at"`
	BackupCodesRemaining int           `json:"backup_codes_remaining"`
	Passkeys             []PasskeyView `json:"passkeys"`
}

// Overview returns the second-factor state of userID.
func (s *Service) Overview(ctx context.Context, userID uint64) (Overview, error) {
	row, err := s.load(ctx, s.a.DB, userID)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Passkeys: []PasskeyView{}}
	if row != nil && row.TOTPEnabled {
		out.TOTPEnabled = true
		out.BackupCodesRemaining = len(row.BackupCodes)
		if row.TOTPEnabledAt != nil {
			t := row.TOTPEnabledAt.UTC()
			out.TOTPEnabledAt = &t
		}
	}
	if out.Passkeys, err = s.ListPasskeys(ctx, userID); err != nil {
		return Overview{}, err
	}
	return out, nil
}

// AdminGate enforces the admin_mfa_required setting: while it is on, administrators and staff who have neither
// TOTP nor a passkey get 403 mfa_setup_required (they can still use /user/security to enrol). Mount it after
// auth.RequireAdmin or auth.RequireStaff:
//
//	g := r.Group(httpx.APIPrefix+"/admin", auth.RequireStaff(a), mfasvc.AdminGate(a))
func AdminGate(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.Settings.Bool("admin_mfa_required") {
			c.Next()
			return
		}
		u := auth.CurrentUser(c)
		if u == nil {
			httpx.Fail(c, httpx.Unauthorized("authentication required"))
			return
		}
		if !u.IsStaffOrAdmin() {
			c.Next()
			return
		}
		ok, err := New(a).Configured(c.Request.Context(), u.ID)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if !ok {
			httpx.Fail(c, httpx.NewError(http.StatusForbidden, CodeSetupRequired,
				"set up two-factor authentication or a passkey to use the admin area"))
			return
		}
		c.Next()
	}
}
