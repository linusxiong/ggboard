// Package content implements notices (announcements) and the knowledge base: queries, manual ordering and the
// user-facing article rendering (placeholders and plan-gated blocks, compatible with Xboard).
package content

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// Errors.
var (
	ErrNotFound  = errors.New("content: not found")
	ErrUnknownID = errors.New("content: unknown id in sort list")
)

// Service is cheap to construct: content.New(a) per request is fine.
type Service struct{ a *app.App }

// New returns a Service bound to a.
func New(a *app.App) *Service { return &Service{a: a} }

// ---------------------------------------------------------------- notices

// NoticeOrder is the display order of notices: manual sort first, newest first within the same sort value.
const NoticeOrder = "sort ASC, id DESC"

// NoticeQuery returns notices for httpx.Paginate / Find; onlyVisible keeps show=true rows.
func (s *Service) NoticeQuery(ctx context.Context, onlyVisible bool) *gorm.DB {
	q := s.a.DB.WithContext(ctx).Model(&model.Notice{})
	if onlyVisible {
		q = q.Where(map[string]any{"show": true})
	}
	return q
}

// SortNotices assigns sort = position (1-based) to the given notice ids.
func (s *Service) SortNotices(ctx context.Context, ids []uint64) error {
	return s.sort(ctx, &model.Notice{}, ids)
}

// ---------------------------------------------------------------- knowledge

// KnowledgeOrder is the display order of knowledge articles.
const KnowledgeOrder = "sort ASC, id ASC"

// KnowledgeSummary is an article in the user index (no body).
type KnowledgeSummary struct {
	ID        uint64    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
}

// KnowledgeFilter narrows knowledge queries; empty fields are ignored.
type KnowledgeFilter struct {
	Language    string // "zh-CN" matches exactly; a bare primary tag ("en") also matches its regional tags ("en-US")
	Category    string
	Keyword     string // case-insensitive substring of title or body
	OnlyVisible bool
}

// KnowledgeQuery returns the knowledge rows matching f (no order).
func (s *Service) KnowledgeQuery(ctx context.Context, f KnowledgeFilter) *gorm.DB {
	q := s.a.DB.WithContext(ctx).Model(&model.Knowledge{})
	if f.OnlyVisible {
		q = q.Where(map[string]any{"show": true})
	}
	if lang := strings.TrimSpace(f.Language); lang != "" {
		if strings.Contains(lang, "-") {
			q = q.Where("language = ?", lang)
		} else {
			q = q.Where("(language = ? OR language LIKE ? ESCAPE '!')", lang, likeEscape(lang)+"-%")
		}
	}
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		pattern := "%" + likeEscape(strings.ToLower(kw)) + "%"
		q = q.Where("(LOWER(title) LIKE ? ESCAPE '!' OR LOWER(body) LIKE ? ESCAPE '!')", pattern, pattern)
	}
	return q
}

// KnowledgeIndex groups the visible articles in language (default: setting default_locale) matching keyword
// by category, each list in display order. hasPlan is whether the reader has a valid plan: without one the
// keyword is matched against the body with its plan-gated blocks removed, otherwise searching for a guessed
// string would reveal what the gated blocks contain.
func (s *Service) KnowledgeIndex(ctx context.Context, language, keyword string, hasPlan bool) (map[string][]KnowledgeSummary, error) {
	if strings.TrimSpace(language) == "" {
		language = s.a.Settings.String("default_locale")
	}
	kw := strings.ToLower(strings.TrimSpace(keyword))
	recheck := kw != "" && !hasPlan
	var rows []struct {
		KnowledgeSummary
		Category string
		Body     string
	}
	cols := []string{"id", "title", "category", "updated_at"}
	if recheck {
		cols = append(cols, "body")
	}
	err := s.KnowledgeQuery(ctx, KnowledgeFilter{Language: language, Keyword: keyword, OnlyVisible: true}).
		Select(cols).Order(KnowledgeOrder).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string][]KnowledgeSummary)
	for _, r := range rows {
		if recheck && !strings.Contains(strings.ToLower(r.Title), kw) &&
			!strings.Contains(strings.ToLower(accessBlock.ReplaceAllLiteralString(r.Body, "")), kw) {
			continue // the SQL match was inside a gated block
		}
		out[r.Category] = append(out[r.Category], r.KnowledgeSummary)
	}
	return out, nil
}

