package user_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/linusxiong/ggboard/internal/api/admin"
	"github.com/linusxiong/ggboard/internal/api/user"
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
	"github.com/linusxiong/ggboard/internal/testutil"
)

const ticketsPath = "/api/v1/user/tickets"

func ticketTestEnv(t *testing.T, settings map[string]any) (*app.App, *gin.Engine) {
	t.Helper()
	opts := []testutil.Option{}
	if settings != nil {
		opts = append(opts, testutil.WithSettings(settings))
	}
	a := testutil.NewTestApp(t, opts...)
	return a, testutil.NewRouterWith(t, a, user.Register, admin.Register)
}

func TestTicketCreateValidation(t *testing.T) {
	a, r := ticketTestEnv(t, nil)
	tok := testutil.Login(t, a, testutil.CreateUser(t, a, "alice@example.com"))
	cases := []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"level": 0, "message": "hi"}, "subject"},
		{map[string]any{"subject": "   ", "level": 0, "message": "hi"}, "subject"},
		{map[string]any{"subject": "s", "message": "hi"}, "level"},
		{map[string]any{"subject": "s", "level": 3, "message": "hi"}, "level"},
		{map[string]any{"subject": "s", "level": 1}, "message"},
		{map[string]any{"subject": "s", "level": 1, "message": " \n "}, "message"},
		{map[string]any{"subject": strings.Repeat("x", 256), "level": 1, "message": "hi"}, "subject"},
	}
	for i, tc := range cases {
		rec := testutil.Do(t, r, http.MethodPost, ticketsPath, tc.body, tok)
		testutil.ExpectStatus(t, rec, http.StatusBadRequest)
		if e := testutil.DecodeError(t, rec); len(e.Errors[tc.field]) == 0 {
			t.Fatalf("case %d: want error on %q, got %+v", i, tc.field, e)
		}
	}
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, ticketsPath, nil, ""), http.StatusUnauthorized)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, ticketsPath+"?status=7", nil, tok), http.StatusBadRequest)
}

