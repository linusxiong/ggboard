package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/settings"
)

// Queue errors.
var (
	ErrQueueFull   = errors.New("mail: queue is full")
	ErrQueueClosed = errors.New("mail: queue is closed")
)

// AnonymousSharePercent is the share of the queue that ClassAnonymous messages may occupy; the rest is always
// left to transactional mail.
const AnonymousSharePercent = 75

// QueueOptions tunes the queue. Zero values use the defaults.
type QueueOptions struct {
	Size    int                             // channel capacity (default 1024); also the cap of the retry list
	Retries int                             // extra attempts after a temporary failure (default 3)
	Backoff func(attempt int) time.Duration // delay before retry n (1-based); default 2^(n-1) seconds
	Timeout time.Duration                   // per-attempt timeout (default 30s)
}

// QueueStats is a snapshot for /admin/system/status.
type QueueStats struct {
	Pending int64  `json:"pending"`
	Sent    int64  `json:"sent"`
	Failed  int64  `json:"failed"`
	Driver  string `json:"driver"`
}

// Queue delivers messages asynchronously and records every outcome in mail_logs. One worker sends them in
// order; a temporary failure is put on the worker's retry list and attempted again after its backoff, while the
// worker goes on with the next message. Permanent failures (IsPermanent) are recorded without a retry.
type Queue struct {
	db       *gorm.DB
	settings *settings.Service
	log      *slog.Logger
	renderer *Renderer
	opts     QueueOptions

	anonMu    sync.Mutex // serializes ClassAnonymous admission so concurrent senders cannot overshoot anonLimit
	anonLimit int        // channel occupancy at which ClassAnonymous messages are refused

	reloadMu sync.Mutex // serializes ReloadTemplates so that the last reload always installs the latest rows

	mu       sync.RWMutex // guards ch close vs. send, and override
	ch       chan Message
	closed   bool
	override Mailer
	stop     chan struct{}
	done     chan struct{}
	started  atomic.Bool

	pending atomic.Int64
	sent    atomic.Int64
	failed  atomic.Int64
}

// NewQueue creates a stopped queue; call Start to begin delivering.
func NewQueue(db *gorm.DB, st *settings.Service, log *slog.Logger, opts QueueOptions) (*Queue, error) {
	if opts.Size <= 0 {
		opts.Size = 1024
	}
	if opts.Retries < 0 {
		opts.Retries = 0
	} else if opts.Retries == 0 {
		opts.Retries = 3
	}
	if opts.Backoff == nil {
		opts.Backoff = func(n int) time.Duration { return time.Duration(1<<(n-1)) * time.Second }
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	r, err := NewRenderer()
	if err != nil {
		return nil, err
	}
	q := &Queue{
		db: db, settings: st, log: log.With("component", "mail"), renderer: r, opts: opts,
		anonLimit: max(1, opts.Size*AnonymousSharePercent/100),
		ch:        make(chan Message, opts.Size), stop: make(chan struct{}), done: make(chan struct{}),
	}
	if err := q.ReloadTemplates(context.Background()); err != nil {
		q.log.Error("mail: custom templates not loaded, using the built-in ones", "err", err)
	}
	return q, nil
}

// ReloadTemplates re-reads every mail_templates row into the renderer (the admin API calls it after each write).
// Rows of unknown templates or locales are ignored. Without a database there are no custom templates.
func (q *Queue) ReloadTemplates(ctx context.Context) error {
	q.reloadMu.Lock()
	defer q.reloadMu.Unlock()
	if q.db == nil {
		q.renderer.SetOverrides(nil)
		return nil
	}
	var rows []model.MailTemplate
	if err := q.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return fmt.Errorf("mail: load custom templates: %w", err)
	}
	m := make(map[string]map[string]Custom, len(rows))
	for _, row := range rows {
		if m[row.Name] == nil {
			m[row.Name] = map[string]Custom{}
		}
		m[row.Name][row.Locale] = Custom{Subject: row.Subject, Body: row.Body}
	}
	q.renderer.SetOverrides(m)
	return nil
}

// RenderCustom renders the draft c of template name in locale for recipient to (brand from the current settings),
// ignoring any stored custom template: the admin preview and test mail.
func (q *Queue) RenderCustom(to, name, locale string, c Custom, data Data) (Message, error) {
	b := q.Brand()
	b.Locale = locale
	msg, err := q.renderer.RenderCustom(name, b, c, data)
	if err != nil {
		return Message{}, err
	}
	msg.To = to
	return msg, nil
}

