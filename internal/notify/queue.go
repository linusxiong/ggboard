package notify

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
)

// Queue errors.
var (
	ErrQueueFull   = errors.New("notify: queue is full")
	ErrQueueClosed = errors.New("notify: queue is closed")
	errShutdown    = errors.New("notify: shutdown before delivery")
)

// Options tunes a queue. Zero values use the defaults.
type Options struct {
	Size          int                             // channel capacity (default 1024)
	Retries       int                             // extra attempts after a failure (default 3; negative = none)
	Backoff       func(attempt int) time.Duration // delay before retry n (1-based); default 2^(n-1) s: 1 s, 2 s, 4 s
	Timeout       time.Duration                   // per attempt (default 20 s; the Telegram client has its own 10 s)
	MaxRetryAfter time.Duration                   // cap on a driver's retry_after (default 60 s)
}

func (o Options) withDefaults() Options {
	if o.Size <= 0 {
		o.Size = 1024
	}
	switch {
	case o.Retries < 0:
		o.Retries = 0
	case o.Retries == 0:
		o.Retries = 3
	}
	if o.Backoff == nil {
		o.Backoff = func(n int) time.Duration { return time.Duration(1<<(n-1)) * time.Second }
	}
	if o.Timeout <= 0 {
		o.Timeout = 20 * time.Second
	}
	if o.MaxRetryAfter <= 0 {
		o.MaxRetryAfter = time.Minute
	}
	return o
}

// QueueStats is the "notify" object of /admin/system/status: queued events and delivery outcomes (one per
// driver and recipient side) since start, plus the configured drivers.
type QueueStats struct {
	Pending int64    `json:"pending"`
	Sent    int64    `json:"sent"`
	Failed  int64    `json:"failed"`
	Drivers []string `json:"drivers"`
}

// Queue delivers events to every configured notifier on one worker goroutine.
type Queue struct {
	a    *app.App
	opts Options

	mu     sync.RWMutex // guards ch close vs. send
	ch     chan Event
	closed bool
	stop   chan struct{}
	done   chan struct{}

	pending atomic.Int64
	sent    atomic.Int64
	failed  atomic.Int64
}

const componentName = "notify.queue"

func newQueue(a *app.App, opts Options) *Queue {
	opts = opts.withDefaults()
	q := &Queue{a: a, opts: opts, ch: make(chan Event, opts.Size), stop: make(chan struct{}), done: make(chan struct{})}
	go q.run()
	return q
}

// Start creates a's queue with opts and starts its worker. Tests use it to shorten the retry backoff; elsewhere
// the first Emit creates the queue with the defaults. It fails when a's queue already exists or a is closed.
func Start(a *app.App, opts Options) (*Queue, error) {
	var created *Queue
	v := a.Component(componentName, func() any {
		created = newQueue(a, opts)
		return created
	})
	if v == nil {
		return nil, ErrQueueClosed
	}
	if created == nil {
		return nil, errors.New("notify: queue already started")
	}
	return created, nil
}

// queueFor returns a's queue, creating it on first use (nil once a is closed).
func queueFor(a *app.App) *Queue {
	q, _ := a.Component(componentName, func() any { return newQueue(a, Options{}) }).(*Queue)
	return q
}

// existing returns a's queue without creating it.
func existing(a *app.App) *Queue {
	q, _ := a.Component(componentName, nil).(*Queue)
	return q
}

// Emit queues ev for delivery without blocking. Nothing happens when no driver is configured. When the queue is
// full or closed the event is dropped, logged (WARN) and counted as failed.
func Emit(a *app.App, ev Event) {
	if a == nil || len(Active(a)) == 0 {
		return
	}
	q := queueFor(a)
	if q == nil {
		a.Log.Warn("notification dropped", "type", ev.Type, "err", ErrQueueClosed)
		return
	}
	if err := q.enqueue(ev); err != nil {
		q.failed.Add(1)
		a.Log.Warn("notification dropped", "type", ev.Type, "err", err)
	}
}

// EmitWait queues ev, waiting while the queue is full until ctx is done. Producers that emit one event per user
// (reminder jobs) use it instead of Emit so a large batch is throttled instead of dropped.
func EmitWait(ctx context.Context, a *app.App, ev Event) error {
	if len(Active(a)) == 0 {
		return nil
	}
	q := queueFor(a)
	if q == nil {
		return ErrQueueClosed
	}
	delay := 20 * time.Millisecond
	for {
		err := q.enqueue(ev)
		if !errors.Is(err, ErrQueueFull) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay = min(2*delay, time.Second)
	}
}

