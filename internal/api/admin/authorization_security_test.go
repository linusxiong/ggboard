package admin

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestCredentialReadsRejectInFlightDemotion(t *testing.T) {
	for _, target := range []string{"payments", "payment", "nodes", "node", "export"} {
		t.Run(target, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			admin := testutil.CreateAdmin(t, a, "admin@example.com")
			tok := testutil.Login(t, a, admin)
			p := model.Payment{Gateway: "epay", Name: "Gateway", UUID: "00000000-0000-0000-0000-000000000001",
				Config: model.JSONObject[string]{"key": "payment-secret"}}
			if err := a.DB.Create(&p).Error; err != nil {
				t.Fatal(err)
			}
			n := model.Node{Type: model.NodeVless, Name: "Private", Host: "node.example.com", Port: "443", ServerPort: 443,
				ProtocolSettings: model.JSONMap{"private_key": "node-secret"}}
			if err := a.DB.Create(&n).Error; err != nil {
				t.Fatal(err)
			}
			r := gin.New()
			r.Use(auth.RequireUser(a), func(c *gin.Context) {
				if err := a.DB.Model(&model.User{}).Where("id = ?", admin.ID).Update("is_admin", false).Error; err != nil {
					t.Error(err)
				}
				c.Next()
			})
			Register(r, a)
			method, path := http.MethodGet, "/api/v1/admin/"+target
			switch target {
			case "payment":
				path = "/api/v1/admin/payments/" + strconv.FormatUint(p.ID, 10)
			case "node":
				path = "/api/v1/admin/nodes/" + strconv.FormatUint(n.ID, 10)
			case "export":
				method, path = http.MethodPost, "/api/v1/admin/users/export"
			}
			rec := testutil.Do(t, r, method, path, map[string]any{}, tok)
			if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "payment-secret") ||
				strings.Contains(rec.Body.String(), "node-secret") || strings.Contains(rec.Body.String(), admin.Email) {
				t.Fatalf("%s after demotion: %d %s", target, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCredentialDeletesRejectInFlightDemotion(t *testing.T) {
	for _, target := range []string{"payment", "node"} {
		t.Run(target, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			admin := testutil.CreateAdmin(t, a, "admin@example.com")
			tok := testutil.Login(t, a, admin)
			var row any
			if target == "payment" {
				row = &model.Payment{Gateway: "epay", Name: "Gateway", UUID: "00000000-0000-0000-0000-000000000001",
					Config: model.JSONObject[string]{"key": "payment-secret"}}
			} else {
				row = &model.Node{Type: model.NodeVless, Name: "Private", Host: "node.example.com", Port: "443", ServerPort: 443}
			}
			if err := a.DB.Create(row).Error; err != nil {
				t.Fatal(err)
			}
			id := uint64(0)
			if target == "payment" {
				id = row.(*model.Payment).ID
			} else {
				id = row.(*model.Node).ID
			}
			r := gin.New()
			r.Use(auth.RequireUser(a), func(c *gin.Context) {
				if err := a.DB.Model(&model.User{}).Where("id = ?", admin.ID).Update("is_admin", false).Error; err != nil {
					t.Error(err)
				}
				c.Next()
			})
			Register(r, a)
			path := "/api/v1/admin/" + target + "s/" + strconv.FormatUint(id, 10)
			rec := testutil.Do(t, r, http.MethodDelete, path, nil, tok)
			var n int64
			if err := a.DB.Model(row).Where("id = ?", id).Count(&n).Error; err != nil || rec.Code != http.StatusForbidden || n != 1 {
				t.Fatalf("%s delete after demotion: status=%d rows=%d err=%v body=%s", target, rec.Code, n, err, rec.Body.String())
			}
		})
	}
}

func TestUserReadsUseLiveStaffRoleForCredentialRedaction(t *testing.T) {
	for _, role := range []string{"staff", "user"} {
		for _, detail := range []bool{false, true} {
			t.Run(role+"/detail="+strconv.FormatBool(detail), func(t *testing.T) {
				a := testutil.NewTestApp(t)
				admin := testutil.CreateAdmin(t, a, "admin@example.com")
				target := testutil.CreateUser(t, a, "target@example.com")
				tok := testutil.Login(t, a, admin)
				r := gin.New()
				r.Use(auth.RequireUser(a), func(c *gin.Context) {
					if err := a.DB.Model(&model.User{}).Where("id = ?", admin.ID).Updates(map[string]any{
						"is_admin": false, "is_staff": role == "staff",
					}).Error; err != nil {
						t.Error(err)
					}
					c.Next()
				})
				Register(r, a)
				path := "/api/v1/admin/users"
				if detail {
					path += "/" + strconv.FormatUint(target.ID, 10)
				}
				rec := testutil.Do(t, r, http.MethodGet, path, nil, tok)
				want := http.StatusForbidden
				if role == "staff" {
					want = http.StatusOK
				}
				if rec.Code != want || strings.Contains(rec.Body.String(), target.UUID) || strings.Contains(rec.Body.String(), target.Token) {
					t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestExternalAdminActionsRecheckBeforeDispatch(t *testing.T) {
	for _, action := range []string{"mail", "template-mail", "bulk-mail", "telegram-test", "telegram-webhook"} {
		t.Run(action, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			admin := testutil.CreateAdmin(t, a, "admin@example.com")
			tok := testutil.Login(t, a, admin)
			claims, err := auth.Parse(a, tok)
			if err != nil {
				t.Fatal(err)
			}
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(auth.CtxClaims, claims)
				c.Set(auth.CtxUser, admin)
				if err := a.DB.Model(&model.User{}).Where("id = ?", admin.ID).Update("is_admin", false).Error; err != nil {
					t.Error(err)
				}
				c.Next()
			})
			method, path, body := http.MethodPost, "/action", any(nil)
			switch action {
			case "mail":
				r.POST(path, settingsMailTestHandler(a))
				body = map[string]any{"email": "ops@example.com"}
			case "template-mail":
				path = "/template/:name/test"
				r.POST(path, mailTemplateTest(a))
				body = map[string]any{"locale": "en", "subject": "{{siteName}}: {{ticketSubject}}", "body": "Re **{{ticketSubject}}**\n\n> {{message}}", "email": "ops@example.com"}
			case "bulk-mail":
				r.POST(path, usersMailHandler(a))
				body = map[string]any{"subject": "Test", "content": "Message"}
			case "telegram-test":
				r.POST(path, settingsTelegramTestHandler(a))
			case "telegram-webhook":
				r.POST(path, settingsTelegramWebhookHandler(a))
			}
			if action == "template-mail" {
				path = "/template/ticket_reply/test"
			}
			rec := testutil.Do(t, r, method, path, body, "")
			if rec.Code != http.StatusForbidden || len(testutil.Mailbox(t, a).Messages()) != 0 {
				t.Fatalf("%s after demotion: %d %s", action, rec.Code, rec.Body.String())
			}
		})
	}
}
