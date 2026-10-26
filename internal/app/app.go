// Package app is the dependency container shared by every package: configuration, database, settings,
// caches, mail queue, logger, runtime node state, traffic accumulator and scheduler.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/linusxiong/ggboard/internal/app/online"
	"github.com/linusxiong/ggboard/internal/app/traffic"
	"github.com/linusxiong/ggboard/internal/cache"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/console"
	"github.com/linusxiong/ggboard/internal/db"
	"github.com/linusxiong/ggboard/internal/logx"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/scheduler"
	"github.com/linusxiong/ggboard/internal/settings"
)

// App holds the process-wide dependencies. Build it with New (or Init on a zero value) and release it with
// Close. Never copy an App.
type App struct {
	Cfg       *config.Config
	DB        *gorm.DB
	Settings  *settings.Service
	Cache     *cache.Cache               // best-effort cache (may evict)
	KV        *cache.TTLMap[string, any] // ephemeral state that must not be evicted early (codes, one-time tokens, counters)
	Mailer    *mail.Queue                // asynchronous mail delivery
	Log       *slog.Logger
	Logs      *logx.Ring // recent log records (/admin/system/logs)
	Version   string
	StartedAt time.Time
	Online    *online.Store        // runtime node status and alive IPs
	Traffic   *traffic.Accumulator // buffered traffic, flushed every 10s
	Scheduler *scheduler.Scheduler

	ownsDB    bool
	closeOnce sync.Once
	closeErr  error

	compMu     sync.Mutex
	components map[string]any
	compOrder  []string
	closed     bool
}

type options struct {
	log     *slog.Logger
	ring    *logx.Ring
	db      *gorm.DB
	version string
	mailer  mail.Mailer
	mailOpt mail.QueueOptions
}

// Option customizes New / Init.
type Option func(*options)

// WithLogger uses log (and ring, which may be nil) instead of building one from cfg.Log.
func WithLogger(log *slog.Logger, ring *logx.Ring) Option {
	return func(o *options) { o.log, o.ring = log, ring }
}

// WithDB uses an already-open database (it is not closed by Close). Migrations still run.
func WithDB(d *gorm.DB) Option { return func(o *options) { o.db = d } }

// WithVersion sets App.Version.
func WithVersion(v string) Option { return func(o *options) { o.version = v } }

// WithMailer overrides the settings-selected mail driver (tests use *mail.Memory).
func WithMailer(m mail.Mailer) Option { return func(o *options) { o.mailer = m } }

// WithMailQueueOptions tunes the mail queue (tests shorten the retry backoff).
func WithMailQueueOptions(q mail.QueueOptions) Option { return func(o *options) { o.mailOpt = q } }

// New builds an App: logger, database (+ migrations), settings, caches, mail queue, runtime stores and the
// scheduler with every registered job. Background work starts only with Start.
func New(cfg *config.Config, opts ...Option) (*App, error) {
	a := &App{}
	if err := a.Init(cfg, opts...); err != nil {
		return nil, err
	}
	return a, nil
}

