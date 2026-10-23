package main

import (
	"strings"
	"testing"
	"time"
)

func TestPercentileNearestRank(t *testing.T) {
	ms := func(v ...int) []time.Duration {
		out := make([]time.Duration, len(v))
		for i, x := range v {
			out[i] = time.Duration(x) * time.Millisecond
		}
		return out
	}
	// 1..100 ms: the p-th percentile is exactly p ms.
	hundred := make([]int, 100)
	for i := range hundred {
		hundred[i] = i + 1
	}
	cases := []struct {
		name   string
		sample []time.Duration
		p      float64
		want   time.Duration
	}{
		{"empty", nil, 50, 0},
		{"single", ms(7), 99, 7 * time.Millisecond},
		{"p50 of 1..100", ms(hundred...), 50, 50 * time.Millisecond},
		{"p95 of 1..100", ms(hundred...), 95, 95 * time.Millisecond},
		{"p99 of 1..100", ms(hundred...), 99, 99 * time.Millisecond},
		{"p100 of 1..100", ms(hundred...), 100, 100 * time.Millisecond},
		{"p50 of four rounds up", ms(1, 2, 3, 4), 50, 2 * time.Millisecond},
		{"p51 of four", ms(1, 2, 3, 4), 51, 3 * time.Millisecond},
		{"tiny p never below the minimum", ms(5, 6), 0.001, 5 * time.Millisecond},
	}
	for _, c := range cases {
		if got := percentile(c.sample, c.p); got != c.want {
			t.Errorf("%s: percentile(%v) = %v, want %v", c.name, c.p, got, c.want)
		}
	}
}

func TestSummarizeSortsAndAggregates(t *testing.T) {
	lat := []time.Duration{9, 1, 5, 3, 7}
	for i := range lat {
		lat[i] *= time.Millisecond
	}
	s := summarize(lat)
	if s.Count != 5 || s.Min != time.Millisecond || s.Max != 9*time.Millisecond || s.Mean != 5*time.Millisecond {
		t.Fatalf("summary = %+v", s)
	}
	if s.P50 != 5*time.Millisecond || s.P95 != 9*time.Millisecond || s.P99 != 9*time.Millisecond {
		t.Fatalf("percentiles = %v %v %v", s.P50, s.P95, s.P99)
	}
	if lat[0] != time.Millisecond || lat[4] != 9*time.Millisecond {
		t.Fatalf("sample not sorted in place: %v", lat)
	}
	if (summarize(nil) != Summary{}) {
		t.Fatal("empty sample must give the zero summary")
	}
}

func TestFormatting(t *testing.T) {
	for in, want := range map[int64]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 48210: "48,210", 1234567: "1,234,567"} {
		if got := groupThousands(in); got != want {
			t.Errorf("groupThousands(%d) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[float64]string{12.34: "12.3", 116.4: "116", 14496.5: "14,497"} {
		if got := fmtRate(in); got != want {
			t.Errorf("fmtRate(%v) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[float64]string{0.4249: "0.42 ms", 3.14: "3.1 ms", 47.6: "48 ms", 1204.2: "1,204 ms"} {
		if got := fmtMs(in); got != want {
			t.Errorf("fmtMs(%v) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[float64]string{812: "812 B", 31948.8: "31.2 KiB", 1468006: "1.4 MiB"} {
		if got := fmtBytes(in); got != want {
			t.Errorf("fmtBytes(%v) = %q, want %q", in, got, want)
		}
	}
	if got := ms(1234567 * time.Nanosecond); got != 1.235 {
		t.Errorf("ms = %v, want 1.235", got)
	}
}

func TestAxisTicks(t *testing.T) {
	for _, c := range []struct {
		max  float64
		want float64
	}{{83252, 25000}, {100, 25}, {9, 2.5}, {62900, 20000}, {1, 0.25}, {0, 1}} {
		if got := niceStep(c.max, 4); got != c.want {
			t.Errorf("niceStep(%v) = %v, want %v", c.max, got, c.want)
		}
	}
	for in, want := range map[float64]string{0: "0", 500: "500", 2000: "2K", 2500: "2.5K", 40000: "40K", 1200000: "1.2M"} {
		if got := compact(in); got != want {
			t.Errorf("compact(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSummarizeLog(t *testing.T) {
	log := strings.Join([]string{
		`time=2026-10-23T10:00:00Z level=INFO msg=http method=GET path=/healthz status=200`,
		`time=2026-10-23T10:00:01Z level=WARN msg="subscription access log: buffer full, rows dropped" dropped=1200`,
		`time=2026-10-23T10:00:11Z level=WARN msg="subscription access log: buffer full, rows dropped" dropped=34`,
		`time=2026-10-23T10:00:12Z level=ERROR msg="request failed" path=/api/v1/x status=500`,
		`time=2026-10-23T10:00:13Z level=WARN msg=bare_word`,
		`time=2026-10-23T10:00:14Z level=WARN msg="quoted \"inner\" text"`,
	}, "\n")
	got, err := summarizeLog(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d entries: %+v", len(got), got)
	}
	if got[0].Message != "subscription access log: buffer full, rows dropped" || got[0].Count != 2 || got[0].Dropped != 1234 {
		t.Fatalf("first entry %+v", got[0])
	}
	found := map[string]bool{}
	for _, e := range got {
		found[e.Level+" "+e.Message] = true
	}
	for _, want := range []string{"ERROR request failed", "WARN bare_word", `WARN quoted "inner" text`} {
		if !found[want] {
			t.Errorf("missing %q in %+v", want, got)
		}
	}
}

func TestParseCPUTime(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"0:03.21":     3210 * time.Millisecond,
		"12:01.50":    721500 * time.Millisecond,
		"1:02:03.45":  3723450 * time.Millisecond,
		"00:00:03":    3 * time.Second,
		"2-01:00:00":  49 * time.Hour,
		"10:00.00":    10 * time.Minute,
		"0:00.00":     0,
		"123:45.67":   123*time.Minute + 45670*time.Millisecond,
		"1-00:00:01":  24*time.Hour + time.Second,
		"0:01.999999": 1999999 * time.Microsecond,
	} {
		got, err := parseCPUTime(in)
		if err != nil || (got-want).Abs() > time.Microsecond {
			t.Errorf("parseCPUTime(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "3", "a:b", "1:2:3:4"} {
		if _, err := parseCPUTime(bad); err == nil {
			t.Errorf("parseCPUTime(%q) accepted", bad)
		}
	}
}
