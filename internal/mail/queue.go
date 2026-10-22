package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// QueueOptions tunes the queue. Zero values use the defaults.
type QueueOptions struct {
	Size    int                             // channel capacity (default 1024)
	Retries int                             // extra attempts after the first failure (default 3)
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

// Queue delivers messages asynchronously with retries and records every outcome in mail_logs.
type Queue struct {
	db       *gorm.DB
	settings *settings.Service
	log      *slog.Logger
	renderer *Renderer
	opts     QueueOptions

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
	return &Queue{
		db: db, settings: st, log: log.With("component", "mail"), renderer: r, opts: opts,
		ch: make(chan Message, opts.Size), stop: make(chan struct{}), done: make(chan struct{}),
	}, nil
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

// Enqueue queues msg without blocking.
func (q *Queue) Enqueue(msg Message) error {
	if msg.To == "" {
		return errors.New("mail: empty recipient")
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrQueueClosed
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

func (q *Queue) run() {
	defer close(q.done)
	for msg := range q.ch {
		err := q.deliver(msg)
		q.record(msg, err)
		if err != nil {
			q.failed.Add(1)
			q.log.Warn("mail delivery failed", "to", msg.To, "template", msg.Template, "err", err)
		} else {
			q.sent.Add(1)
		}
		q.pending.Add(-1)
	}
}

func (q *Queue) deliver(msg Message) error {
	var err error
	for attempt := 0; attempt <= q.opts.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-q.stop:
				return fmt.Errorf("mail: shutdown before retry: %w", err)
			case <-time.After(q.opts.Backoff(attempt)):
			}
		}
		var m Mailer
		if m, err = q.Mailer(); err != nil {
			return err // configuration error: retrying will not help
		}
		ctx, cancel := context.WithTimeout(context.Background(), q.opts.Timeout)
		err = m.Send(ctx, msg)
		cancel()
		if err == nil {
			return nil
		}
	}
	return err
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
