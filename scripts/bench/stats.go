package main

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Summary describes a latency sample.
type Summary struct {
	Count int
	Min   time.Duration
	Mean  time.Duration
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
	Max   time.Duration
}

// percentile returns the nearest-rank percentile p (0 < p <= 100) of an ascending sample: the smallest value
// with at least p % of the sample at or below it. An empty sample gives 0.
func percentile(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(n)))
	rank = min(max(rank, 1), n)
	return sorted[rank-1]
}

// summarize sorts lat in place and returns its summary.
func summarize(lat []time.Duration) Summary {
	if len(lat) == 0 {
		return Summary{}
	}
	slices.Sort(lat)
	var sum float64
	for _, d := range lat {
		sum += float64(d)
	}
	return Summary{
		Count: len(lat),
		Min:   lat[0],
		Mean:  time.Duration(sum / float64(len(lat))),
		P50:   percentile(lat, 50),
		P95:   percentile(lat, 95),
		P99:   percentile(lat, 99),
		Max:   lat[len(lat)-1],
	}
}

// ms converts d to milliseconds rounded to 3 decimals (µs resolution) for the JSON report.
func ms(d time.Duration) float64 { return math.Round(float64(d)/1e3) / 1e3 }

// groupThousands formats a non-negative integer with comma separators: 48210 → "48,210".
func groupThousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	if n < 0 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// fmtRate formats requests per second: integers with separators, one decimal below 100.
func fmtRate(v float64) string {
	if v < 100 {
		return strconv.FormatFloat(v, 'f', 1, 64)
	}
	return groupThousands(int64(math.Round(v)))
}

// fmtMs formats a latency in milliseconds with two significant digits below 10 ms ("0.42 ms", "3.1 ms") and
// whole milliseconds above ("48 ms", "1,204 ms").
func fmtMs(v float64) string {
	switch {
	case v < 1:
		return strconv.FormatFloat(v, 'f', 2, 64) + " ms"
	case v < 10:
		return strconv.FormatFloat(v, 'f', 1, 64) + " ms"
	default:
		return groupThousands(int64(math.Round(v))) + " ms"
	}
}

// fmtBytes formats a byte count with binary units ("812 B", "31.2 KiB", "1.4 MiB").
func fmtBytes(v float64) string {
	switch {
	case v < 1024:
		return strconv.FormatFloat(v, 'f', 0, 64) + " B"
	case v < 1024*1024:
		return strconv.FormatFloat(v/1024, 'f', 1, 64) + " KiB"
	default:
		return strconv.FormatFloat(v/1024/1024, 'f', 1, 64) + " MiB"
	}
}

// niceStep returns a round axis step (1, 2, 2.5 or 5 × 10^k) giving at most maxTicks intervals up to max.
func niceStep(maxV float64, maxTicks int) float64 {
	if maxV <= 0 || maxTicks < 1 {
		return 1
	}
	raw := maxV / float64(maxTicks)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if step := m * mag; step >= raw {
			return step
		}
	}
	return 10 * mag
}

// compact formats an axis tick: 0, 500, 2K, 2.5K, 40K, 1.2M.
func compact(v float64) string {
	trim := func(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
	switch {
	case v >= 1e6:
		return trim(math.Round(v/1e5)/10) + "M"
	case v >= 1e3:
		return trim(math.Round(v/1e2)/10) + "K"
	default:
		return trim(v)
	}
}
