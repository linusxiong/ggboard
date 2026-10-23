// Command bench is GG Bond's load generator. Against a running panel it seeds a realistic dataset through the
// admin API (idempotent), runs fixed-concurrency closed-loop scenarios (subscriptions, UniProxy node API, login,
// static files) and writes docs/perf/results.json, results.md and benchmark.svg. See docs/perf/README.md.
//
//	make bench                                              # throwaway server on a free port, full run
//	BENCH_ADMIN_PASSWORD=… go run ./scripts/bench -base http://127.0.0.1:23456 -admin-email admin@example.com
//	go run ./scripts/bench -render docs/perf/results.json   # re-render results.md and benchmark.svg
//	go run ./scripts/bench -free-port                       # print a free TCP port >= 20000
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		base      = flag.String("base", "", "panel base URL, e.g. http://127.0.0.1:23456 (required unless -render/-free-port)")
		email     = flag.String("admin-email", "admin@example.com", "administrator account (password in $BENCH_ADMIN_PASSWORD)")
		nUsers    = flag.Int("users", 10000, "benchmark users to seed and use")
		nNodes    = flag.Int("nodes", 50, "benchmark nodes to seed and use")
		conc      = flag.Int("c", 64, "connections per scenario")
		loginConc = flag.Int("login-c", 16, "connections for the login scenario (argon2id bound)")
		dur       = flag.Duration("d", 30*time.Second, "measured duration per scenario")
		warmup    = flag.Duration("warmup", 3*time.Second, "unrecorded warm-up per scenario")
		pause     = flag.Duration("pause", 3*time.Second, "idle time between scenarios")
		pushBatch = flag.Int("push-batch", 1000, "users per UniProxy push report")
		perToken  = flag.Int("per-token", 25, "max subscription requests per token per run (the server allows a burst of 30)")
		nodeEnc   = flag.String("node-encoding", "gzip", "Accept-Encoding of the UniProxy user-list requests (gzip or identity, a diagnostic)")
		only      = flag.String("only", "", "comma-separated scenario ids to run (default: all)")
		out       = flag.String("out", "docs/perf", "output directory for results.json, results.md and benchmark.svg")
		commit    = flag.String("commit", "", "commit label for the report (default: the server's version)")
		pid       = flag.Int("server-pid", 0, "server process id: sample its CPU time and RSS with ps")
		logPath   = flag.String("server-log", "", "server log file (text format): count its WARN and ERROR lines into the report")
		render    = flag.String("render", "", "re-render results.md and benchmark.svg from this results.json and exit")
		port      = flag.Bool("free-port", false, "print a free TCP port >= 20000 and exit")
	)
	flag.Parse()

	if *port {
		p, err := freePort()
		if err != nil {
			return err
		}
		fmt.Println(p)
		return nil
	}
	if *render != "" {
		r, err := loadResults(*render)
		if err != nil {
			return err
		}
		return writeOutputs(filepath.Dir(*render), r, false)
	}
	if *base == "" {
		return errors.New("-base is required")
	}
	password := os.Getenv("BENCH_ADMIN_PASSWORD")
	if password == "" {
		return errors.New("set BENCH_ADMIN_PASSWORD to the administrator's password")
	}
	if *nUsers < 2 || *nNodes < 1 || *conc < 1 || *loginConc < 1 || *pushBatch < 1 || *perToken < 1 || *perToken > 29 {
		return errors.New("invalid -users, -nodes, -c, -login-c, -push-batch or -per-token (1..29)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logf := func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }

	version, err := waitHealthy(ctx, *base, 10*time.Second)
	if err != nil {
		return err
	}
	admin := newAPI(*base)
	if err := admin.login(ctx, *email, password); err != nil {
		return fmt.Errorf("admin login: %w", err)
	}

	logf("==> seeding %d users and %d nodes on %s", *nUsers, *nNodes, *base)
	t0 := time.Now()
	ds, err := seed(ctx, admin, *nUsers, *nNodes, logf)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	seedTime := time.Since(t0)
	logf("    %d users (%d new), %d nodes (%d new) in %s", len(ds.Users), ds.Created.Users, len(ds.Nodes), ds.Created.Nodes,
		seedTime.Round(time.Millisecond))

	maxConc := max(*conc, *loginConc)
	e := &env{base: strings.TrimRight(*base, "/"), ds: ds, admin: admin, load: newLoadClient(maxConc), conc: *conc,
		loginConc: *loginConc, pushBatch: *pushBatch, perToken: *perToken, logf: logf, nodeEncoding: *nodeEnc}
	all, err := e.scenarios()
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, id := range strings.Split(*only, ",") {
		if id = strings.TrimSpace(id); id != "" {
			want[id] = true
		}
	}

	res := &Results{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Commit:      *commit,
		Machine:     machineInfo(),
		Settings: RunSettings{
			Concurrency: *conc, LoginConcurrency: *loginConc, Duration: dur.String(), Warmup: warmup.String(),
			PushBatch: *pushBatch, PerTokenBudget: *perToken, ToolGoVersion: runtime.Version(),
			Model: "Closed loop: every connection sends its next request as soon as it has read the previous response.",
		},
		Dataset: DatasetInfo{Users: len(ds.Users), Nodes: len(ds.Nodes), NodeTypes: map[string]int{}, Plan: benchPlan,
			RuleTemplate: ruleTemplate, SeedSeconds: float64(seedTime.Milliseconds()) / 1000},
	}
	if res.Commit == "" {
		res.Commit = version
	}
	for _, n := range ds.Nodes {
		res.Dataset.NodeTypes[n.UniType]++
	}

	var peak uint64
	for i, s := range all {
		if len(want) > 0 && !want[s.ID] && !(s.ID == "static_index" && want["static_asset"]) {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		logf("==> %-16s %s × %d connections", s.ID, dur, s.Conc)
		r, err := measure(ctx, e.load, s, *warmup, *dur, *pid)
		if err != nil {
			return fmt.Errorf("%s: %w", s.ID, err)
		}
		if len(want) > 0 && !want[s.ID] {
			continue // static_index only ran to resolve the asset name
		}
		res.Scenarios = append(res.Scenarios, *r)
		peak = max(peak, r.ServerPeakRSS)
		logf("    %s req/s · p50 %s · p95 %s · p99 %s · %s requests · %d errors%s", fmtRate(r.RPS), fmtMs(r.P50Ms),
			fmtMs(r.P95Ms), fmtMs(r.P99Ms), groupThousands(r.Requests), r.Errors, errSuffix(r))
		if i < len(all)-1 && *pause > 0 {
			time.Sleep(*pause)
		}
	}
	if len(res.Scenarios) == 0 {
		return errors.New("no scenario matched -only")
	}

	st, err := admin.status(ctx)
	if err != nil {
		return err
	}
	res.Server = ServerInfo{
		Version: version, GoVersion: st.GoVersion, DBDriver: st.DBDriver, Goroutines: st.Goroutines,
		HeapAllocBytes: st.Memory.Alloc, SysBytes: st.Memory.Sys, CacheHitRatio: st.Cache.HitRatio, PeakRSSBytes: peak,
		TrafficFlush: e.flush, TrafficPairs: e.flushPairs,
	}
	if *logPath != "" {
		w, err := summarizeLogFile(*logPath)
		if err != nil {
			return err
		}
		res.Server.LogChecked, res.Server.LogWarnings = true, w
	}
	if st.DBDriver == "sqlite" {
		res.Server.SQLite = "WAL, synchronous=NORMAL, busy_timeout=5000, foreign_keys=1, _txlock=immediate, 8 connections"
	}
	if err := writeOutputs(*out, res, true); err != nil {
		return err
	}
	logf("==> wrote %s/results.json, results.md and benchmark.svg", *out)
	var bad []string
	for _, s := range res.Scenarios {
		if s.Errors > 0 {
			bad = append(bad, fmt.Sprintf("%s (%d)", s.ID, s.Errors))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("scenarios with errors: %s (see the statuses in results.json)", strings.Join(bad, ", "))
	}
	return nil
}

func errSuffix(r *ScenarioResult) string {
	var parts []string
	if r.HitRate != nil {
		parts = append(parts, fmt.Sprintf("304 rate %.1f %%", *r.HitRate*100))
	}
	if r.ServerCPUCores != nil {
		parts = append(parts, fmt.Sprintf("server %.1f cores", *r.ServerCPUCores))
	}
	if r.StoppedEarly {
		parts = append(parts, fmt.Sprintf("token budget used up after %.1f s", r.DurationS))
	}
	if r.LastError != "" {
		parts = append(parts, "last error: "+r.LastError)
	}
	if len(parts) == 0 {
		return ""
	}
	return " · " + strings.Join(parts, " · ")
}
