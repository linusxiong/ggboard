package admin

import (
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/linusxiong/ggboard/internal/subscription"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	nodesvc "github.com/linusxiong/ggboard/internal/service/node"
	"github.com/linusxiong/ggboard/internal/settings"
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
	Token            nodeOpt[string]        `json:"token"`
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
	if in.Token.Set {
		n.Token = in.Token.V
	}
}

type adminNodeRef struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// adminNodeView is the admin Node shape: the node, its own backend token ("" = server_token), its parent reference
// and the runtime status of P.
type adminNodeView struct {
	*model.Node
	Token  string             `json:"token"`
	Parent *adminNodeRef      `json:"parent"`
	Status nodesvc.StatusView `json:"status"`
}

func adminNodeViewOf(svc *nodesvc.Service, n *model.Node, names map[uint64]string) adminNodeView {
	v := adminNodeView{Node: n, Token: n.Token, Status: svc.Status(nodesvc.EffectiveID(n))}
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
		err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			return tx.Order(clause.OrderByColumn{Column: clause.Column{Name: "sort"}}).Order("id").Find(&nodes).Error
		})
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
		var v adminNodeView
		if err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			if err := tx.Omit(clause.Associations).Create(n).Error; err != nil {
				return err
			}
			var err error
			v, err = loadAdminNodeView(tx, svc, n.ID)
			return err
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		svc.Invalidate()
		subscription.InvalidateNodes(a)
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
		err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
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
		subscription.InvalidateNodes(a)
		httpx.OK(c, true)
	})

	gr.GET("/:id", func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var v adminNodeView
		err = withLiveAdmin(c, a, func(tx *gorm.DB) error {
			var loadErr error
			v, loadErr = loadAdminNodeView(tx, svc, id)
			return loadErr
		})
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
		n := new(model.Node)
		var v adminNodeView
		if err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			if err := tx.Where("id = ?", id).First(n).Error; err != nil {
				return err
			}
			in.apply(n)
			if err := svc.CheckNode(ctx, n); err != nil {
				return err
			}
			if err := tx.Omit(clause.Associations).Save(n).Error; err != nil {
				return err
			}
			var err error
			v, err = loadAdminNodeView(tx, svc, id)
			return err
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		svc.Invalidate()
		subscription.InvalidateNodes(a)
		httpx.OK(c, v)
	})

	gr.DELETE("/:id", func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		err = withLiveAdmin(c, a, func(tx *gorm.DB) error {
			var n model.Node
			if err := tx.First(&n, id).Error; err != nil {
				return err
			}
			var children int64
			if err := tx.Model(&model.Node{}).Where("parent_id = ?", id).Count(&children).Error; err != nil {
				return err
			}
			if children > 0 {
				return httpx.Conflict(fmt.Sprintf("node has %d child nodes; delete or reassign them first", children))
			}
			// stat_nodes / stat_users rows are kept for history.
			return tx.Delete(&model.Node{}, id).Error
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		a.Online.Forget(id)
		svc.Invalidate()
		subscription.InvalidateNodes(a)
		httpx.OK(c, true)
	})

	gr.POST("/:id/copy", func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var v adminNodeView
		if err := withLiveAdmin(c, a, func(tx *gorm.DB) error {
			var src model.Node
			if err := tx.First(&src, id).Error; err != nil {
				return err
			}
			cp := src
			cp.ID, cp.Parent, cp.Show = 0, nil, false
			cp.CreatedAt, cp.UpdatedAt = time.Time{}, time.Time{}
			cp.Name = copyName(src.Name)
			if cp.Token != "" {
				// A copy is a new node: sharing the key would let either node's operator act as the other.
				cp.Token = settings.RandomToken(32)
			}
			nodesvc.NormalizeNode(&cp)
			if err := tx.Omit(clause.Associations).Create(&cp).Error; err != nil {
				return err
			}
			var err error
			v, err = loadAdminNodeView(tx, svc, cp.ID)
			return err
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		svc.Invalidate()
		subscription.InvalidateNodes(a)
		httpx.Created(c, v)
	})

	gr.GET("/:id/stats", func(c *gin.Context) {
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
		err := adminNodeSnapshot(c, a, func(tx *gorm.DB, n *model.Node) error {
			return tx.Where("node_id = ? AND record_at >= ?", nodesvc.EffectiveID(n), from).Find(&rows).Error
		})
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
		var users []nodesvc.OnlineUser
		err := adminNodeSnapshot(c, a, func(tx *gorm.DB, n *model.Node) error {
			var loadErr error
			users, loadErr = svc.OnlineUsersWithDB(c.Request.Context(), tx, nodesvc.EffectiveID(n))
			return loadErr
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, users)
	})

	gr.GET("/:id/audit", func(c *gin.Context) {
		var hits []nodesvc.AuditHit
		if err := adminNodeSnapshot(c, a, func(_ *gorm.DB, n *model.Node) error {
			hits = svc.AuditHits(nodesvc.EffectiveID(n))
			return nil
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, hits)
	})
}

// adminNodeSnapshot keeps the node and its sensitive data under one live-admin lock.
func adminNodeSnapshot(c *gin.Context, a *app.App, run func(*gorm.DB, *model.Node) error) error {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		return err
	}
	return withLiveAdmin(c, a, func(tx *gorm.DB) error {
		n := new(model.Node)
		if err := tx.Where("id = ?", id).First(n).Error; err != nil {
			return err
		}
		return run(tx, n)
	})
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
