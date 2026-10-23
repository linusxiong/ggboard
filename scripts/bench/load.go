package main

import (
	"context"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// scenario is one fixed-concurrency, closed-loop load test.
type scenario struct {
	ID, Group, Title, Request string
	Detail                    string // short request line for the chart
	Conc                      int
	Expect                    []int // statuses that count as success
	Hit                       int   // status counted as a cache hit for the hit rate (0 = none)
	Budget                    int64 // max requests over warm-up and measurement together (0 = unlimited)
	Note                      string
	// Make builds the i-th request (i counts from 0 across warm-up and measurement).
	Make func(rng *rand.Rand, i int64) (*http.Request, error)
	// Probe checks one response for correctness before the load starts.
	Probe func(ctx context.Context, s *scenario) error
	// After runs once the measurement is done (e.g. measuring the traffic flush) and may add to the note.
	After func(ctx context.Context, res *ScenarioResult) error
}

// worker holds one connection's counters; workers never share memory while running.
type worker struct {
	lat      []time.Duration
	statuses map[int]int64
	bytes    int64
	bad      int64 // unexpected status
	netErr   int64 // transport errors
	lastErr  string
}

type phase struct {
	workers   []*worker
	elapsed   time.Duration
	exhausted bool
}

// runPhase runs s.Conc workers until d has elapsed or the request budget is used up. Each worker sends its next
// request as soon as it has read the previous response (closed loop).
func runPhase(hc *http.Client, s *scenario, seq *atomic.Int64, d time.Duration, salt uint64) phase {
	var ok [600]bool
	for _, c := range s.Expect {
		ok[c] = true
	}
	var exhausted atomic.Bool
	next := func() (int64, bool) {
		i := seq.Add(1) - 1
		if s.Budget > 0 && i >= s.Budget {
			exhausted.Store(true)
			return 0, false
		}
		return i, true
	}
	ws := make([]*worker, s.Conc)
	var wg sync.WaitGroup
	start := time.Now()
	deadline := start.Add(d)
	for k := range ws {
		w := &worker{statuses: map[int]int64{}, lat: make([]time.Duration, 0, 4096)}
		ws[k] = w
		rng := rand.New(rand.NewPCG(salt, uint64(k))) //nolint:gosec // picks the next node or token of the load pattern, no secret
		wg.Go(func() {
			for time.Now().Before(deadline) {
				i, more := next()
				if !more {
					return
				}
				req, err := s.Make(rng, i)
				if err != nil {
					w.netErr++
					w.lastErr = err.Error()
					continue
				}
				t0 := time.Now()
				resp, err := hc.Do(req)
				if err != nil {
					w.netErr++
					w.lastErr = err.Error()
					continue
				}
				n, err := io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				lat := time.Since(t0)
				if err != nil {
					w.netErr++
					w.lastErr = err.Error()
					continue
				}
				w.statuses[resp.StatusCode]++
				w.bytes += n
				if resp.StatusCode < len(ok) && ok[resp.StatusCode] {
					w.lat = append(w.lat, lat)
				} else {
					w.bad++
					w.lastErr = "unexpected status " + strconv.Itoa(resp.StatusCode)
				}
			}
		})
	}
	wg.Wait()
	return phase{workers: ws, elapsed: time.Since(start), exhausted: exhausted.Load()}
}

// measure probes s, warms it up and measures it.
func measure(ctx context.Context, hc *http.Client, s *scenario, warmup, d time.Duration, pid int) (*ScenarioResult, error) {
	if s.Probe != nil {
		if err := s.Probe(ctx, s); err != nil {
			return nil, err
		}
	}
	var seq atomic.Int64
	if warmup > 0 {
		runPhase(hc, s, &seq, warmup, 1)
	}
	mon := startMonitor(pid, 250*time.Millisecond)
	cpu0, cpuOK := selfCPU()
	ph := runPhase(hc, s, &seq, d, 2)
	cpu1, _ := selfCPU()
	cores, peak := mon.finish()
	var client *float64
	if cpuOK && ph.elapsed > 0 {
		c := math.Round(float64(cpu1-cpu0)/float64(ph.elapsed)*10) / 10
		client = &c
	}

	res := &ScenarioResult{
		ID: s.ID, Group: s.Group, Title: s.Title, Request: s.Request, Detail: s.Detail, Concurrency: s.Conc,
		DurationS: float64(ph.elapsed.Milliseconds()) / 1000, Statuses: map[string]int64{},
		ServerCPUCores: cores, ClientCPUCores: client, ServerPeakRSS: peak, StoppedEarly: ph.exhausted, Note: s.Note,
	}
	var lat []time.Duration
	var bytes int64
	for _, w := range ph.workers {
		lat = append(lat, w.lat...)
		bytes += w.bytes
		res.Errors += w.bad + w.netErr
		for code, n := range w.statuses {
			res.Statuses[strconv.Itoa(code)] += n
			res.Requests += n
		}
		if w.netErr > 0 {
			res.Statuses["transport_error"] += w.netErr
			res.Requests += w.netErr
		}
		if w.lastErr != "" {
			res.LastError = w.lastErr
		}
	}
	if res.Errors == 0 {
		res.LastError = ""
	}
	sum := summarize(lat)
	res.OK = int64(sum.Count)
	if ph.elapsed > 0 {
		res.RPS = float64(res.OK) / ph.elapsed.Seconds()
	}
	res.P50Ms, res.P95Ms, res.P99Ms = ms(sum.P50), ms(sum.P95), ms(sum.P99)
	res.MeanMs, res.MaxMs = ms(sum.Mean), ms(sum.Max)
	if res.Requests > 0 {
		res.BytesPerReq = float64(bytes) / float64(res.Requests)
	}
	if s.Hit != 0 {
		hits := res.Statuses[strconv.Itoa(s.Hit)]
		if res.OK > 0 {
			r := float64(hits) / float64(res.OK)
			res.HitRate = &r
		}
	}
	if s.After != nil {
		if err := s.After(ctx, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}
