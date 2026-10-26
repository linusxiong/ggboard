// Package geoip resolves client IPv4 addresses to a human-readable location ("中国 广东 深圳 电信") for the
// subscription access log. The data is the 纯真 (CZ88) community IP database in the qqwry.dat format, as published
// weekly by github.com/metowolf/qqwry.dat; the file lives in <data_dir>/geoip/ and is refreshed by the daily
// geoip_update job (docs/ARCHITECTURE.md "Scheduler jobs", docs/API.md "GeoIP").
//
// The reader is our own (qqwry.go, on top of golang.org/x/text's GB18030 decoder) rather than a library:
// github.com/xiaoqidun/qqwry (MIT) indexes the bytes without bounds checks, so a truncated or corrupt download
// panics inside a lookup instead of failing validation; it also cannot resolve the last range, keeps its own LRU
// cache and pulls in ipipdotnet/ipdb-go. github.com/zu1k/nali's qqwry package (MIT) only opens a path,
// downloads from its own redirect when the file is missing and calls log.Fatalln on a corrupt file, and the
// module brings a CLI's dependency tree (cobra, viper v1.16, go-github, 7z…). The format is small and stable
// (header, sorted 7-byte index, records with two redirect modes), so ~200 lines that validate every record
// once, then serve lock-free lookups from an immutable []byte swapped atomically, are the smaller and safer
// dependency.
package geoip

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/cache"
)

// Settings keys (internal/settings/defs.go, group subscribe). The URL defaults are the latest release asset and
// the version file of metowolf/qqwry.dat.
const (
	KeyEnable      = "geoip_enable"
	KeyDownloadURL = "geoip_download_url"
	KeyVersionURL  = "geoip_version_url"
)

// JobUpdate is the daily update job (05:23 server time: upstream publishes weekly, a daily check is a small
// version.json request, and the odd minute keeps it away from the hourly jobs).
const (
	JobUpdate     = "geoip_update"
	JobUpdateSpec = "23 5 * * *"
)

// Locator resolves an IP address to a location string; "" means unknown (IPv6, private ranges without a name,
// no database). It never fails.
type Locator interface {
	Locate(ip string) string
}

var _ Locator = (*Service)(nil)

// Errors of Update and Begin.
var (
	ErrBusy     = errors.New("geoip: an update is already running")
	ErrDisabled = errors.New("geoip: IP location lookups are disabled (geoip_enable)")
	ErrStopped  = errors.New("geoip: stopped")
)

// File names inside <data_dir>/geoip.
const (
	datName  = "qqwry.dat"
	metaName = "qqwry.json"
)

// Tunables (variables so tests can use small synthetic files and short delays).
var (
	// minRecords is the least number of ranges a downloaded file must have (the 2026 files have ~1.58 million,
	// the 2022 ones ~530 000).
	minRecords = 100_000
	// probeIP must resolve to a non-empty location in a downloaded file.
	probeIP = netip.MustParseAddr("114.114.114.114")
	// maxDownload caps the download (the file is ~27 MB).
	maxDownload int64 = 128 << 20
	// maxVersionFile caps version.json.
	maxVersionFile int64 = 1 << 20
	// bootstrapDelay is how long after start the first download waits when there is no database.
	bootstrapDelay = 30 * time.Second
	// updateTimeout bounds one update (version check + download).
	updateTimeout = 10 * time.Minute
)

const (
	componentName = "geoip"
	cacheNS       = "geoip"
	cacheTTL      = 24 * time.Hour
)

