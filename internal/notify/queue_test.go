package notify_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// fake is a scriptable notifier registered as driver "fake" for the Apps in fakes.
type fake struct {
	mu       sync.Mutex
	attempts int
	admin    []notify.Event
	user     []string // "<chatRef>:<type>"
	script   []error  // errors returned by successive SendAdmin attempts (nil = success); empty = always succeed
	block    chan struct{}
	entered  chan struct{} // receives once per SendAdmin call while block is set
	targets  []string      // fan-out destinations handled with Done/MarkDone
	sentTo   []string
	failOnce map[string]bool // fan-out targets failing on their first attempt
}

var fakes sync.Map // *app.App -> *fake

func init() {
	notify.Register("fake", func(a *app.App) notify.Notifier {
		if f, ok := fakes.Load(a); ok {
			return f.(*fake)
		}
		return nil
	})
}

func (f *fake) Name() string { return "fake" }

func (f *fake) ChatRef(u *model.User) string {
	if u.TelegramID == nil {
		return ""
	}
	return strconv.FormatInt(*u.TelegramID, 10)
}

func (f *fake) SendAdmin(ctx context.Context, ev notify.Event) error {
	if f.block != nil {
		f.entered <- struct{}{}
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if len(f.targets) > 0 {
		var errs []error
		for _, t := range f.targets {
			if notify.Done(ctx, t) {
				continue
			}
			if f.failOnce[t] {
				delete(f.failOnce, t)
				errs = append(errs, fmt.Errorf("target %s down", t))
				continue
			}
			f.sentTo = append(f.sentTo, t)
			notify.MarkDone(ctx, t)
		}
		return errors.Join(errs...)
	}
	if len(f.script) > 0 {
		err := f.script[0]
		f.script = f.script[1:]
		if err != nil {
			return err
		}
	}
	f.admin = append(f.admin, ev)
	return nil
}

func (f *fake) SendUser(_ context.Context, ref string, ev notify.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.user = append(f.user, ref+":"+string(ev.Type))
	return nil
}

func (f *fake) snapshot() (attempts int, admin []notify.Event, user, sentTo []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attempts, append([]notify.Event(nil), f.admin...), append([]string(nil), f.user...), append([]string(nil), f.sentTo...)
}

type retryAfter struct{ d time.Duration }

func (e retryAfter) Error() string             { return "slow down" }
func (e retryAfter) RetryAfter() time.Duration { return e.d }

func setup(t *testing.T, f *fake, opts notify.Options) *app.App {
	t.Helper()
	a := testutil.NewTestApp(t)
	fakes.Store(a, f)
	t.Cleanup(func() { fakes.Delete(a) })
	if opts.Backoff == nil {
		opts.Backoff = func(int) time.Duration { return time.Millisecond }
	}
	if _, err := notify.Start(a, opts); err != nil {
		t.Fatal(err)
	}
	return a
}

func drain(t *testing.T, a *app.App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := notify.Wait(ctx, a); err != nil {
		t.Fatal(err)
	}
}

func event(a *app.App) notify.Event {
	return notify.New(a, notify.TypeOrderNew, "body", notify.F("email", "a@example.com"))
}

func TestRetryWithBackoffThenSuccess(t *testing.T) {
	var (
		mu    sync.Mutex
		waits []int
	)
	f := &fake{script: []error{errors.New("boom"), errors.New("boom")}}
	a := setup(t, f, notify.Options{Backoff: func(n int) time.Duration {
		mu.Lock()
		waits = append(waits, n)
		mu.Unlock()
		return time.Millisecond
	}})
	notify.Emit(a, event(a))
	drain(t, a)
	attempts, admin, _, _ := f.snapshot()
	if attempts != 3 || len(admin) != 1 {
		t.Fatalf("attempts = %d, delivered = %d", attempts, len(admin))
	}
	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(waits) != "[1 2]" {
		t.Fatalf("backoff attempts = %v", waits)
	}
	st := notify.Stats(a)
	if st.Sent != 1 || st.Failed != 0 || st.Pending != 0 || fmt.Sprint(st.Drivers) != "[fake]" {
		t.Fatalf("stats = %+v", st)
	}
}

func TestRetriesExhaustedAreCountedAndLogged(t *testing.T) {
	boom := errors.New("upstream down")
	f := &fake{script: []error{boom, boom, boom, boom, boom}}
	a := setup(t, f, notify.Options{})
	notify.Emit(a, event(a))
	drain(t, a)
	if attempts, _, _, _ := f.snapshot(); attempts != 4 { // first try + 3 retries
		t.Fatalf("attempts = %d", attempts)
	}
	if st := notify.Stats(a); st.Sent != 0 || st.Failed != 1 {
		t.Fatalf("stats = %+v", st)
	}
	found := false
	for _, e := range a.Logs.Entries(slog.LevelWarn, 0) {
		if e.Msg == "notification failed" && e.Level == "warn" && strings.Contains(fmt.Sprint(e.Attrs["err"]), "upstream down") {
			found = true
		}
	}
	if !found {
		t.Fatal("a failed delivery must be logged at WARN")
	}
}

func TestPermanentAndSkippedErrors(t *testing.T) {
	f := &fake{script: []error{notify.Permanent(errors.New("chat not found")), notify.ErrSkipped}}
	a := setup(t, f, notify.Options{})
	notify.Emit(a, event(a))
	notify.Emit(a, event(a))
	drain(t, a)
	if attempts, _, _, _ := f.snapshot(); attempts != 2 {
		t.Fatalf("permanent errors and skips must not be retried: %d attempts", attempts)
	}
	if st := notify.Stats(a); st.Sent != 0 || st.Failed != 1 {
		t.Fatalf("stats = %+v (skipped counts as neither)", st)
	}
}

func TestRetryAfterIsHonouredAndCapped(t *testing.T) {
	f := &fake{script: []error{retryAfter{80 * time.Millisecond}}}
	a := setup(t, f, notify.Options{})
	start := time.Now()
	notify.Emit(a, event(a))
	drain(t, a)
	if d := time.Since(start); d < 80*time.Millisecond {
		t.Fatalf("retried after %s, before retry_after", d)
	}

	f2 := &fake{script: []error{retryAfter{time.Hour}}}
	a2 := setup(t, f2, notify.Options{MaxRetryAfter: 20 * time.Millisecond})
	start = time.Now()
	notify.Emit(a2, event(a2))
	drain(t, a2)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("retry_after must be capped by MaxRetryAfter, waited %s", d)
	}
	if _, admin, _, _ := f2.snapshot(); len(admin) != 1 {
		t.Fatal("event not delivered after the capped wait")
	}
}

