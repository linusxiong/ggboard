package user_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestSubguardAgeKeyEndpoints(t *testing.T) {
	a, r := setup(t)
	u := testutil.CreateUser(t, a, "age@example.com")
	tok := testutil.Login(t, a, u)
	path := base + "/subscription/age"

	v := testutil.Decode[guard.KeyView](t, testutil.Do(t, r, http.MethodGet, path, nil, tok))
	if v.PublicKey != nil || v.PrivateKey != nil || v.CreatedAt != nil || v.Enabled || len(v.Formats) != 1 || !v.Optional || v.Strict {
		t.Fatalf("empty = %+v", v)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, path, map[string]any{"enabled": true}, tok), http.StatusNotFound)

	v = testutil.Decode[guard.KeyView](t, testutil.Do(t, r, http.MethodPost, path, nil, tok))
	if v.PublicKey == nil || v.PrivateKey == nil || v.CreatedAt == nil || v.Enabled {
		t.Fatalf("created = %+v", v)
	}
	id, err := age.ParseX25519Identity(*v.PrivateKey)
	if err != nil || id.Recipient().String() != *v.PublicKey {
		t.Fatalf("key pair mismatch: %v", err)
	}
	// Shown again on GET (owner only).
	again := testutil.Decode[guard.KeyView](t, testutil.Do(t, r, http.MethodGet, path, nil, tok))
	if again.PrivateKey == nil || *again.PrivateKey != *v.PrivateKey {
		t.Fatal("private key not shown again")
	}
	e := testutil.DecodeError(t, testutil.Do(t, r, http.MethodPost, path, map[string]any{}, tok))
	if e.Status != http.StatusConflict {
		t.Fatalf("second create = %+v", e)
	}
	on := testutil.Decode[guard.KeyView](t, testutil.Do(t, r, http.MethodPatch, path, map[string]any{"enabled": true}, tok))
	if !on.Enabled {
		t.Fatal("not enabled")
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, path, map[string]any{}, tok), http.StatusBadRequest)
	re := testutil.Decode[guard.KeyView](t, testutil.Do(t, r, http.MethodPost, path, map[string]any{"regenerate": true}, tok))
	if !re.Enabled || *re.PublicKey == *v.PublicKey {
		t.Fatalf("regenerated = %+v", re)
	}
	if ok := testutil.Decode[bool](t, testutil.Do(t, r, http.MethodDelete, path, nil, tok)); !ok {
		t.Fatal("delete")
	}
	if v := testutil.Decode[guard.KeyView](t, testutil.Do(t, r, http.MethodGet, path, nil, tok)); v.PublicKey != nil {
		t.Fatal("key still there")
	}
	// Created enabled in one call.
	if v := testutil.Decode[guard.KeyView](t, testutil.Do(t, r, http.MethodPost, path, map[string]any{"enabled": true}, tok)); !v.Enabled {
		t.Fatal("enabled on create")
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, path, nil, ""), http.StatusUnauthorized)
}

func TestSubguardAccessLogAndRotate(t *testing.T) {
	a, r := setup(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	u := testutil.CreateUser(t, a, "log@example.com")
	other := testutil.CreateUser(t, a, "other@example.com")
	tok := testutil.Login(t, a, u)
	for i := range 60 {
		guard.Record(a, guard.Access{UserID: u.ID, Token: u.Token, IP: "203.0.113.9", UserAgent: "mihomo/1.19.27",
			Format: "clash-meta", At: time.Now().Add(time.Duration(i-60) * time.Minute)})
	}
	guard.Record(a, guard.Access{UserID: u.ID, Token: u.Token, IP: "2001:db8:7:8::9", UserAgent: "FlClash/0.9.1", Format: "clash-meta"})
	guard.Record(a, guard.Access{UserID: other.ID, Token: other.Token, IP: "192.0.2.1", Format: "v2ray"})

	// Pending rows are flushed by the endpoint itself.
	type entry struct {
		IP, Client, Format string
		IPBucket           string `json:"ip_bucket"`
		At                 time.Time
	}
	list := testutil.Decode[[]entry](t, testutil.Do(t, r, http.MethodGet, base+"/subscription/access-log", nil, tok))
	if len(list) != guard.UserLogLimit || list[0].IP != "2001:db8:7:8:*" || list[0].Client != "FlClash/0.9.1" ||
		list[1].IP != "203.0.113.*" || list[0].IPBucket != "" || !list[0].At.After(list[1].At) {
		t.Fatalf("access log = %+v", list[:2])
	}

	// rotate-token: new token only, sessions and uuid kept.
	res := testutil.Decode[user.TokenRotation](t, testutil.Do(t, r, http.MethodPost, base+"/subscription/rotate-token", nil, tok))
	got := reload(t, a, u.ID)
	if res.Token == u.Token || res.Token != got.Token || got.UUID != u.UUID || got.PasswordVersion != u.PasswordVersion ||
		res.SubscribeURL != "https://panel.example.com"+usersvc.SubscribePath+got.Token {
		t.Fatalf("rotate = %+v stored = %+v", res, got)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, base+"/me", nil, tok), http.StatusOK)
	// The legacy reset (uuid + token) still exists.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/subscription/reset", nil, tok), http.StatusOK)
}