// Wait blocks until every queued event has been processed or ctx is done (tests, shutdown checks).
func Wait(ctx context.Context, a *app.App) error {
	q := existing(a)
	if q == nil {
		return nil
	}
	t := time.NewTicker(5 * time.Millisecond)
	defer t.Stop()
	for q.pending.Load() > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	return nil
}

// Stats returns a's queue counters and the configured drivers.
func Stats(a *app.App) QueueStats {
	st := QueueStats{Drivers: ActiveNames(a)}
	if q := existing(a); q != nil {
		st.Pending, st.Sent, st.Failed = q.pending.Load(), q.sent.Load(), q.failed.Load()
	}
	return st
}

func (q *Queue) enqueue(ev Event) error {
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrQueueClosed
	}
	q.pending.Add(1)
	select {
	case q.ch <- ev:
		return nil
	default:
		q.pending.Add(-1)
		return ErrQueueFull
	}
}

// Stop stops accepting events and drains the queue until ctx is done; then pending retries are abandoned and
// the remaining events are counted as failed. Close calls it (the queue is an app.Component).
func (q *Queue) Stop(ctx context.Context) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.closed = true
	close(q.ch)
	q.mu.Unlock()
	select {
	case <-q.done:
		return nil
	case <-ctx.Done():
		close(q.stop)
		<-q.done
		return ctx.Err()
	}
}

func (q *Queue) run() {
	defer close(q.done)
	for ev := range q.ch {
		q.process(ev)
		q.pending.Add(-1)
	}
}

// process hands ev to every configured notifier: the administrator side when ev.Admin, the user side when
// ev.UserID is reachable on that channel.
func (q *Queue) process(ev Event) {
	var user *model.User
	if ev.UserID != nil {
		user = q.loadUser(*ev.UserID)
	}
	for _, n := range Active(q.a) {
		if ev.Admin {
			q.deliver(n, ev, "admin", func(ctx context.Context) error { return n.SendAdmin(ctx, ev) })
		}
		if user == nil {
			continue
		}
		if ua, ok := n.(UserAddresser); ok {
			if ref := ua.ChatRef(user); ref != "" {
				q.deliver(n, ev, "user", func(ctx context.Context) error { return n.SendUser(ctx, ref, ev) })
			}
		}
	}
}

func (q *Queue) loadUser(id uint64) *model.User {
	var u model.User
	err := q.a.DB.Select("id", "email", "telegram_id", "telegram_username", "banned").Where("id = ?", id).Limit(1).Find(&u).Error
	if err != nil {
		q.a.Log.Warn("notify: load recipient", "user_id", id, "err", err)
		return nil
	}
	if u.ID == 0 {
		return nil
	}
	return &u
}

// deliver runs send with retries and counts the outcome. Retries wait opts.Backoff, or the driver's
// retry_after when longer (capped at opts.MaxRetryAfter); permanent errors and ErrSkipped end at once.
func (q *Queue) deliver(n Notifier, ev Event, side string, send func(context.Context) error) {
	p := &progress{}
	var err error
	for attempt := 0; ; attempt++ {
		if q.stopping() {
			err = shutdownError(err)
			break
		}
		err = q.sendOnce(p, send)
		if err == nil || errors.Is(err, ErrSkipped) || IsPermanent(err) || attempt >= q.opts.Retries {
			break
		}
		wait := q.opts.Backoff(attempt + 1)
		var ra RetryAfterError
		if errors.As(err, &ra) {
			wait = max(wait, min(ra.RetryAfter(), q.opts.MaxRetryAfter))
		}
		select {
		case <-q.stop:
		case <-time.After(wait):
		}
	}
	switch {
	case err == nil:
		q.sent.Add(1)
	case errors.Is(err, ErrSkipped):
	default:
		q.failed.Add(1)
		q.a.Log.Warn("notification failed", "driver", n.Name(), "type", ev.Type, "to", side, "err", err)
	}
}

// stopping reports whether Stop's deadline has passed.
func (q *Queue) stopping() bool {
	select {
	case <-q.stop:
		return true
	default:
		return false
	}
}

func shutdownError(last error) error {
	if last == nil {
		return errShutdown
	}
	return fmt.Errorf("%w (last error: %w)", errShutdown, last)
}

// sendOnce is one attempt bounded by opts.Timeout and aborted when Stop's deadline passes.
func (q *Queue) sendOnce(p *progress, send func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(withProgress(context.Background(), p), q.opts.Timeout)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-q.stop:
			cancel()
		case <-done:
		}
	}()
	return send(ctx)
}
