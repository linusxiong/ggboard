package user

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	contentsvc "github.com/linusxiong/ggboard/internal/service/content"
)

// registerContent mounts GET /user/notices, GET /user/knowledge, GET /user/knowledge/:id.
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerContent(g *gin.RouterGroup, a *app.App) {
	g.GET("/notices", contentNotices(a))
	g.GET("/knowledge", contentKnowledgeIndex(a))
	g.GET("/knowledge/:id", contentKnowledgeArticle(a))
}

// contentNotice is a notice as shown to users.
type contentNotice struct {
	ID        uint64                  `json:"id"`
	Title     string                  `json:"title"`
	Content   string                  `json:"content"`
	ImgURL    string                  `json:"img_url"`
	Tags      model.JSONSlice[string] `json:"tags"`
	CreatedAt time.Time               `json:"created_at"`
}

// contentNotices: GET /user/notices?page&page_size — visible notices by sort, newest first.
func contentNotices(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := contentsvc.New(a).NoticeQuery(c.Request.Context(), true)
		rows, total, err := httpx.Paginate[model.Notice](c, q, nil, contentsvc.NoticeOrder)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out := make([]contentNotice, 0, len(rows))
		for _, n := range rows {
			out = append(out, contentNotice{ID: n.ID, Title: n.Title, Content: n.Content, ImgURL: n.ImgURL, Tags: n.Tags, CreatedAt: n.CreatedAt})
		}
		httpx.List(c, out, total)
	}
}

// contentKnowledgeIndex: GET /user/knowledge?language&keyword → {"<category>": [{id, title, updated_at}]}.
func contentKnowledgeIndex(a *app.App) gin.HandlerFunc {
	type query struct {
		Language string `form:"language" binding:"max=10"`
		Keyword  string `form:"keyword" binding:"max=255"`
	}
	return func(c *gin.Context) {
		var in query
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		u := auth.CurrentUser(c)
		hasPlan := u.PlanID != nil && u.IsAvailable(time.Now())
		idx, err := contentsvc.New(a).KnowledgeIndex(c.Request.Context(), in.Language, in.Keyword, hasPlan)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, idx)
	}
}

// contentKnowledgeArticle: GET /user/knowledge/:id → {id, title, body, category, language, updated_at} with
// placeholders replaced and plan-gated blocks hidden from users without a valid plan.
func contentKnowledgeArticle(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		art, err := contentsvc.New(a).Article(c.Request.Context(), id, auth.CurrentUser(c), time.Now())
		if errors.Is(err, contentsvc.ErrNotFound) {
			httpx.Fail(c, httpx.NotFound("article not found"))
			return
		}
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, art)
	}
}
