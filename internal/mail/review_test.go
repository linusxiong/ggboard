package mail

import (
	"context"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/logx"
)

// blocking is a mailer that never completes a delivery before its context ends.
type blocking struct{}

func (blocking) Send(ctx context.Context, _ Message) error { <-ctx.Done(); return ctx.Err() }

// Review 2026-10-22: after Stop's deadline the worker still made one full attempt (opts.Timeout, 30 s by
// default) for every queued message, holding shutdown for up to 1024 × 30 s.
func TestReviewStopAbortsAfterDeadline(t *testing.T) {
	d, st := setup(t)
	q, err := NewQueue(d, st, logx.Discard(), QueueOptions{Retries: 1, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	q.SetMailer(blocking{})
	q.Start()
	for range 20 {
		if err := q.Enqueue(Message{To: "a@example.com", Subject: "s", Text: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_ = q.Stop(ctx)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Stop took %v", d)
	}
	if f := q.Stats().Failed; f != 20 {
		t.Fatalf("failed = %d, want 20", f)
	}
}
