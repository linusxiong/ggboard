package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// newLoadClient returns the client the scenarios use: keep-alive connections, at most conns per host, no
// transparent decompression (scenarios set Accept-Encoding themselves and count the bytes on the wire).
func newLoadClient(conns int) *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			MaxIdleConns:        conns * 2,
			MaxIdleConnsPerHost: conns * 2,
			MaxConnsPerHost:     conns,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true,
		},
	}
}

// api is a small JSON client for the admin API (seeding, status).
type api struct {
	base  string
	hc    *http.Client
	token string
}

func newAPI(base string) *api {
	return &api{base: strings.TrimRight(base, "/"), hc: &http.Client{Timeout: 60 * time.Second}}
}

// apiError is a non-2xx answer.
type apiError struct {
	Method, Path string
	Status       int
	Body         string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s %s → %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// call sends in as JSON (when not nil) and decodes a 2xx answer into out (when not nil).
func (a *api) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &apiError{Method: method, Path: path, Status: resp.StatusCode, Body: snippet(b, 300)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("%s %s: decode: %w", method, path, err)
	}
	return nil
}

// login signs in and keeps the session token.
func (a *api) login(ctx context.Context, email, password string) error {
	var out struct {
		Data struct {
			Token       string `json:"token"`
			MFARequired bool   `json:"mfa_required"`
			User        struct {
				IsAdmin bool `json:"is_admin"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := a.call(ctx, http.MethodPost, "/api/v1/passport/login", map[string]string{"email": email, "password": password}, &out); err != nil {
		return err
	}
	switch {
	case out.Data.MFARequired:
		return errors.New("the admin account has two-factor authentication enabled; use an account without it for the benchmark")
	case !out.Data.User.IsAdmin:
		return fmt.Errorf("%s is not an administrator", email)
	case out.Data.Token == "":
		return errors.New("login returned no token")
	}
	a.token = out.Data.Token
	return nil
}

func snippet(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		s = s[:n] + "…"
	}
	return s
}

// systemStatus is the part of GET /admin/system/status the report uses.
type systemStatus struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	DBDriver  string `json:"db_driver"`
	Scheduler []struct {
		Name         string     `json:"name"`
		LastRun      *time.Time `json:"last_run"`
		LastDuration string     `json:"last_duration"`
		LastError    string     `json:"last_error"`
		Runs         int64      `json:"runs"`
	} `json:"scheduler"`
	Cache struct {
		HitRatio float64 `json:"hit_ratio"`
	} `json:"cache"`
	Goroutines int `json:"goroutines"`
	Memory     struct {
		Alloc uint64 `json:"alloc"`
		Sys   uint64 `json:"sys"`
	} `json:"memory"`
}

func (a *api) status(ctx context.Context) (*systemStatus, error) {
	var out struct {
		Data systemStatus `json:"data"`
	}
	if err := a.call(ctx, http.MethodGet, "/api/v1/admin/system/status", nil, &out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

// waitHealthy polls /healthz until it answers 200 or the timeout expires.
func waitHealthy(ctx context.Context, base string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	a := newAPI(base)
	for {
		var out struct {
			Data struct {
				Version string `json:"version"`
			} `json:"data"`
		}
		err := a.call(ctx, http.MethodGet, "/healthz", nil, &out)
		if err == nil {
			return out.Data.Version, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%s/healthz: %w", base, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// freePort returns a TCP port >= 20000 that is free on 127.0.0.1 right now (never 8080).
func freePort() (int, error) {
	for p := 20000 + int(time.Now().UnixNano()%30000); p < 65000; p += 7 {
		l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
		if err == nil {
			_ = l.Close()
			return p, nil
		}
	}
	return 0, errors.New("no free port found")
}

// machineInfo describes the host (best effort: sysctl on macOS, /proc on Linux).
func machineInfo() Machine {
	m := Machine{CPU: runtime.GOARCH, Cores: runtime.NumCPU(), OS: runtime.GOOS, Arch: runtime.GOARCH}
	sysctl := func(name string) string {
		out, err := exec.CommandContext(context.Background(), "sysctl", "-n", name).Output() //nolint:gosec // name is one of the constant keys below
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	switch runtime.GOOS {
	case "darwin":
		if v := sysctl("machdep.cpu.brand_string"); v != "" {
			m.CPU = v
		}
		if v, err := strconv.ParseUint(sysctl("hw.memsize"), 10, 64); err == nil {
			m.MemoryBytes = v
		}
		if v, err := strconv.Atoi(sysctl("hw.ncpu")); err == nil {
			m.Cores = v
		}
		p0, e0 := strconv.Atoi(sysctl("hw.perflevel0.physicalcpu"))
		p1, e1 := strconv.Atoi(sysctl("hw.perflevel1.physicalcpu"))
		if e0 == nil && e1 == nil {
			n0, n1 := sysctl("hw.perflevel0.name"), sysctl("hw.perflevel1.name")
			if n0 == "" || n1 == "" {
				n0, n1 = "level 0", "level 1"
			}
			m.CoreLayout = fmt.Sprintf("%d %s + %d %s", p0, n0, p1, n1)
		}
		if out, err := exec.CommandContext(context.Background(), "sw_vers", "-productVersion").Output(); err == nil {
			m.OS = "macOS " + strings.TrimSpace(string(out))
		}
		m.LoadAverage = strings.Trim(sysctl("vm.loadavg"), "{} ")
	case "linux":
		if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "model name" {
					m.CPU = strings.TrimSpace(v)
					break
				}
			}
		}
		if b, err := os.ReadFile("/proc/meminfo"); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if f := strings.Fields(line); len(f) >= 2 && f[0] == "MemTotal:" {
					if kb, err := strconv.ParseUint(f[1], 10, 64); err == nil {
						m.MemoryBytes = kb * 1024
					}
				}
			}
		}
		m.OS = "Linux"
		if b, err := os.ReadFile("/proc/loadavg"); err == nil {
			if f := strings.Fields(string(b)); len(f) >= 3 {
				m.LoadAverage = strings.Join(f[:3], " ")
			}
		}
	}
	return m
}

// procSample is one `ps` reading of the server process.
type procSample struct {
	CPU time.Duration // cumulative user+system CPU time
	RSS uint64        // bytes
}

// sampleProc reads the CPU time and RSS of pid with ps (macOS and Linux).
func sampleProc(pid int) (procSample, error) {
	out, err := exec.CommandContext(context.Background(), "ps", "-o", "time=,rss=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // pid is an integer
	if err != nil {
		return procSample{}, err
	}
	f := strings.Fields(string(out))
	if len(f) != 2 {
		return procSample{}, fmt.Errorf("ps: unexpected output %q", out)
	}
	cpu, err := parseCPUTime(f[0])
	if err != nil {
		return procSample{}, err
	}
	kb, err := strconv.ParseUint(f[1], 10, 64)
	if err != nil {
		return procSample{}, err
	}
	return procSample{CPU: cpu, RSS: kb * 1024}, nil
}

// parseCPUTime parses ps TIME values: "0:03.21", "12:01.50", "1:02:03.45" (macOS) or "[dd-]hh:mm:ss" (Linux).
func parseCPUTime(s string) (time.Duration, error) {
	var days float64
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.ParseFloat(d, 64)
		if err != nil {
			return 0, fmt.Errorf("cpu time %q: %w", s, err)
		}
		days, s = n, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("cpu time %q: unexpected format", s)
	}
	total := days * 86400
	mult := 1.0
	for i := len(parts) - 1; i >= 0; i-- {
		v, err := strconv.ParseFloat(parts[i], 64)
		if err != nil {
			return 0, fmt.Errorf("cpu time %q: %w", s, err)
		}
		total += v * mult
		mult *= 60
	}
	return time.Duration(total * float64(time.Second)), nil
}

// procMonitor samples a process every interval and keeps the peak RSS.
type procMonitor struct {
	pid   int
	stop  chan struct{}
	done  chan struct{}
	mu    sync.Mutex
	first procSample
	last  procSample
	peak  uint64
	ok    bool
	start time.Time
	end   time.Time
}

func startMonitor(pid int, every time.Duration) *procMonitor {
	m := &procMonitor{pid: pid, stop: make(chan struct{}), done: make(chan struct{})}
	if pid <= 0 {
		close(m.done)
		return m
	}
	first, err := sampleProc(pid)
	m.start = time.Now()
	if err == nil {
		m.first, m.last, m.peak, m.ok = first, first, first.RSS, true
	}
	go func() {
		defer close(m.done)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-t.C:
				if s, err := sampleProc(pid); err == nil {
					m.mu.Lock()
					m.peak = max(m.peak, s.RSS)
					m.mu.Unlock()
				}
			}
		}
	}()
	return m
}

// finish stops sampling and returns the average number of cores the process used and its peak RSS.
func (m *procMonitor) finish() (cores *float64, peak uint64) {
	if m.pid <= 0 {
		return nil, 0
	}
	close(m.stop)
	<-m.done
	s, err := sampleProc(m.pid)
	m.end = time.Now()
	if err != nil || !m.ok {
		return nil, 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last = s
	m.peak = max(m.peak, s.RSS)
	wall := m.end.Sub(m.start)
	if wall <= 0 {
		return nil, m.peak
	}
	c := float64(m.last.CPU-m.first.CPU) / float64(wall)
	c = float64(int(c*10+0.5)) / 10
	return &c, m.peak
}
