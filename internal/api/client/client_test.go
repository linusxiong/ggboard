package client_test

import (
	"crypto/sha1" //nolint:gosec // test recomputes the ETag
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"github.com/linusxiong/ggboard/internal/api/client"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/subscription/fixture"
	"github.com/linusxiong/ggboard/internal/testutil"
)

type env struct {
	a *app.App
	r *gin.Engine
	u *model.User
}

func setup(t *testing.T, settings map[string]any) env {
	t.Helper()
	base := map[string]any{"app_name": "GG Bond", "app_url": "https://panel.example.com"}
	for k, v := range settings {
		base[k] = v
	}
	a := testutil.NewTestApp(t, testutil.WithSettings(base))
	if _, err := fixture.SeedNodes(a.DB); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(30 * 24 * time.Hour)
	gid := uint64(fixture.GroupID)
	u := testutil.CreateUser(t, a, "alice@example.com", usersvc.CreateOptions{GroupID: &gid, TransferEnable: 100 * fixture.GiB, ExpiredAt: &exp})
	return env{a: a, r: testutil.NewRouterWith(t, a, client.Register), u: u}
}

func (e env) get(t *testing.T, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return testutil.DoWithHeaders(t, e.r, http.MethodGet, path, nil, "", headers)
}

func (e env) sub(query string) string {
	return "/api/v1/client/subscribe?token=" + e.u.Token + query
}

func decoded(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(rec.Body.String())
	if err != nil {
		t.Fatalf("not base64: %v", err)
	}
	return string(b)
}

func TestUserAgentDetection(t *testing.T) {
	e := setup(t, nil)
	cases := []struct {
		ua, flag, contentType string
		check                 func(body string) bool
	}{
		{"clash.meta/1.19.0", "", "text/yaml", func(b string) bool {
			return strings.Contains(b, "unified-delay: true") && strings.Contains(b, "type: mieru")
		}},
		{"FlClash/0.8.80", "", "text/yaml", func(b string) bool { return strings.Contains(b, "unified-delay: true") }},
		{"Stash/2.4.0 Clash/1.9.0", "", "text/yaml", func(b string) bool { return strings.HasPrefix(b, "mode: rule") && strings.Contains(b, "auth: ") }},
		{"ClashForAndroid/2.5.12", "", "text/yaml", func(b string) bool {
			return strings.HasPrefix(b, "mixed-port") && !strings.Contains(b, "unified-delay") && !strings.Contains(b, "rule-providers")
		}},
		{"SFA/1.12.0 (sing-box 1.12.0)", "", "application/json", func(b string) bool { return json.Valid([]byte(b)) && strings.Contains(b, `"hijack-dns"`) }},
		{"Surge iOS/3000", "", "text/plain", func(b string) bool {
			return strings.Contains(b, "= tuic-v5,") && strings.HasPrefix(b, "#!MANAGED-CONFIG")
		}},
		{"Surfboard/2.24", "", "text/plain", func(b string) bool { return strings.HasPrefix(b, "#!MANAGED-CONFIG") && !strings.Contains(b, "tuic") }},
		{"Quantumult%20X/1.4.1", "", "text/plain", func(b string) bool { return strings.Contains(b, "[server_local]") }},
		{"Loon/3.2.4", "", "text/plain", func(b string) bool { return strings.Contains(b, "[Remote Rule]") }},
		{"Mozilla/5.0", "loon", "text/plain", func(b string) bool { return strings.Contains(b, "[Remote Rule]") }},
		{"Mozilla/5.0", "singbox", "application/json", func(b string) bool { return json.Valid([]byte(b)) }},
	}
	for _, tc := range cases {
		q := ""
		if tc.flag != "" {
			q = "&flag=" + tc.flag
		}
		rec := e.get(t, e.sub(q), map[string]string{"User-Agent": tc.ua})
		testutil.ExpectStatus(t, rec, http.StatusOK)
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.contentType) {
			t.Errorf("%s: content type %s", tc.ua, ct)
		}
		if !tc.check(rec.Body.String()) {
			t.Errorf("%s (flag %q): unexpected body:\n%.400s", tc.ua, tc.flag, rec.Body.String())
		}
	}
	// Base64 formats.
	for ua, prefix := range map[string]string{"Shadowrocket/2070": "STATUS=", "v2rayN/7.0": "ss://", "": "ss://"} {
		rec := e.get(t, e.sub(""), map[string]string{"User-Agent": ua})
		if body := decoded(t, rec); !strings.HasPrefix(body, prefix) {
			t.Errorf("%q: body starts %.40q", ua, body)
		}
	}
	if body := decoded(t, e.get(t, e.sub("&flag=ssr"), nil)); !strings.HasPrefix(body, "ssr://") {
		t.Errorf("ssr body %.40q", body)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(e.get(t, e.sub("&flag=clash-meta"), nil).Body.Bytes(), &doc); err != nil || doc["proxies"] == nil {
		t.Fatalf("yaml: %v", err)
	}
}

