// Package scheduler runs named cron jobs (robfig/cron v3) with per-job status tracking, panic recovery and
// overlap prevention. Specs use the standard 5-field format or descriptors such as "@every 10s".
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Func is a job body. ctx is cancelled when the scheduler stops.
type Func func(ctx context.Context) error

// JobStatus is the observable state of a job (served by /admin/system/status).
type JobStatus struct {
	Name         string     `json:"name"`
	Spec         string     `json:"spec"`
	Running      bool       `json:"running"`
	LastRun      *time.Time `json:"last_run"`
	LastDuration string     `json:"last_duration,omitempty"`
	LastError    string     `json:"last_error"`
	NextRun      *time.Time `json:"next_run"`
	Runs         int64      `json:"runs"`
	Failures     int64      `json:"failures"`
}

type job struct {
	name, spec string
	fn         Func
	id         cron.EntryID

	mu      sync.Mutex
	running bool
	lastRun time.Time
	lastDur time.Duration
	lastErr string
	runs    int64
	fails   int64
}

// Scheduler owns a cron instance and its registered jobs.
type Scheduler struct {
	cron   *cron.Cron
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	jobs    map[string]*job
	started bool
}

// ErrUnknownJob is returned by RunNow for unregistered names.
var ErrUnknownJob = errors.New("scheduler: unknown job")

// New creates a stopped scheduler evaluating specs in loc (time.Local when nil).
func New(log *slog.Logger, loc *time.Location) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	if loc == nil {
		loc = time.Local
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		cron:   cron.New(cron.WithLocation(loc)),
		log:    log.With("component", "scheduler"),
		ctx:    ctx,
		cancel: cancel,
		jobs:   map[string]*job{},
	}
}

// Register adds a job. Names must be unique; spec is validated immediately.
func (s *Scheduler) Register(name, spec string, fn Func) error {
	if name == "" || fn == nil {
		return errors.New("scheduler: name and fn are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.jobs[name]; dup {
		return fmt.Errorf("scheduler: duplicate job %q", name)
	}
	j := &job{name: name, spec: spec, fn: fn}
	id, err := s.cron.AddFunc(spec, func() { _ = s.run(j) }) // errors are logged and recorded by run
	if err != nil {
		return fmt.Errorf("scheduler: job %q: invalid spec %q: %w", name, spec, err)
	}
	j.id = id
	s.jobs[name] = j
	return nil
}

// MustRegister is Register that panics on error (for static registrations).
func (s *Scheduler) MustRegister(name, spec string, fn Func) {
	if err := s.Register(name, spec, fn); err != nil {
		panic(err)
	}
}

// Start begins scheduling. It is a no-op when already started.
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	s.cron.Start()
	s.log.Info("scheduler started", "jobs", len(s.jobs))
}

// Stop stops scheduling, cancels running jobs' context and waits for them until ctx is done.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	wasStarted := s.started
	s.started = false
	s.mu.Unlock()
	if wasStarted {
		<-s.cron.Stop().Done()
	}
	s.cancel()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RunNow runs a job synchronously (skipped, returning nil, if it is already running).
func (s *Scheduler) RunNow(name string) error {
	s.mu.Lock()
	j, ok := s.jobs[name]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w %q", ErrUnknownJob, name)
	}
	return s.run(j)
}

// Status returns every job's status sorted by name.
func (s *Scheduler) Status() []JobStatus {
	s.mu.Lock()
	jobs := make([]*job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	s.mu.Unlock()
	sort.Slice(jobs, func(a, b int) bool { return jobs[a].name < jobs[b].name })
	out := make([]JobStatus, 0, len(jobs))
	for _, j := range jobs {
		next := s.cron.Entry(j.id).Next
		j.mu.Lock()
		st := JobStatus{Name: j.name, Spec: j.spec, Running: j.running, LastError: j.lastErr, Runs: j.runs, Failures: j.fails}
		if !j.lastRun.IsZero() {
			t := j.lastRun
			st.LastRun = &t
			st.LastDuration = j.lastDur.String()
		}
		j.mu.Unlock()
		if !next.IsZero() {
			st.NextRun = &next
		}
		out = append(out, st)
	}
	return out
}

func (s *Scheduler) run(j *job) (err error) {
	j.mu.Lock()
	if j.running {
		j.mu.Unlock()
		s.log.Warn("job still running; skipped", "job", j.name)
		return nil
	}
	j.running = true
	j.mu.Unlock()
	s.wg.Add(1)
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			s.log.Error("job panicked", "job", j.name, "panic", r, "stack", string(debug.Stack()))
		}
		dur := time.Since(start)
		j.mu.Lock()
		j.running = false
		j.lastRun = start
		j.lastDur = dur
		j.runs++
		if err != nil {
			j.fails++
			j.lastErr = err.Error()
		} else {
			j.lastErr = ""
		}
		j.mu.Unlock()
		s.wg.Done()
		if err != nil {
			s.log.Error("job failed", "job", j.name, "err", err, "duration", dur)
		} else {
			s.log.Debug("job done", "job", j.name, "duration", dur)
		}
	}()
	return j.fn(s.ctx)
}
