package admin

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription/guard"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestRemainingAdminReadsAndAgeResetRejectInFlightDemotion(t *testing.T) {
	for _, target := range []string{"logs", "status", "stats", "node-stats", "node-online", "node-audit", "age", "age-reset", "access-log", "mail-list", "mail-show", "mail-preview", "geoip", "geoip-update"} {
		t.Run(target, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			admin := testutil.CreateAdmin(t, a, "admin@example.com")
			user := testutil.CreateUser(t, a, "user@example.com")
			tok := testutil.Login(t, a, admin)
			node := model.Node{Type: model.NodeVless, Name: "Private", Host: "node.example.com", Port: "443", ServerPort: 443}
			if err := a.DB.Create(&node).Error; err != nil {
				t.Fatal(err)
			}
			key, _, err := guard.GenerateKey(t.Context(), a, user.ID, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.DB.Create(&model.MailTemplate{Name: "verify_code", Locale: "en", Subject: "Secret", Body: "template-secret"}).Error; err != nil {
				t.Fatal(err)
			}
			a.Log.Warn("log-secret")
			r := gin.New()
			r.Use(auth.RequireUser(a), func(c *gin.Context) {
				if err := a.DB.Model(&model.User{}).Where("id = ?", admin.ID).Update("is_admin", false).Error; err != nil {
					t.Error(err)
				}
				c.Next()
			})
			Register(r, a)
			method, path, body := http.MethodGet, "/api/v1/admin/", any(nil)
			switch target {
			case "logs":
				path += "system/logs"
			case "status":
				path += "system/status"
			case "stats":
				path += "stats/users/ranking"
			case "node-stats":
				path += "nodes/" + strconv.FormatUint(node.ID, 10) + "/stats"
			case "node-online":
				path += "nodes/" + strconv.FormatUint(node.ID, 10) + "/online"
			case "node-audit":
				path += "nodes/" + strconv.FormatUint(node.ID, 10) + "/audit"
			case "age":
				path += "users/" + strconv.FormatUint(user.ID, 10) + "/subscription/age"
			case "age-reset":
				method, path = http.MethodPost, path+"users/"+strconv.FormatUint(user.ID, 10)+"/subscription/age/reset"
			case "access-log":
				path += "users/" + strconv.FormatUint(user.ID, 10) + "/subscription/access-log"
			case "mail-list":
				path += "mail-templates"
			case "mail-show":
				path += "mail-templates/verify_code?locale=en"
			case "mail-preview":
				method, path, body = http.MethodPost, path+"mail-templates/verify_code/preview", map[string]any{"locale": "en", "subject": "Secret", "body": "template-secret"}
			case "geoip":
				path += "geoip"
			case "geoip-update":
				method, path = http.MethodPost, path+"geoip/update"
			}
			rec := testutil.Do(t, r, method, path, body, tok)
			if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "template-secret") || strings.Contains(rec.Body.String(), "log-secret") {
				t.Fatalf("%s after demotion: %d %s", target, rec.Code, rec.Body.String())
			}
			if target == "age-reset" {
				got, err := guard.LoadKey(t.Context(), a, user.ID)
				if err != nil || got.PublicKey != key.PublicKey {
					t.Fatalf("age key changed after rejected reset: %v %+v", err, got)
				}
			}
		})
	}
}

