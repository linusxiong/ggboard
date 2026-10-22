package user

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/account"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// registerAccount mounts GET/PATCH /user/me, POST /user/password, GET /user/stats, GET /user/subscription, POST /user/subscription/reset, GET /user/traffic-logs, POST /user/logout.
func registerAccount(g *gin.RouterGroup, a *app.App) {
	g.GET("/me", getMe(a))
	g.PATCH("/me", updateMe(a))
	g.POST("/password", changePassword(a))
	g.GET("/stats", getStats(a))
	g.GET("/subscription", getSubscription(a))
	g.POST("/subscription/reset", resetSubscription(a))
	g.GET("/traffic-logs", trafficLogs(a))
	g.POST("/logout", logout())
}

// freshUser reloads the authenticated user from the database (auth.CurrentUser may be 30 s stale).
func freshUser(c *gin.Context, a *app.App) (*model.User, error) {
	u, err := usersvc.New(a).FindByID(c.Request.Context(), auth.CurrentUser(c).ID)
	if errors.Is(err, usersvc.ErrNotFound) {
		return nil, httpx.Unauthorized("invalid or expired token")
	}
	return u, err
}

func writeView(c *gin.Context, a *app.App, u *model.User) {
	view, err := account.New(a).View(c.Request.Context(), u)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}

func getMe(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		writeView(c, a, u)
	}
}

