package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/geoip"
	"github.com/linusxiong/ggboard/internal/httpx"
)

// IP location database (internal/geoip, docs/API.md "GeoIP"). Admin only.

// Error codes of POST /admin/geoip/update.
const (
	codeGeoIPRunning  = "geoip_update_running" // 409
	codeGeoIPDisabled = "geoip_disabled"       // 409
	codeGeoIPFailed   = "geoip_update_failed"  // 502
	codeGeoIPStopped  = "geoip_unavailable"    // 503
)

// geoipUpdateWait is how long POST /admin/geoip/update waits for the update before answering 202 (the HTTP
// write timeout is 120 s; the download goes on in the background and the client polls GET /admin/geoip).
var geoipUpdateWait = 60 * time.Second

func registerGeoIP(g *gin.RouterGroup, a *app.App) {
	r := g.Group("/geoip", auth.RequireAdmin(a))
	r.GET("", geoipStatusHandler(a))
	r.POST("/update", geoipUpdateHandler(a))
}

// geoipStatusHandler: GET /admin/geoip → geoip.Status.
func geoipStatusHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := geoip.Of(a)
		if s == nil {
			httpx.Fail(c, httpx.NewError(http.StatusServiceUnavailable, codeGeoIPStopped, "shutting down"))
			return
		}
		if !requireLiveAdmin(c, a) {
			return
		}
		httpx.OK(c, s.Status())
	}
}

// geoipUpdateResult is the answer of POST /admin/geoip/update.
type geoipUpdateResult struct {
	geoip.Status
	// Updated: a new file was installed (false = already current, or still running with 202).
	Updated bool `json:"updated"`
}

// geoipUpdateHandler: POST /admin/geoip/update runs an update now. 200 {status…, updated} when it finished
// within geoipUpdateWait, 202 {status… (updating: true), updated: false} when it is still downloading,
// 409 while another update runs or lookups are disabled, 502 when it failed (the current database is kept).
func geoipUpdateHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		s := geoip.Of(a)
		if s == nil {
			httpx.Fail(c, httpx.NewError(http.StatusServiceUnavailable, codeGeoIPStopped, "shutting down"))
			return
		}
		adminID := auth.CurrentUser(c).ID
		var ch <-chan geoip.Outcome
		var beginErr error
		if err := withLiveAdmin(c, a, func(_ *gorm.DB) error {
			ch, beginErr = s.Begin()
			return nil
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		switch {
		case errors.Is(beginErr, geoip.ErrBusy):
			httpx.Fail(c, httpx.NewError(http.StatusConflict, codeGeoIPRunning, "an update is already running"))
			return
		case errors.Is(beginErr, geoip.ErrDisabled):
			httpx.Fail(c, httpx.NewError(http.StatusConflict, codeGeoIPDisabled, "IP locations are turned off (geoip_enable)"))
			return
		case beginErr != nil:
			httpx.Fail(c, httpx.NewError(http.StatusServiceUnavailable, codeGeoIPStopped, beginErr.Error()))
			return
		}
		log := httpx.Logger(c)
		log.Info("admin started a geoip update", "admin_id", adminID)
		timer := time.NewTimer(geoipUpdateWait)
		defer timer.Stop()
		select {
		case out := <-ch:
			if out.Err != nil {
				log.Warn("admin geoip update failed", "admin_id", adminID, "err", out.Err, "version", out.Version)
				httpx.Fail(c, httpx.NewError(http.StatusBadGateway, codeGeoIPFailed, out.Err.Error()))
				return
			}
			log.Info("admin geoip update finished", "admin_id", adminID, "updated", out.Updated, "version", out.Version)
			httpx.OK(c, geoipUpdateResult{Status: s.Status(), Updated: out.Updated})
		case <-timer.C:
			c.JSON(http.StatusAccepted, gin.H{"data": geoipUpdateResult{Status: s.Status()}})
		case <-c.Request.Context().Done():
		}
	}
}
