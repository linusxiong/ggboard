package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/subscription/fixture"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestSubscriptionRejectsRevocationAfterRendering(t *testing.T) {
	mutations := map[string]func(*gorm.DB, *model.User) error{
		"token": func(db *gorm.DB, u *model.User) error {
			return db.Model(u).Update("token", strings.Repeat("a", 32)).Error
		},
		"banned":  func(db *gorm.DB, u *model.User) error { return db.Model(u).Update("banned", true).Error },
		"quota":   func(db *gorm.DB, u *model.User) error { return db.Model(u).Update("transfer_enable", 0).Error },
		"traffic": func(db *gorm.DB, u *model.User) error { return db.Model(u).Update("u", u.TransferEnable).Error },
		"group":   func(db *gorm.DB, u *model.User) error { return db.Model(u).Update("group_id", nil).Error },
		"plan":    func(db *gorm.DB, u *model.User) error { return db.Model(u).Update("plan_id", nil).Error },
		"expiry": func(db *gorm.DB, u *model.User) error {
			return db.Model(u).Update("expired_at", time.Now().Add(-time.Minute)).Error
		},
		"account": func(db *gorm.DB, u *model.User) error {
			return db.Model(u).Update("uuid", "00000000-0000-0000-0000-000000000002").Error
		},
	}
	for _, route := range []string{"query", "short"} {
		for name, mutate := range mutations {
			t.Run(route+"/"+name, func(t *testing.T) {
				a := testutil.NewTestApp(t)
				if _, err := fixture.SeedNodes(a.DB); err != nil {
					t.Fatal(err)
				}
				if err := a.DB.Create(&model.Plan{ID: 1, GroupID: fixture.GroupID, Name: "Test", TransferEnable: 100}).Error; err != nil {
					t.Fatal(err)
				}
				exp := time.Now().Add(time.Hour)
				gid := uint64(fixture.GroupID)
				pid := uint64(1)
				u := testutil.CreateUser(t, a, "alice@example.com", usersvc.CreateOptions{
					PlanID: &pid, GroupID: &gid, TransferEnable: 100 * fixture.GiB, ExpiredAt: &exp,
				})
				generated := make(chan int, 1)
				release := make(chan struct{})
				var releaseOnce sync.Once
				defer releaseOnce.Do(func() { close(release) })
				gen := func(ctx context.Context, appArg *app.App, user *model.User, req engine.Request) (*engine.Result, error) {
					res, err := engine.Generate(ctx, appArg, user, req)
					if err != nil {
						generated <- -1
						return nil, err
					}
					plain, decodeErr := base64.StdEncoding.DecodeString(string(res.Body))
					if decodeErr != nil || !bytes.Contains(plain, []byte("us1.example.com")) {
						generated <- -1
					} else {
						generated <- res.Proxies
					}
					<-release
					return res, nil
				}
				lim := &limits{
					token: httpx.NewLimiter(httpx.Per(SubscribeRate, time.Hour), SubscribeBurst),
					miss:  httpx.NewLimiter(httpx.Per(MissRate, time.Hour), MissBurst),
				}
				r := gin.New()
				method, path := http.MethodGet, "/api/v1/client/subscribe?token="+u.Token+"&flag=v2ray"
				if route == "query" {
					r.GET("/api/v1/client/subscribe", subscribeWithGenerator(a, lim, func(c *gin.Context) string { return c.Query("token") }, false, gen))
				} else {
					method, path = http.MethodHead, "/s/"+u.Token+"?flag=v2ray"
					r.HEAD("/s/:token", subscribeWithGenerator(a, lim, func(c *gin.Context) string { return c.Param("token") }, true, gen))
				}
				req := httptest.NewRequest(method, path, nil)
				if route == "short" {
					req.Header.Set("If-None-Match", "*")
				}
				done := make(chan *httptest.ResponseRecorder, 1)
				go func() {
					rec := httptest.NewRecorder()
					r.ServeHTTP(rec, req)
					done <- rec
				}()
				select {
				case count := <-generated:
					if count < 2 {
						t.Fatalf("expected a rendered paid subscription, proxies=%d", count)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("generation did not reach the response boundary")
				}
				if err := mutate(a.DB, u); err != nil {
					t.Fatal(err)
				}
				releaseOnce.Do(func() { close(release) })
				select {
				case rec := <-done:
					if rec.Code != http.StatusForbidden || rec.Header().Get("Subscription-Userinfo") != "" ||
						rec.Header().Get("ETag") != "" || rec.Header().Get(guard.EncryptionHeader) != "" ||
						strings.Contains(rec.Body.String(), "us1.example.com") {
						t.Fatalf("stale subscription: %d headers=%v body=%q", rec.Code, rec.Header(), rec.Body.String())
					}
				case <-time.After(5 * time.Second):
					t.Fatal("request did not finish")
				}
				if err := guard.FlushAccess(t.Context(), a); err != nil {
					t.Fatal(err)
				}
				var accesses int64
				if err := a.DB.Model(&model.SubscriptionAccess{}).Count(&accesses).Error; err != nil || accesses != 0 {
					t.Fatalf("aborted request was logged: count=%d err=%v", accesses, err)
				}
				if route == "query" && name == "plan" {
					freshRouter := gin.New()
					Register(freshRouter, a)
					fresh := httptest.NewRecorder()
					freshRouter.ServeHTTP(fresh, httptest.NewRequest(http.MethodGet, path, nil))
					plain, decodeErr := base64.StdEncoding.DecodeString(fresh.Body.String())
					if fresh.Code != http.StatusOK || decodeErr != nil || !bytes.Contains(plain, []byte("us1.example.com")) {
						t.Fatalf("plan_id-only clearing removed manual entitlement: %d err=%v", fresh.Code, decodeErr)
					}
				}
			})
		}
	}
}
