package admin

import (
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
)

// nodeOpt is a JSON field that remembers whether it was present (PATCH semantics); null sets the zero value.
type nodeOpt[T any] struct {
	Set bool
	V   T
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *nodeOpt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	return json.Unmarshal(b, &o.V)
}

// nodePortValue accepts a client port as a JSON string ("443", "20000-30000") or number (443).
type nodePortValue string

// UnmarshalJSON implements json.Unmarshaler.
func (p *nodePortValue) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*p = nodePortValue(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*p = nodePortValue(n.String())
	return nil
}

// nodeInput is the body of POST /admin/nodes and PATCH /admin/nodes/:id (absent fields are left unchanged).
type nodeInput struct {
	Type             nodeOpt[string]        `json:"type"`
	Name             nodeOpt[string]        `json:"name"`
	GroupIDs         nodeOpt[[]uint64]      `json:"group_ids"`
	RouteIDs         nodeOpt[[]uint64]      `json:"route_ids"`
	ParentID         nodeOpt[*uint64]       `json:"parent_id"`
	Host             nodeOpt[string]        `json:"host"`
	Port             nodeOpt[nodePortValue] `json:"port"`
	ServerPort       nodeOpt[int]           `json:"server_port"`
	Tags             nodeOpt[[]string]      `json:"tags"`
	Rate             nodeOpt[float64]       `json:"rate"`
	Show             nodeOpt[bool]          `json:"show"`
	Sort             nodeOpt[int]           `json:"sort"`
	ProtocolSettings nodeOpt[model.JSONMap] `json:"protocol_settings"`
}

func (in *nodeInput) apply(n *model.Node) {
	if in.Type.Set {
		n.Type = in.Type.V
	}
	if in.Name.Set {
		n.Name = in.Name.V
	}
	if in.GroupIDs.Set {
		n.GroupIDs = in.GroupIDs.V
	}
	if in.RouteIDs.Set {
		n.RouteIDs = in.RouteIDs.V
	}
	if in.ParentID.Set {
		n.ParentID = in.ParentID.V
		n.Parent = nil
	}
	if in.Host.Set {
		n.Host = in.Host.V
	}
	if in.Port.Set {
		n.Port = string(in.Port.V)
	}
	if in.ServerPort.Set {
		n.ServerPort = in.ServerPort.V
	}
	if in.Tags.Set {
		n.Tags = in.Tags.V
	}
	if in.Rate.Set {
		n.Rate = in.Rate.V
	}
	if in.Show.Set {
		n.Show = in.Show.V
	}
	if in.Sort.Set {
		n.Sort = in.Sort.V
	}
	if in.ProtocolSettings.Set {
		n.ProtocolSettings = in.ProtocolSettings.V
	}
}

type adminNodeRef struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// adminNodeView is the admin Node shape: the node, its parent reference and the runtime status of P.
type adminNodeView struct {
	*model.Node
	Parent *adminNodeRef      `json:"parent"`
	Status nodesvc.StatusView `json:"status"`
}

func adminNodeViewOf(svc *nodesvc.Service, n *model.Node, names map[uint64]string) adminNodeView {
	v := adminNodeView{Node: n, Status: svc.Status(nodesvc.EffectiveID(n))}
	if n.ParentID != nil {
		v.Parent = &adminNodeRef{ID: *n.ParentID, Name: names[*n.ParentID]}
	}
	return v
}

// loadAdminNodeView reloads node id and renders it (with its parent's name).
func loadAdminNodeView(db *gorm.DB, svc *nodesvc.Service, id uint64) (adminNodeView, error) {
	n := new(model.Node)
	if err := db.Preload("Parent").Where("id = ?", id).First(n).Error; err != nil {
		return adminNodeView{}, err
	}
	names := map[uint64]string{}
	if n.Parent != nil {
		names[n.Parent.ID] = n.Parent.Name
	}
	return adminNodeViewOf(svc, n, names), nil
}

type adminNodeStat struct {
	Date string `json:"date"`
	U    int64  `json:"u"`
	D    int64  `json:"d"`
}

