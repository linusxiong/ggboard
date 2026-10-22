package admin_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/api/admin"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// contentAdminEnv returns an app, a router with the admin API and tokens for an admin, a staff member and a
// plain user.
func contentAdminEnv(t *testing.T) (a *app.App, r *gin.Engine, adminTok, staffTok, userTok string) {
	t.Helper()
	a = testutil.NewTestApp(t)
	r = testutil.NewRouterWith(t, a, admin.Register)
	adminTok = testutil.Login(t, a, testutil.CreateAdmin(t, a, "root@example.com"))
	staffTok = testutil.Login(t, a, testutil.CreateStaff(t, a, "staff@example.com"))
	userTok = testutil.Login(t, a, testutil.CreateUser(t, a, "plain@example.com"))
	return a, r, adminTok, staffTok, userTok
}

func ticketSeed(t *testing.T, a *app.App, u *model.User, subject string, status model.TicketStatus, reply model.TicketReplyStatus) *model.Ticket {
	t.Helper()
	tk, err := ticketsvc.New(a).Create(t.Context(), u.ID, subject, model.TicketLevelLow, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Model(&model.Ticket{}).Where("id = ?", tk.ID).Updates(map[string]any{"status": status, "reply_status": reply}).Error; err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestAdminTicketAccess(t *testing.T) {
	a, r, adminTok, staffTok, userTok := contentAdminEnv(t)
	tk := ticketSeed(t, a, testutil.CreateUser(t, a, "alice@example.com"), "Help", model.TicketStatusOpen, model.TicketAwaitingStaff)
	path := fmt.Sprintf("/api/v1/admin/tickets/%d", tk.ID)
	for _, req := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/tickets"}, {http.MethodGet, path},
		{http.MethodPost, path + "/reply"}, {http.MethodPost, path + "/close"},
	} {
		testutil.ExpectStatus(t, testutil.Do(t, r, req.method, req.path, map[string]any{"message": "x"}, userTok), http.StatusForbidden)
		testutil.ExpectStatus(t, testutil.Do(t, r, req.method, req.path, map[string]any{"message": "x"}, ""), http.StatusUnauthorized)
	}
	for _, tok := range []string{staffTok, adminTok} {
		testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/admin/tickets", nil, tok), http.StatusOK)
		testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, path, nil, tok), http.StatusOK)
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, path+"/reply", map[string]any{"message": "  "}, staffTok), http.StatusBadRequest)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/admin/tickets/999/reply", map[string]any{"message": "x"}, staffTok), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, "/api/v1/admin/tickets/999/close", nil, staffTok), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/admin/tickets/999", nil, staffTok), http.StatusNotFound)

	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, path+"/close", nil, adminTok), http.StatusOK)
	v := testutil.Decode[ticketsvc.View](t, testutil.Do(t, r, http.MethodGet, path, nil, staffTok))
	if v.Status != model.TicketStatusClosed || v.User == nil || v.User.Email != "alice@example.com" || len(v.Messages) != 1 || v.Messages[0].IsMe {
		t.Fatalf("detail = %+v", v)
	}
}

func TestAdminTicketFilters(t *testing.T) {
	a, r, _, staffTok, _ := contentAdminEnv(t)
	alice := testutil.CreateUser(t, a, "alice@example.com")
	bob := testutil.CreateUser(t, a, "bob@example.com")
	t1 := ticketSeed(t, a, alice, "A closed", model.TicketStatusClosed, model.TicketReplied)
	t2 := ticketSeed(t, a, alice, "A open waiting", model.TicketStatusOpen, model.TicketAwaitingStaff)
	t3 := ticketSeed(t, a, bob, "B open replied", model.TicketStatusOpen, model.TicketReplied)

	ids := func(q string) []uint64 {
		t.Helper()
		list, total := testutil.DecodeList[ticketsvc.View](t, testutil.Do(t, r, http.MethodGet, "/api/v1/admin/tickets"+q, nil, staffTok))
		if int(total) < len(list) {
			t.Fatalf("total %d < len %d", total, len(list))
		}
		out := make([]uint64, 0, len(list))
		for _, v := range list {
			if v.User == nil || v.User.Email == "" || v.Messages != nil {
				t.Fatalf("list item = %+v", v)
			}
			out = append(out, v.ID)
		}
		return out
	}
	eq := func(got []uint64, want ...uint64) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
	eq(ids(""), t3.ID, t2.ID, t1.ID)
	eq(ids("?status=0"), t3.ID, t2.ID)
	eq(ids("?status=0&reply_status=0"), t2.ID)
	eq(ids("?reply_status=1"), t3.ID, t1.ID)
	eq(ids(fmt.Sprintf("?user_id=%d", alice.ID)), t2.ID, t1.ID)
	eq(ids("?email=BOB@example.com"), t3.ID)
	eq(ids("?email=nobody@example.com"))
	eq(ids("?sort=id"), t1.ID, t2.ID, t3.ID)
	eq(ids("?page=2&page_size=2"), t1.ID)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, "/api/v1/admin/tickets?reply_status=2", nil, staffTok), http.StatusBadRequest)
}
