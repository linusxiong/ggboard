package user

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestTicketAndInviteRoutesRejectAccountReplacedAfterMiddleware(t *testing.T) {
	cases := []struct {
		name, method, path, body string
		ticket, invite           bool
	}{
		{"ticket list", http.MethodGet, "/tickets", "", true, false},
		{"ticket detail", http.MethodGet, "/tickets/77", "", true, false},
		{"ticket create", http.MethodPost, "/tickets", `{"subject":"Help","level":0,"message":"Old account"}`, false, false},
		{"ticket reply", http.MethodPost, "/tickets/77/reply", `{"message":"Old account"}`, true, false},
		{"ticket close", http.MethodPost, "/tickets/77/close", "", true, false},
		{"invite overview", http.MethodGet, "/invite", "", false, true},
		{"invite commissions", http.MethodGet, "/invite/commissions", "", false, true},
		{"dashboard stats", http.MethodGet, "/stats", "", false, false},
		{"traffic logs", http.MethodGet, "/traffic-logs", "", false, false},
		{"account preference", http.MethodPatch, "/me", `{"remind_expire":false}`, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := testutil.NewTestApp(t)
			u := testutil.CreateUser(t, a, "stale-owner@example.com")
			if tc.method == http.MethodPatch {
				if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("remind_expire", true).Error; err != nil {
					t.Fatal(err)
				}
			}
			token := testutil.Login(t, a, u)
			authenticated, resume := make(chan struct{}), make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(resume) }) }
			t.Cleanup(release)
			r := gin.New()
			g := r.Group("", auth.RequireUser(a), func(c *gin.Context) {
				close(authenticated)
				<-resume
				c.Next()
			})
			registerTickets(g, a)
			registerInvite(g, a)
			registerAccount(g, a)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { r.ServeHTTP(rec, req); close(done) }()
			select {
			case <-authenticated:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not pass authentication")
			}
			if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("uuid", "replacement-account-uuid").Error; err != nil {
				t.Fatal(err)
			}
			if tc.ticket {
				if err := a.DB.Create(&model.Ticket{ID: 77, UserID: u.ID, Subject: "Replacement ticket",
					Status: model.TicketStatusOpen}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if tc.invite {
				if err := a.DB.Create(&model.InviteCode{UserID: u.ID, Code: "replacement-invite-code"}).Error; err != nil {
					t.Fatal(err)
				}
				if err := a.DB.Create(&model.CommissionLog{InviteUserID: u.ID, TradeNo: "replacement-trade", GetAmount: 100}).Error; err != nil {
					t.Fatal(err)
				}
			}
			release()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("request did not finish")
			}
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("stale request = %d, body=%s", rec.Code, rec.Body.String())
			}
			if tc.method == http.MethodPatch {
				var current model.User
				if err := a.DB.First(&current, u.ID).Error; err != nil || !current.RemindExpire {
					t.Fatalf("stale request changed account preference: %+v, %v", current, err)
				}
			}
			if tc.method == http.MethodPost {
				var tickets []model.Ticket
				if err := a.DB.Where("user_id = ?", u.ID).Find(&tickets).Error; err != nil {
					t.Fatal(err)
				}
				wantTickets := 0
				if tc.ticket {
					wantTickets = 1
				}
				if len(tickets) != wantTickets || (tc.ticket && tickets[0].Status != model.TicketStatusOpen) {
					t.Fatalf("stale request changed tickets: %+v", tickets)
				}
				var count int64
				if err := a.DB.Model(&model.TicketMessage{}).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("stale request added %d ticket messages: %v", count, err)
				}
			}
		})
	}
}

func TestSubscriptionProfilesRejectReplacedAccount(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "stale-profiles@example.com")
	if err := a.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("uuid", "replacement-account-uuid").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Create(&model.RuleProfile{UserID: u.ID, Slug: "replacement", Name: "Replacement profile"}).Error; err != nil {
		t.Fatal(err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/subscription", nil)
	if profiles, err := subscriptionProfiles(c, a, u, "/s/old"); httpx.AsError(err).Status != http.StatusUnauthorized || len(profiles) != 0 {
		t.Fatalf("stale subscription profiles = %+v, %v", profiles, err)
	}
}