// Init initializes a zero App in place (used by the CLI, which hands *App to subcommand factories before
// configuration is parsed). On error, everything opened so far is closed.
func (a *App) Init(cfg *config.Config, opts ...Option) (err error) {
	if cfg == nil {
		return errors.New("app: nil config")
	}
	o := options{version: "dev"}
	for _, fn := range opts {
		fn(&o)
	}
	a.Cfg, a.Version, a.StartedAt = cfg, o.version, time.Now()

	if o.log == nil {
		ring := logx.NewRing(0)
		var lopts []logx.Option
		// Decided once here, never per record: a terminal gets tint's layout (coloured unless NO_COLOR /
		// log.color=never); systemd, docker and files keep slog's logfmt.
		if st := console.Detect(cfg.Log.Color, os.Stderr); st.Pretty {
			lopts = append(lopts, logx.Pretty(st.Color))
		}
		log, lerr := logx.New(cfg.Log.Level, cfg.Log.Format, os.Stderr, ring, lopts...)
		if lerr != nil {
			return fmt.Errorf("app: logger: %w", lerr)
		}
		o.log, o.ring = log, ring
	}
	if o.ring == nil {
		o.ring = logx.NewRing(0)
	}
	a.Log, a.Logs = o.log, o.ring

	defer func() {
		if err != nil {
			_ = a.Close(context.Background())
		}
	}()

	if o.db != nil {
		a.DB = o.db
	} else {
		if a.DB, err = db.Open(cfg.Database, a.Log); err != nil {
			return err
		}
		a.ownsDB = true
	}
	if err = db.Migrate(a.DB); err != nil {
		return err
	}
	if a.Settings, err = settings.New(a.DB); err != nil {
		return err
	}
	if a.Cache, err = cache.New(0); err != nil {
		return err
	}
	a.KV = cache.NewTTLMap[string, any](10 * time.Minute)
	a.Online = online.New(a.aliveTTL)
	a.Traffic = traffic.New()
	if a.Mailer, err = mail.NewQueue(a.DB, a.Settings, a.Log, o.mailOpt); err != nil {
		return err
	}
	if o.mailer != nil {
		a.Mailer.SetMailer(o.mailer)
	}
	a.Scheduler = scheduler.New(a.Log, time.Local)
	return a.registerJobs()
}

// AliveTTL is max(300s, 2 x server_push_interval) (docs/NODE_PROTOCOLS.md §5.2).
func (a *App) aliveTTL() time.Duration {
	return max(online.DefaultTTL, 2*a.Settings.Duration("server_push_interval", time.Second))
}

// Start launches the mail worker and the scheduler, then runs the start hooks (see RegisterStartHook).
func (a *App) Start() {
	a.Mailer.Start()
	a.Scheduler.Start()
	a.runStartHooks()
}

// FlushTraffic writes buffered traffic to the database.
func (a *App) FlushTraffic(ctx context.Context) error { return a.Traffic.Flush(ctx, a.DB) }

// Close stops the scheduler and the components (see Component), flushes traffic, drains the mail queue and
// closes the cache and database.
// It is safe to call more than once.
func (a *App) Close(ctx context.Context) error {
	a.closeOnce.Do(func() {
		var errs []error
		// Each phase gets its own deadline: a scheduler stop that used up a shared one would leave the final
		// traffic flush with an expired context, losing every buffered report.
		phase := func(fn func(context.Context) error) {
			pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			errs = append(errs, fn(pctx))
		}
		if a.Scheduler != nil {
			phase(a.Scheduler.Stop)
		}
		for _, c := range a.closeComponents() {
			phase(c.Stop)
		}
		if a.Traffic != nil && a.DB != nil {
			phase(a.FlushTraffic)
		}
		if a.Mailer != nil {
			phase(a.Mailer.Stop)
		}
		if a.Cache != nil {
			a.Cache.Close()
		}
		if a.DB != nil && a.ownsDB {
			errs = append(errs, db.Close(a.DB))
		}
		a.closeErr = errors.Join(errs...)
	})
	return a.closeErr
}

// DBDriver returns the dialect name (sqlite, postgres, mysql).
func (a *App) DBDriver() string { return a.DB.Dialector.Name() }

// JobFunc is the body of a job registered with RegisterJob.
type JobFunc func(ctx context.Context, a *App) error

type jobDef struct {
	name, spec string
	fn         JobFunc
}

var (
	jobsMu sync.Mutex
	jobs   = map[string]jobDef{}
)

// RegisterJob declares a cron job for every App built afterwards. Feature packages call it from init():
//
//	func init() { app.RegisterJob("order_timeout", "*/1 * * * *", func(ctx context.Context, a *app.App) error { ... }) }
//
// It panics on duplicate names. Jobs run only after App.Start (i.e. in `ggboard serve`).
func RegisterJob(name, spec string, fn JobFunc) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	if _, dup := jobs[name]; dup {
		panic("app: duplicate job " + name)
	}
	jobs[name] = jobDef{name: name, spec: spec, fn: fn}
}

