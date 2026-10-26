package geoip_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/geoip"
	"github.com/linusxiong/ggboard/internal/geoip/geoiptest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// mirror is a fake release server: /version.json and /qqwry.dat.
type mirror struct {
	*httptest.Server
	mu        sync.Mutex
	latest    string
	file      []byte
	gate      chan struct{} // when set, /qqwry.dat waits for it to be closed
	versions  atomic.Int32
	downloads atomic.Int32
	agent     atomic.Value
}

func newMirror(t *testing.T, latest string, file []byte) *mirror {
	t.Helper()
	m := &mirror{latest: latest, file: file}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.agent.Store(r.UserAgent())
		m.mu.Lock()
		latest, file, gate := m.latest, m.file, m.gate
		m.mu.Unlock()
		switch r.URL.Path {
		case "/version.json":
			m.versions.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"latest": latest, "versions": map[string]any{latest: map[string]int{"count": 1}}})
		case "/qqwry.dat":
			m.downloads.Add(1)
			if gate != nil {
				<-gate
			}
			_, _ = w.Write(file)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(m.Close)
	return m
}

func (m *mirror) set(latest string, file []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latest, m.file = latest, file
}

// hold makes downloads wait until the returned release is called (also at cleanup, before the server closes,
// so a failing test cannot hang on a blocked handler).
func (m *mirror) hold(t *testing.T) (release func()) {
	t.Helper()
	gate := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	m.mu.Lock()
	m.gate = gate
	m.mu.Unlock()
	t.Cleanup(release)
	return release
}

func newApp(t *testing.T, m *mirror, extra map[string]any) *app.App {
	t.Helper()
	geoip.SetLimits(t, 5, 1<<20, 10*time.Millisecond)
	s := map[string]any{"geoip_download_url": m.URL + "/qqwry.dat", "geoip_version_url": m.URL + "/version.json"}
	for k, v := range extra {
		s[k] = v
	}
	return testutil.NewTestApp(t, testutil.WithSettings(s))
}

