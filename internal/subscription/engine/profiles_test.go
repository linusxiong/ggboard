package engine_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/subscription/rulesets"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestProfileStore(t *testing.T) {
	a := testutil.NewTestApp(t)
	ctx := t.Context()
	u := testutil.CreateUser(t, a, "alice@example.com")
	c := rulesets.Current(a.Settings)

	// Virtual default: no rows, "" and "default" resolve to nothing stored; other slugs are unknown.
	if row, p, err := engine.StoredProfileSlug(ctx, a, c, u.ID, "default"); row != nil || p != nil || err != nil {
		t.Fatalf("virtual default = %v %v %v", row, p, err)
	}
	if _, _, err := engine.StoredProfileSlug(ctx, a, c, u.ID, "work"); !errors.Is(err, engine.ErrProfileNotFound) {
		t.Fatalf("unknown slug = %v", err)
	}
	if err := engine.SaveProfileSlug(ctx, a, u.ID, "work", []byte(`{}`)); !errors.Is(err, engine.ErrProfileNotFound) {
		t.Fatalf("save unknown = %v", err)
	}

	// Creating a profile first creates the default, so the new one never becomes the default by accident.
	names := map[string]string{
		"Work Laptop": "work-laptop", "Work Laptop ": "work-laptop-2", "手机": "profile-2", "!!Home  Office--2026": "home-office-2026",
		strings.Repeat("x", 60): strings.Repeat("x", 28),
	}
	for _, name := range []string{"Work Laptop", "Work Laptop ", "手机", "!!Home  Office--2026", strings.Repeat("x", 60)} {
		r, err := engine.CreateProfile(ctx, a, u.ID, name, "", "")
		if err != nil || r.Slug != names[name] || r.IsDefault {
			t.Fatalf("create %q = %+v %v", name, r, err)
		}
	}
	rows, err := engine.LoadProfiles(ctx, a, u.ID)
	if err != nil || len(rows) != 6 || rows[0].Slug != model.RuleProfileDefaultSlug || engine.DefaultOf(rows).ID != rows[0].ID {
		t.Fatalf("rows = %+v %v", rows, err)
	}
	if _, err := engine.CreateProfile(ctx, a, u.ID, "x", "work-laptop", ""); !errors.Is(err, engine.ErrSlugTaken) {
		t.Fatalf("taken = %v", err)
	}
	if err := engine.DeleteProfile(ctx, a, u.ID, "default"); !errors.Is(err, engine.ErrDefaultProfile) {
		t.Fatalf("delete default = %v", err)
	}

	// Concurrent creates never exceed the limit.
	var wg sync.WaitGroup
	var mu sync.Mutex
	limited := 0
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := engine.CreateProfile(ctx, a, u.ID, "P", "", ""); errors.Is(err, engine.ErrProfileLimit) {
				mu.Lock()
				limited++
				mu.Unlock()
			} else if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var n int64
	a.DB.Model(&model.RuleProfile{}).Where("user_id = ?", u.ID).Count(&n)
	if n != model.MaxRuleProfiles || limited != 4 {
		t.Fatalf("rows = %d, refused = %d", n, limited)
	}

	// ?profile= resolution: slug, then template id, else the default profile.
	if err := engine.SaveProfileSlug(ctx, a, u.ID, "work-laptop", []byte(`{"template":"full"}`)); err != nil {
		t.Fatal(err)
	}
	if err := engine.SaveProfile(ctx, a, u.ID, []byte(`{"template":"minimal"}`)); err != nil {
		t.Fatal(err)
	}
	for q, want := range map[string]string{"": "minimal", "work-laptop": "full", "none": "none", "gone": "minimal", "home-office-2026": "standard"} {
		p, err := engine.EffectiveProfile(ctx, a, c, u, rulesets.Overrides{Template: q})
		if err != nil || p.Template != want {
			t.Errorf("profile=%q → %q %v, want %q", q, p.Template, err, want)
		}
	}
	if _, err := engine.SetDefaultProfile(ctx, a, u.ID, "work-laptop"); err != nil {
		t.Fatal(err)
	}
	rows, _ = engine.LoadProfiles(ctx, a, u.ID)
	defaults := 0
	for _, r := range rows {
		if r.IsDefault {
			defaults++
		}
	}
	if defaults != 1 || engine.DefaultOf(rows).Slug != "work-laptop" {
		t.Fatalf("default = %+v", engine.DefaultOf(rows))
	}
	if raw, _ := engine.LoadStoredProfile(ctx, a, u.ID); !strings.Contains(string(raw), "full") {
		t.Fatalf("default raw = %s", raw)
	}
	if err := engine.DeleteProfile(ctx, a, u.ID, "default"); err != nil {
		t.Fatal(err)
	}
	newSlug := "main"
	r, err := engine.UpdateProfileMeta(ctx, a, u.ID, "work-laptop", nil, &newSlug)
	if err != nil || r.Slug != "main" || !r.IsDefault {
		t.Fatalf("re-slug = %+v %v", r, err)
	}
}