// Meta describes the database file; it is stored next to it as qqwry.json.
type Meta struct {
	Version   string    `json:"version"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	Records   int       `json:"records"`
	FetchedAt time.Time `json:"fetched_at"`
	CheckedAt time.Time `json:"checked_at"`
	Source    string    `json:"source,omitempty"`
}

// Status is GET /admin/geoip.
type Status struct {
	Enabled     bool       `json:"enabled"`
	Loaded      bool       `json:"loaded"`
	Updating    bool       `json:"updating"`
	Version     string     `json:"version"`
	Records     int        `json:"records"`
	Size        int64      `json:"size"`
	SHA256      string     `json:"sha256"`
	UpdatedAt   *time.Time `json:"updated_at"`
	CheckedAt   *time.Time `json:"checked_at"`
	LastError   string     `json:"last_error"`
	LastErrorAt *time.Time `json:"last_error_at"`
}

// Outcome is the result of one update run.
type Outcome struct {
	Updated bool   // a new file was installed
	Version string // the version in use afterwards
	Err     error
}

// Service owns the database of one App: lookups, status and updates. Build it with Of.
type Service struct {
	a      *app.App
	dir    string
	client *http.Client

	db       atomic.Pointer[DB]
	loadOnce sync.Once
	busy     atomic.Bool

	mu        sync.Mutex // guards the fields below
	meta      Meta
	lastErr   string
	lastErrAt time.Time
	stopped   bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Of returns the App's Service, created on first use (nil after the App is closed; a nil *Service locates
// nothing).
func Of(a *app.App) *Service {
	s, _ := a.Component(componentName, func() any { return newService(a) }).(*Service)
	return s
}

// Locate is Of(a).Locate(ip).
func Locate(a *app.App, ip string) string { return Of(a).Locate(ip) }

func newService(a *app.App) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		a:   a,
		dir: filepath.Join(a.Cfg.DataDir, "geoip"),
		// The default transport: honours HTTPS_PROXY / HTTP_PROXY / NO_PROXY.
		client: &http.Client{Timeout: updateTimeout},
		ctx:    ctx,
		cancel: cancel,
	}
}

func init() {
	app.RegisterJob(JobUpdate, JobUpdateSpec, func(ctx context.Context, a *app.App) error {
		s := Of(a)
		if s == nil {
			return nil
		}
		_, err := s.Update(ctx)
		if errors.Is(err, ErrBusy) || errors.Is(err, ErrDisabled) {
			return nil
		}
		return err
	})
	app.RegisterStartHook(componentName, func(a *app.App) {
		if s := Of(a); s != nil {
			s.boot()
		}
	})
}

// Enabled reports the geoip_enable setting.
func (s *Service) Enabled() bool { return s != nil && s.a.Settings.Bool(KeyEnable) }

// Locate returns the location of ip ("" when unknown or disabled). Results are cached per address until the
// database changes.
func (s *Service) Locate(ip string) string {
	if !s.Enabled() {
		return ""
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return ""
	}
	if addr = addr.Unmap(); !addr.Is4() {
		return ""
	}
	s.ensureLoaded()
	// The key is built before the database is read: a swap stores the new DB and then moves the namespace on,
	// so a result computed from the old DB can only land under an unreachable key.
	key := s.a.Cache.Key(cacheNS, addr.String())
	if v, ok := cache.GetAs[string](s.a.Cache, key); ok {
		return v
	}
	db := s.db.Load()
	if db == nil {
		return ""
	}
	loc := db.Locate(addr)
	s.a.Cache.Set(key, loc, cacheTTL)
	return loc
}

// Status returns the database state, loading the local file first while enabled (read from qqwry.json when
// nothing is loaded, e.g. while disabled).
func (s *Service) Status() Status {
	st := Status{Enabled: s.Enabled(), Updating: s.busy.Load()}
	if st.Enabled {
		s.ensureLoaded()
	}
	s.mu.Lock()
	m, lastErr, lastErrAt := s.meta, s.lastErr, s.lastErrAt
	s.mu.Unlock()
	if db := s.db.Load(); db != nil {
		st.Loaded = true
	} else if m.SHA256 == "" {
		m, _ = s.readMeta()
	}
	st.Version, st.Records, st.Size, st.SHA256 = m.Version, m.Records, m.Size, m.SHA256
	st.UpdatedAt, st.CheckedAt = timePtr(m.FetchedAt), timePtr(m.CheckedAt)
	st.LastError, st.LastErrorAt = lastErr, timePtr(lastErrAt)
	return st
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// Stop cancels a running update and waits for the background work (app.Stopper).
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.cancel()
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// background runs fn on a goroutine that Stop waits for; false once stopped.
func (s *Service) background(fn func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return false
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		fn()
	}()
	return true
}

// boot loads the local file in the background and, when there is none (or it is invalid) and a download URL
// is set, downloads one bootstrapDelay after start. Startup never waits for either.
func (s *Service) boot() {
	if !s.Enabled() {
		return
	}
	s.background(func() {
		s.ensureLoaded()
		if s.db.Load() != nil || s.a.Settings.String(KeyDownloadURL) == "" {
			return
		}
		t := time.NewTimer(bootstrapDelay)
		defer t.Stop()
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
		}
		if _, err := s.Update(s.ctx); err != nil && !errors.Is(err, ErrBusy) && !errors.Is(err, ErrDisabled) {
			s.a.Log.Warn("geoip: first download failed; retrying with the daily job", "err", err)
		}
	})
}

func (s *Service) datPath() string  { return filepath.Join(s.dir, datName) }
func (s *Service) metaPath() string { return filepath.Join(s.dir, metaName) }

func (s *Service) ensureLoaded() { s.loadOnce.Do(s.loadLocal) }

// loadLocal installs <data_dir>/geoip/qqwry.dat when it exists and parses. A file placed by hand (no or
// mismatching qqwry.json) is described from its own content: the embedded release date and the file time.
func (s *Service) loadLocal() {
	path := s.datPath()
	data, err := os.ReadFile(path) //nolint:gosec // G304: <data_dir>/geoip/qqwry.dat, a fixed name under the configured data dir
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		s.recordError(fmt.Errorf("read %s: %w", datName, err))
		return
	}
	db, err := Parse(data)
	if err != nil {
		s.recordError(fmt.Errorf("local %s: %w", datName, err))
		return
	}
	sum := sha256.Sum256(data)
	m, _ := s.readMeta()
	if m.SHA256 != hex.EncodeToString(sum[:]) {
		m = Meta{Version: db.Version(), SHA256: hex.EncodeToString(sum[:])}
		if fi, err := os.Stat(path); err == nil {
			m.FetchedAt = fi.ModTime().UTC()
		}
	}
	m.Size, m.Records = int64(len(data)), db.Count()
	if m.Version == "" {
		m.Version = db.Version()
	}
	s.install(db, m)
	s.a.Log.Info("geoip: database loaded", "version", m.Version, "records", m.Records)
}

// install makes db the active database (then invalidates the cached lookups).
func (s *Service) install(db *DB, m Meta) {
	s.mu.Lock()
	s.meta = m
	s.mu.Unlock()
	s.db.Store(db)
	s.a.Cache.InvalidateNamespace(cacheNS)
}

func (s *Service) recordError(err error) {
	s.mu.Lock()
	s.lastErr, s.lastErrAt = err.Error(), time.Now().UTC()
	s.mu.Unlock()
	s.a.Log.Warn("geoip: "+err.Error(), "dir", s.dir)
}

func (s *Service) readMeta() (Meta, error) {
	var m Meta
	b, err := os.ReadFile(s.metaPath())
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(b, &m)
	return m, err
}

// writeMeta replaces qqwry.json atomically.
func (s *Service) writeMeta(m Meta) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.dir, metaName, b)
}

// writeFileAtomic writes data to dir/name through a synced temporary file and a rename.
func writeFileAtomic(dir, name string, data []byte) error {
	f, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // no-op after the rename
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return err
	}
	syncDir(dir)
	return nil
}

// syncDir makes a rename durable where the platform supports syncing a directory (best effort).
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil { //nolint:gosec // G304: the service's own data directory
		_ = d.Sync()
		_ = d.Close()
	}
}
