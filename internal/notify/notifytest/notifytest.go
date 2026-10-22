// Package notifytest provides a recording notify driver for tests:
//
//	a := testutil.NewTestApp(t)
//	rec := notifytest.Attach(t, a)
//	... // code that calls notify.Emit
//	ev := rec.Last(t, notify.TypeOrderPaid)
//
// The driver ("record") is active only for Apps passed to Attach, so tests of other Apps are unaffected.
package notifytest

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
)

// DriverName is the name of the recording driver.
const DriverName = "record"

var recorders sync.Map // *app.App -> *Recorder

func init() {
	notify.Register(DriverName, func(a *app.App) notify.Notifier {
		if r, ok := recorders.Load(a); ok {
			return r.(*Recorder)
		}
		return nil
	})
}

// Delivery is one recorded delivery.
type Delivery struct {
	Admin   bool   // SendAdmin (else SendUser)
	ChatRef string // SendUser: "user:<id>"
	Event   notify.Event
}

// Recorder records every event delivered for one App. Every user is reachable as "user:<id>".
type Recorder struct {
	a  *app.App
	mu sync.Mutex
	d  []Delivery
}

// Attach makes the recording driver active for a and starts a's notify queue with a 1 ms retry backoff.
// Call it before anything emits.
func Attach(t testing.TB, a *app.App) *Recorder {
	t.Helper()
	r := &Recorder{a: a}
	recorders.Store(a, r)
	t.Cleanup(func() { recorders.Delete(a) })
	if _, err := notify.Start(a, notify.Options{Retries: 2, Backoff: func(int) time.Duration { return time.Millisecond }}); err != nil {
		t.Fatalf("notifytest: start queue: %v", err)
	}
	return r
}

// Name implements notify.Notifier.
func (r *Recorder) Name() string { return DriverName }

// ChatRef implements notify.UserAddresser.
func (r *Recorder) ChatRef(u *model.User) string { return "user:" + strconv.FormatUint(u.ID, 10) }

// SendAdmin implements notify.Notifier.
func (r *Recorder) SendAdmin(_ context.Context, ev notify.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.d = append(r.d, Delivery{Admin: true, Event: ev})
	return nil
}

// SendUser implements notify.Notifier.
func (r *Recorder) SendUser(_ context.Context, chatRef string, ev notify.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.d = append(r.d, Delivery{ChatRef: chatRef, Event: ev})
	return nil
}

// Deliveries waits for the queue to drain and returns every delivery so far.
func (r *Recorder) Deliveries(t testing.TB) []Delivery {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := notify.Wait(ctx, r.a); err != nil {
		t.Fatalf("notifytest: queue did not drain: %v", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Delivery(nil), r.d...)
}

// Events returns the administrator events of type typ (all types when typ is "").
func (r *Recorder) Events(t testing.TB, typ notify.Type) []notify.Event {
	t.Helper()
	var out []notify.Event
	for _, d := range r.Deliveries(t) {
		if d.Admin && (typ == "" || d.Event.Type == typ) {
			out = append(out, d.Event)
		}
	}
	return out
}

// UserEvents returns the events of type typ delivered to user id.
func (r *Recorder) UserEvents(t testing.TB, id uint64, typ notify.Type) []notify.Event {
	t.Helper()
	ref := "user:" + strconv.FormatUint(id, 10)
	var out []notify.Event
	for _, d := range r.Deliveries(t) {
		if !d.Admin && d.ChatRef == ref && (typ == "" || d.Event.Type == typ) {
			out = append(out, d.Event)
		}
	}
	return out
}

// Last returns the last administrator event of type typ and fails the test when there is none.
func (r *Recorder) Last(t testing.TB, typ notify.Type) notify.Event {
	t.Helper()
	evs := r.Events(t, typ)
	if len(evs) == 0 {
		t.Fatalf("notifytest: no %s event (got %v)", typ, r.types(t))
	}
	return evs[len(evs)-1]
}

// None fails the test when an administrator event of type typ was delivered.
func (r *Recorder) None(t testing.TB, typ notify.Type) {
	t.Helper()
	if evs := r.Events(t, typ); len(evs) > 0 {
		t.Fatalf("notifytest: unexpected %s event: %+v", typ, evs)
	}
}

func (r *Recorder) types(t testing.TB) []string {
	var out []string
	for _, d := range r.Deliveries(t) {
		side := "user"
		if d.Admin {
			side = "admin"
		}
		out = append(out, side+":"+string(d.Event.Type))
	}
	return out
}

// Fields returns ev's fields as a map.
func Fields(ev notify.Event) map[string]string {
	m := make(map[string]string, len(ev.Fields))
	for _, f := range ev.Fields {
		m[f.Key] = f.Value
	}
	return m
}
