package admin

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
)

// serverGroupView is a server group with its usage counts.
type serverGroupView struct {
	model.ServerGroup
	UserCount int64 `json:"user_count"`
	PlanCount int64 `json:"plan_count"`
	NodeCount int64 `json:"node_count"`
}

type serverGroupInput struct {
	Name string `json:"name" binding:"required,max=191"`
}

// registerGroups mounts /admin/groups* (admin only).
// Owner: BE-B nodes. Implement in this file; do not add routes elsewhere.
func registerGroups(g *gin.RouterGroup, a *app.App) {
	gr := g.Group("/groups", auth.RequireAdmin(a))
	gr.GET("", func(c *gin.Context) {
		views, err := serverGroupViews(a.DB.WithContext(c.Request.Context()), 0)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, views, int64(len(views)))
	})
	gr.POST("", func(c *gin.Context) {
		var in serverGroupInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		name, err := serverGroupName(in.Name)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		grp := model.ServerGroup{Name: name}
		if err := a.DB.WithContext(c.Request.Context()).Create(&grp).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Created(c, serverGroupView{ServerGroup: grp})
	})
	gr.PATCH("/:id", func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var in serverGroupInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		name, err := serverGroupName(in.Name)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		db := a.DB.WithContext(c.Request.Context())
		var grp model.ServerGroup
		if err := db.Where("id = ?", id).First(&grp).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		grp.Name = name
		if err := db.Save(&grp).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		views, err := serverGroupViews(db, id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, views[0])
	})
	gr.DELETE("/:id", func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		db := a.DB.WithContext(c.Request.Context())
		views, err := serverGroupViews(db, id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		v := views[0]
		if v.UserCount+v.PlanCount+v.NodeCount > 0 {
			httpx.Fail(c, httpx.Conflict(fmt.Sprintf("group is still used by %d users, %d plans and %d nodes", v.UserCount, v.PlanCount, v.NodeCount)))
			return
		}
		if err := db.Delete(&model.ServerGroup{}, id).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		nodesvc.New(a).Invalidate()
		httpx.OK(c, true)
	})
}

func serverGroupName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", httpx.FieldError("name", "is required")
	}
	if utf8.RuneCountInString(name) > 191 {
		return "", httpx.FieldError("name", "must be at most 191 characters")
	}
	return name, nil
}

type serverGroupCount struct {
	GroupID uint64
	N       int64
}

// serverGroupViews loads groups (all, or only id when id != 0; 404 when missing) with user/plan/node counts in
// four queries (no N+1). Node group_ids is a JSON column, so nodes are counted in Go.
func serverGroupViews(db *gorm.DB, id uint64) ([]serverGroupView, error) {
	q := db.Order("id")
	if id != 0 {
		q = q.Where("id = ?", id)
	}
	var groups []model.ServerGroup
	if err := q.Find(&groups).Error; err != nil {
		return nil, err
	}
	if id != 0 && len(groups) == 0 {
		return nil, httpx.NotFound("group not found")
	}
	count := func(table any, extra func(*gorm.DB) *gorm.DB) (map[uint64]int64, error) {
		var rows []serverGroupCount
		q := db.Model(table).Select("group_id, COUNT(*) AS n").Where("group_id IS NOT NULL")
		if id != 0 {
			q = q.Where("group_id = ?", id)
		}
		if extra != nil {
			q = extra(q)
		}
		if err := q.Group("group_id").Scan(&rows).Error; err != nil {
			return nil, err
		}
		out := make(map[uint64]int64, len(rows))
		for _, r := range rows {
			out[r.GroupID] = r.N
		}
		return out, nil
	}
	users, err := count(&model.User{}, nil)
	if err != nil {
		return nil, err
	}
	plans, err := count(&model.Plan{}, nil)
	if err != nil {
		return nil, err
	}
	var nodes []model.Node
	if err := db.Select("id", "group_ids").Find(&nodes).Error; err != nil {
		return nil, err
	}
	nodeCounts := map[uint64]int64{}
	for _, n := range nodes {
		for _, gid := range n.GroupIDs {
			nodeCounts[gid]++
		}
	}
	out := make([]serverGroupView, len(groups))
	for i, g := range groups {
		out[i] = serverGroupView{ServerGroup: g, UserCount: users[g.ID], PlanCount: plans[g.ID], NodeCount: nodeCounts[g.ID]}
	}
	return out, nil
}
