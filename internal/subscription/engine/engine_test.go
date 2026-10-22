package engine_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestOverridesFromQuery(t *testing.T) {
	q, _ := url.ParseQuery("profile=full&source=metacubex&cats=openai%2Ctelegram&final=DIRECT&mirror=1")
	ov := engine.OverridesFromQuery(q)
	if ov.Template != "full" || ov.Source != "metacubex" || !ov.HasCats || len(ov.Cats) != 2 || ov.Final != "DIRECT" || ov.Mirror != "1" {
		t.Fatalf("overrides = %+v", ov)
	}
	if !engine.OverridesFromQuery(url.Values{}).IsZero() {
		t.Fatal("empty query")
	}
	if ov := engine.OverridesFromQuery(url.Values{"cats": {""}}); !ov.HasCats || len(ov.Cats) != 0 {
		t.Fatal("cats= present but empty disables every category")
	}
}

func TestStoredProfiles(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, a, "alice@example.com")
	c := rulesets.Current(a.Settings)
	if p, err := engine.StoredProfile(ctx, a, c, u.ID); err != nil || p != nil {
		t.Fatalf("none stored = %v %v", p, err)
	}
	if err := engine.SaveProfile(ctx, a, u.ID, []byte(`{"template":"full","dns":"none"}`)); err != nil {
		t.Fatal(err)
	}
	p, err := engine.StoredProfile(ctx, a, c, u.ID)
	if err != nil || p == nil || p.Template != "full" || p.DNS != subscription.DNSNone || len(p.Categories) != len(c.Categories) {
		t.Fatalf("stored = %+v %v", p, err)
	}
	// Upsert replaces the row; a broken row falls back to the defaults instead of failing subscriptions.
	if err := engine.SaveProfile(ctx, a, u.ID, []byte(`{"template":"minimal"}`)); err != nil {
		t.Fatal(err)
	}
	var n int64
	a.DB.Model(&model.RuleProfile{}).Count(&n)
	if p, _ := engine.StoredProfile(ctx, a, c, u.ID); n != 1 || p.Template != "minimal" {
		t.Fatalf("upsert: rows=%d", n)
	}
	if err := a.DB.Model(&model.RuleProfile{}).Where("user_id = ?", u.ID).Update("data", `{"template":"nope"}`).Error; err != nil {
		t.Fatal(err)
	}
	a.Cache.InvalidateNamespace(engine.ProfileNamespace)
	if p, err := engine.StoredProfile(ctx, a, c, u.ID); err != nil || p != nil {
		t.Fatalf("broken = %+v %v", p, err)
	}
	if p, err := engine.EffectiveProfile(ctx, a, c, u, rulesets.Overrides{}); err != nil || p.Template != "standard" {
		t.Fatalf("effective = %+v %v", p, err)
	}
}

func TestGenerateNilUser(t *testing.T) {
	a := testutil.NewTestApp(t)
	if _, err := engine.Generate(context.Background(), a, nil, engine.Request{}); err == nil {
		t.Fatal("nil user")
	}
}

func TestRenderWithoutGroups(t *testing.T) {
	f, _ := subscription.GetFormat(subscription.FormatClashMeta)
	res, err := engine.Render(f, engine.Input{User: &model.User{}, Catalog: rulesets.Builtin(), Profile: subscription.RuleProfile{Final: "Proxy"}})
	if err != nil || !strings.Contains(string(res.Body), "- name: Proxy") || !strings.Contains(string(res.Body), "MATCH,Proxy") {
		t.Fatalf("%s %v", res.Body, err)
	}
}