func updateMe(a *app.App) gin.HandlerFunc {
	type req struct {
		RemindExpire  *bool `json:"remind_expire"`
		RemindTraffic *bool `json:"remind_traffic"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		id := auth.CurrentUser(c).ID
		changes := map[string]any{}
		if in.RemindExpire != nil {
			changes["remind_expire"] = *in.RemindExpire
		}
		if in.RemindTraffic != nil {
			changes["remind_traffic"] = *in.RemindTraffic
		}
		if len(changes) > 0 {
			if err := a.DB.WithContext(c.Request.Context()).Model(&model.User{}).Where("id = ?", id).
				Updates(changes).Error; err != nil {
				httpx.Fail(c, err)
				return
			}
			auth.InvalidateUser(a, id)
		}
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		writeView(c, a, u)
	}
}

func changePassword(a *app.App) gin.HandlerFunc {
	type req struct {
		OldPassword string `json:"old_password" binding:"required,max=256"`
		NewPassword string `json:"new_password" binding:"required,min=8,max=128"`
	}
	return func(c *gin.Context) {
		var in req
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		id := auth.CurrentUser(c).ID
		if _, err := account.New(a).ChangePassword(c.Request.Context(), id, in.OldPassword, in.NewPassword); err != nil {
			if e := httpx.AsError(err); e.Status < 500 {
				httpx.Logger(c).Warn("password change rejected", "user_id", id, "reason", e.Message)
			}
			account.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("password changed", "user_id", id)
		httpx.OK(c, true)
	}
}

func getStats(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		st, err := account.New(a).Stats(c.Request.Context(), auth.CurrentUser(c).ID)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, st)
	}
}

// Subscription is the response of GET /user/subscription.
type Subscription struct {
	SubscribeURL   string           `json:"subscribe_url"`
	Token          string           `json:"token"`
	Plan           *account.PlanRef `json:"plan"`
	TransferEnable int64            `json:"transfer_enable"`
	U              int64            `json:"u"`
	D              int64            `json:"d"`
	ExpiredAt      *time.Time       `json:"expired_at"`
	NextResetAt    *time.Time       `json:"next_reset_at"`
	ResetDay       *int             `json:"reset_day"`
	DeviceLimit    *int             `json:"device_limit"`
	SpeedLimit     *int             `json:"speed_limit"`
	// Profiles are the user's rule profiles with their subscription links (subscribe_url + &profile=<slug>;
	// the default profile's link is subscribe_url itself).
	Profiles []SubscriptionProfile `json:"profiles"`
}

func getSubscription(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		plan, err := account.LoadPlan(c.Request.Context(), a.DB, u.PlanID)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		users := usersvc.New(a)
		view := account.ToUserView(u, plan) // UTC timestamps
		subURL := users.SubscribeURL(u)
		profiles, err := subscriptionProfiles(c, a, u.ID, subURL)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, Subscription{
			Profiles:       profiles,
			SubscribeURL:   subURL,
			Token:          u.Token,
			Plan:           view.Plan,
			TransferEnable: u.TransferEnable,
			U:              u.U,
			D:              u.D,
			ExpiredAt:      view.ExpiredAt,
			NextResetAt:    view.NextResetAt,
			ResetDay:       users.ResetDay(u),
			DeviceLimit:    u.DeviceLimit,
			SpeedLimit:     u.SpeedLimit,
		})
	}
}

// SubscriptionReset is the response of POST /user/subscription/reset. AuthToken is a fresh session token:
// the reset bumps password_version, which ends every existing session including the caller's.
type SubscriptionReset struct {
	SubscribeURL string `json:"subscribe_url"`
	Token        string `json:"token"`
	AuthToken    string `json:"auth_token"`
}

func resetSubscription(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		u, err := freshUser(c, a)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		users := usersvc.New(a)
		if err := users.ResetSecurity(ctx, u); err != nil {
			httpx.Fail(c, err)
			return
		}
		auth.InvalidateUser(a, u.ID)
		users.InvalidateNodeUsers()
		// Reload so the session token carries the stored password_version even if it moved concurrently.
		if u, err = freshUser(c, a); err != nil {
			httpx.Fail(c, err)
			return
		}
		tok, err := auth.Issue(a, u)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("subscription credentials reset", "user_id", u.ID)
		httpx.OK(c, SubscriptionReset{SubscribeURL: users.SubscribeURL(u), Token: u.Token, AuthToken: tok})
	}
}

// TrafficLog is one row of GET /user/traffic-logs (per day and node; u/d already multiplied by rate).
type TrafficLog struct {
	RecordAt string  `json:"record_at"` // YYYY-MM-DD
	NodeID   uint64  `json:"node_id"`
	NodeName *string `json:"node_name"` // null when the node is unknown or deleted
	Rate     float64 `json:"rate"`
	U        int64   `json:"u"`
	D        int64   `json:"d"`
}

func trafficLogs(a *app.App) gin.HandlerFunc {
	type row struct {
		RecordAt time.Time
		NodeID   uint64
		NodeName *string
		Rate     float64
		U        int64
		D        int64
	}
	return func(c *gin.Context) {
		db := a.DB.WithContext(c.Request.Context())
		id := auth.CurrentUser(c).ID
		_, size, offset := httpx.ParsePage(c)
		var total int64
		if err := db.Model(&model.StatUser{}).Where("user_id = ?", id).Count(&total).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		out := []TrafficLog{}
		if total > int64(offset) {
			var rows []row
			err := db.Table("stat_users").
				Select("stat_users.record_at, stat_users.node_id, nodes.name AS node_name, stat_users.rate, stat_users.u, stat_users.d").
				Joins("LEFT JOIN nodes ON nodes.id = stat_users.node_id").
				Where("stat_users.user_id = ?", id).
				Order("stat_users.record_at DESC, stat_users.id DESC").
				Limit(size).Offset(offset).
				Scan(&rows).Error
			if err != nil {
				httpx.Fail(c, err)
				return
			}
			for _, r := range rows {
				out = append(out, TrafficLog{
					RecordAt: r.RecordAt.UTC().Format(time.DateOnly), NodeID: r.NodeID, NodeName: r.NodeName,
					Rate: r.Rate, U: r.U, D: r.D,
				})
			}
		}
		httpx.List(c, out, total)
	}
}

func logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		httpx.Logger(c).Info("user logged out", "user_id", auth.CurrentUser(c).ID)
		httpx.OK(c, true)
	}
}