// SetMailer overrides the settings-selected driver (nil restores it). Used by tests and the admin mail test.
func (q *Queue) SetMailer(m Mailer) {
	q.mu.Lock()
	q.override = m
	q.mu.Unlock()
}

// Mailer returns the driver that would be used now.
func (q *Queue) Mailer() (Mailer, error) {
	q.mu.RLock()
	m := q.override
	q.mu.RUnlock()
	if m != nil {
		return m, nil
	}
	return FromSettings(q.settings, q.log)
}

// Brand returns the site identity and locale from settings.
func (q *Queue) Brand() Brand {
	return Brand{
		AppName: q.settings.String("app_name"),
		AppURL:  q.settings.String("app_url"),
		Logo:    q.settings.String("logo"),
		Locale:  q.settings.String("default_locale"),
	}
}

// Render renders a built-in template for recipient to using the current settings.
func (q *Queue) Render(to, template string, data Data) (Message, error) {
	msg, err := q.renderer.Render(template, q.Brand(), data)
	if err != nil {
		return Message{}, err
	}
	msg.To = to
	return msg, nil
}

// SendTemplate renders template and enqueues it for to.
func (q *Queue) SendTemplate(to, template string, data Data) error {
	msg, err := q.Render(to, template, data)
	if err != nil {
		return err
	}
	return q.Enqueue(msg)
}

// Enqueue queues msg without blocking. It returns ErrQueueFull when the queue is full, and for a ClassAnonymous
// message already when AnonymousSharePercent of it is occupied.
func (q *Queue) Enqueue(msg Message) error {
	if msg.To == "" {
		return errors.New("mail: empty recipient")
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrQueueClosed
	}
	if msg.Class == ClassAnonymous {
		q.anonMu.Lock()
		defer q.anonMu.Unlock()
		if len(q.ch) >= q.anonLimit {
			return ErrQueueFull
		}
	}
	q.pending.Add(1)
	select {
	case q.ch <- msg:
		return nil
	default:
		q.pending.Add(-1)
		return ErrQueueFull
	}
}

// SendNow delivers msg synchronously (one attempt) and records it in mail_logs. Used by the admin "test mail".
func (q *Queue) SendNow(ctx context.Context, msg Message) error {
	m, err := q.Mailer()
	if err == nil {
		ctx, cancel := context.WithTimeout(ctx, q.opts.Timeout)
		err = m.Send(ctx, msg)
		cancel()
	}
	q.record(msg, err)
	if err != nil {
		q.failed.Add(1)
		return err
	}
	q.sent.Add(1)
	return nil
}

// Start launches the delivery worker. Calling Start more than once is a no-op.
func (q *Queue) Start() {
	if !q.started.CompareAndSwap(false, true) {
		return
	}
	go q.run()
}

// Stop stops accepting messages and drains the queue until ctx is done. Undelivered messages are logged.
func (q *Queue) Stop(ctx context.Context) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.closed = true
	close(q.ch)
	q.mu.Unlock()
	if !q.started.Load() {
		return nil
	}
	select {
	case <-q.done:
		return nil
	case <-ctx.Done():
		close(q.stop) // abort retries/backoff; the worker records remaining messages as failed
		<-q.done
		return ctx.Err()
	}
}

