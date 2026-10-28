package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// A request may pass both middleware role checks and then wait while another admin demotes it.
// Each user-management handler must reject it before changing or disclosing anything.
func TestUsersRejectInFlightAdminDemotion(t *testing.T) {
	tests := []struct {
		name, method, path string
		body               any
	}{
		{"create", http.MethodPost, "/users", map[string]any{"email": "new@example.com", "password": "password123"}},
		{"update", http.MethodPatch, "/users/target", map[string]any{"remarks": "changed", "password": "newpassword123"}},
		{"delete", http.MethodDelete, "/users/target", nil},
		{"reset-secret", http.MethodPost, "/users/target/reset-secret", nil},
		{"login-token", http.MethodPost, "/users/target/login-token", nil},
		{"traffic-reset", http.MethodPost, "/users/target/traffic-reset", nil},
		{"generate", http.MethodPost, "/users/generate", map[string]any{"email_suffix": "example.com", "email_prefix": "bulk", "count": 1}},
		{"mail", http.MethodPost, "/users/mail", map[string]any{"subject": "hello", "content": "secret"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			actor := testutil.CreateAdmin(t, a, "actor@example.com")
			target := testutil.CreateUser(t, a, "target@example.com")
			if err := a.DB.Model(target).UpdateColumns(map[string]any{"u": 123, "d": 456}).Error; err != nil {
				t.Fatal(err)
			}
			tok := testutil.Login(t, a, actor)
			entered, release := make(chan struct{}), make(chan struct{})
			r := gin.New()
			g := r.Group("/api/v1/admin", auth.RequireStaff(a), auth.RequireAdmin(a), func(c *gin.Context) {
				close(entered)
				<-release
				c.Next()
			})
			g.POST("/users", usersCreateHandler(a))
			g.PATCH("/users/:id", usersUpdateHandler(a))
			g.DELETE("/users/:id", usersDeleteHandler(a))
			g.POST("/users/:id/reset-secret", usersResetSecretHandler(a))
			g.POST("/users/:id/login-token", usersLoginTokenHandler(a))
			g.POST("/users/:id/traffic-reset", usersTrafficResetHandler(a))
			g.POST("/users/generate", usersGenerateHandler(a))
			g.POST("/users/mail", usersMailHandler(a))
			var body []byte
			if tt.body != nil {
				var err error
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatal(err)
				}
			}
			path := tt.path
			if tt.path == "/users/target" {
				path = "/users/" + strconv.FormatUint(target.ID, 10)
			} else if len(tt.path) > len("/users/target") && tt.path[:len("/users/target")] == "/users/target" {
				path = "/users/" + strconv.FormatUint(target.ID, 10) + tt.path[len("/users/target"):]
			}
			req := httptest.NewRequest(tt.method, "/api/v1/admin"+path, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+tok)
			if tt.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); r.ServeHTTP(rec, req) }()
			<-entered
			if err := a.DB.Model(&model.User{}).Where("id = ?", actor.ID).Update("is_admin", false).Error; err != nil {
				close(release)
				<-done
				t.Fatal(err)
			}
			close(release)
			<-done
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var current model.User
			if err := a.DB.First(&current, target.ID).Error; err != nil {
				t.Fatalf("target deleted: %v", err)
			}
			if current.Remarks != target.Remarks || current.PasswordVersion != target.PasswordVersion ||
				current.PasswordHash != target.PasswordHash || current.UUID != target.UUID || current.Token != target.Token ||
				current.U != 123 || current.D != 456 {
				t.Fatalf("target changed after demotion: %+v", current)
			}
			var count int64
			if err := a.DB.Model(&model.User{}).Count(&count).Error; err != nil || count != 2 {
				t.Fatalf("user count = %d, err = %v", count, err)
			}
		})
	}
}