func TestStaffContentAndTicketsRejectInFlightRevocation(t *testing.T) {
	for _, target := range []string{"notices", "notice-create", "knowledge", "knowledge-show", "knowledge-create", "tickets", "ticket-show", "ticket-reply", "ticket-close"} {
		t.Run(target, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			staff := testutil.CreateStaff(t, a, "staff@example.com")
			user := testutil.CreateUser(t, a, "user@example.com")
			tok := testutil.Login(t, a, staff)
			k := model.Knowledge{Language: "en", Category: "help", Title: "Private", Body: "knowledge-secret", Show: false}
			if err := a.DB.Create(&k).Error; err != nil {
				t.Fatal(err)
			}
			ticket := model.Ticket{UserID: user.ID, Subject: "Private", Status: model.TicketStatusOpen}
			if err := a.DB.Create(&ticket).Error; err != nil {
				t.Fatal(err)
			}
			r := gin.New()
			r.Use(auth.RequireUser(a), func(c *gin.Context) {
				if err := a.DB.Model(&model.User{}).Where("id = ?", staff.ID).Update("is_staff", false).Error; err != nil {
					t.Error(err)
				}
				c.Next()
			})
			Register(r, a)
			method, path, body := http.MethodGet, "/api/v1/admin/", any(nil)
			switch target {
			case "notices":
				path += "notices"
			case "notice-create":
				method, path, body = http.MethodPost, path+"notices", map[string]any{"title": "Injected", "content": "hello"}
			case "knowledge":
				path += "knowledge"
			case "knowledge-show":
				path += "knowledge/" + strconv.FormatUint(k.ID, 10)
			case "knowledge-create":
				method, path, body = http.MethodPost, path+"knowledge", map[string]any{"language": "en", "category": "help", "title": "Injected", "body": "hello"}
			case "tickets":
				path += "tickets"
			case "ticket-show":
				path += "tickets/" + strconv.FormatUint(ticket.ID, 10)
			case "ticket-reply":
				method, path, body = http.MethodPost, path+"tickets/"+strconv.FormatUint(ticket.ID, 10)+"/reply", map[string]any{"message": "Injected"}
			case "ticket-close":
				method, path = http.MethodPost, path+"tickets/"+strconv.FormatUint(ticket.ID, 10)+"/close"
			}
			rec := testutil.Do(t, r, method, path, body, tok)
			if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "knowledge-secret") {
				t.Fatalf("%s after revocation: %d %s", target, rec.Code, rec.Body.String())
			}
			var count int64
			if err := a.DB.Model(&model.TicketMessage{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("ticket message written: count=%d err=%v", count, err)
			}
			var latest model.Ticket
			if err := a.DB.First(&latest, ticket.ID).Error; err != nil || latest.Status != model.TicketStatusOpen {
				t.Fatalf("ticket closed: %+v %v", latest, err)
			}
		})
	}
}

func TestKnowledgePlaceholderUsesLiveAdminRole(t *testing.T) {
	for _, update := range []bool{false, true} {
		t.Run(strconv.FormatBool(update), func(t *testing.T) {
			a := testutil.NewTestApp(t)
			admin := testutil.CreateAdmin(t, a, "admin@example.com")
			tok := testutil.Login(t, a, admin)
			var existing model.Knowledge
			if update {
				existing = model.Knowledge{Language: "en", Category: "help", Title: "Safe", Body: "safe", Show: true}
				if err := a.DB.Create(&existing).Error; err != nil {
					t.Fatal(err)
				}
			}
			r := gin.New()
			r.Use(auth.RequireUser(a), func(c *gin.Context) {
				if err := a.DB.Model(&model.User{}).Where("id = ?", admin.ID).Updates(map[string]any{"is_admin": false, "is_staff": true}).Error; err != nil {
					t.Error(err)
				}
				c.Next()
			})
			Register(r, a)
			method, path := http.MethodPost, "/api/v1/admin/knowledge"
			if update {
				method, path = http.MethodPatch, path+"/"+strconv.FormatUint(existing.ID, 10)
			}
			rec := testutil.Do(t, r, method, path, map[string]any{
				"language": "en", "category": "help", "title": "Injected", "body": "{{subscribeUrl}}",
			}, tok)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("placeholder after demotion: %d %s", rec.Code, rec.Body.String())
			}
			var rows []model.Knowledge
			if err := a.DB.Find(&rows).Error; err != nil || len(rows) != btoi(update) || update && rows[0].Body != "safe" {
				t.Fatalf("placeholder article persisted: %+v %v", rows, err)
			}
		})
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
