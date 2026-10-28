package admin

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
)

// liveStaffRole verifies the request's account binding and returns its current admin role.
func liveStaffRole(c *gin.Context, db *gorm.DB, lock bool) (bool, error) {
	claims := auth.CurrentClaims(c)
	if claims == nil {
		return false, httpx.Unauthorized("session expired, please sign in again")
	}
	id, err := claims.UserID()
	if err != nil {
		return false, httpx.Unauthorized("session expired, please sign in again")
	}
	if lock {
		if err := db.Model(&model.User{}).Where("id = ?", id).UpdateColumn("token", gorm.Expr("token")).Error; err != nil {
			return false, err
		}
	}
	var current model.User
	err = db.Select("id", "uuid", "email", "password_version", "banned", "is_admin", "is_staff").First(&current, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !claims.BoundTo(&current) {
		return false, httpx.Unauthorized("session expired, please sign in again")
	}
	if err != nil {
		return false, err
	}
	if current.Banned || !current.IsStaffOrAdmin() {
		return false, httpx.Forbidden("staff only")
	}
	return current.IsAdmin, nil
}

func settingsLiveAdmin(c *gin.Context, db *gorm.DB, lock bool) error {
	admin, err := liveStaffRole(c, db, lock)
	if err != nil {
		return err
	}
	if !admin {
		return httpx.Forbidden("administrator only")
	}
	return nil
}

func requireLiveAdmin(c *gin.Context, a *app.App) bool {
	if err := settingsLiveAdmin(c, a.DB.WithContext(c.Request.Context()), false); err != nil {
		httpx.Fail(c, err)
		return false
	}
	return true
}

// withLiveStaff binds a response snapshot to the caller's current role.
func withLiveStaff(c *gin.Context, a *app.App, run func(bool) error) error {
	return withLiveStaffTx(c, a, func(_ *gorm.DB, admin bool) error { return run(admin) })
}

func withLiveStaffTx(c *gin.Context, a *app.App, run func(*gorm.DB, bool) error) error {
	return a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		admin, err := liveStaffRole(c, tx, true)
		if err != nil {
			return err
		}
		return run(tx, admin)
	})
}

// withLiveAdmin serializes a sensitive read or write with changes to the caller's admin account.
func withLiveAdmin(c *gin.Context, a *app.App, run func(*gorm.DB) error) error {
	return a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := settingsLiveAdmin(c, tx, true); err != nil {
			return err
		}
		return run(tx)
	})
}