func TestHeadersETagAndShortLink(t *testing.T) {
	e := setup(t, map[string]any{"subscribe_update_interval": 12})
	rec := e.get(t, e.sub("&flag=clash-meta"), nil)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	h := rec.Header()
	want := fmt.Sprintf("upload=0; download=0; total=%d; expire=%d", 100*fixture.GiB, e.u.ExpiredAt.Unix())
	if got := h.Get("subscription-userinfo"); got != want {
		t.Fatalf("userinfo = %q, want %q", got, want)
	}
	sum := sha1.Sum(rec.Body.Bytes()) //nolint:gosec // ETag
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	checks := map[string]string{
		"profile-update-interval": "12",
		"content-disposition":     `attachment; filename="GG Bond"; filename*=UTF-8''GG%20Bond`,
		"profile-web-page-url":    "https://panel.example.com",
		"Cache-Control":           "no-store",
		"ETag":                    etag,
	}
	for k, v := range checks {
		if h.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, h.Get(k), v)
		}
	}
	for _, inm := range []string{etag, "W/" + etag, strings.Trim(etag, `"`), `"other", ` + etag} {
		rec := e.get(t, e.sub("&flag=clash-meta"), map[string]string{"If-None-Match": inm})
		if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != etag {
			t.Fatalf("If-None-Match %s: %d", inm, rec.Code)
		}
	}
	if rec := e.get(t, e.sub("&flag=clash-meta"), map[string]string{"If-None-Match": `"stale"`}); rec.Code != http.StatusOK {
		t.Fatal("stale etag must get the body")
	}

	// Short link: same body; the token never reaches the logs.
	short := e.get(t, "/s/"+e.u.Token+"?flag=clash-meta", nil)
	if short.Code != http.StatusOK || short.Body.String() != rec.Body.String() {
		t.Fatalf("short link = %d", short.Code)
	}
	head := testutil.DoWithHeaders(t, e.r, http.MethodHead, "/s/"+e.u.Token, nil, "", nil)
	if head.Code != http.StatusOK || head.Header().Get("subscription-userinfo") == "" {
		t.Fatalf("HEAD = %d", head.Code)
	}
	for _, entry := range e.a.Logs.Entries(slog.LevelDebug, 0) {
		if b, _ := json.Marshal(entry); strings.Contains(string(b), e.u.Token) {
			t.Fatalf("token leaked into logs: %s", b)
		}
	}
	served := false
	for _, entry := range e.a.Logs.Entries(slog.LevelInfo, 0) {
		if entry.Msg == "subscription served" && entry.Attrs["format"] == "clash-meta" {
			served = true
		}
	}
	if !served {
		t.Fatal("subscription not logged")
	}
	if cd := client.ContentDisposition("猪猪侠 \"VPN\""); cd != `attachment; filename="___ _VPN_"; filename*=UTF-8''%E7%8C%AA%E7%8C%AA%E4%BE%A0%20%22VPN%22` {
		t.Fatalf("content-disposition = %s", cd)
	}
	if client.ContentDisposition(" ") != `attachment; filename="subscription"; filename*=UTF-8''subscription` {
		t.Fatal("empty app name")
	}
}

func TestInvalidTokens(t *testing.T) {
	e := setup(t, nil)
	for _, path := range []string{
		"/api/v1/client/subscribe",
		"/api/v1/client/subscribe?token=",
		"/api/v1/client/subscribe?token=ffffffffffffffffffffffffffffffff",
		"/api/v1/client/subscribe?token=" + strings.Repeat("a", 65),
		"/api/v1/client/subscribe?token=a%27%20OR%201%3D1",
		"/s/ffffffffffffffffffffffffffffffff",
	} {
		rec := e.get(t, path, nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		if err := testutil.DecodeError(t, rec); err.Message != "forbidden" || len(err.Errors) != 0 {
			t.Fatalf("%s: %+v", path, err)
		}
	}
}

func TestRateLimit(t *testing.T) {
	e := setup(t, nil)
	codes := map[int]int{}
	for range client.SubscribeBurst + 1 {
		codes[e.get(t, "/s/"+e.u.Token+"?flag=v2ray", nil).Code]++
	}
	if codes[http.StatusOK] != client.SubscribeBurst || codes[http.StatusTooManyRequests] != 1 {
		t.Fatalf("codes = %v", codes)
	}
	// The query route shares the bucket.
	if rec := e.get(t, e.sub("&flag=v2ray"), nil); rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("shared bucket = %d", rec.Code)
	}
}

