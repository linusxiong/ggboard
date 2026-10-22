package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/logx"
)

func TestRegisterAndRunNow(t *testing.T) {
	s := New(logx.Discard(), time.UTC)
	var runs atomic.Int32
	if err := s.Register("ok", "*/5 * * * *", func(context.Context) error { runs.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("ok", "* * * * *", func(context.Context) error { return nil }); err == nil {
		t.Fatal("duplicate must fail")
	}
	if err := s.Register("bad", "not a spec", func(context.Context) error { return nil }); err == nil {
		t.Fatal("invalid spec must fail")
	}
	if err := s.Register("fail", "@every 1h", func(context.Context) error { return errors.New("boom") }); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("panic", "@daily", func(context.Context) error { panic("kaboom") }); err != nil {
		t.Fatal(err)
	}

	if err := s.RunNow("ok"); err != nil || runs.Load() != 1 {
		t.Fatal("RunNow ok")
	}
	if err := s.RunNow("fail"); err == nil {
		t.Fatal("RunNow must return job errors")
	}
	if err := s.RunNow("panic"); err == nil {
		t.Fatal("panics must become errors")
	}
	if err := s.RunNow("missing"); !errors.Is(err, ErrUnknownJob) {
		t.Fatal("unknown job")
	}

	st := s.Status()
	if len(st) != 3 || st[0].Name != "fail" || st[1].Name != "ok" || st[2].Name != "panic" {
		t.Fatalf("status = %+v", st)
	}
	if st[0].LastError != "boom" || st[0].Failures != 1 || st[1].Runs != 1 || st[1].LastRun == nil || st[1].LastError != "" {
		t.Fatalf("status details = %+v", st)
	}
	if st[2].LastError == "" {
		t.Fatal("panic not recorded")
	}
}

func TestScheduledExecutionAndStop(t *testing.T) {
	s := New(logx.Discard(), nil)
	ran := make(chan struct{}, 1)
	s.MustRegister("tick", "@every 1s", func(ctx context.Context) error {
		select {
		case ran <- struct{}{}:
		default:
		}
		return nil
	})
	s.Start()
	s.Start() // idempotent
	select {
	case <-ran:
	case <-time.After(3 * time.Second):
		t.Fatal("job did not run")
	}
	if st := s.Status(); st[0].NextRun == nil {
		t.Fatal("next run missing while started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOverlapSkipped(t *testing.T) {
	s := New(logx.Discard(), nil)
	release := make(chan struct{})
	started := make(chan struct{})
	var runs atomic.Int32
	s.MustRegister("slow", "@daily", func(context.Context) error {
		runs.Add(1)
		close(started)
		<-release
		return nil
	})
	go func() { _ = s.RunNow("slow") }()
	<-started
	if err := s.RunNow("slow"); err != nil {
		t.Fatal(err)
	}
	if !s.Status()[0].Running {
		t.Fatal("job must be running")
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if runs.Load() != 1 {
		t.Fatalf("runs = %d", runs.Load())
	}
}
