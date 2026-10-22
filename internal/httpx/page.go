package httpx

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 200
)

// ParsePage reads ?page (1-based, default 1) and ?page_size (default 20, max 200).
func ParsePage(c *gin.Context) (page, size, offset int) {
	page, _ = strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	size, _ = strconv.Atoi(c.Query("page_size"))
	if size < 1 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	return page, size, (page - 1) * size
}

// ParseSort turns ?sort=field,-other into an ORDER BY clause using only columns from allowed
// (API field -> column). Unknown fields are ignored; "" means no (valid) sort was requested.
func ParseSort(c *gin.Context, allowed map[string]string) string {
	raw := c.Query("sort")
	if raw == "" {
		return ""
	}
	var parts []string
	seen := map[string]bool{}
	for _, f := range strings.Split(raw, ",") {
		f = strings.TrimSpace(f)
		dir := "ASC"
		if strings.HasPrefix(f, "-") {
			dir, f = "DESC", f[1:]
		} else {
			f = strings.TrimPrefix(f, "+")
		}
		col, ok := allowed[f]
		if !ok || seen[col] {
			continue
		}
		seen[col] = true
		parts = append(parts, col+" "+dir)
	}
	return strings.Join(parts, ", ")
}

// Paginate counts and loads one page of q into []T using ParsePage and ParseSort (falling back to
// defaultOrder, e.g. "id DESC"). q should carry the filters; its model defaults to T.
func Paginate[T any](c *gin.Context, q *gorm.DB, allowedSort map[string]string, defaultOrder string) ([]T, int64, error) {
	_, size, offset := ParsePage(c)
	if q.Statement.Model == nil && q.Statement.Table == "" {
		q = q.Model(new(T))
	}
	base := q.Session(&gorm.Session{})
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []T{}
	if total == 0 || int64(offset) >= total {
		return items, total, nil
	}
	order := ParseSort(c, allowedSort)
	if order == "" {
		order = defaultOrder
	}
	find := base.Limit(size).Offset(offset)
	if order != "" {
		find = find.Order(order)
	}
	if err := find.Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
