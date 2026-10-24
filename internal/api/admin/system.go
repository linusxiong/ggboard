package admin

import (
	"log/slog"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/logx"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/scheduler"
	_ "github.com/linusxiong/ggboard/internal/service/maintenance" // registers traffic_reset, expire_remind, traffic_remind, stats_daily
	statsvc "github.com/linusxiong/ggboard/internal/service/stat"
)

// registerSystem mounts /admin/system/* (admin only).
// Owner: BE-E1 admin ops. Implement in this file; do not add routes elsewhere.
func registerSystem(g *gin.RouterGroup, a *app.App) {
	s := g.Group("/system", auth.RequireAdmin(a))
	s.GET("/status", systemStatusHandler(a))
	s.GET("/logs", systemLogsHandler(a))
}

// systemStatus is GET /admin/system/status.
type systemStatus struct {
	Version    string                `json:"version"`
	GoVersion  string                `json:"go_version"`
	Uptime     int64                 `json:"uptime"` // seconds
	DBDriver   string                `json:"db_driver"`
	StartedAt  time.Time             `json:"started_at"`
	Scheduler  []scheduler.JobStatus `json:"scheduler"`
	Mail       mail.QueueStats       `json:"mail"`
	MailQueue  int64                 `json:"mail_queue"` // = mail.pending
	Notify     notify.QueueStats     `json:"notify"`
	Cache      cache.Stats           `json:"cache"`
	Online     systemOnline          `json:"online"`
	Goroutines int                   `json:"goroutines"`
	Memory     systemMemory          `json:"memory"`
}

type systemOnline struct {
	Nodes int `json:"nodes"`
	Users int `json:"users"`
}

type systemMemory struct {
	Alloc uint64 `json:"alloc"` // bytes of live heap objects
	Sys   uint64 `json:"sys"`   // bytes obtained from the OS
}

func systemStatusHandler(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		stats := statsvc.New(a)
		_, onlineNodes, err := stats.NodeCounts(c.Request.Context(), now)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		mailStats := a.Mailer.Stats()
		httpx.OK(c, systemStatus{
			Version:    a.Version,
			GoVersion:  runtime.Version(),
			Uptime:     int64(now.Sub(a.StartedAt) / time.Second),
			DBDriver:   a.DBDriver(),
			StartedAt:  a.StartedAt,
			Scheduler:  a.Scheduler.Status(),
			Mail:       mailStats,
			MailQueue:  mailStats.Pending,
			Notify:     notify.Stats(a),
			Cache:      a.Cache.Stats(),
			Online:     systemOnline{Nodes: onlineNodes, Users: stats.OnlineUsers()},
			Goroutines: runtime.NumGoroutine(),
			Memory:     systemMemory{Alloc: ms.Alloc, Sys: ms.Sys},
		})
	}
}

// systemLogEntry is one record of GET /admin/system/logs (attrs is always an object).
type systemLogEntry struct {
	Time  time.Time      `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs"`
}

// systemLogsHandler returns the newest buffered log records at or above ?level (default debug: everything),
// at most ?limit (default 200, max logx.DefaultRingSize).
func systemLogsHandler(a *app.App) gin.HandlerFunc {
	type req struct {
		Level string `form:"level" binding:"omitempty,oneof=debug info warn warning error"`
		Limit int    `form:"limit" binding:"omitempty,gte=1,lte=1000"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.BindQuery(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		lvl := slog.LevelDebug
		if in.Level != "" {
			var err error
			if lvl, err = logx.ParseLevel(in.Level); err != nil {
				httpx.Fail(c, httpx.FieldError("level", err.Error()))
				return
			}
		}
		if in.Limit == 0 {
			in.Limit = 200
		}
		entries := a.Logs.Entries(lvl, in.Limit)
		out := make([]systemLogEntry, len(entries))
		for i, e := range entries {
			attrs := e.Attrs
			if attrs == nil {
				attrs = map[string]any{}
			}
			out[i] = systemLogEntry{Time: e.Time, Level: e.Level, Msg: e.Msg, Attrs: attrs}
		}
		httpx.OK(c, out)
	}
}
