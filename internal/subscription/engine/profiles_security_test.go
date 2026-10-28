package engine_test

import (
	"errors"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestProfileWritesRejectStaleAccountSnapshot(t *testing.T) {
	for name, change := range map[string]map[string]any{
		"password reset":   {"password_version": 1},
		"email change":     {"email": "new@example.com"},
		"account replaced": {"uuid": "00000000-0000-0000-0000-000000000002"},
		"ban":              {"banned": true},
	} {
		t.Run(name, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			u := testutil.CreateUser(t, a, "alice@example.com")
			ctx := t.Context()
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(change).Error; err != nil {
				t.Fatal(err)
			}
			if err := engine.EnsureDefault(ctx, a, u.ID, u); !errors.Is(err, engine.ErrProfileSessionExpired) {
				t.Fatalf("ensure default: %v", err)
			}
			if err := engine.SaveProfileSlug(ctx, a, u.ID, "", []byte(`{}`), u); !errors.Is(err, engine.ErrProfileSessionExpired) {
				t.Fatalf("save: %v", err)
			}
			if _, err := engine.CreateProfile(ctx, a, u.ID, "Work", "work", "", u); !errors.Is(err, engine.ErrProfileSessionExpired) {
				t.Fatalf("create: %v", err)
			}
			if _, err := engine.UpdateProfileMeta(ctx, a, u.ID, "default", nil, nil, u); !errors.Is(err, engine.ErrProfileSessionExpired) {
				t.Fatalf("rename: %v", err)
			}
			if err := engine.DeleteProfile(ctx, a, u.ID, "work", u); !errors.Is(err, engine.ErrProfileSessionExpired) {
				t.Fatalf("delete: %v", err)
			}
			if _, err := engine.SetDefaultProfile(ctx, a, u.ID, "work", u); !errors.Is(err, engine.ErrProfileSessionExpired) {
				t.Fatalf("set default: %v", err)
			}
			var n int64
			if err := a.DB.Model(&model.RuleProfile{}).Where("user_id = ?", u.ID).Count(&n).Error; err != nil || n != 0 {
				t.Fatalf("profiles changed: %d, %v", n, err)
			}
		})
	}
}
