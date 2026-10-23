package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Results is docs/perf/results.json.
type Results struct {
	GeneratedAt string           `json:"generated_at"`
	Commit      string           `json:"commit"`
	Machine     Machine          `json:"machine"`
	Server      ServerInfo       `json:"server"`
	Dataset     DatasetInfo      `json:"dataset"`
	Settings    RunSettings      `json:"settings"`
	Scenarios   []ScenarioResult `json:"scenarios"`
}

// Machine is the host the run happened on (server and load generator share it).
type Machine struct {
	CPU         string `json:"cpu"`
	Cores       int    `json:"cores"`
	CoreLayout  string `json:"core_layout,omitempty"`
	MemoryBytes uint64 `json:"memory_bytes"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	LoadAverage string `json:"load_average_at_start,omitempty"`
}

// ServerInfo is what the panel reported about itself (/healthz, /admin/system/status) at the end of the run.
type ServerInfo struct {
	Version        string     `json:"version"`
	GoVersion      string     `json:"go_version"`
	DBDriver       string     `json:"db_driver"`
	SQLite         string     `json:"sqlite,omitempty"`
	LogLevel       string     `json:"log_level,omitempty"`
	Goroutines     int        `json:"goroutines"`
	HeapAllocBytes uint64     `json:"heap_alloc_bytes"`
	SysBytes       uint64     `json:"sys_bytes"`
	CacheHitRatio  float64    `json:"cache_hit_ratio"`
	PeakRSSBytes   uint64     `json:"peak_rss_bytes,omitempty"`
	TrafficFlush   string     `json:"traffic_flush_last_duration,omitempty"`
	TrafficPairs   int64      `json:"traffic_flush_pairs,omitempty"`
	LogChecked     bool       `json:"log_checked"`
	LogWarnings    []LogEntry `json:"log_warnings,omitempty"` // WARN/ERROR lines of the server log during the run
}

// DatasetInfo describes the seeded data.
type DatasetInfo struct {
	Users        int            `json:"users"`
	Nodes        int            `json:"nodes"`
	NodeTypes    map[string]int `json:"node_types"`
	Plan         string         `json:"plan"`
	RuleTemplate string         `json:"rule_template"`
	SeedSeconds  float64        `json:"seed_seconds"`
}

// RunSettings are the load-generator parameters.
type RunSettings struct {
	Concurrency      int    `json:"concurrency"`
	LoginConcurrency int    `json:"login_concurrency"`
	Duration         string `json:"duration"`
	Warmup           string `json:"warmup"`
	PushBatch        int    `json:"push_batch"`
	PerTokenBudget   int    `json:"subscribe_requests_per_token"`
	ToolGoVersion    string `json:"tool_go_version"`
	Model            string `json:"model"`
}

// ScenarioResult is one measured scenario.
type ScenarioResult struct {
	ID             string           `json:"id"`
	Group          string           `json:"group"`
	Title          string           `json:"title"`
	Request        string           `json:"request"`
	Detail         string           `json:"detail,omitempty"`
	Concurrency    int              `json:"concurrency"`
	DurationS      float64          `json:"duration_s"`
	Requests       int64            `json:"requests"`
	OK             int64            `json:"ok"`
	Errors         int64            `json:"errors"`
	Statuses       map[string]int64 `json:"statuses"`
	RPS            float64          `json:"rps"`
	P50Ms          float64          `json:"p50_ms"`
	P95Ms          float64          `json:"p95_ms"`
	P99Ms          float64          `json:"p99_ms"`
	MeanMs         float64          `json:"mean_ms"`
	MaxMs          float64          `json:"max_ms"`
	BytesPerReq    float64          `json:"bytes_per_request"`
	HitRate        *float64         `json:"hit_rate,omitempty"`
	ServerCPUCores *float64         `json:"server_cpu_cores,omitempty"`
	ClientCPUCores *float64         `json:"load_generator_cpu_cores,omitempty"`
	ServerPeakRSS  uint64           `json:"server_peak_rss_bytes,omitempty"`
	StoppedEarly   bool             `json:"stopped_early,omitempty"`
	LastError      string           `json:"last_error,omitempty"`
	Note           string           `json:"note,omitempty"`
}

func loadResults(path string) (*Results, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the -render path given by the operator
	if err != nil {
		return nil, err
	}
	var r Results
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &r, nil
}

// writeOutputs writes results.json, results.md and benchmark.svg into dir. They are published documentation
// (committed under docs/perf), hence world-readable.
func writeOutputs(dir string, r *Results, writeJSON bool) error {
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // published docs directory
		return err
	}
	if writeJSON {
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "results.json"), append(b, '\n'), 0o644); err != nil { //nolint:gosec // published docs
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "results.md"), []byte(markdown(r)), 0o644); err != nil { //nolint:gosec // published docs
		return err
	}
	return os.WriteFile(filepath.Join(dir, "benchmark.svg"), renderSVG(r), 0o644) //nolint:gosec // published docs
}

// machineLine is the one-line description of the host used in every output.
func machineLine(m Machine) string {
	parts := []string{m.CPU}
	if m.Cores > 0 {
		c := fmt.Sprintf("%d cores", m.Cores)
		if m.CoreLayout != "" {
			c += " (" + m.CoreLayout + ")"
		}
		parts = append(parts, c)
	}
	if m.MemoryBytes > 0 {
		parts = append(parts, fmt.Sprintf("%d GB RAM", m.MemoryBytes>>30))
	}
	if m.OS != "" {
		parts = append(parts, m.OS)
	}
	return strings.Join(parts, " · ")
}

// markdown renders results.md: the full table plus the run parameters.
func markdown(r *Results) string {
	var b strings.Builder
	date := r.GeneratedAt
	if len(date) >= 10 {
		date = date[:10]
	}
	fmt.Fprintf(&b, "<!-- generated by `go run ./scripts/bench` (make bench); do not edit by hand -->\n\n")
	fmt.Fprintf(&b, "# Benchmark results\n\n")
	fmt.Fprintf(&b, "Measured %s on %s; server `%s` (%s, %s), commit `%s`.\n", date, machineLine(r.Machine),
		r.Server.Version, r.Server.GoVersion, r.Server.DBDriver, r.Commit)
	if r.Machine.LoadAverage != "" {
		fmt.Fprintf(&b, "Host load average when the measurements started: %s (1 / 5 / 15 min; other processes were not stopped).\n",
			r.Machine.LoadAverage)
	}
	fmt.Fprintf(&b, "Dataset: %s users on one plan, %d nodes; %s per scenario after a %s warm-up, %d connections (login %d). %s\n\n",
		groupThousands(int64(r.Dataset.Users)), r.Dataset.Nodes, r.Settings.Duration, r.Settings.Warmup,
		r.Settings.Concurrency, r.Settings.LoginConcurrency, r.Settings.Model)
	b.WriteString("| Scenario | Request | Conn | Requests | req/s | p50 | p95 | p99 | Errors | Body | CPU server / load gen |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, s := range r.Scenarios {
		cores := func(v *float64) string {
			if v == nil {
				return "–"
			}
			return fmt.Sprintf("%.1f", *v)
		}
		cpu := cores(s.ServerCPUCores) + " / " + cores(s.ClientCPUCores) + " cores"
		title := s.Title
		if s.HitRate != nil {
			title += fmt.Sprintf(" (304 rate %.1f %%)", *s.HitRate*100)
		}
		if s.StoppedEarly {
			title += " †"
		}
		fmt.Fprintf(&b, "| %s · %s | `%s` | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			s.Group, title, s.Request, s.Concurrency, groupThousands(s.Requests), fmtRate(s.RPS),
			fmtMs(s.P50Ms), fmtMs(s.P95Ms), fmtMs(s.P99Ms), groupThousands(s.Errors), fmtBytes(s.BytesPerReq), cpu)
	}
	var stopped []string
	for _, s := range r.Scenarios {
		if s.StoppedEarly {
			stopped = append(stopped, fmt.Sprintf("%s after %.1f s", s.ID, s.DurationS))
		}
	}
	if len(stopped) > 0 {
		fmt.Fprintf(&b, "\n† stopped early (%s): every subscription token had used its share of the per-token rate limit "+
			"(%d requests each including warm-up, below the burst of 30), so no request was rate limited.\n",
			strings.Join(stopped, ", "), r.Settings.PerTokenBudget)
	}
	for _, s := range r.Scenarios {
		if s.Note != "" {
			fmt.Fprintf(&b, "\n- **%s · %s**: %s", s.Group, s.Title, s.Note)
		}
	}
	b.WriteString("\n\nBody = average bytes on the wire per response (after compression). CPU = CPU time ÷ wall time during the measured window, for the " +
		"server process (ps) and for the load generator (getrusage); both run on the same machine. Latency is measured by a closed-loop client (each connection sends its next request when " +
		"the previous answer has been read), so p50–p99 are latencies at saturation, not at a fixed arrival rate.\n")
	fmt.Fprintf(&b, "\nServer after the run: %d goroutines, heap %s, runtime sys %s", r.Server.Goroutines,
		fmtBytes(float64(r.Server.HeapAllocBytes)), fmtBytes(float64(r.Server.SysBytes)))
	if r.Server.PeakRSSBytes > 0 {
		fmt.Fprintf(&b, ", peak RSS %s", fmtBytes(float64(r.Server.PeakRSSBytes)))
	}
	fmt.Fprintf(&b, "; cache hit ratio %.3f.\n", r.Server.CacheHitRatio)
	if r.Server.LogChecked {
		if len(r.Server.LogWarnings) == 0 {
			b.WriteString("\nServer log: no WARN or ERROR lines during the run.\n")
		} else {
			b.WriteString("\nServer log, WARN and ERROR lines during the run:\n\n")
			for _, w := range r.Server.LogWarnings {
				fmt.Fprintf(&b, "- %s `%s` × %s", w.Level, w.Message, groupThousands(int64(w.Count)))
				if w.Dropped > 0 {
					fmt.Fprintf(&b, " (%s rows dropped in total)", groupThousands(w.Dropped))
				}
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}
