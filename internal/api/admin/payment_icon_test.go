package admin_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// TestAdminPaymentIcon covers the Payment.icon formats (docs/API.md → payments): preset references,
// URLs, paths, data URIs and single emoji are accepted; everything else is a field error on `icon`.
func TestAdminPaymentIcon(t *testing.T) {
	e := newCommerceEnv(t)
	type view struct {
		ID   uint64 `json:"id"`
		Icon string `json:"icon"`
	}
	epay := func(icon any) map[string]any {
		return map[string]any{"gateway": "epay", "name": "Alipay", "icon": icon,
			"config": map[string]string{"url": "https://pay.example.com", "pid": "1001", "key": "k"}}
	}

	for _, icon := range []string{
		"", "preset:alipay", "preset:usdt", "preset:my-icon_2", "preset:" + strings.Repeat("a", 32),
		"https://cdn.example.com/alipay.svg", "HTTP://cdn.example.com/a.png?v=1", "/static/pay.png",
		"//cdn.example.com/pay.png", "data:image/svg+xml;base64,PHN2Zy8+", "💳", "¥", "🇨🇳", "👨‍👩‍👧",
	} {
		rec := e.do(t, http.MethodPost, "/payments", epay(icon))
		testutil.ExpectStatus(t, rec, http.StatusCreated)
		if got := testutil.Decode[view](t, rec); got.Icon != icon {
			t.Fatalf("icon %q stored as %q", icon, got.Icon)
		}
	}
	// Surrounding whitespace is trimmed.
	if got := testutil.Decode[view](t, e.do(t, http.MethodPost, "/payments", epay("  preset:wechat \n"))); got.Icon != "preset:wechat" {
		t.Fatalf("trimmed icon = %q", got.Icon)
	}

	for _, icon := range []string{
		"preset:", "preset:Alipay", "preset:ali pay", "preset:" + strings.Repeat("a", 33), "preset:a/b",
		"alipay", "alipay.svg", "javascript:alert(1)", "ftp://files.example.com/a.png", "https://",
		"https://cdn.example.com/a b.png", "💳 💳", "AB", strings.Repeat("💳", 17),
	} {
		rec := e.do(t, http.MethodPost, "/payments", epay(icon))
		testutil.ExpectStatus(t, rec, http.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), `"icon"`) {
			t.Fatalf("%q: error not on the icon field: %s", icon, rec.Body.String())
		}
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPost, "/payments", epay(strings.Repeat("a", 256))), http.StatusBadRequest)

	// PATCH validates the icon only when it is part of the request: an imported legacy value keeps working.
	legacy := &model.Payment{UUID: strings.Repeat("c", 32), Gateway: "epay", Name: "Legacy", Icon: "alipay.svg",
		Config: model.JSONObject[string]{"url": "https://pay.example.com", "pid": "1", "key": "k"}}
	if err := e.a.DB.Create(legacy).Error; err != nil {
		t.Fatal(err)
	}
	path := commercePath("/payments", legacy.ID)
	if got := testutil.Decode[view](t, e.do(t, http.MethodPatch, path, map[string]any{"enabled": true})); got.Icon != "alipay.svg" {
		t.Fatalf("legacy icon = %q", got.Icon)
	}
	testutil.ExpectStatus(t, e.do(t, http.MethodPatch, path, map[string]any{"icon": "alipay.svg"}), http.StatusBadRequest)
	if got := testutil.Decode[view](t, e.do(t, http.MethodPatch, path, map[string]any{"icon": "preset:epay"})); got.Icon != "preset:epay" {
		t.Fatalf("patched icon = %q", got.Icon)
	}
}
