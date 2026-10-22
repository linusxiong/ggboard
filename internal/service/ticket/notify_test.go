package ticket_test

import (
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/notifytest"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
	"github.com/linusxiong/ggboard/internal/testutil"
)

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

	// The owner's own replies are not notified; every staff reply is (to the owner, and to subscribed admins).
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
