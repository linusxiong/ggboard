package admin_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestAdminNoticesCRUD(t *testing.T) {
	a, r, adminTok, staffTok, userTok := contentAdminEnv(t)
	base := "/api/v1/admin/notices"

	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, base, nil, userTok), http.StatusForbidden)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base, map[string]any{"title": "x", "content": "y"}, userTok), http.StatusForbidden)

	// Validation.
	for field, body := range map[string]map[string]any{
		"title":   {"content": "c"},
		"content": {"title": "t", "content": "  "},
		"img_url": {"title": "t", "content": "c", "img_url": "javascript:alert(1)"},
		"tags[0]": {"title": "t", "content": "c", "tags": []string{strings.Repeat("x", 33)}},
	} {
		rec := testutil.Do(t, r, http.MethodPost, base, body, staffTok)
		testutil.ExpectStatus(t, rec, http.StatusBadRequest)
		if e := testutil.DecodeError(t, rec); len(e.Errors[field]) == 0 {
			t.Fatalf("%s: %+v", field, e)
		}
	}

	// Create: show defaults to true; tags are trimmed.
	rec := testutil.Do(t, r, http.MethodPost, base, map[string]any{
		"title": " Maintenance ", "content": "Tonight **22:00**", "img_url": "https://cdn.example.com/m.png", "tags": []string{" ops ", ""},
	}, staffTok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	n1 := testutil.Decode[model.Notice](t, rec)
	if n1.ID == 0 || n1.Title != "Maintenance" || !n1.Show || n1.ImgURL != "https://cdn.example.com/m.png" || fmt.Sprint(n1.Tags) != "[ops]" {
		t.Fatalf("created = %+v", n1)
	}
	rec = testutil.Do(t, r, http.MethodPost, base, map[string]any{"title": "Draft", "content": "wip", "show": false}, adminTok)
	n2 := testutil.Decode[model.Notice](t, rec)
	if n2.Show || n2.Tags == nil {
		t.Fatalf("draft = %+v", n2)
	}
	n3 := testutil.Decode[model.Notice](t, testutil.Do(t, r, http.MethodPost, base, map[string]any{"title": "Third", "content": "c", "sort": 5}, staffTok))

	// List: hidden included; sort ASC then newest first.
	order := func() string {
		t.Helper()
		list, total := testutil.DecodeList[model.Notice](t, testutil.Do(t, r, http.MethodGet, base, nil, staffTok))
		if int(total) != len(list) {
			t.Fatalf("total %d != %d", total, len(list))
		}
		var s []string
		for _, n := range list {
			s = append(s, n.Title)
		}
		return strings.Join(s, ",")
	}
	if got := order(); got != "Draft,Maintenance,Third" {
		t.Fatalf("order = %s", got)
	}

	// Patch: partial update.
	p := fmt.Sprintf("%s/%d", base, n2.ID)
	rec = testutil.Do(t, r, http.MethodPatch, p, map[string]any{"show": true, "tags": []string{"a", "b"}, "img_url": ""}, staffTok)
	got := testutil.Decode[model.Notice](t, rec)
	if !got.Show || got.Title != "Draft" || got.Content != "wip" || fmt.Sprint(got.Tags) != "[a b]" || got.ImgURL != "" {
		t.Fatalf("patched = %+v", got)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, p, map[string]any{"title": ""}, staffTok), http.StatusBadRequest)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, base+"/999", map[string]any{"show": false}, staffTok), http.StatusNotFound)
	if got := testutil.Decode[model.Notice](t, testutil.Do(t, r, http.MethodPatch, p, map[string]any{}, staffTok)); got.Title != "Draft" {
		t.Fatalf("empty patch = %+v", got)
	}

	// Sort.
	rec = testutil.Do(t, r, http.MethodPost, base+"/sort", map[string]any{"ids": []uint64{n3.ID, n1.ID, n2.ID}}, staffTok)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	if got := order(); got != "Third,Maintenance,Draft" {
		t.Fatalf("sorted = %s", got)
	}
	for _, body := range []map[string]any{{"ids": []uint64{}}, {"ids": []uint64{n1.ID, n1.ID}}, {"ids": []uint64{n1.ID, 999}}, {}} {
		testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/sort", body, staffTok), http.StatusBadRequest)
	}

	// Delete.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodDelete, p, nil, staffTok), http.StatusOK)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodDelete, p, nil, staffTok), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodDelete, p, nil, userTok), http.StatusForbidden)
	if got := order(); got != "Third,Maintenance" {
		t.Fatalf("after delete = %s", got)
	}
	var count int64
	a.DB.Model(&model.Notice{}).Count(&count)
	if count != 2 {
		t.Fatalf("rows = %d", count)
	}
}