func TestTicketLifecycle(t *testing.T) {
	a, r := ticketTestEnv(t, map[string]any{"app_url": "https://panel.example.com/"})
	alice := testutil.CreateUser(t, a, "alice@example.com")
	staff := testutil.CreateStaff(t, a, "staff@example.com")
	ut, st := testutil.Login(t, a, alice), testutil.Login(t, a, staff)

	// Open a ticket: it awaits staff and carries the first message.
	rec := testutil.Do(t, r, http.MethodPost, ticketsPath, map[string]any{"subject": " Cannot connect ", "level": 1, "message": "Nothing works."}, ut)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	tk := testutil.Decode[ticketsvc.View](t, rec)
	if tk.Subject != "Cannot connect" || tk.Level != model.TicketLevelMedium || tk.Status != model.TicketStatusOpen ||
		tk.ReplyStatus != model.TicketAwaitingStaff || tk.UserID != alice.ID || tk.LastReplyUserID == nil || *tk.LastReplyUserID != alice.ID {
		t.Fatalf("created ticket = %+v", tk)
	}
	if len(tk.Messages) != 1 || !tk.Messages[0].IsMe || tk.Messages[0].Message != "Nothing works." {
		t.Fatalf("messages = %+v", tk.Messages)
	}

	// One open ticket per user.
	rec = testutil.Do(t, r, http.MethodPost, ticketsPath, map[string]any{"subject": "Again", "level": 0, "message": "?"}, ut)
	testutil.ExpectStatus(t, rec, http.StatusConflict)

	// List: no messages.
	rec = testutil.Do(t, r, http.MethodGet, ticketsPath, nil, ut)
	list, total := testutil.DecodeList[ticketsvc.View](t, rec)
	if total != 1 || len(list) != 1 || list[0].ID != tk.ID || list[0].Messages != nil || strings.Contains(rec.Body.String(), `"messages"`) {
		t.Fatalf("list = %s", rec.Body.String())
	}
	tpath := fmt.Sprintf("%s/%d", ticketsPath, tk.ID)
	apath := fmt.Sprintf("/api/v1/admin/tickets/%d", tk.ID)

	// Staff reply: marked replied, owner e-mailed with the ticket subject, the message and a link.
	rec = testutil.Do(t, r, http.MethodPost, apath+"/reply", map[string]any{"message": "Please update the client."}, st)
	sv := testutil.Decode[ticketsvc.View](t, rec)
	if sv.ReplyStatus != model.TicketReplied || *sv.LastReplyUserID != staff.ID || len(sv.Messages) != 2 ||
		sv.User == nil || sv.User.Email != "alice@example.com" {
		t.Fatalf("staff view = %+v", sv)
	}
	if sv.Messages[0].IsMe || !sv.Messages[1].IsMe {
		t.Fatalf("staff is_me should mark staff messages: %+v", sv.Messages)
	}
	if !sv.UpdatedAt.After(tk.UpdatedAt) {
		t.Fatalf("updated_at not bumped: %v -> %v", tk.UpdatedAt, sv.UpdatedAt)
	}
	mails := testutil.Mails(t, a)
	if len(mails) != 1 {
		t.Fatalf("mails = %d, want 1", len(mails))
	}
	m := mails[0]
	link := fmt.Sprintf("https://panel.example.com/tickets/%d", tk.ID)
	if m.To != "alice@example.com" || m.Template != "ticket_reply" || !strings.Contains(m.Text, "Cannot connect") ||
		!strings.Contains(m.Text, "Please update the client.") || !strings.Contains(m.Text, link) || !strings.Contains(m.HTML, link) {
		t.Fatalf("mail = %+v", m)
	}

	// A burst of staff messages sends one e-mail.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, apath+"/reply", map[string]any{"message": "Also restart."}, st), http.StatusOK)
	if n := len(testutil.Mails(t, a)); n != 1 {
		t.Fatalf("mails after burst = %d, want 1", n)
	}

	// Owner reply: awaiting staff again; is_me from the owner's point of view.
	rec = testutil.Do(t, r, http.MethodPost, tpath+"/reply", map[string]any{"message": "Still broken."}, ut)
	uv := testutil.Decode[ticketsvc.View](t, rec)
	if uv.ReplyStatus != model.TicketAwaitingStaff || *uv.LastReplyUserID != alice.ID || len(uv.Messages) != 4 || uv.User != nil {
		t.Fatalf("owner view = %+v", uv)
	}
	if !uv.Messages[0].IsMe || uv.Messages[1].IsMe || uv.Messages[2].IsMe || !uv.Messages[3].IsMe {
		t.Fatalf("owner is_me = %+v", uv.Messages)
	}
	// The owner's reply re-arms the notification.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, apath+"/reply", map[string]any{"message": "Try now."}, st), http.StatusOK)
	if n := len(testutil.Mails(t, a)); n != 2 {
		t.Fatalf("mails after owner reply = %d, want 2", n)
	}

	// Detail.
	rec = testutil.Do(t, r, http.MethodGet, tpath, nil, ut)
	if d := testutil.Decode[ticketsvc.View](t, rec); len(d.Messages) != 5 || d.Messages[4].Message != "Try now." {
		t.Fatalf("detail = %+v", d)
	}

	// Close (idempotent); closed tickets cannot be answered by the owner.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, tpath+"/close", nil, ut), http.StatusOK)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, tpath+"/close", nil, ut), http.StatusOK)
	rec = testutil.Do(t, r, http.MethodPost, tpath+"/reply", map[string]any{"message": "One more thing"}, ut)
	testutil.ExpectStatus(t, rec, http.StatusBadRequest)
	if rec = testutil.Do(t, r, http.MethodGet, ticketsPath+"?status=1", nil, ut); !strings.Contains(rec.Body.String(), `"status":1`) {
		t.Fatalf("closed filter = %s", rec.Body.String())
	}
	if _, total := testutil.DecodeList[ticketsvc.View](t, testutil.Do(t, r, http.MethodGet, ticketsPath+"?status=0", nil, ut)); total != 0 {
		t.Fatalf("open tickets = %d", total)
	}

	// With the first one closed, a new ticket may be opened.
	rec = testutil.Do(t, r, http.MethodPost, ticketsPath, map[string]any{"subject": "Billing", "level": 0, "message": "Invoice?"}, ut)
	testutil.ExpectStatus(t, rec, http.StatusCreated)
	second := testutil.Decode[ticketsvc.View](t, rec)

	// A staff reply reopens a closed ticket; the list is ordered by latest activity.
	rec = testutil.Do(t, r, http.MethodPost, apath+"/reply", map[string]any{"message": "Reopening to follow up."}, st)
	if v := testutil.Decode[ticketsvc.View](t, rec); v.Status != model.TicketStatusOpen {
		t.Fatalf("staff reply should reopen: %+v", v)
	}
	list, _ = testutil.DecodeList[ticketsvc.View](t, testutil.Do(t, r, http.MethodGet, ticketsPath, nil, ut))
	if len(list) != 2 || list[0].ID != tk.ID || list[1].ID != second.ID {
		t.Fatalf("order = %+v", list)
	}
}

func TestTicketOwnership(t *testing.T) {
	a, r := ticketTestEnv(t, nil)
	alice := testutil.CreateUser(t, a, "alice@example.com")
	bob := testutil.CreateUser(t, a, "bob@example.com")
	at, bt := testutil.Login(t, a, alice), testutil.Login(t, a, bob)
	tk := testutil.Decode[ticketsvc.View](t, testutil.Do(t, r, http.MethodPost, ticketsPath, map[string]any{"subject": "Mine", "level": 2, "message": "secret"}, at))
	tpath := fmt.Sprintf("%s/%d", ticketsPath, tk.ID)

	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, tpath, nil, bt), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, tpath+"/reply", map[string]any{"message": "hijack"}, bt), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, tpath+"/close", nil, bt), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, ticketsPath+"/999", nil, at), http.StatusNotFound)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodGet, ticketsPath+"/abc", nil, at), http.StatusNotFound)
	if _, total := testutil.DecodeList[ticketsvc.View](t, testutil.Do(t, r, http.MethodGet, ticketsPath, nil, bt)); total != 0 {
		t.Fatalf("bob sees %d tickets", total)
	}
	// Bob's own ticket is independent of Alice's open one.
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, ticketsPath, map[string]any{"subject": "Bob", "level": 0, "message": "hi"}, bt), http.StatusCreated)

	// Nothing changed on Alice's ticket.
	var got model.Ticket
	if err := a.DB.Preload("Messages").First(&got, tk.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TicketStatusOpen || len(got.Messages) != 1 {
		t.Fatalf("alice's ticket changed: %+v", got)
	}
}
