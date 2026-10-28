package mfasvc

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
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
	return s.overview(ctx, s.a.DB, userID)
}

// OverviewForUser keeps the account binding checked while reading its second-factor details.
func (s *Service) OverviewForUser(ctx context.Context, u *model.User) (Overview, error) {
	var out Overview
	err := s.withUser(ctx, u, func(tx *gorm.DB) error {
		var err error
		out, err = s.overview(ctx, tx, u.ID)
		return err
	})
	return out, err
}

func (s *Service) overview(ctx context.Context, db *gorm.DB, userID uint64) (Overview, error) {
	row, err := s.load(ctx, db, userID)
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
	if out.Passkeys, err = s.listPasskeys(ctx, db, userID); err != nil {
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