// Built-in foundation jobs.
const (
	JobOnlineCleanup = "online_cleanup"
	JobTrafficFlush  = "traffic_flush"
)

func (a *App) registerJobs() error {
	if err := a.Scheduler.Register(JobOnlineCleanup, "*/5 * * * *", func(context.Context) error {
		a.Online.Purge()
		a.KV.Purge()
		return nil
	}); err != nil {
		return err
	}
	if err := a.Scheduler.Register(JobTrafficFlush, "@every 10s", a.FlushTraffic); err != nil {
		return err
	}
	jobsMu.Lock()
	defs := make([]jobDef, 0, len(jobs))
	for _, d := range jobs {
		defs = append(defs, d)
	}
	jobsMu.Unlock()
	sort.Slice(defs, func(i, j int) bool { return defs[i].name < defs[j].name })
	for _, d := range defs {
		fn := d.fn
		if err := a.Scheduler.Register(d.name, d.spec, func(ctx context.Context) error { return fn(ctx, a) }); err != nil {
			return err
		}
	}
	return nil
}

// Stopper is implemented by components that own background goroutines (see Component).
type Stopper interface {
	Stop(ctx context.Context) error
}

// Component returns the per-App singleton stored under name, building it with build on first use (build must
// not call Component). Feature packages use it for state that lives as long as the App, such as the notification
// queue. Components implementing Stopper are stopped by Close, newest first, after the scheduler and before the
// final traffic flush (each with its own 10 s budget). After Close, Component returns nil without calling build.
func (a *App) Component(name string, build func() any) any {
	a.compMu.Lock()
	defer a.compMu.Unlock()
	if v, ok := a.components[name]; ok {
		return v
	}
	if a.closed || build == nil {
		return nil
	}
	v := build()
	if a.components == nil {
		a.components = map[string]any{}
	}
	a.components[name] = v
	a.compOrder = append(a.compOrder, name)
	return v
}

// closeComponents marks the App closed for Component and returns the stoppable components, newest first.
func (a *App) closeComponents() []Stopper {
	a.compMu.Lock()
	defer a.compMu.Unlock()
	a.closed = true
	var out []Stopper
	for i := len(a.compOrder) - 1; i >= 0; i-- {
		if s, ok := a.components[a.compOrder[i]].(Stopper); ok {
			out = append(out, s)
		}
	}
	return out
}

// StartHook is the body of a hook registered with RegisterStartHook.
type StartHook func(a *App)

var (
	startHooksMu sync.Mutex
	startHooks   = map[string]StartHook{}
)

// RegisterStartHook declares fn to run once in App.Start (i.e. in `ggboard serve`, after the scheduler started)
// for every App built afterwards; hooks run in name order. Feature packages call it from init() for work that
// must begin with the server, such as a first download. fn must return quickly: slow work belongs in a goroutine
// owned by a Component that implements Stopper, so App.Close can cancel it. It panics on duplicate names.
func RegisterStartHook(name string, fn StartHook) {
	startHooksMu.Lock()
	defer startHooksMu.Unlock()
	if _, dup := startHooks[name]; dup || fn == nil {
		panic("app: duplicate or nil start hook " + name)
	}
	startHooks[name] = fn
}

func (a *App) runStartHooks() {
	startHooksMu.Lock()
	names := make([]string, 0, len(startHooks))
	for n := range startHooks {
		names = append(names, n)
	}
	sort.Strings(names)
	hooks := make([]StartHook, 0, len(names))
	for _, n := range names {
		hooks = append(hooks, startHooks[n])
	}
	startHooksMu.Unlock()
	for _, fn := range hooks {
		fn(a)
	}
}