func TestFanOutRetrySkipsDoneTargets(t *testing.T) {
	f := &fake{targets: []string{"a", "b", "c"}, failOnce: map[string]bool{"b": true}}
	a := setup(t, f, notify.Options{})
	notify.Emit(a, event(a))
	drain(t, a)
	_, _, _, sentTo := f.snapshot()
	if strings.Join(sentTo, ",") != "a,c,b" {
		t.Fatalf("sent to %v: a retry must only resend to the failed target", sentTo)
	}
	if st := notify.Stats(a); st.Sent != 1 || st.Failed != 0 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestUserDelivery(t *testing.T) {
	f := &fake{}
	a := setup(t, f, notify.Options{})
	bound := testutil.CreateUser(t, a, "bound@example.com")
	tg := int64(4242)
	if err := a.DB.Model(&model.User{}).Where("id = ?", bound.ID).Update("telegram_id", tg).Error; err != nil {
		t.Fatal(err)
	}
	unbound := testutil.CreateUser(t, a, "unbound@example.com")

	notify.Emit(a, notify.RemindExpire(a, bound.ID, "2026-10-23 10:00"))
	notify.Emit(a, notify.RemindExpire(a, unbound.ID, "2026-10-23 10:00"))
	notify.Emit(a, notify.RemindExpire(a, 999999, "2026-10-23 10:00")) // unknown user
	notify.Emit(a, notify.OrderPaid(a, notify.Order{UserID: bound.ID, Email: bound.Email, Plan: "Pro", Period: model.PeriodMonthly, Amount: 1200, TradeNo: "T1"}))
	drain(t, a)
	_, admin, user, _ := f.snapshot()
	if fmt.Sprint(user) != "[4242:remind_expire 4242:order_paid]" {
		t.Fatalf("user deliveries = %v", user)
	}
	if len(admin) != 1 || admin[0].Type != notify.TypeOrderPaid {
		t.Fatalf("reminders are user-only, order_paid goes to admins too: %+v", admin)
	}
	if !notify.Reachable(a, &model.User{TelegramID: &tg}) || notify.Reachable(a, unbound) {
		t.Fatal("Reachable")
	}
}

func TestQueueFullDropsAndEmitWaitThrottles(t *testing.T) {
	f := &fake{block: make(chan struct{}), entered: make(chan struct{}, 8)}
	a := setup(t, f, notify.Options{Size: 2})
	notify.Emit(a, event(a))
	<-f.entered // the worker holds the first event
	for range 5 {
		notify.Emit(a, event(a)) // 2 queued, the other 3 dropped
	}
	st := notify.Stats(a)
	if st.Failed != 3 || st.Pending != 3 {
		t.Fatalf("stats while full = %+v", st)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	if err := notify.EmitWait(ctx, a, event(a)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("EmitWait on a full queue = %v", err)
	}
	cancel()
	close(f.block)
	drain(t, a)
	if _, admin, _, _ := f.snapshot(); len(admin) != int(st.Pending) {
		t.Fatalf("delivered %d, queued %d", len(admin), st.Pending)
	}
}

// Bulk producers (reminders) must not lose events beyond the queue capacity (MISTAKES #40).
func TestEmitWaitDeliversMoreThanCapacity(t *testing.T) {
	f := &fake{}
	a := setup(t, f, notify.Options{})
	const n = 1100
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for range n {
		if err := notify.EmitWait(ctx, a, event(a)); err != nil {
			t.Fatal(err)
		}
	}
	drain(t, a)
	if _, admin, _, _ := f.snapshot(); len(admin) != n {
		t.Fatalf("delivered %d of %d", len(admin), n)
	}
	if st := notify.Stats(a); st.Sent != n || st.Failed != 0 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestCloseDrainsTheQueue(t *testing.T) {
	f := &fake{}
	a := setup(t, f, notify.Options{})
	for range 20 {
		notify.Emit(a, event(a))
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, admin, _, _ := f.snapshot(); len(admin) != 20 {
		t.Fatalf("delivered %d of 20 before Close returned", len(admin))
	}
	notify.Emit(a, event(a)) // after Close: dropped, no panic
	if err := notify.EmitWait(context.Background(), a, event(a)); !errors.Is(err, notify.ErrQueueClosed) {
		t.Fatalf("EmitWait after Close = %v", err)
	}
}

func TestStopDeadlineAbandonsRetries(t *testing.T) {
	var attempts atomic.Int32
	f := &fake{script: []error{errors.New("down"), errors.New("down"), errors.New("down"), errors.New("down")}}
	a := setup(t, f, notify.Options{Backoff: func(int) time.Duration {
		attempts.Add(1)
		return time.Hour
	}})
	notify.Emit(a, event(a))
	for attempts.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	start := time.Now()
	// app.Close gives the queue 10 s; a component stopped with a short deadline must abandon the hour-long wait.
	q, _ := a.Component("notify.queue", nil).(interface{ Stop(context.Context) error })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := q.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop = %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("Stop waited %s", d)
	}
	if st := notify.Stats(a); st.Failed != 1 || st.Pending != 0 {
		t.Fatalf("stats = %+v", st)
	}
}

// EmitSubscriptionLeak can be assigned to subscription.LeakHook (func(a *app.App, u *model.User, distinct int)).
var _ func(*app.App, *model.User, int) = notify.EmitSubscriptionLeak

func TestEmitSubscriptionLeak(t *testing.T) {
	f := &fake{}
	a := setup(t, f, notify.Options{})
	notify.EmitSubscriptionLeak(a, &model.User{ID: 1, Email: "leak@example.com"}, 9)
	drain(t, a)
	if _, admin, _, _ := f.snapshot(); len(admin) != 1 || admin[0].Type != notify.TypeSubscriptionLeak || admin[0].Field("distinct") != "9" {
		t.Fatalf("delivered = %+v", admin)
	}
}

func TestNoDriverNoQueue(t *testing.T) {
	a := testutil.NewTestApp(t)
	notify.Emit(a, event(a))
	st := notify.Stats(a)
	if st.Pending != 0 || st.Sent != 0 || st.Failed != 0 || st.Drivers == nil || len(st.Drivers) != 0 {
		t.Fatalf("stats = %+v", st)
	}
	if a.Component("notify.queue", nil) != nil {
		t.Fatal("Emit without a configured driver must not start a queue")
	}
	if notify.Enabled(a) {
		t.Fatal("Enabled")
	}
	if _, err := notify.Start(a, notify.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := notify.Start(a, notify.Options{}); err == nil {
		t.Fatal("a second Start must fail")
	}
}
