package passport_test

import (
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/passport"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestLoginTokenCannotAdoptChangedAccount(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change map[string]any
	}{
		{"password", map[string]any{"password_version": 1}},
		{"email", map[string]any{"email": "new-owner@example.com"}},
		{"account instance", map[string]any{"uuid": "10000000-0000-4000-8000-000000000002"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			u := testutil.CreateUser(t, a, "one-time@example.com")
			r := testutil.NewRouterWith(t, a, passport.Register)
			token := auth.IssueLoginToken(a, u)
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(tc.change).Error; err != nil {
				t.Fatal(err)
			}
			testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/passport/login/token", map[string]any{"token": token}, ""), http.StatusUnauthorized)
			var fresh model.User
			if err := a.DB.First(&fresh, u.ID).Error; err != nil {
				t.Fatal(err)
			}
			freshToken := auth.IssueLoginToken(a, &fresh)
			testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/passport/login/token", map[string]any{"token": freshToken}, ""), http.StatusOK)
		})
	}
}
