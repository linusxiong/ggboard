package admin_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestAdminKnowledgeCRUD(t *testing.T) {
	_, r, adminTok, staffTok, userTok := contentAdminEnv(t)
	base := "/api/v1/admin/knowledge"

	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, base, nil, userTok), http.StatusForbidden)

	for field, body := range map[string]map[string]any{
		"language": {"category": "c", "title": "t", "body": "b"},
		"category": {"language": "en", "category": " ", "title": "t", "body": "b"},
		"title":    {"language": "en", "category": "c", "body": "b"},
		"body":     {"language": "en", "category": "c", "title": "t", "body": "\n\n"},
	} {
		rec := testutil.Do(t, r, http.MethodPost, base, body, staffTok)
		testutil.ExpectStatus(t, rec, http.StatusBadRequest)
		if e := testutil.DecodeError(t, rec); len(e.Errors[field]) == 0 {
			t.Fatalf("%s: %+v", field, e)
		}
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base, map[string]any{"language": "zh-Hans", "category": "c", "title": "t", "body": "b"}, staffTok), http.StatusBadRequest)

	create := func(tok string, body map[string]any) model.Knowledge {
		t.Helper()
		rec := testutil.Do(t, r, http.MethodPost, base, body, tok)
		testutil.ExpectStatus(t, rec, http.StatusCreated)
		return testutil.Decode[model.Knowledge](t, rec)
	}
	body := "# Setup\n\nOpen {{subscribeUrl}}\n<!--access start-->secret<!--access end-->\n"
	k1 := create(staffTok, map[string]any{"language": "en", "category": "Start", "title": "Windows", "body": body})
	k2 := create(adminTok, map[string]any{"language": "en", "category": "Start", "title": "macOS", "body": "mac", "show": false})
	k3 := create(staffTok, map[string]any{"language": "zh-CN", "category": "入门", "title": "安卓", "body": "android"})
	if !k1.Show || k2.Show || k1.Body != body || k1.Language != "en" || k3.Category != "入门" {
		t.Fatalf("created = %+v / %+v / %+v", k1, k2, k3)
	}

	type item struct {
		ID       uint64 `json:"id"`
		Language string `json:"language"`
		Category string `json:"category"`
		Title    string `json:"title"`
		Show     bool   `json:"show"`
		Sort     int    `json:"sort"`
	}
	list := func(q string) string {
		t.Helper()
		rec := testutil.Do(t, r, http.MethodGet, base+q, nil, staffTok)
		if strings.Contains(rec.Body.String(), `"body"`) {
			t.Fatalf("list leaks bodies: %s", rec.Body.String())
		}
		items, total := testutil.DecodeList[item](t, rec)
		if int(total) != len(items) {
			t.Fatalf("total %d != %d", total, len(items))
		}
		var s []string
		for _, it := range items {
			s = append(s, it.Title)
		}
		return strings.Join(s, ",")
	}
	if got := list(""); got != "Windows,macOS,安卓" {
		t.Fatalf("list = %s", got)
	}
	if got := list("?language=zh-CN"); got != "安卓" {
		t.Fatalf("zh-CN = %s", got)
	}
	if got := list("?category=Start&keyword=MAC"); got != "macOS" {
		t.Fatalf("filtered = %s", got)
	}

	// Detail keeps the raw body (no placeholder replacement for staff).
	p := fmt.Sprintf("%s/%d", base, k1.ID)
	if got := testutil.Decode[model.Knowledge](t, testutil.Do(t, r, http.MethodGet, p, nil, staffTok)); got.Body != body {
		t.Fatalf("detail body = %q", got.Body)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, base+"/999", nil, staffTok), http.StatusNotFound)

	// Patch.
	rec := testutil.Do(t, r, http.MethodPatch, p, map[string]any{"title": "Windows 11", "show": false, "sort": 9}, staffTok)
	got := testutil.Decode[model.Knowledge](t, rec)
	if got.Title != "Windows 11" || got.Show || got.Sort != 9 || got.Body != body || got.Category != "Start" {
		t.Fatalf("patched = %+v", got)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, p, map[string]any{"body": " "}, staffTok), http.StatusBadRequest)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, base+"/999", map[string]any{"title": "x"}, staffTok), http.StatusNotFound)

	// Sort.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/sort", map[string]any{"ids": []uint64{k3.ID, k2.ID, k1.ID}}, staffTok), http.StatusOK)
	if got := list(""); got != "安卓,macOS,Windows 11" {
		t.Fatalf("sorted = %s", got)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/sort", map[string]any{"ids": []uint64{12345}}, staffTok), http.StatusBadRequest)

	// Delete.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodDelete, p, nil, staffTok), http.StatusOK)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, p, nil, staffTok), http.StatusNotFound)
	if got := list(""); got != "安卓,macOS" {
		t.Fatalf("after delete = %s", got)
	}
}