// registerNodes mounts /admin/nodes* (admin only).
// Owner: BE-B nodes. Implement in this file; do not add routes elsewhere.
func registerNodes(g *gin.RouterGroup, a *app.App) {
	svc := nodesvc.New(a)
	gr := g.Group("/nodes", auth.RequireAdmin(a))

	gr.GET("", func(c *gin.Context) {
		var nodes []*model.Node
		err := a.DB.WithContext(c.Request.Context()).
			Order(clause.OrderByColumn{Column: clause.Column{Name: "sort"}}).Order("id").
			Find(&nodes).Error
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		names := make(map[uint64]string, len(nodes))
		for _, n := range nodes {
			names[n.ID] = n.Name
		}
		views := make([]adminNodeView, len(nodes))
		for i, n := range nodes {
			views[i] = adminNodeViewOf(svc, n, names)
		}
		httpx.List(c, views, int64(len(views)))
	})

	gr.POST("", func(c *gin.Context) {
		var in nodeInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		n := &model.Node{Rate: 1}
		in.apply(n)
		ctx := c.Request.Context()
		if err := svc.CheckNode(ctx, n); err != nil {
			httpx.Fail(c, err)
			return
		}
		db := a.DB.WithContext(ctx)
		if err := db.Omit(clause.Associations).Create(n).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		svc.Invalidate()
		v, err := loadAdminNodeView(db, svc, n.ID)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Created(c, v)
	})

	gr.POST("/sort", func(c *gin.Context) {
		var in struct {
			IDs []uint64 `json:"ids" binding:"required,min=1,max=10000"`
		}
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			for i, id := range in.IDs {
				if err := tx.Model(&model.Node{}).Where("id = ?", id).Update("sort", i).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		svc.Invalidate()
		httpx.OK(c, true)
	})

	gr.GET("/:id", func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		v, err := loadAdminNodeView(a.DB.WithContext(c.Request.Context()), svc, id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, v)
	})

	gr.PATCH("/:id", func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var in nodeInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		ctx := c.Request.Context()
		db := a.DB.WithContext(ctx)
		n := new(model.Node)
		if err := db.Where("id = ?", id).First(n).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		in.apply(n)
		if err := svc.CheckNode(ctx, n); err != nil {
			httpx.Fail(c, err)
			return
		}
		if err := db.Omit(clause.Associations).Save(n).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		svc.Invalidate()
		v, err := loadAdminNodeView(db, svc, id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, v)
	})

	gr.DELETE("/:id", func(c *gin.Context) {
		n, ok := adminNodeLoad(c, a)
		if !ok {
			return
		}
		id, db := n.ID, a.DB.WithContext(c.Request.Context())
		var children int64
		if err := db.Model(&model.Node{}).Where("parent_id = ?", id).Count(&children).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		if children > 0 {
			httpx.Fail(c, httpx.Conflict(fmt.Sprintf("node has %d child nodes; delete or reassign them first", children)))
			return
		}
		// stat_nodes / stat_users rows are kept for history.
		if err := db.Delete(&model.Node{}, id).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		a.Online.Forget(id)
		svc.Invalidate()
		httpx.OK(c, true)
	})

	gr.POST("/:id/copy", func(c *gin.Context) {
		src, ok := adminNodeLoad(c, a)
		if !ok {
			return
		}
		db := a.DB.WithContext(c.Request.Context())
		cp := *src
		cp.ID, cp.Parent, cp.Show = 0, nil, false
		cp.CreatedAt, cp.UpdatedAt = time.Time{}, time.Time{}
		cp.Name = copyName(src.Name)
		nodesvc.NormalizeNode(&cp)
		if err := db.Omit(clause.Associations).Create(&cp).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		svc.Invalidate()
		v, err := loadAdminNodeView(db, svc, cp.ID)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Created(c, v)
	})

	gr.GET("/:id/stats", func(c *gin.Context) {
		n, ok := adminNodeLoad(c, a)
		if !ok {
			return
		}
		days := 0
		switch c.DefaultQuery("range", "7d") {
		case "7d":
			days = 7
		case "30d":
			days = 30
		default:
			httpx.Fail(c, httpx.FieldError("range", "must be 7d or 30d"))
			return
		}
		today := model.Today()
		from := today.AddDate(0, 0, -(days - 1))
		var rows []model.StatNode
		err := a.DB.WithContext(c.Request.Context()).Where("node_id = ? AND record_at >= ?", nodesvc.EffectiveID(n), from).Find(&rows).Error
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		byDay := make(map[string]model.StatNode, len(rows))
		for _, r := range rows {
			k := r.RecordAt.UTC().Format(time.DateOnly)
			cur := byDay[k]
			cur.U += r.U
			cur.D += r.D
			byDay[k] = cur
		}
		out := make([]adminNodeStat, days)
		for i := range days {
			k := from.AddDate(0, 0, i).Format(time.DateOnly)
			out[i] = adminNodeStat{Date: k, U: byDay[k].U, D: byDay[k].D}
		}
		httpx.OK(c, out)
	})

	gr.GET("/:id/online", func(c *gin.Context) {
		n, ok := adminNodeLoad(c, a)
		if !ok {
			return
		}
		users, err := svc.OnlineUsers(c.Request.Context(), nodesvc.EffectiveID(n))
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, users)
	})

	gr.GET("/:id/audit", func(c *gin.Context) {
		n, ok := adminNodeLoad(c, a)
		if !ok {
			return
		}
		httpx.OK(c, svc.AuditHits(nodesvc.EffectiveID(n)))
	})
}

// adminNodeLoad loads the node of the :id parameter or fails the request.
func adminNodeLoad(c *gin.Context, a *app.App) (*model.Node, bool) {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	n := new(model.Node)
	if err := a.DB.WithContext(c.Request.Context()).Where("id = ?", id).First(n).Error; err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	return n, true
}

// copyName appends " (copy)", trimming the original so the result fits 191 characters.
func copyName(name string) string {
	const suffix = " (copy)"
	for utf8.RuneCountInString(name)+len(suffix) > 191 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name + suffix
}
