package user

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
)

// userNodeView is one entry of GET /user/nodes. Host and port are present only when show_info_to_user is on.
type userNodeView struct {
	ID       uint64   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Tags     []string `json:"tags"`
	Rate     float64  `json:"rate"`
	Host     *string  `json:"host,omitempty"`
	Port     *string  `json:"port,omitempty"`
	IsOnline bool     `json:"is_online"`
	Sort     int      `json:"sort"`
}

// registerNodes mounts GET /user/nodes.
// Owner: BE-B nodes. Implement in this file; do not add routes elsewhere.
func registerNodes(g *gin.RouterGroup, a *app.App) {
	svc := nodesvc.New(a)
	g.GET("/nodes", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		out := []userNodeView{}
		// Like Xboard, a user whose subscription is unusable (banned, expired, out of traffic) sees no nodes.
		if !u.IsAvailable(time.Now()) {
			httpx.OK(c, out)
			return
		}
		nodes, err := svc.AvailableForUser(c.Request.Context(), u)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		showInfo := a.Settings.Bool("show_info_to_user")
		for _, n := range nodes {
			tags := []string(n.Tags)
			if tags == nil {
				tags = []string{}
			}
			v := userNodeView{ID: n.ID, Name: n.Name, Type: n.Type, Tags: tags, Rate: n.Rate, IsOnline: n.IsOnline, Sort: n.Sort}
			if showInfo {
				host, port := n.Host, n.Port
				v.Host, v.Port = &host, &port
			}
			out = append(out, v)
		}
		httpx.OK(c, out)
	})
}
