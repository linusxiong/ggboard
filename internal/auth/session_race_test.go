package auth_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestInvalidationDuringUserLoadRejectsOldAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change map[string]any
		path   string
		status int
	}{
		{"password", map[string]any{"password_version": 1}, "/user", http.StatusUnauthorized},
		{"email", map[string]any{"email": "renamed@example.com"}, "/user", http.StatusUnauthorized},
		{"ban", map[string]any{"banned": true}, "/user", http.StatusForbidden},
		{"role", map[string]any{"is_admin": false}, "/admin", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			u := testutil.CreateAdmin(t, a, "cache-race@example.com")
			r := auditRouter(t, a)
			oldToken := testutil.Login(t, a, u)
			loaded, resume := make(chan struct{}), make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(resume) }) }
			t.Cleanup(release)
			var paused atomic.Bool
			if err := a.DB.Callback().Query().After("gorm:query").Register("test:pause_auth_load", func(tx *gorm.DB) {
				if tx.Statement.Table == "users" && paused.CompareAndSwap(false, true) {
					close(loaded)
					<-resume
				}
			}); err != nil {
				t.Fatal(err)
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() { done <- testutil.Do(t, r, http.MethodGet, tc.path, nil, oldToken) }()
			select {
			case <-loaded:
			case <-time.After(5 * time.Second):
				t.Fatal("user load did not reach the barrier")
			}
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Updates(tc.change).Error; err != nil {
				t.Fatal(err)
			}
			auth.InvalidateUser(a, u.ID)
			release()
			select {
			case rec := <-done:
				testutil.ExpectStatus(t, rec, tc.status)
			case <-time.After(5 * time.Second):
				t.Fatal("authentication did not finish")
			}
			// A stale load must not restore the old cache for subsequent requests either.
			testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, tc.path, nil, oldToken), tc.status)
			if tc.name == "password" || tc.name == "email" {
				var fresh model.User
				if err := a.DB.First(&fresh, u.ID).Error; err != nil {
					t.Fatal(err)
				}
				testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/user", nil, testutil.Login(t, a, &fresh)), http.StatusOK)
			}
		})
	}
}