// Wait blocks until every queued message has been processed or ctx is done.
func (q *Queue) Wait(ctx context.Context) error {
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

// Stats returns queue counters.
func (q *Queue) Stats() QueueStats {
	driver := q.settings.String("email_driver")
	q.mu.RLock()
	if q.override != nil {
		driver = fmt.Sprintf("%T", q.override)
	}
	q.mu.RUnlock()
	return QueueStats{Pending: q.pending.Load(), Sent: q.sent.Load(), Failed: q.failed.Load(), Driver: driver}
}

// job is a message and its delivery history.
type job struct {
	msg      Message
	attempts int       // attempts made so far
	err      error     // error of the last attempt
	due      time.Time // earliest time of the next attempt (retry list only)
}

// run is the single delivery worker. It takes new messages from ch and keeps temporary failures on its own
// retry list (ordered by due time), so a backoff never holds up the messages behind it. It returns when ch is
// closed and every retry has finished, or at once when Stop's deadline passes.
func (q *Queue) run() {
	defer close(q.done)
	ch := q.ch
	var retry []job
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	for ch != nil || len(retry) > 0 {
		if len(retry) > 0 && !time.Now().Before(retry[0].due) {
			j := retry[0]
			retry = slices.Delete(retry, 0, 1)
			retry = q.attempt(j, retry)
			continue
		}
		var wake <-chan time.Time
		if len(retry) > 0 {
			timer.Reset(time.Until(retry[0].due))
			wake = timer.C
		}
		select {
		case msg, ok := <-ch:
			if !ok {
				ch = nil
				continue
			}
			retry = q.attempt(job{msg: msg}, retry)
		case <-wake:
		case <-q.stop:
			q.abort(ch, retry)
			return
		}
	}
}

// errShutdown is recorded for messages still queued (or waiting for a retry) when Stop's deadline passed.
var errShutdown = errors.New("mail: shutdown before delivery")

// attempt makes one delivery attempt of j. A temporary failure with attempts left goes back on retry (returned);
// any other outcome is final.
func (q *Queue) attempt(j job, retry []job) []job {
	err := q.try(j.msg)
	if errors.Is(err, errShutdown) {
		q.finish(j, err)
		return retry
	}
	j.attempts++
	if err == nil || IsPermanent(err) {
		q.finish(j, err)
		return retry
	}
	j.err = err
	switch {
	case q.stopping():
		q.finish(j, errShutdown) // the attempt was cancelled by Stop's deadline
	case j.attempts > q.opts.Retries:
		q.finish(j, err)
	case len(retry) >= q.opts.Size:
		q.log.Warn("mail retry list full, not retrying", "to", j.msg.To, "template", j.msg.Template)
		q.finish(j, err)
	default:
		j.due = time.Now().Add(q.opts.Backoff(j.attempts))
		i := sort.Search(len(retry), func(k int) bool { return retry[k].due.After(j.due) }) // FIFO on ties
		retry = slices.Insert(retry, i, j)
	}
	return retry
}

// try is one delivery attempt with the current driver; it fails with errShutdown after Stop's deadline (one more
// full attempt per queued message, up to opts.Timeout each, would hold shutdown for hours).
func (q *Queue) try(msg Message) error {
	if q.stopping() {
		return errShutdown
	}
	m, err := q.Mailer()
	if err != nil {
		return Permanent(err) // configuration error: an immediate retry will not help
	}
	return q.sendOnce(m, msg)
}

// finish records the final outcome of j.
func (q *Queue) finish(j job, err error) {
	if errors.Is(err, errShutdown) && j.err != nil {
		err = fmt.Errorf("%w (last error: %w)", errShutdown, j.err)
	}
	q.record(j.msg, err)
	if err != nil {
		q.failed.Add(1)
		q.log.Warn("mail delivery failed", "to", j.msg.To, "template", j.msg.Template, "attempts", j.attempts,
			"permanent", IsPermanent(err), "err", err)
	} else {
		q.sent.Add(1)
	}
	q.pending.Add(-1)
}

// abort records every message still waiting as failed (Stop's deadline has passed; Stop closed ch before).
func (q *Queue) abort(ch <-chan Message, retry []job) {
	for _, j := range retry {
		q.finish(j, errShutdown)
	}
	if ch == nil {
		return
	}
	for msg := range ch {
		q.finish(job{msg: msg}, errShutdown)
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

// sendOnce is one delivery attempt bounded by opts.Timeout and aborted when Stop's deadline passes.
func (q *Queue) sendOnce(m Mailer, msg Message) error {
	ctx, cancel := context.WithTimeout(context.Background(), q.opts.Timeout)
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
	return m.Send(ctx, msg)
}

func (q *Queue) record(msg Message, sendErr error) {
	if q.db == nil {
		return
	}
	entry := model.MailLog{Email: msg.To, Subject: truncate(msg.Subject, 255), Template: truncate(msg.Template, 64)}
	if sendErr != nil {
		s := sendErr.Error()
		entry.Error = &s
	}
	if err := q.db.Create(&entry).Error; err != nil {
		q.log.Error("mail: write mail_logs", "err", err)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
