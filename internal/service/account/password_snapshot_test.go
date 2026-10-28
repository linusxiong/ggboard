package account_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/service/account"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestChangePasswordRejectsRevokedUserSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change map[string]any
	}{
		{"version", map[string]any{"password_version": 1}},
		{"email", map[string]any{"email": "new-owner@example.com"}},
		{"account instance", map[string]any{"uuid": "10000000-0000-4000-8000-000000000004"}},
		{"ban", map[string]any{"banned": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			u := testutil.CreateUser(t, a, "password-snapshot@example.com")
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(tc.change).Error; err != nil {
				t.Fatal(err)
			}
			_, err := account.New(a).ChangePassword(context.Background(), u, testutil.Password, "must-not-be-stored")
			if err == nil || httpx.AsError(err).Status != http.StatusUnauthorized {
				t.Fatalf("revoked password change = %v", err)
			}
			var fresh model.User
			if err := a.DB.First(&fresh, u.ID).Error; err != nil {
				t.Fatal(err)
			}
			if fresh.PasswordHash != u.PasswordHash {
				t.Fatal("a revoked request replaced the current password")
			}
		})
	}
}
