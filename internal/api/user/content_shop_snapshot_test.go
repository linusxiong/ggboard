package user

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestContentAndPlansUseLiveAccountSnapshot(t *testing.T) {
	reads := []struct {
		name, route, path string
		handler           func(*app.App) gin.HandlerFunc
	}{
		{"knowledge-index", "/knowledge", "/knowledge?keyword=gated-secret", contentKnowledgeIndex},
		{"knowledge-article", "/knowledge/:id", "/knowledge/ARTICLE", contentKnowledgeArticle},
		{"plans-list", "/plans", "/plans", shopListPlans},
		{"plans-detail", "/plans/:id", "/plans/PLAN", shopGetPlan},
	}
	mutations := []struct {
		name       string
		wantStatus int
		change     func(*testing.T, *app.App, *model.User)
	}{
		{"password-changed", http.StatusUnauthorized, func(t *testing.T, a *app.App, u *model.User) {
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("password_version", u.PasswordVersion+1).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"banned", http.StatusForbidden, func(t *testing.T, a *app.App, u *model.User) {
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("banned", true).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"same-id-replacement", http.StatusUnauthorized, func(t *testing.T, a *app.App, u *model.User) {
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).
				Updates(map[string]any{"uuid": "replacement-uuid", "token": "replacement-subscription-token"}).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{"plan-revoked", http.StatusOK, func(t *testing.T, a *app.App, u *model.User) {
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("plan_id", nil).Error; err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, read := range reads {
		for _, mutation := range mutations {
			t.Run(read.name+"/"+mutation.name, func(t *testing.T) {
				a := testutil.NewTestApp(t)
				plan := &model.Plan{Name: "Hidden member plan", Show: false, Sell: false}
				if err := a.DB.Create(plan).Error; err != nil {
					t.Fatal(err)
				}
				future := time.Now().Add(time.Hour)
				u := testutil.CreateUser(t, a, "old@example.com", usersvc.CreateOptions{
					PlanID: &plan.ID, TransferEnable: 1 << 30, ExpiredAt: &future,
				})
				tok := testutil.Login(t, a, u)
				article := &model.Knowledge{Language: "zh-CN", Category: "Help", Title: "Guide", Show: true,
					Body: "<!--access start-->gated-secret<!--access end--> {{subscribeUrl}}"}
				if err := a.DB.Create(article).Error; err != nil {
					t.Fatal(err)
				}
				path := strings.ReplaceAll(strings.ReplaceAll(read.path, "ARTICLE", strconv.FormatUint(article.ID, 10)),
					"PLAN", strconv.FormatUint(plan.ID, 10))
				r := gin.New()
				r.GET(read.route, auth.RequireUser(a), func(c *gin.Context) {
					mutation.change(t, a, u)
					c.Next()
				}, read.handler(a))
				rec := testutil.Do(t, r, http.MethodGet, path, nil, tok)
				want := mutation.wantStatus
				if mutation.name == "plan-revoked" && read.name == "plans-detail" {
					want = http.StatusNotFound
				}
				if rec.Code != want {
					t.Fatalf("status = %d, want %d; body = %s", rec.Code, want, rec.Body.String())
				}
				if mutation.name == "plan-revoked" {
					body := rec.Body.String()
					if strings.Contains(body, "gated-secret") || strings.Contains(body, "Hidden member plan") || strings.Contains(body, `"is_current":true`) {
						t.Fatalf("revoked plan data leaked: %s", body)
					}
				}
				if mutation.name == "same-id-replacement" && strings.Contains(rec.Body.String(), "replacement-subscription-token") {
					t.Fatalf("replacement token leaked: %s", rec.Body.String())
				}
			})
		}
	}
}
