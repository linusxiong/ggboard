package admin

import (
	"errors"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	contentsvc "github.com/linusxiong/ggboard/internal/service/content"
)

// registerNotices mounts /admin/notices* (staff).
// Owner: BE-E2 content. Implement in this file; do not add routes elsewhere.
func registerNotices(g *gin.RouterGroup, a *app.App) {
	g.GET("/notices", noticeList(a))
	g.POST("/notices", noticeCreate(a))
	g.PATCH("/notices/:id", noticeUpdate(a))
	g.DELETE("/notices/:id", noticeDelete(a))
	g.POST("/notices/sort", noticeSort(a))
}

// noticeList: GET /admin/notices → every notice (hidden included) in display order; unpaginated so the
// whole list can be reordered.
func noticeList(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var rows []model.Notice
		if err := withLiveStaffTx(c, a, func(tx *gorm.DB, _ bool) error {
			return contentsvc.New(a).NoticeQueryWithDB(tx, false).Order(contentsvc.NoticeOrder).Find(&rows).Error
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.List(c, rows, int64(len(rows)))
	}
}

// noticeInput is the body of POST (required fields enforced by the handler) and PATCH /admin/notices.
type noticeInput struct {
	Title   *string   `json:"title" binding:"omitnil,max=255"`
	Content *string   `json:"content"`
	ImgURL  *string   `json:"img_url" binding:"omitnil,max=255"`
	Tags    *[]string `json:"tags" binding:"omitnil,max=20,dive,max=32"`
	Show    *bool     `json:"show"`
	Sort    *int      `json:"sort" binding:"omitnil,gte=-1000000,lte=1000000"`
}

// updates validates the input and returns the columns to write. create requires title and content.
func (in *noticeInput) updates(create bool) (map[string]any, error) {
	m := map[string]any{}
	e := httpx.BadRequest("validation failed")
	if v, ok, err := contentRequiredText(in.Title, create); err != nil {
		e = e.WithField("title", err.Error())
	} else if ok {
		m["title"] = v
	}
	if v, ok, err := contentRequiredText(in.Content, create); err != nil {
		e = e.WithField("content", err.Error())
	} else if len(v) > contentMaxBytes {
		e = e.WithField("content", errContentTooLong.Error())
	} else if ok {
		m["content"] = v
	}
	if in.ImgURL != nil {
		v := strings.TrimSpace(*in.ImgURL)
		if v != "" && !contentHTTPURL(v) {
			e = e.WithField("img_url", "must be a valid http(s) URL")
		}
		m["img_url"] = v
	}
	if in.Tags != nil {
		tags := make(model.JSONSlice[string], 0, len(*in.Tags))
		for _, t := range *in.Tags {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		m["tags"] = tags
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

// noticeCreate: POST /admin/notices {title*, content*, img_url?, tags?, show? (default true), sort?} → 201 Notice.
func noticeCreate(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in noticeInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		m, err := in.updates(true)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		n := model.Notice{Tags: model.JSONSlice[string]{}}
		n.Title, _ = m["title"].(string)
		n.Content, _ = m["content"].(string)
		n.ImgURL, _ = m["img_url"].(string)
		if tags, ok := m["tags"].(model.JSONSlice[string]); ok {
			n.Tags = tags
		}
		n.Show, _ = m["show"].(bool)
		n.Sort, _ = m["sort"].(int)
		if err := withLiveStaffTx(c, a, func(tx *gorm.DB, _ bool) error {
			return tx.Create(&n).Error
		}); err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("admin: notice created", "notice_id", n.ID, "staff_id", auth.CurrentUser(c).ID)
		httpx.Created(c, n)
	}
}

// noticeUpdate: PATCH /admin/notices/:id {any field of noticeInput} → Notice.
func noticeUpdate(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := httpx.ParamID(c, "id")
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		var in noticeInput
		if err := httpx.Bind(c, &in); err != nil {
			httpx.Fail(c, err)
			return
		}
		m, err := in.updates(false)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		n, err := contentUpdate[model.Notice](c, a, id, m)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		httpx.Logger(c).Info("admin: notice updated", "notice_id", id, "staff_id", auth.CurrentUser(c).ID)
		httpx.OK(c, n)
	}
}

// noticeDelete: DELETE /admin/notices/:id → true.
func noticeDelete(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !contentDelete[model.Notice](c, a, "notice") {
			return
		}
		httpx.OK(c, true)
	}
}

// noticeSort: POST /admin/notices/sort {ids} → true (sort = position, 1-based).
func noticeSort(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		ids, ok := contentSortIDs(c)
		if !ok {
			return
		}
		if err := contentsvc.New(a).SortNoticesIf(c.Request.Context(), ids, func(tx *gorm.DB) error {
			_, err := liveStaffRole(c, tx, true)
			return err
		}); err != nil {
			httpx.Fail(c, contentSortError(err))
			return
		}
		httpx.OK(c, true)
	}
}

// ---------------------------------------------------------------- helpers shared with knowledge.go

var errContentRequired = errors.New("is required")

// contentRequiredText trims *p. It reports whether the field was given; an empty value is an error when the
// field is given or required.
// contentMaxBytes caps notice contents and knowledge bodies at what a MySQL TEXT column holds (65535 bytes):
// longer values fail with "Data too long" in strict mode, or are silently truncated (possibly cutting an
// <!--access end--> marker and exposing gated text) otherwise. The request body limit alone allows 1 MiB.
const contentMaxBytes = 65535

var errContentTooLong = errors.New("must be at most 65535 bytes")

func contentRequiredText(p *string, required bool) (string, bool, error) {
	if p == nil {
		if required {
			return "", false, errContentRequired
		}
		return "", false, nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return "", false, errContentRequired
	}
	return v, true, nil
}

func contentHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// contentUpdate writes m to row id of T (404 when missing) and returns the reloaded row.
func contentUpdate[T any](c *gin.Context, a *app.App, id uint64, m map[string]any) (*T, error) {
	var out T
	err := withLiveStaffTx(c, a, func(tx *gorm.DB, _ bool) error {
		var cur T
		if err := tx.Where("id = ?", id).First(&cur).Error; err != nil {
			return err
		}
		if len(m) > 0 {
			if err := tx.Model(new(T)).Where("id = ?", id).Updates(m).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", id).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// contentDelete deletes row :id of T, writing the response on failure (404 when missing).
func contentDelete[T any](c *gin.Context, a *app.App, kind string) bool {
	id, err := httpx.ParamID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return false
	}
	var affected int64
	err = withLiveStaffTx(c, a, func(tx *gorm.DB, _ bool) error {
		res := tx.Where("id = ?", id).Delete(new(T))
		affected = res.RowsAffected
		return res.Error
	})
	if err != nil {
		httpx.Fail(c, err)
		return false
	}
	if affected == 0 {
		httpx.Fail(c, httpx.NotFound(kind+" not found"))
		return false
	}
	httpx.Logger(c).Info("admin: "+kind+" deleted", "id", id, "staff_id", auth.CurrentUser(c).ID)
	return true
}

// contentSortIDs binds {ids: [...]} (non-empty, unique, positive), writing the response on failure.
func contentSortIDs(c *gin.Context) ([]uint64, bool) {
	var in struct {
		IDs []uint64 `json:"ids" binding:"required,min=1,max=1000,unique,dive,gt=0"`
	}
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return nil, false
	}
	return in.IDs, true
}

func contentSortError(err error) error {
	if errors.Is(err, contentsvc.ErrUnknownID) {
		return httpx.FieldError("ids", "contains an unknown id")
	}
	return err
}
