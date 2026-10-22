package admin

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	statsvc "github.com/linusxiong/ggboard/internal/service/stat"
)

// registerStats mounts /admin/stats/* (admin only).
// Owner: BE-E1 admin ops. Implement in this file; do not add routes elsewhere.
func registerStats(g *gin.RouterGroup, a *app.App) {
	s := g.Group("/stats", auth.RequireAdmin(a))
	s.GET("/overview", statsOverviewHandler(a))
	s.GET("/series", statsSeriesHandler(a))
	s.GET("/nodes/ranking", statsNodeRankingHandler(a))
	s.GET("/users/ranking", statsUserRankingHandler(a))
}

func statsOverviewHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := statsvc.New(a).Overview(c.Request.Context(), time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}

func statsSeriesHandler(a *app.App) gin.HandlerFunc {
	type req struct {
		Metric string `form:"metric" binding:"required,oneof=income orders registers traffic"`
		Range  string `form:"range" binding:"omitempty,oneof=7d 30d 90d"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.BindQuery(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		if in.Range == "" {
			in.Range = "30d"
		}
		out, err := statsvc.New(a).Series(c.Request.Context(), in.Metric, statsvc.Ranges[in.Range], time.Now())
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}

// statsDay parses ?date=YYYY-MM-DD as a local calendar day (default today).
func statsDay(c *gin.Context) (time.Time, error) {
	raw := c.Query("date")
	if raw == "" {
		return statsvc.DayStart(time.Now()), nil
	}
	d, err := time.ParseInLocation(time.DateOnly, raw, time.Local)
	if err != nil {
		return time.Time{}, httpx.FieldError("date", "must be a date in format YYYY-MM-DD")
	}
	return d, nil
}

func statsNodeRankingHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		day, err := statsDay(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out, err := statsvc.New(a).NodeRanking(c.Request.Context(), day)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}

func statsUserRankingHandler(a *app.App) gin.HandlerFunc {
	type req struct {
		Limit int `form:"limit" binding:"omitempty,gte=1,lte=500"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.BindQuery(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		if in.Limit == 0 {
			in.Limit = 20
		}
		day, err := statsDay(c)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out, err := statsvc.New(a).UserRanking(c.Request.Context(), day, in.Limit)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, out)
	}
}