type profileSummary struct {
	ID         uint64 `json:"id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	IsDefault  bool   `json:"is_default"`
	Customized bool   `json:"customized"`
}

type slugProfileView struct {
	subscription.RuleProfile
	Customized bool                     `json:"customized"`
	Effective  subscription.RuleProfile `json:"effective"`
	ID         uint64                   `json:"id"`
	Slug       string                   `json:"slug"`
	Name       string                   `json:"name"`
	IsDefault  bool                     `json:"is_default"`
}

func TestRuleProfilesCRUD(t *testing.T) {
	a, r := setup(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	u := testutil.CreateUser(t, a, "profiles@example.com")
	tok := testutil.Login(t, a, u)
	p := base + "/rule-profiles"

	// /user/subscription lists the virtual default profile before anything is stored.
	sub := testutil.Decode[user.Subscription](t, testutil.Do(t, r, http.MethodGet, base+"/subscription", nil, tok))
	if len(sub.Profiles) != 1 || sub.Profiles[0].Slug != model.RuleProfileDefaultSlug || sub.Profiles[0].SubscribeURL != sub.SubscribeURL {
		t.Fatalf("virtual default = %+v", sub.Profiles)
	}

	// The list creates the default row.
	list := testutil.Decode[[]profileSummary](t, testutil.Do(t, r, http.MethodGet, p, nil, tok))
	if len(list) != 1 || list[0].Slug != "default" || list[0].Name != "默认" || !list[0].IsDefault || list[0].Customized || list[0].ID == 0 {
		t.Fatalf("list = %+v", list)
	}

	// Alias: PUT /user/rule-profile writes the default profile.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPut, base+"/rule-profile", map[string]any{"template": "minimal"}, tok), http.StatusOK)
	v := testutil.Decode[slugProfileView](t, testutil.Do(t, r, http.MethodGet, p+"/default", nil, tok))
	if !v.Customized || v.Template != "minimal" || !v.IsDefault || v.Slug != "default" {
		t.Fatalf("default view = %+v", v)
	}

	// Create: derived slug, explicit slug, copy.
	rec := testutil.Do(t, r, http.MethodPost, p, map[string]any{"name": "Work Laptop"}, tok)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	if s := testutil.Decode[profileSummary](t, rec); s.Slug != "work-laptop" || s.IsDefault || s.Customized {
		t.Fatalf("created = %+v", s)
	}
	s := testutil.Decode[profileSummary](t, testutil.Do(t, r, http.MethodPost, p, map[string]any{"name": "手机", "copy_from": "default"}, tok))
	if s.Slug != "profile-2" || !s.Customized {
		t.Fatalf("copied = %+v", s)
	}
	if v := testutil.Decode[slugProfileView](t, testutil.Do(t, r, http.MethodGet, p+"/profile-2", nil, tok)); v.Template != "minimal" || v.Name != "手机" {
		t.Fatalf("copy view = %+v", v)
	}
	for _, bad := range []struct {
		body   map[string]any
		status int
		field  string
	}{
		{map[string]any{"name": ""}, http.StatusBadRequest, "name"},
		{map[string]any{"name": "x", "slug": "Bad Slug"}, http.StatusBadRequest, "slug"},
		{map[string]any{"name": "x", "slug": "work-laptop"}, http.StatusConflict, "slug"},
		{map[string]any{"name": "x", "copy_from": "nope"}, http.StatusBadRequest, "copy_from"},
	} {
		e := testutil.DecodeError(t, testutil.Do(t, r, http.MethodPost, p, bad.body, tok))
		if e.Status != bad.status || len(e.Errors[bad.field]) == 0 {
			t.Errorf("%v → %+v", bad.body, e)
		}
	}

	// PUT for a slug: same semantics as the alias, with node operations.
	rec = testutil.Do(t, r, http.MethodPut, p+"/work-laptop", map[string]any{"template": "full", "nodes": map[string]any{
		"regions": []string{"JP"}, "sort": "name", "rename": []any{map[string]any{"match": "(", "replace": ""}},
	}}, tok)
	if e := testutil.DecodeError(t, rec); e.Status != http.StatusBadRequest || len(e.Errors["nodes.rename[0].match"]) == 0 {
		t.Fatalf("node ops validation = %+v", e)
	}
	v = testutil.Decode[slugProfileView](t, testutil.Do(t, r, http.MethodPut, p+"/work-laptop", map[string]any{"template": "full",
		"nodes": map[string]any{"regions": []string{"JP"}, "sort": "name"}}, tok))
	if v.Template != "full" || v.Nodes.Regions[0] != "JP" || v.Nodes.Emoji != "keep" || v.Effective.Template != "full" || v.IsDefault {
		t.Fatalf("put slug = %+v", v)
	}
	// A PUT without "nodes" keeps the stored node operations.
	v = testutil.Decode[slugProfileView](t, testutil.Do(t, r, http.MethodPut, p+"/work-laptop", map[string]any{"template": "full", "dns": "none"}, tok))
	if v.DNS != "none" || len(v.Nodes.Regions) != 1 || v.Nodes.Sort != "name" {
		t.Fatalf("nodes dropped = %+v", v.Nodes)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPut, p+"/nope", map[string]any{"template": "full"}, tok), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, p+"/nope", nil, tok), http.StatusNotFound)
	// The alias still shows the default profile.
	if v := testutil.Decode[slugProfileView](t, testutil.Do(t, r, http.MethodGet, base+"/rule-profile", nil, tok)); v.Template != "minimal" {
		t.Fatalf("alias = %+v", v.Template)
	}

	// Rename / re-slug.
	s = testutil.Decode[profileSummary](t, testutil.Do(t, r, http.MethodPatch, p+"/profile-2", map[string]any{"name": "Phone", "slug": "phone"}, tok))
	if s.Slug != "phone" || s.Name != "Phone" {
		t.Fatalf("patched = %+v", s)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPatch, p+"/phone", map[string]any{"slug": "work-laptop"}, tok), http.StatusConflict)

	// Default switch; the default cannot be deleted.
	s = testutil.Decode[profileSummary](t, testutil.Do(t, r, http.MethodPost, p+"/work-laptop/default", nil, tok))
	if !s.IsDefault {
		t.Fatal("default not switched")
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodDelete, p+"/work-laptop", nil, tok), http.StatusConflict)
	if v := testutil.Decode[slugProfileView](t, testutil.Do(t, r, http.MethodGet, base+"/rule-profile", nil, tok)); v.Template != "full" || v.Slug != "work-laptop" {
		t.Fatalf("alias after default switch = %+v", v.Slug)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodDelete, p+"/default", nil, tok), http.StatusOK)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodDelete, p+"/default", nil, tok), http.StatusNotFound)

	// /user/subscription links: the default profile has the plain link.
	sub = testutil.Decode[user.Subscription](t, testutil.Do(t, r, http.MethodGet, base+"/subscription", nil, tok))
	if len(sub.Profiles) != 2 || sub.Profiles[0].Slug != "work-laptop" || sub.Profiles[0].SubscribeURL != sub.SubscribeURL ||
		sub.Profiles[1].SubscribeURL != sub.SubscribeURL+"&profile=phone" || !strings.HasPrefix(sub.SubscribeURL, "https://panel.example.com/") {
		t.Fatalf("subscription profiles = %+v", sub.Profiles)
	}

	// At most 10 profiles.
	for i := 3; i <= 10; i++ {
		testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, p, map[string]any{"name": "P"}, tok), http.StatusCreated)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, p, map[string]any{"name": "P"}, tok), http.StatusConflict)

	// Customization disabled: writes are refused, reads still work.
	if err := a.Settings.Set(map[string]any{"allow_user_rule_profile": false}); err != nil {
		t.Fatal(err)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, p+"/phone/default", nil, tok), http.StatusForbidden)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, p, nil, tok), http.StatusOK)
}
