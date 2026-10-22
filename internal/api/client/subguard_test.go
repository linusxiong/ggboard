package client_test

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"
	"gopkg.in/yaml.v3"

	"github.com/linusxiong/ggboard/internal/api/client"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func (e env) from(t *testing.T, method, path, ip string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = net.JoinHostPort(ip, "40000")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.r.ServeHTTP(rec, req)
	return rec
}

func ageOpen(t *testing.T, body []byte, identity string) []byte {
	t.Helper()
	id, err := age.ParseX25519Identity(identity)
	if err != nil {
		t.Fatal(err)
	}
	r, err := age.Decrypt(armor.NewReader(bytes.NewReader(body)), id)
	if err != nil {
		t.Fatalf("decrypt: %v\n%.200s", err, body)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAgeEncryptedSubscription(t *testing.T) {
	e := setup(t, nil)
	meta := map[string]string{"User-Agent": "mihomo/1.19.27"}

	// No key: plaintext.
	rec := e.get(t, e.sub(""), meta)
	if !strings.HasPrefix(rec.Body.String(), "mixed-port") || rec.Header().Get(guard.EncryptionHeader) != "" {
		t.Fatalf("plaintext expected:\n%.100s", rec.Body.String())
	}
	plainTag := strings.Trim(rec.Header().Get("ETag"), `"`)

	_, priv, err := guard.GenerateKey(t.Context(), e.a, e.u.ID, false, testutil.Ptr(true))
	if err != nil {
		t.Fatal(err)
	}
	k, _ := guard.LoadKey(t.Context(), e.a, e.u.ID)

	rec = e.get(t, e.sub(""), meta)
	testutil.ExpectStatus(t, rec, http.StatusOK)
	body := rec.Body.Bytes()
	if !bytes.HasPrefix(body, []byte("-----BEGIN AGE ENCRYPTED FILE-----\n")) {
		t.Fatalf("not age armored:\n%.200s", body)
	}
	h := rec.Header()
	if !strings.HasPrefix(h.Get("Content-Type"), "text/plain") || h.Get(guard.EncryptionHeader) != "age" ||
		!strings.HasPrefix(h.Get("Subscription-Userinfo"), "upload=0; download=0; total=") || h.Get("Content-Disposition") == "" {
		t.Fatalf("headers = %v", h)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(ageOpen(t, body, priv), &doc); err != nil || doc["proxies"] == nil {
		t.Fatalf("decrypted body is not the config: %v", err)
	}
	tag := strings.Trim(h.Get("ETag"), `"`)
	if tag != plainTag+"-age-"+guard.KeyTag(k.PublicKey) {
		t.Fatalf("etag %s (plain %s)", tag, plainTag)
	}
	// 304 for the encrypted tag only; the plaintext tag gets a new (encrypted) body.
	if rec := e.get(t, e.sub(""), map[string]string{"User-Agent": "mihomo/1.19.27", "If-None-Match": `"` + tag + `"`}); rec.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match = %d", rec.Code)
	}
	if rec := e.get(t, e.sub(""), map[string]string{"User-Agent": "mihomo/1.19.27", "If-None-Match": `W/"` + plainTag + `"`}); rec.Code != http.StatusOK {
		t.Fatalf("plaintext etag must not match: %d", rec.Code)
	}
	// HEAD works (clients read the traffic header).
	if rec := e.from(t, http.MethodHead, e.sub(""), "192.0.2.10", meta); rec.Code != http.StatusOK || rec.Header().Get("Subscription-Userinfo") == "" {
		t.Fatalf("HEAD = %d", rec.Code)
	}

	// ?age=0 forces plaintext while subscribe_age_optional is on.
	if body := e.get(t, e.sub("&age=0"), meta).Body.String(); !strings.HasPrefix(body, "mixed-port") {
		t.Fatalf("age=0:\n%.100s", body)
	}
	if err := e.a.Settings.Set(map[string]any{"subscribe_age_optional": false}); err != nil {
		t.Fatal(err)
	}
	if body := e.get(t, e.sub("&age=0"), meta).Body.String(); !strings.HasPrefix(body, "-----BEGIN AGE") {
		t.Fatalf("age=0 must be ignored when not optional:\n%.100s", body)
	}

	// Formats without age support: plaintext, or 406 with subscribe_age_strict.
	if body := decoded(t, e.get(t, e.sub(""), map[string]string{"User-Agent": "v2rayN/7.0"})); !strings.HasPrefix(body, "ss://") {
		t.Fatalf("v2ray plaintext:\n%.100s", body)
	}
	if err := e.a.Settings.Set(map[string]any{"subscribe_age_strict": true}); err != nil {
		t.Fatal(err)
	}
	rec = e.get(t, e.sub("&flag=stash"), nil)
	if err := testutil.DecodeError(t, rec); err.Status != http.StatusNotAcceptable || err.Code != "not_acceptable" || !strings.Contains(err.Message, "age") {
		t.Fatalf("strict = %+v", err)
	}
	if rec.Header().Get("Subscription-Userinfo") != "" {
		t.Fatal("a refused subscription must not leak the traffic header")
	}
	if err := e.a.Settings.Set(map[string]any{"subscribe_age_optional": true}); err != nil {
		t.Fatal(err)
	}
	if rec := e.get(t, e.sub("&flag=stash&age=0"), nil); rec.Code != http.StatusOK {
		t.Fatalf("strict + optional + age=0 = %d", rec.Code)
	}

	// Disabled key: plaintext again.
	if _, err := guard.SetKeyEnabled(t.Context(), e.a, e.u.ID, false); err != nil {
		t.Fatal(err)
	}
	if rec := e.get(t, e.sub("&flag=stash"), nil); rec.Code != http.StatusOK || strings.HasPrefix(rec.Body.String(), "-----BEGIN") {
		t.Fatalf("disabled key = %d", rec.Code)
	}
}

func TestSubscribeAccessLog(t *testing.T) {
	e := setup(t, nil)
	e.from(t, http.MethodGet, e.sub(""), "198.51.100.7", map[string]string{"User-Agent": "clash-verge/v2.2.3"})
	e.from(t, http.MethodGet, "/s/"+e.u.Token, "2001:db8:5::1", map[string]string{"User-Agent": "v2rayN/7.0"})
	e.from(t, http.MethodGet, "/s/nope", "203.0.113.1", nil) // unknown token: not logged
	if err := guard.FlushAccess(t.Context(), e.a); err != nil {
		t.Fatal(err)
	}
	var rows []model.SubscriptionAccess
	e.a.DB.Order("id").Find(&rows)
	if len(rows) != 2 || rows[0].IP != "198.51.100.7" || rows[0].Client != "clash-verge/v2.2.3" || rows[0].Format != "clash-meta" ||
		rows[1].IPBucket != "2001:db8:5::/64" || rows[1].Format != "v2ray" || rows[0].TokenTag != guard.TokenTag(e.u.Token) ||
		rows[0].At.IsZero() || time.Since(rows[0].At) > time.Minute {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestSubscribeProfileSelection(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	if err := engine.SaveProfile(ctx, e.a, e.u.ID, []byte(`{"template":"none"}`)); err != nil {
		t.Fatal(err)
	}
	work, err := engine.CreateProfile(ctx, e.a, e.u.ID, "Work", "", "")
	if err != nil || work.Slug != "work" {
		t.Fatalf("create = %+v %v", work, err)
	}
	raw := []byte(`{"template":"minimal","nodes":{"regions":["JP"],"rename":[{"match":"^JP ","replace":"日本 "}]}}`)
	if err := engine.SaveProfileSlug(ctx, e.a, e.u.ID, "work", raw); err != nil {
		t.Fatal(err)
	}
	def := e.get(t, e.sub("&flag=clash-meta"), nil).Body.String()
	if strings.Contains(def, "rule-providers") || !strings.Contains(def, "HK VMess WS") {
		t.Fatalf("default profile (template none):\n%.800s", def)
	}
	w := e.get(t, e.sub("&flag=clash-meta&profile=work"), nil).Body.String()
	if !strings.Contains(w, "rule-providers") || strings.Contains(w, "HK VMess WS") || !strings.Contains(w, `name: "日本 VMess TCP"`) ||
		!strings.Contains(w, "剩余流量") {
		t.Fatalf("work profile:\n%.1500s", w)
	}
	// Unknown slugs fall back to the default profile; template ids keep their old meaning.
	if body := e.get(t, e.sub("&flag=clash-meta&profile=gone"), nil).Body.String(); body != def {
		t.Fatal("unknown slug must render the default profile")
	}
	if body := e.get(t, e.sub("&flag=clash-meta&profile=standard"), nil).Body.String(); !strings.Contains(body, "rule-providers") {
		t.Fatal("template override")
	}
	if _, err := engine.SetDefaultProfile(ctx, e.a, e.u.ID, "work"); err != nil {
		t.Fatal(err)
	}
	if body := e.get(t, e.sub("&flag=clash-meta"), nil).Body.String(); body != w {
		t.Fatal("the new default profile must be used without ?profile=")
	}
}

func TestETagMatches(t *testing.T) {
	cases := []struct {
		inm, tag string
		want     bool
	}{
		{`"abc"`, "abc", true}, {`W/"abc"`, "abc", true}, {`abc`, "abc", true}, {`"x", W/"abc"`, "abc", true},
		{`*`, "abc", true}, {`"abc-age-1"`, "abc", false}, {`"abc"`, "abc-age-1", false}, {``, "abc", false},
	}
	for _, c := range cases {
		if got := client.ETagMatches(c.inm, c.tag); got != c.want {
			t.Errorf("ETagMatches(%q, %q) = %v", c.inm, c.tag, got)
		}
	}
}
