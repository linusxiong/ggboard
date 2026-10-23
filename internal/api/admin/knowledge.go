package admin

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	contentsvc "github.com/linusxiong/ggboard/internal/service/content"
)

// registerKnowledge mounts /admin/knowledge* (staff).
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerKnowledge(g *gin.RouterGroup, a *app.App) {
	g.GET("/knowledge", knowledgeList(a))
	g.POST("/knowledge", knowledgeCreate(a))
	g.GET("/knowledge/:id", knowledgeShow(a))
	g.PATCH("/knowledge/:id", knowledgeUpdate(a))
	g.DELETE("/knowledge/:id", knowledgeDelete(a))
	g.POST("/knowledge/sort", knowledgeSort(a))
}

// knowledgeItem is an admin list row (no body).
type knowledgeItem struct {
	ID        uint64    `json:"id"`
	Language  string    `json:"language"`
	Category  string    `json:"category"`
	Title     string    `json:"title"`
	Show      bool      `json:"show"`
	Sort      int       `json:"sort"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// knowledgeList: GET /admin/knowledge?language&category&keyword → every matching article (hidden included, no
// body) in display order; unpaginated so the whole list can be reordered.
func knowledgeList(a *app.App) gin.HandlerFunc {
	type query struct {
		Language string `form:"language" binding:"max=10"`
		Category string `form:"category" binding:"max=191"`
		Keyword  string `form:"keyword" binding:"max=255"`
	}
	return func(c *gin.Context) {
		var in query
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		f := contentsvc.KnowledgeFilter{Language: in.Language, Category: strings.TrimSpace(in.Category), Keyword: in.Keyword}
		var rows []knowledgeItem
		if err := contentsvc.New(a).KnowledgeQuery(c.Request.Context(), f).Order(contentsvc.KnowledgeOrder).Find(&rows).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, rows, int64(len(rows)))
	}
}

// knowledgeInput is the body of POST (required fields enforced by the handler) and PATCH /admin/knowledge.
type knowledgeInput struct {
	Language *string `json:"language" binding:"omitnil,max=5"`
	Category *string `json:"category" binding:"omitnil,max=191"`
	Title    *string `json:"title" binding:"omitnil,max=255"`
	Body     *string `json:"body"`
	Show     *bool   `json:"show"`
	Sort     *int    `json:"sort" binding:"omitnil,gte=-1000000,lte=1000000"`
}

func (in *knowledgeInput) updates(create bool) (map[string]any, error) {
	m := map[string]any{}
	e := httpx.BadRequest("validation failed")
	for _, f := range []struct {
		name string
		p    *string
	}{{"language", in.Language}, {"category", in.Category}, {"title", in.Title}} {
		if v, ok, err := contentRequiredText(f.p, create); err != nil {
			e = e.WithField(f.name, err.Error())
		} else if ok {
			m[f.name] = v
		}
	}
	// The body keeps its whitespace (markdown) but must not be blank.
	if in.Body != nil && len(*in.Body) > contentMaxBytes {
		e = e.WithField("body", errContentTooLong.Error())
	} else if in.Body != nil && strings.TrimSpace(*in.Body) != "" {
		m["body"] = *in.Body
	} else if in.Body != nil || create {
		e = e.WithField("body", errContentRequired.Error())
	}
	if in.Show != nil {
		m["show"] = *in.Show
	} else if create {
		m["show"] = true
	}
	if in.Sort != nil {
		m["sort"] = *in.Sort
	}
	if len(e.Errors) > 0 {
		return nil, e
	}
	return m, nil
}

// knowledgePlaceholderForbidden is the 403 for a staff body with subscription placeholders. They expand to every
// reader's own subscription URL, whose token staff must not see, and a staff-written body could send it to
// another host (a remote image or link), so only administrators may write them. The body field error lets the
// editor show it inline. attrs are logged with the refusal.
func knowledgePlaceholderForbidden(c *gin.Context, attrs ...any) *httpx.Error {
	httpx.Logger(c).Warn("admin: staff knowledge body with subscription placeholders refused",
		append(attrs, "staff_id", auth.CurrentUser(c).ID)...)
	msg := "only administrators can use the subscription placeholders " + strings.Join(contentsvc.SubscribePlaceholders(), ", ")
	return httpx.Forbidden(msg).WithField("body", msg)
}

// knowledgeCreate: POST /admin/knowledge {language*, category*, title*, body*, show? (default true), sort?} → 201 Knowledge.
// Staff: 403 when the body contains a subscription placeholder.
func knowledgeCreate(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in knowledgeInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		m, err := in.updates(true)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if body, _ := m["body"].(string); !auth.CurrentUser(c).IsAdmin && contentsvc.HasSubscribePlaceholder(body) {
			httpx.Fail(c, knowledgePlaceholderForbidden(c))
			return
		}
		var k model.Knowledge
		k.Language, _ = m["language"].(string)
		k.Category, _ = m["category"].(string)
		k.Title, _ = m["title"].(string)
		k.Body, _ = m["body"].(string)
		k.Show, _ = m["show"].(bool)
		k.Sort, _ = m["sort"].(int)
		if err := a.DB.WithContext(c.Request.Context()).Create(&k).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("admin: knowledge created", "knowledge_id", k.ID, "staff_id", auth.CurrentUser(c).ID)
		httpx.Created(c, k)
	}
}

// knowledgeShow: GET /admin/knowledge/:id → Knowledge with the raw body.
func knowledgeShow(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var k model.Knowledge
		if err := a.DB.WithContext(c.Request.Context()).Where("id = ?", id).First(&k).Error; err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.OK(c, k)
	}
}

// knowledgeUpdate: PATCH /admin/knowledge/:id {any field of knowledgeInput} → Knowledge. Staff: 403 when the
// new body contains a subscription placeholder; sending the stored body back unchanged is allowed (and leaves
// it alone), so staff can still edit the other fields of an administrator's article in the full-form editor.
func knowledgeUpdate(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var in knowledgeInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		m, err := in.updates(false)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if body, ok := m["body"].(string); ok && !auth.CurrentUser(c).IsAdmin && contentsvc.HasSubscribePlaceholder(body) {
			var cur model.Knowledge
			if err := a.DB.WithContext(c.Request.Context()).Select("id", "body").Where("id = ?", id).First(&cur).Error; err != nil {
				httpx.Fail(c, err) // 404 when missing
				return
			}
			if cur.Body != body {
				httpx.Fail(c, knowledgePlaceholderForbidden(c, "knowledge_id", id))
				return
			}
			delete(m, "body") // unchanged: never written by staff, even if an administrator changes it meanwhile
		}
		k, err := contentUpdate[model.Knowledge](c, a, id, m)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("admin: knowledge updated", "knowledge_id", id, "staff_id", auth.CurrentUser(c).ID)
		httpx.OK(c, k)
	}
}

// knowledgeDelete: DELETE /admin/knowledge/:id → true.
func knowledgeDelete(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !contentDelete[model.Knowledge](c, a, "knowledge") {
			return
		}
		httpx.OK(c, true)
	}
}

// knowledgeSort: POST /admin/knowledge/sort {ids} → true (sort = position, 1-based).
func knowledgeSort(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		ids, ok := contentSortIDs(c)
		if !ok {
			return
		}
		if err := contentsvc.New(a).SortKnowledge(c.Request.Context(), ids); err != nil {
			httpx.Fail(c, contentSortError(err))
			return
		}
		httpx.OK(c, true)
	}
}
