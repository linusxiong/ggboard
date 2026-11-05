package ticket_test

import (
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/notifytest"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Administrators hear about an owner's reply at most once per ticket within NotifyThrottle. A staff answer starts
// over, so a reply to that answer always alerts.
func TestTicketUserReplyNotification(t *testing.T) {
	a := testutil.NewTestApp(t)
	rec := notifytest.Attach(t, a)
	u := testutil.CreateUser(t, a, "alice@example.com")
	other := testutil.CreateUser(t, a, "bob@example.com")
	staff := testutil.CreateStaff(t, a, "staff@example.com")
	svc := ticketsvc.New(a)
	open := func(id uint64) *model.Ticket {
		t.Helper()
		tk, err := svc.Create(t.Context(), id, "VPN down", model.TicketLevelHigh, "first message")
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}
	tk, otherTicket := open(u.ID), open(other.ID)
	reply := func(tk *model.Ticket, owner uint64, msg string) {
		t.Helper()
		if _, err := svc.ReplyAsOwner(t.Context(), tk.ID, owner, msg); err != nil {
			t.Fatal(err)
		}
	}
	count := func() int {
		t.Helper()
		return len(rec.Events(t, notify.TypeTicketUserReply))
	}

	reply(tk, u.ID, "first follow-up")
	reply(tk, u.ID, "second follow-up")
	got := rec.Events(t, notify.TypeTicketUserReply)
	if len(got) != 1 || got[0].Body != "first follow-up" || got[0].UserID != nil || !got[0].Admin {
		t.Fatalf("burst = %+v", got)
	}
	fs := notifytest.Fields(got[0])
	if fs["subject"] != "VPN down" || fs["level"] != "高" || fs["user"] != "alice@example.com" {
		t.Fatalf("fields = %v", fs)
	}

	reply(otherTicket, other.ID, "another ticket has its own limit")
	if n := count(); n != 2 {
		t.Fatalf("after a second ticket: %d events", n)
	}

	if _, err := svc.ReplyAsStaff(t.Context(), tk.ID, staff.ID, "please retry"); err != nil {
		t.Fatal(err)
	}
	reply(tk, u.ID, "still broken")
	if n := count(); n != 3 {
		t.Fatalf("after a staff answer: %d events", n)
	}
	allow := func(*gorm.DB) error { return nil }
	if _, _, err := svc.ReplyAsStaffIf(t.Context(), tk.ID, staff.ID, "and now?", allow); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReplyForOwner(t.Context(), tk.ID, u, "works, thanks"); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 4 {
		t.Fatalf("after the authorized staff answer: %d events", n)
	}
}

func TestTicketNotifications(t *testing.T) {
	a := testutil.NewTestApp(t)
	rec := notifytest.Attach(t, a)
	u := testutil.CreateUser(t, a, "alice@example.com")
	staff := testutil.CreateStaff(t, a, "staff@example.com")
	svc := ticketsvc.New(a)

	tk, err := svc.Create(t.Context(), u.ID, "VPN down", model.TicketLevelHigh, "Nothing <works> since today")
	if err != nil {
		t.Fatal(err)
	}
	ev := rec.Last(t, notify.TypeTicketNew)
	fs := notifytest.Fields(ev)
	if fs["subject"] != "VPN down" || fs["level"] != "高" || fs["user"] != "alice@example.com" || ev.Body != "Nothing <works> since today" || ev.UserID != nil {
		t.Fatalf("ticket_new = %+v", ev)
	}

	// Every staff reply is notified to the owner (and to subscribed admins); the owner's own replies are not.
	if _, err := svc.ReplyAsOwner(t.Context(), tk.ID, u.ID, "any news?"); err != nil {
		t.Fatal(err)
	}
	for _, msg := range []string{"Looking into it", "Fixed, please retry"} {
		if _, err := svc.ReplyAsStaff(t.Context(), tk.ID, staff.ID, msg); err != nil {
			t.Fatal(err)
		}
	}
	got := rec.UserEvents(t, u.ID, notify.TypeTicketReply)
	if len(got) != 2 || got[1].Body != "Fixed, please retry" || notifytest.Fields(got[1])["subject"] != "VPN down" || !got[1].Admin {
		t.Fatalf("ticket_reply = %+v", got)
	}
	if n := len(rec.Events(t, notify.TypeTicketNew)); n != 1 {
		t.Fatalf("ticket_new events = %d", n)
	}

	// A staff member answering their own ticket is an owner reply.
	own, err := svc.Create(t.Context(), staff.ID, "Mine", model.TicketLevelLow, strings.Repeat("x", 10))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReplyAsStaff(t.Context(), own.ID, staff.ID, "note to self"); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.UserEvents(t, staff.ID, notify.TypeTicketReply)); n != 0 {
		t.Fatalf("self reply notified %d times", n)
	}

	// System tickets (withdrawals) send no ticket_new.
	v := testutil.CreateUser(t, a, "bob@example.com")
	if _, err := svc.CreateSystem(t.Context(), v.ID, "[Withdrawal]", model.TicketLevelHigh, "x"); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Events(t, notify.TypeTicketNew)); n != 2 {
		t.Fatalf("ticket_new events = %d (CreateSystem must not emit)", n)
	}
}
