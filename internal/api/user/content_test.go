package user_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	contentsvc "github.com/linusxiong/ggboard/internal/service/content"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func contentSeed[T any](t *testing.T, a *app.App, rows ...*T) {
	t.Helper()
	for _, r := range rows {
		if err := a.DB.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestUserNotices(t *testing.T) {
	a, r := ticketTestEnv(t, nil)
	tok := testutil.Login(t, a, testutil.CreateUser(t, a, "alice@example.com"))
	n1 := &model.Notice{Title: "Old pinned", Content: "c1", Show: true, Sort: 1, Tags: model.JSONSlice[string]{"pin"}}
	n2 := &model.Notice{Title: "Hidden", Content: "c2", Show: false}
	n3 := &model.Notice{Title: "Unsorted older", Content: "c3", Show: true, ImgURL: "https://cdn.example.com/a.png"}
	n4 := &model.Notice{Title: "Unsorted newer", Content: "c4", Show: true}
	n5 := &model.Notice{Title: "Last", Content: "c5", Show: true, Sort: 2}
	contentSeed(t, a, n1, n2, n3, n4, n5)

	type notice struct {
		ID        uint64    `json:"id"`
		Title     string    `json:"title"`
		Content   string    `json:"content"`
		ImgURL    string    `json:"img_url"`
		Tags      []string  `json:"tags"`
		CreatedAt time.Time `json:"created_at"`
	}
	rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/notices", nil, tok)
	list, total := testutil.DecodeList[notice](t, rec)
	var titles []string
	for _, n := range list {
		titles = append(titles, n.Title)
	}
	if total != 4 || strings.Join(titles, ",") != "Unsorted newer,Unsorted older,Old pinned,Last" {
		t.Fatalf("notices = %v (total %d)", titles, total)
	}
	if list[1].ImgURL != "https://cdn.example.com/a.png" || list[2].Tags[0] != "pin" || list[0].Tags == nil || list[0].CreatedAt.IsZero() {
		t.Fatalf("fields = %+v", list)
	}
	for _, leak := range []string{`"show"`, `"sort"`, `"updated_at"`} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("user view leaks %s: %s", leak, rec.Body.String())
		}
	}
	list, total = testutil.DecodeList[notice](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/notices?page=2&page_size=3", nil, tok))
	if total != 4 || len(list) != 1 || list[0].Title != "Last" {
		t.Fatalf("page 2 = %+v", list)
	}
}

func TestUserKnowledgeIndex(t *testing.T) {
	a, r := ticketTestEnv(t, nil)
	tok := testutil.Login(t, a, testutil.CreateUser(t, a, "alice@example.com"))
	contentSeed(t, a,
		&model.Knowledge{Language: "zh-CN", Category: "入门", Title: "Windows 教程", Body: "下载 Clash", Show: true, Sort: 2},
		&model.Knowledge{Language: "zh-CN", Category: "入门", Title: "macOS 教程", Body: "下载 ClashX", Show: true, Sort: 1},
		&model.Knowledge{Language: "zh-CN", Category: "常见问题", Title: "无法连接", Body: "检查 100% 的设置", Show: true},
		&model.Knowledge{Language: "zh-CN", Category: "入门", Title: "草稿", Body: "hidden", Show: false},
		&model.Knowledge{Language: "en", Category: "Start", Title: "Getting started", Body: "Install the APP", Show: true},
		&model.Knowledge{Language: "en-US", Category: "FAQ", Title: "Billing", Body: "Refunds", Show: true},
		&model.Knowledge{Language: "zh-TW", Category: "入門", Title: "繁體", Body: "x", Show: true},
	)
	type item struct {
		ID        uint64    `json:"id"`
		Title     string    `json:"title"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	get := func(q string) map[string][]item {
		t.Helper()
		rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/knowledge"+q, nil, tok)
		if strings.Contains(rec.Body.String(), `"body"`) {
			t.Fatalf("index leaks bodies: %s", rec.Body.String())
		}
		return testutil.Decode[map[string][]item](t, rec)
	}
	titles := func(items []item) string {
		var s []string
		for _, it := range items {
			s = append(s, it.Title)
		}
		return strings.Join(s, ",")
	}

	// Default language = default_locale (zh-CN); hidden articles and other languages excluded; sort order.
	idx := get("")
	if len(idx) != 2 || titles(idx["入门"]) != "macOS 教程,Windows 教程" || titles(idx["常见问题"]) != "无法连接" || idx["入门"][0].UpdatedAt.IsZero() {
		t.Fatalf("zh-CN index = %+v", idx)
	}
	// A bare primary tag also matches regional variants.
	if idx = get("?language=en"); len(idx) != 2 || titles(idx["Start"]) != "Getting started" || titles(idx["FAQ"]) != "Billing" {
		t.Fatalf("en index = %+v", idx)
	}
	if idx = get("?language=en-US"); len(idx) != 1 || titles(idx["FAQ"]) != "Billing" {
		t.Fatalf("en-US index = %+v", idx)
	}
	// Keyword: title or body, case-insensitive, LIKE wildcards are literal.
	if idx = get("?language=en&keyword=app"); titles(idx["Start"]) != "Getting started" || len(idx) != 1 {
		t.Fatalf("keyword body = %+v", idx)
	}
	if idx = get("?language=en&keyword=BILL"); titles(idx["FAQ"]) != "Billing" || len(idx) != 1 {
		t.Fatalf("keyword title = %+v", idx)
	}
	if idx = get("?keyword=" + url.QueryEscape("100%")); len(idx) != 1 || titles(idx["常见问题"]) != "无法连接" {
		t.Fatalf("keyword %% = %+v", idx)
	}
	if idx = get("?keyword=" + url.QueryEscape("%")); len(idx) != 1 {
		t.Fatalf("keyword %% must be literal: %+v", idx)
	}
	if idx = get("?keyword=nothing-matches"); len(idx) != 0 {
		t.Fatalf("no match = %+v", idx)
	}
	// Empty result is an object, not null.
	if rec := testutil.Do(t, r, http.MethodGet, "/api/v1/user/knowledge?language=ja-JP", nil, tok); !strings.Contains(rec.Body.String(), `"data":{}`) {
		t.Fatalf("empty index = %s", rec.Body.String())
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/knowledge?language="+strings.Repeat("x", 11), nil, tok), http.StatusBadRequest)
}

func TestUserKnowledgeArticle(t *testing.T) {
	a, r := ticketTestEnv(t, map[string]any{"app_url": "https://panel.example.com", "app_name": "Example Panel"})
	future, past := time.Now().Add(24*time.Hour), time.Now().Add(-time.Hour)
	planID := uint64(1)
	paid := testutil.CreateUser(t, a, "paid@example.com", usersvc.CreateOptions{PlanID: &planID, TransferEnable: 1 << 30, ExpiredAt: &future})
	never := testutil.CreateUser(t, a, "never@example.com", usersvc.CreateOptions{PlanID: &planID, TransferEnable: 1 << 30})
	expired := testutil.CreateUser(t, a, "expired@example.com", usersvc.CreateOptions{PlanID: &planID, TransferEnable: 1 << 30, ExpiredAt: &past})
	free := testutil.CreateUser(t, a, "free@example.com")

	body := "Welcome to {{siteName}}.\n<!--access start-->Import {{subscribeUrl}}\nclash://install-config?url={{urlEncodeSubscribeUrl}}<!--access end-->\n" +
		"sing-box: {{safeBase64SubscribeUrl}}\n<!--access start-->second block<!--access end-->"
	zh := &model.Knowledge{Language: "zh-CN", Category: "入门", Title: "教程", Body: body, Show: true}
	en := &model.Knowledge{Language: "en", Category: "Start", Title: "Guide", Body: body, Show: true}
	hidden := &model.Knowledge{Language: "en", Category: "Start", Title: "Draft", Body: "x", Show: false}
	contentSeed(t, a, zh, en, hidden)

	type article struct {
		ID        uint64    `json:"id"`
		Title     string    `json:"title"`
		Body      string    `json:"body"`
		Category  string    `json:"category"`
		Language  string    `json:"language"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	fetch := func(u *model.User, id uint64) article {
		t.Helper()
		return testutil.Decode[article](t, testutil.Do(t, r, http.MethodGet, fmt.Sprintf("/api/v1/user/knowledge/%d", id), nil, testutil.Login(t, a, u)))
	}

	for _, u := range []*model.User{paid, never} {
		got := fetch(u, en.ID)
		sub := "https://panel.example.com/api/v1/client/subscribe?token=" + u.Token
		b64 := base64.RawURLEncoding.EncodeToString([]byte(sub))
		want := "Welcome to Example Panel.\n<!--access start-->Import " + sub + "\nclash://install-config?url=" + url.QueryEscape(sub) +
			"<!--access end-->\nsing-box: " + b64 + "\n<!--access start-->second block<!--access end-->"
		if got.Body != want {
			t.Fatalf("%s body =\n%s\nwant\n%s", u.Email, got.Body, want)
		}
		if strings.ContainsAny(b64, "+/=") || got.Title != "Guide" || got.Category != "Start" || got.Language != "en" || got.UpdatedAt.IsZero() {
			t.Fatalf("article = %+v", got)
		}
	}

	for _, u := range []*model.User{free, expired} {
		got := fetch(u, en.ID)
		if strings.Contains(got.Body, "Import") || strings.Contains(got.Body, "second block") || strings.Contains(got.Body, "install-config") {
			t.Fatalf("%s sees gated content:\n%s", u.Email, got.Body)
		}
		notice := `<div class="` + contentsvc.NoAccessClass + `">You must have a valid subscription to view content in this area</div>`
		if strings.Count(got.Body, notice) != 2 || !strings.Contains(got.Body, "Welcome to Example Panel.") ||
			!strings.Contains(got.Body, "sing-box: "+base64.RawURLEncoding.EncodeToString([]byte("https://panel.example.com/api/v1/client/subscribe?token="+u.Token))) {
			t.Fatalf("%s gated body =\n%s", u.Email, got.Body)
		}
	}
	if got := fetch(free, zh.ID); !strings.Contains(got.Body, "您必须拥有有效的订阅才可以查看该区域的内容") {
		t.Fatalf("zh notice =\n%s", got.Body)
	}

	// Traffic exhausted counts as no valid plan.
	if err := a.DB.Model(&model.User{}).Where("id = ?", paid.ID).Update("u", int64(1<<30)).Error; err != nil {
		t.Fatal(err)
	}
	auth.InvalidateUser(a, paid.ID)
	if got := fetch(paid, en.ID); strings.Contains(got.Body, "second block") || !strings.Contains(got.Body, contentsvc.NoAccessClass) {
		t.Fatalf("exhausted traffic still sees gated content:\n%s", got.Body)
	}

	tok := testutil.Login(t, a, paid)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, fmt.Sprintf("/api/v1/user/knowledge/%d", hidden.ID), nil, tok), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/knowledge/9999", nil, tok), http.StatusNotFound)
}
