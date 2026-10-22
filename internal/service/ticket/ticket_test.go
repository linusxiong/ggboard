package ticket_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	ticketsvc "github.com/linusxiong/ggboard/internal/service/ticket"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Concurrent creations for one user must leave exactly one open ticket.
func TestCreateOneOpenTicketUnderConcurrency(t *testing.T) {
	a := testutil.NewTestApp(t)
	u := testutil.CreateUser(t, a, "alice@example.com")
	svc := ticketsvc.New(a)
	const n = 8
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		ok   int
		errs []error
	)
	for range n {
		wg.Go(func() {
			_, err := svc.Create(t.Context(), u.ID, "Help", model.TicketLevelLow, "hi")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			} else if !errors.Is(err, ticketsvc.ErrOpenExists) {
				errs = append(errs, err)
			}
		})
	}
	wg.Wait()
	if ok != 1 || len(errs) != 0 {
		t.Fatalf("created %d tickets, unexpected errors %v", ok, errs)
	}
	if n, err := svc.CountOpen(t.Context(), u.ID); err != nil || n != 1 {
		t.Fatalf("open = %d, %v", n, err)
	}
	if _, err := svc.Create(t.Context(), 999999, "Ghost", model.TicketLevelLow, "x"); !errors.Is(err, ticketsvc.ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
}

func TestStaffReplyToOwnTicketAwaitsStaff(t *testing.T) {
	a := testutil.NewTestApp(t)
	staff := testutil.CreateStaff(t, a, "staff@example.com")
	svc := ticketsvc.New(a)
	tk, err := svc.Create(t.Context(), staff.ID, "Mine", model.TicketLevelLow, "first")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.ReplyAsStaff(t.Context(), tk.ID, staff.ID, "more")
	if err != nil {
		t.Fatal(err)
	}
	if got.ReplyStatus != model.TicketAwaitingStaff || len(got.Messages) != 2 {
		t.Fatalf("ticket = %+v", got)
	}
	if mails := testutil.Mails(t, a); len(mails) != 0 {
		t.Fatalf("no mail expected for a self reply, got %d", len(mails))
	}
}