// release returns Sample with one range relabelled, so two releases differ in content.
func release(version, shenzhen string) []byte {
	rs := append([]geoiptest.Range(nil), geoiptest.Sample...)
	for i := range rs {
		if rs[i].Start == "203.0.113.0" {
			rs[i].Area = shenzhen
		}
	}
	return geoiptest.Build(rs, version)
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestUpdateDownloadsSwapsAndSkips(t *testing.T) {
	v1 := release("20261014", "电信")
	m := newMirror(t, "20261014", v1)
	a := newApp(t, m, nil)
	s := geoip.Of(a)
	if got := s.Locate("203.0.113.9"); got != "" {
		t.Fatalf("no database yet, Locate = %q", got)
	}
	if st := s.Status(); st.Loaded || st.Version != "" || st.UpdatedAt != nil || st.CheckedAt != nil || !st.Enabled {
		t.Fatalf("empty status = %+v", st)
	}

	out, err := s.Update(t.Context())
	if err != nil || !out.Updated || out.Version != "20261014" {
		t.Fatalf("first update = %+v, %v", out, err)
	}
	if got := s.Locate("203.0.113.9"); got != "中国 广东 深圳 电信" {
		t.Fatalf("Locate = %q", got)
	}
	st := s.Status()
	if !st.Loaded || st.Version != "20261014" || st.Records != len(geoiptest.Sample)+1 || st.Size != int64(len(v1)) ||
		st.SHA256 != sha(v1) || st.UpdatedAt == nil || st.CheckedAt == nil || st.LastError != "" || st.Updating {
		t.Fatalf("status = %+v", st)
	}
	if ua, _ := m.agent.Load().(string); !strings.HasPrefix(ua, "ggboard/test") {
		t.Fatalf("User-Agent = %q", ua)
	}
	dir := filepath.Join(a.Cfg.DataDir, "geoip")
	if b, err := os.ReadFile(filepath.Join(dir, "qqwry.dat")); err != nil || sha(b) != sha(v1) {
		t.Fatalf("file on disk: %v", err)
	}
	var meta geoip.Meta
	if b, err := os.ReadFile(filepath.Join(dir, "qqwry.json")); err != nil || json.Unmarshal(b, &meta) != nil ||
		meta.Version != "20261014" || meta.SHA256 != sha(v1) || meta.Source != m.URL+"/qqwry.dat" {
		t.Fatalf("meta = %+v (%v)", meta, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatalf("leftover files: %v", entries)
	}

	// Same version: only version.json is fetched.
	out, err = s.Update(t.Context())
	if err != nil || out.Updated || m.downloads.Load() != 1 || m.versions.Load() != 2 {
		t.Fatalf("same version = %+v %v (downloads %d)", out, err, m.downloads.Load())
	}

	// A new release is downloaded and swapped in; the cached lookup follows the new data.
	v2 := release("20261021", "移动")
	m.set("20261021", v2)
	out, err = s.Update(t.Context())
	if err != nil || !out.Updated || out.Version != "20261021" {
		t.Fatalf("second update = %+v, %v", out, err)
	}
	if got := s.Locate("203.0.113.9"); got != "中国 广东 深圳 移动" {
		t.Fatalf("after the swap Locate = %q", got)
	}

	// A new App on the same data dir loads the file and its metadata without downloading.
	b := testutil.NewTestApp(t, testutil.WithConfig(func(c *config.Config) { c.DataDir = a.Cfg.DataDir }))
	if got := geoip.Locate(b, "203.0.113.9"); got != "中国 广东 深圳 移动" {
		t.Fatalf("reloaded Locate = %q", got)
	}
	if st := geoip.Of(b).Status(); st.Version != "20261021" || st.SHA256 != sha(v2) || st.CheckedAt == nil {
		t.Fatalf("reloaded status = %+v", st)
	}
}

func TestUpdateRejectsBadFilesAndKeepsTheOldOne(t *testing.T) {
	good := release("20261014", "电信")
	m := newMirror(t, "20261014", good)
	a := newApp(t, m, nil)
	s := geoip.Of(a)
	if _, err := s.Update(t.Context()); err != nil {
		t.Fatal(err)
	}
	tooFew := geoiptest.Build(geoiptest.Sample[:2], "20261021")
	noProbe := geoiptest.Build(append(append([]geoiptest.Range(nil), geoiptest.Sample[:5]...), geoiptest.Sample[7:]...), "20261021")
	cases := []struct {
		name string
		file []byte
	}{
		{"html page", []byte("<html>blocked</html>")},
		{"truncated", good[:len(good)/2]},
		{"too few ranges", tooFew},
		{"probe missing", noProbe},
		{"too large", append(release("20261021", "联通"), make([]byte, 2<<20)...)}, // last: checked below
	}
	for _, c := range cases {
		name := c.name
		m.set("20261021", c.file)
		_, err := s.Update(t.Context())
		if err == nil {
			t.Fatalf("%s: accepted", name)
		}
		st := s.Status()
		if st.Version != "20261014" || st.SHA256 != sha(good) || st.LastError == "" || st.LastErrorAt == nil {
			t.Fatalf("%s: status = %+v", name, st)
		}
		if got := s.Locate("203.0.113.9"); got != "中国 广东 深圳 电信" {
			t.Fatalf("%s: Locate = %q", name, got)
		}
		if b, _ := os.ReadFile(filepath.Join(a.Cfg.DataDir, "geoip", "qqwry.dat")); sha(b) != sha(good) {
			t.Fatalf("%s: the file on disk changed", name)
		}
		if entries, _ := os.ReadDir(filepath.Join(a.Cfg.DataDir, "geoip")); len(entries) != 2 {
			t.Fatalf("%s: leftover temporary files: %v", name, entries)
		}
	}
	if !strings.Contains(s.Status().LastError, "too large") {
		t.Fatalf("size cap error = %q", s.Status().LastError)
	}
	// Errors in the version check, too.
	m.set("latest", good)
	if _, err := s.Update(t.Context()); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("bad version.json: %v", err)
	}
	// A successful check clears the error.
	m.set("20261014", good)
	if _, err := s.Update(t.Context()); err != nil || s.Status().LastError != "" || s.Status().LastErrorAt != nil {
		t.Fatalf("recovered: %v %+v", err, s.Status())
	}
}

func TestUpdateWithoutVersionURLComparesChecksums(t *testing.T) {
	file := release("20261014", "电信")
	m := newMirror(t, "unused", file)
	a := newApp(t, m, map[string]any{"geoip_version_url": ""})
	s := geoip.Of(a)
	if out, err := s.Update(t.Context()); err != nil || !out.Updated || out.Version != "20261014" {
		t.Fatalf("first = %+v %v", out, err)
	}
	if out, err := s.Update(t.Context()); err != nil || out.Updated || m.downloads.Load() != 2 || m.versions.Load() != 0 {
		t.Fatalf("unchanged = %+v %v", out, err)
	}
}

func TestUpdateGuards(t *testing.T) {
	m := newMirror(t, "20261014", release("20261014", "电信"))
	open := m.hold(t)
	a := newApp(t, m, nil)
	s := geoip.Of(a)

	ch, err := s.Begin()
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return m.downloads.Load() == 1 })
	if !s.Status().Updating {
		t.Fatal("status does not show the running update")
	}
	if _, err := s.Begin(); !errors.Is(err, geoip.ErrBusy) {
		t.Fatalf("second Begin = %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second) // a missing guard would wait on the download
	defer cancel()
	if _, err := s.Update(ctx); !errors.Is(err, geoip.ErrBusy) {
		t.Fatalf("Update while busy = %v", err)
	}
	// The job skips instead of failing.
	if err := a.Scheduler.RunNow(geoip.JobUpdate); err != nil {
		t.Fatalf("job while busy = %v", err)
	}
	if n := m.downloads.Load(); n != 1 {
		t.Fatalf("%d downloads while busy", n)
	}
	open()
	if out := <-ch; out.Err != nil || !out.Updated {
		t.Fatalf("outcome = %+v", out)
	}
	if s.Status().Updating {
		t.Fatal("still updating")
	}

	// Disabled: no lookups, no updates.
	if err := a.Settings.Set(map[string]any{"geoip_enable": false}); err != nil {
		t.Fatal(err)
	}
	if got := s.Locate("203.0.113.9"); got != "" {
		t.Fatalf("disabled Locate = %q", got)
	}
	if _, err := s.Begin(); !errors.Is(err, geoip.ErrDisabled) {
		t.Fatalf("disabled Begin = %v", err)
	}
	if err := a.Scheduler.RunNow(geoip.JobUpdate); err != nil {
		t.Fatalf("disabled job = %v", err)
	}
	if st := s.Status(); st.Enabled || st.Version != "20261014" {
		t.Fatalf("disabled status = %+v", st)
	}
}

func TestBootDownloadsAMissingDatabase(t *testing.T) {
	m := newMirror(t, "20261014", release("20261014", "电信"))
	a := newApp(t, m, nil)
	s := geoip.Of(a)
	s.Boot()
	waitFor(t, func() bool { return s.Status().Loaded })
	if got := s.Locate("203.0.113.9"); got != "中国 广东 深圳 电信" || m.downloads.Load() != 1 {
		t.Fatalf("Locate = %q, downloads %d", got, m.downloads.Load())
	}

	// With a file in place, boot only loads it.
	b := testutil.NewTestApp(t, testutil.WithConfig(func(c *config.Config) { c.DataDir = a.Cfg.DataDir }),
		testutil.WithSettings(map[string]any{"geoip_download_url": m.URL + "/qqwry.dat", "geoip_version_url": m.URL + "/version.json"}))
	geoip.Of(b).Boot()
	waitFor(t, func() bool { return geoip.Of(b).Status().Loaded })
	time.Sleep(50 * time.Millisecond) // longer than the bootstrap delay
	if m.downloads.Load() != 1 || m.versions.Load() != 1 {
		t.Fatalf("boot with a file downloaded again: %d/%d", m.downloads.Load(), m.versions.Load())
	}
}

func TestStopCancelsTheBootDownload(t *testing.T) {
	m := newMirror(t, "20261014", release("20261014", "电信"))
	m.hold(t)
	a := newApp(t, m, nil)
	s := geoip.Of(a)
	s.Boot()
	waitFor(t, func() bool { return m.downloads.Load() == 1 })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	if _, err := s.Begin(); !errors.Is(err, geoip.ErrStopped) {
		t.Fatalf("Begin after Stop = %v", err)
	}
}

func TestLocateWithoutDatabaseOrForIPv6(t *testing.T) {
	a := testutil.NewTestApp(t)
	for _, ip := range []string{"203.0.113.9", "2001:db8::1", "not an ip", ""} {
		if got := geoip.Locate(a, ip); got != "" {
			t.Fatalf("Locate(%q) = %q", ip, got)
		}
	}
	geoiptest.Install(t, a.Cfg.DataDir, geoiptest.Sample, geoiptest.SampleVersion)
	b := testutil.NewTestApp(t, testutil.WithConfig(func(c *config.Config) { c.DataDir = a.Cfg.DataDir }))
	if got := geoip.Locate(b, "::ffff:203.0.113.9"); got != "中国 广东 深圳 电信" {
		t.Fatalf("installed file: %q", got)
	}
	if got := geoip.Locate(b, "2001:db8::1"); got != "" {
		t.Fatalf("IPv6 = %q", got)
	}
	var nilService *geoip.Service
	if nilService.Locate("203.0.113.9") != "" || nilService.Enabled() {
		t.Fatal("a nil service locates nothing")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached within 5 s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