// Article is a knowledge article rendered for a user.
type Article struct {
	ID        uint64    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Category  string    `json:"category"`
	Language  string    `json:"language"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Article loads a visible article and renders it for u at now (see RenderBody).
func (s *Service) Article(ctx context.Context, id uint64, u *model.User, now time.Time) (*Article, error) {
	var k model.Knowledge
	err := s.a.DB.WithContext(ctx).Where(map[string]any{"id": id, "show": true}).First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	hasPlan := u.PlanID != nil && u.IsAvailable(now)
	body := RenderBody(k.Body, k.Language, s.a.Settings.String("app_name"), usersvc.New(s.a).SubscribeURL(u), hasPlan)
	return &Article{ID: k.ID, Title: k.Title, Body: body, Category: k.Category, Language: k.Language, UpdatedAt: k.UpdatedAt}, nil
}

// SortKnowledge assigns sort = position (1-based) to the given article ids.
func (s *Service) SortKnowledge(ctx context.Context, ids []uint64) error {
	return s.sort(ctx, &model.Knowledge{}, ids)
}

// Access markers of plan-gated blocks (Xboard syntax).
var accessBlock = regexp.MustCompile(`(?s)<!--access start-->.*?<!--access end-->`)

// NoAccessClass is the CSS class of the notice that replaces plan-gated blocks.
const NoAccessClass = "knowledge-no-access"

// Subscription placeholders: RenderBody replaces them with the reader's own subscription URL, which carries the
// reader's token. Only administrators may write them (HasSubscribePlaceholder): staff must not see users' tokens,
// and a body they wrote could send the expanded URL to another host (for example inside a remote image URL).
const (
	PlaceholderSubscribeURL           = "{{subscribeUrl}}"
	PlaceholderURLEncodeSubscribeURL  = "{{urlEncodeSubscribeUrl}}"
	PlaceholderSafeBase64SubscribeURL = "{{safeBase64SubscribeUrl}}"
)

// SubscribePlaceholders returns the subscription placeholders (a new slice).
func SubscribePlaceholders() []string {
	return []string{PlaceholderSubscribeURL, PlaceholderURLEncodeSubscribeURL, PlaceholderSafeBase64SubscribeURL}
}

// HasSubscribePlaceholder reports whether body contains a subscription placeholder. It matches exactly what
// RenderBody replaces (case-sensitive substrings of the raw body: the plan-gate notice inserted before the
// replacement contains no braces, and replaced text is never scanned again).
func HasSubscribePlaceholder(body string) bool {
	for _, p := range SubscribePlaceholders() {
		if strings.Contains(body, p) {
			return true
		}
	}
	return false
}

// RenderBody prepares an article body for a user, in Xboard order: without a valid plan every
// <!--access start-->…<!--access end--> block becomes a localized notice; then {{siteName}}, {{subscribeUrl}},
// {{urlEncodeSubscribeUrl}} (query escaping, like PHP urlencode) and {{safeBase64SubscribeUrl}} (URL-safe
// base64 without padding) are replaced.
func RenderBody(body, language, siteName, subscribeURL string, hasPlan bool) string {
	if !hasPlan {
		notice := `<div class="` + NoAccessClass + `">` + noAccessText(language) + `</div>`
		body = accessBlock.ReplaceAllLiteralString(body, notice)
	}
	return strings.NewReplacer(
		"{{siteName}}", siteName,
		PlaceholderSubscribeURL, subscribeURL,
		PlaceholderURLEncodeSubscribeURL, url.QueryEscape(subscribeURL),
		PlaceholderSafeBase64SubscribeURL, base64.RawURLEncoding.EncodeToString([]byte(subscribeURL)),
	).Replace(body)
}

func noAccessText(language string) string {
	if strings.HasPrefix(strings.ToLower(language), "zh") {
		return "您必须拥有有效的订阅才可以查看该区域的内容"
	}
	return "You must have a valid subscription to view content in this area"
}

// ---------------------------------------------------------------- helpers

// sort writes sort = index+1 for each id of table (model) in one transaction without touching updated_at.
func (s *Service) sort(ctx context.Context, m any, ids []uint64) error {
	return s.a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(m).Where("id IN ?", ids).Count(&n).Error; err != nil {
			return err
		}
		if n != int64(len(ids)) {
			return ErrUnknownID
		}
		for i, id := range ids {
			if err := tx.Model(m).Where("id = ?", id).UpdateColumn("sort", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// likeEscape escapes LIKE wildcards with '!' (portable ESCAPE character on SQLite, PostgreSQL and MySQL).
func likeEscape(s string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(s)
}
