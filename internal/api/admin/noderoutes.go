package admin

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
)

// nodeRouteInput is the body of POST /admin/routes and PATCH /admin/routes/:id (absent fields unchanged).
type nodeRouteInput struct {
	Remarks     nodeOpt[string]   `json:"remarks"`
	Match       nodeOpt[[]string] `json:"match"`
	Action      nodeOpt[string]   `json:"action"`
	ActionValue nodeOpt[string]   `json:"action_value"`
}

func (in *nodeRouteInput) apply(r *model.NodeRoute) {
	if in.Remarks.Set {
		r.Remarks = in.Remarks.V
	}
	if in.Match.Set {
		r.Match = in.Match.V
	}
	if in.Action.Set {
		r.Action = in.Action.V
	}
	if in.ActionValue.Set {
		r.ActionValue = in.ActionValue.V
	}
}

// registerRoutes mounts /admin/routes* (admin only).
// Owner: BE-B nodes. Implement in this file; do not add routes elsewhere.
func registerRoutes(g *gin.RouterGroup, a *app.App) {
	svc := nodesvc.New(a)
	gr := g.Group("/routes", auth.RequireAdmin(a))

	gr.GET("", func(c *gin.Context) {
		var routes []model.NodeRoute
		if err := a.DB.WithContext(c.Request.Context()).Order("id").Find(&routes).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, routes, int64(len(routes)))
	})

	gr.POST("", func(c *gin.Context) {
		var in nodeRouteInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		var r model.NodeRoute
		in.apply(&r)
		nodesvc.NormalizeRoute(&r)
		if err := nodesvc.ValidateRoute(&r); err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := a.DB.WithContext(c.Request.Context()).Create(&r).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		svc.Invalidate()
		httpx.Created(c, r)
	})

	gr.PATCH("/:id", func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var in nodeRouteInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		db := a.DB.WithContext(c.Request.Context())
		var r model.NodeRoute
		if err := db.Where("id = ?", id).First(&r).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		in.apply(&r)
		nodesvc.NormalizeRoute(&r)
		if err := nodesvc.ValidateRoute(&r); err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := db.Save(&r).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		svc.Invalidate()
		httpx.OK(c, r)
	})

	gr.DELETE("/:id", func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		db := a.DB.WithContext(c.Request.Context())
		var r model.NodeRoute
		if err := db.Where("id = ?", id).First(&r).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		var nodes []model.Node
		if err := db.Select("id", "route_ids").Find(&nodes).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		used := 0
		for _, n := range nodes {
			if model.SliceContains(n.RouteIDs, id) {
				used++
			}
		}
		if used > 0 {
			httpx.Fail(c, httpx.Conflict(fmt.Sprintf("route is still used by %d nodes", used)))
			return
		}
		if err := db.Delete(&model.NodeRoute{}, id).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		svc.Invalidate()
		httpx.OK(c, true)
	})
}
