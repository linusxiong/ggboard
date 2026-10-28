package user

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestResetSubscriptionRejectsSessionChangedAfterMiddleware(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "reset-race@example.com")
	token := testutil.Login(t, a, u)
	oldUUID, oldToken := u.UUID, u.Token

	authenticated, resume := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(resume) }) }
	t.Cleanup(release)

	r := gin.New()
	r.POST("/reset", auth.RequireUser(a), func(c *gin.Context) {
		close(authenticated)
		<-resume
		resetSubscription(a)(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/reset", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		r.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-authenticated:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not pass authentication")
	}
	if err := usersvc.New(a).SetPassword(t.Context(), u, "new-password-1"); err != nil {
		t.Fatal(err)
	}
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not finish")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reset after password change = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}

	var stored model.User
	if err := a.DB.First(&stored, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PasswordVersion != 1 || stored.UUID != oldUUID || stored.Token != oldToken {
		t.Fatalf("stale request changed credentials: %+v", stored)
	}
}

func TestAgeReadRejectsAccountReplacedAfterMiddleware(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "age-read-race@example.com")
	if _, _, err := guard.GenerateKey(t.Context(), a, u.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	token := testutil.Login(t, a, u)
	authenticated, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(resume) }) }
	t.Cleanup(release)
	r := gin.New()
	r.GET("/age", auth.RequireUser(a), func(c *gin.Context) {
		close(authenticated)
		<-resume
		getAgeKey(a)(c)
	})
	req := httptest.NewRequest(http.MethodGet, "/age", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		r.ServeHTTP(rec, req)
		close(done)
	}()
	select {
	case <-authenticated:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not pass authentication")
	}
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("uuid", "replacement-account-uuid").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := guard.GenerateKey(t.Context(), a, u.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not finish")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old account received age key: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