func TestFiltersAndNames(t *testing.T) {
	e := setup(t, map[string]any{"show_protocol_to_server_enable": true, "show_info_to_server_enable": false})
	body := decoded(t, e.get(t, e.sub("&flag=v2ray&types=vmess,hy2"), nil))
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 6 { // vmess 1, 2, 23, 24 + hysteria2 15, 16
		t.Fatalf("types filter = %d lines\n%s", len(lines), body)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "vmess://") && !strings.HasPrefix(l, "hysteria2://") {
			t.Fatalf("unexpected %s", l)
		}
	}
	yamlBody := e.get(t, e.sub("&flag=clash-meta&filter=HK|relay&emoji=0"), nil).Body.String()
	for _, want := range []string{`"[vmess] HK VMess WS"`, `"[ss] HK SS obfs"`, `"[vless] US Relay"`, `"[hysteria] HK Hysteria1"`} {
		if !strings.Contains(yamlBody, "- name: "+want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(yamlBody, "JP VMess") || strings.Contains(yamlBody, "🇭🇰") || strings.Contains(yamlBody, "剩余流量") {
		t.Fatalf("filter/emoji/info not applied:\n%s", yamlBody)
	}
}

func TestProfilesAndOverrides(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	raw := []byte(`{"template":"minimal","groups":[{"name":"Proxy","type":"select","include":"*"},{"name":"Fast","type":"url-test","include":"HK","url":"http://www.gstatic.com/generate_204","interval":120}],"categories":{"cn":"DIRECT","openai":"Fast"}}`)
	if err := engine.SaveProfile(ctx, e.a, e.u.ID, raw); err != nil {
		t.Fatal(err)
	}
	body := e.get(t, e.sub("&flag=clash-meta"), nil).Body.String()
	if !strings.Contains(body, "- name: Fast") || !strings.Contains(body, "RULE-SET,openai-domain,Fast") || strings.Contains(body, "name: AI") {
		t.Fatalf("stored profile not applied:\n%.2000s", body)
	}
	body = e.get(t, e.sub("&flag=clash-meta&profile=none"), nil).Body.String()
	if strings.Contains(body, "rule-providers") || !strings.Contains(body, "MATCH,Proxy") {
		t.Fatal("profile=none")
	}
	body = e.get(t, e.sub("&flag=surge&cats=openai&final=DIRECT&mirror=ghfast"), nil).Body.String()
	if !strings.Contains(body, "RULE-SET,https://ghfast.top/https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Surge/OpenAI/OpenAI.list,Fast") ||
		strings.Contains(body, "ChinaMax") || !strings.Contains(body, "FINAL,DIRECT,dns-failed") {
		t.Fatalf("overrides:\n%s", body)
	}
	rec := e.get(t, e.sub("&flag=surge&profile=nope&cats=x"), nil)
	if err := testutil.DecodeError(t, rec); err.Status != http.StatusBadRequest || len(err.Errors["cats"]) == 0 {
		t.Fatalf("bad override = %+v", err)
	}
	// cats is bounded like types and filter: one short error instead of an error line per unknown entry.
	rec = e.get(t, e.sub("&flag=surge&cats="+strings.Repeat("x,", 3000)), nil)
	if err := testutil.DecodeError(t, rec); err.Status != http.StatusBadRequest ||
		!slices.Equal(err.Errors["cats"], []string{"at most 64 entries"}) || rec.Body.Len() > 512 {
		t.Fatalf("oversized cats = %d bytes %+v", rec.Body.Len(), err)
	}
	// An unknown ?profile= (neither one of the user's slugs nor a template) selects the default profile.
	body = e.get(t, e.sub("&flag=clash-meta&profile=nope"), nil).Body.String()
	if !strings.Contains(body, "- name: Fast") {
		t.Fatalf("unknown profile must fall back to the default profile:\n%.1500s", body)
	}

	// Customization disabled: everyone gets the default template, overrides are ignored.
	if err := e.a.Settings.Set(map[string]any{"allow_user_rule_profile": false, "ruleset_default_template": "standard"}); err != nil {
		t.Fatal(err)
	}
	body = e.get(t, e.sub("&flag=clash-meta&profile=none&cats=x"), nil).Body.String()
	if !strings.Contains(body, "name: AI") || strings.Contains(body, "name: Fast") || !strings.Contains(body, "rule-providers") {
		t.Fatalf("forced template:\n%.1500s", body)
	}
}

func TestUnavailableUsers(t *testing.T) {
	e := setup(t, nil)
	past := time.Now().Add(-time.Hour)
	if err := e.a.DB.Model(&model.User{}).Where("id = ?", e.u.ID).Update("expired_at", past).Error; err != nil {
		t.Fatal(err)
	}
	body := decoded(t, e.get(t, e.sub("&flag=v2ray"), nil))
	if lines := strings.Split(strings.TrimSpace(body), "\n"); len(lines) != 3 || !strings.Contains(body, "%E5%A5%97%E9%A4%90%E5%B7%B2%E8%BF%87%E6%9C%9F") {
		t.Fatalf("expired user = %s", body)
	}
	if err := e.a.Settings.Set(map[string]any{"show_info_to_server_enable": false, "default_locale": "en"}); err != nil {
		t.Fatal(err)
	}
	rec := e.get(t, e.sub("&flag=clash-meta"), nil)
	var doc struct {
		Proxies []any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(rec.Body.Bytes(), &doc); err != nil || rec.Code != http.StatusOK || len(doc.Proxies) != 0 {
		t.Fatalf("no info: %d %v %d", rec.Code, err, len(doc.Proxies))
	}
	if err := e.a.Settings.Set(map[string]any{"show_info_to_server_enable": true}); err != nil {
		t.Fatal(err)
	}
	if body := decoded(t, e.get(t, e.sub("&flag=v2ray"), nil)); !strings.Contains(body, "Plan%20expired") {
		t.Fatalf("english info = %s", body)
	}
}
