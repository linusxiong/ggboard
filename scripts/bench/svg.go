package main

import (
	"fmt"
	"math"
	"strings"
)

// Chart palette (dataviz reference palette, validated with validate_palette.js for both surfaces): one series,
// so every bar is categorical slot 1; text uses ink tokens, never the bar colour.
const svgStyle = `
svg{font-family:-apple-system,BlinkMacSystemFont,"SF Pro Text","PingFang SC","Helvetica Neue","Segoe UI","Inter",system-ui,sans-serif}
.bg{fill:#fcfcfb;stroke:rgba(11,11,11,.10)}
.t1{fill:#0b0b0b}.t2{fill:#52514e}.t3{fill:#6b6964}
.grid{stroke:#e1e0d9}.base{stroke:#c3c2b7}.bar{fill:#2a78d6}
.num{font-variant-numeric:tabular-nums}
@media (prefers-color-scheme:dark){
.bg{fill:#1a1a19;stroke:rgba(255,255,255,.10)}
.t1{fill:#f0efec}.t2{fill:#c3c2b7}.t3{fill:#a3a29b}
.grid{stroke:#2c2c2a}.base{stroke:#383835}.bar{fill:#3987e5}
}`

// groupHeading is the bilingual heading of a scenario group, Chinese first.
var groupHeading = map[string]string{
	groupSub:    "订阅 · Subscription",
	groupNode:   "节点接口 · Node API (UniProxy)",
	groupLogin:  "登录 · Login",
	groupStatic: "静态资源 · Static files",
}

func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;").Replace(s)
}

// barPath is a horizontal bar from (x, y) of width w and height h, square at the baseline and with a 4px
// rounded data end.
func barPath(x, y, w, h float64) string {
	r := math.Min(4, math.Min(w/2, h/2))
	return fmt.Sprintf("M%.1f %.1fh%.1fa%.1f %.1f 0 0 1 %.1f %.1fv%.1fa%.1f %.1f 0 0 1 %.1f %.1fh%.1fz",
		x, y, w-r, r, r, r, r, h-2*r, r, r, -r, r, -(w - r))
}

// renderSVG draws the throughput chart: one horizontal bar per scenario (requests per second, linear scale from
// zero), grouped by area, with the measured value and p99 written at each bar end.
func renderSVG(r *Results) []byte {
	const (
		width     = 960.0
		pad       = 28.0
		barX      = 352.0
		labelRoom = 210.0
		barH      = 18.0
		rowH      = 40.0
		headH     = 30.0
		top       = 128.0
	)
	barMax := width - pad - labelRoom - barX

	maxRPS := 0.0
	for _, s := range r.Scenarios {
		maxRPS = math.Max(maxRPS, s.RPS)
	}
	step := niceStep(maxRPS, 4)
	domain := math.Max(step, math.Ceil(maxRPS/step)*step)
	xOf := func(v float64) float64 { return barX + v/domain*barMax }

	var body strings.Builder
	y := top
	group := ""
	var desc []string
	for _, s := range r.Scenarios {
		if s.Group != group {
			group = s.Group
			h := groupHeading[group]
			if h == "" {
				h = group
			}
			fmt.Fprintf(&body, `<text x="%.0f" y="%.1f" class="t2" font-size="12" font-weight="600" letter-spacing=".02em">%s</text>`+"\n",
				pad, y+20, esc(h))
			y += headH
		}
		mid := y + rowH/2
		detail := s.Detail
		if detail == "" {
			detail = s.Request
		}
		if s.Concurrency != r.Settings.Concurrency {
			detail += fmt.Sprintf(" · %d conn", s.Concurrency)
		}
		fmt.Fprintf(&body, `<text x="%.0f" y="%.1f" class="t1" font-size="13.5">%s</text>`+"\n", pad, mid-3, esc(s.Title))
		fmt.Fprintf(&body, `<text x="%.0f" y="%.1f" class="t3" font-size="11">%s</text>`+"\n", pad, mid+12, esc(detail))

		w := xOf(s.RPS) - barX
		if s.RPS > 0 {
			w = math.Max(w, 2)
		}
		tip := fmt.Sprintf("%s: %s req/s; p50 %s, p95 %s, p99 %s; %s requests, %s errors", s.Title, fmtRate(s.RPS),
			fmtMs(s.P50Ms), fmtMs(s.P95Ms), fmtMs(s.P99Ms), groupThousands(s.Requests), groupThousands(s.Errors))
		desc = append(desc, fmt.Sprintf("%s %s req/s (p99 %s)", s.Title, fmtRate(s.RPS), fmtMs(s.P99Ms)))
		if w > 0 {
			fmt.Fprintf(&body, `<path class="bar" d="%s"><title>%s</title></path>`+"\n", barPath(barX, mid-barH/2, w, barH), esc(tip))
		}
		value := fmtRate(s.RPS) + " req/s"
		extra := " · p99 " + fmtMs(s.P99Ms)
		if s.Errors > 0 {
			extra += fmt.Sprintf(" · %s errors", groupThousands(s.Errors))
		}
		fmt.Fprintf(&body, `<text x="%.1f" y="%.1f" class="num"><tspan class="t1" font-size="13.5" font-weight="600">%s</tspan><tspan class="t2" font-size="12">%s</tspan></text>`+"\n",
			barX+w+8, mid+4.5, esc(value), esc(extra))
		y += rowH
	}
	bottom := y + 4

	var grid strings.Builder
	for v := 0.0; v <= domain+step/2; v += step {
		x := xOf(v)
		cls := "grid"
		if v == 0 {
			cls = "base"
		}
		fmt.Fprintf(&grid, `<line class="%s" x1="%.1f" y1="%.0f" x2="%.1f" y2="%.0f" stroke-width="1"/>`+"\n", cls, x, top+4, x, bottom)
		fmt.Fprintf(&grid, `<text class="t3 num" x="%.1f" y="%.0f" font-size="11" text-anchor="middle">%s</text>`+"\n", x, bottom+16, compact(v))
	}
	fmt.Fprintf(&grid, `<text class="t3" x="%.1f" y="%.0f" font-size="11" text-anchor="end">req/s</text>`+"\n", barX+barMax, bottom+32)

	date := r.GeneratedAt
	if len(date) >= 10 {
		date = date[:10]
	}
	title := "GG Bond 性能 · requests per second (higher is better)"
	sub1 := machineLine(r.Machine)
	sub2 := fmt.Sprintf("%s users · %d nodes · %d connections · %s per scenario · %s · commit %s · %s",
		groupThousands(int64(r.Dataset.Users)), r.Dataset.Nodes, r.Settings.Concurrency, r.Settings.Duration,
		r.Server.DBDriver, r.Commit, date)
	notes := []string{
		"Server and load generator on the same machine over loopback: no TLS, no reverse proxy, one ggboard process.",
		"Closed loop (each connection waits for its answer), so p99 is the latency at saturation. Login is argon2id-bound by design.",
		"Data: docs/perf/results.json · method and caveats: docs/perf/README.md",
	}
	height := bottom + 48 + float64(len(notes))*18 + 16

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" role="img" aria-labelledby="title desc">`+"\n",
		width, height, width, height)
	fmt.Fprintf(&b, "<title id=\"title\">%s</title>\n<desc id=\"desc\">%s. %s.</desc>\n<style>%s\n</style>\n",
		esc(title), esc(sub1), esc(strings.Join(desc, "; ")), svgStyle)
	fmt.Fprintf(&b, `<rect class="bg" x="0.5" y="0.5" width="%.0f" height="%.0f" rx="16"/>`+"\n", width-1, height-1)
	fmt.Fprintf(&b, `<text x="%.0f" y="48" class="t1" font-size="20" font-weight="600">%s</text>`+"\n", pad, esc(title))
	fmt.Fprintf(&b, `<text x="%.0f" y="74" class="t2" font-size="13">%s</text>`+"\n", pad, esc(sub1))
	fmt.Fprintf(&b, `<text x="%.0f" y="94" class="t2" font-size="12">%s</text>`+"\n", pad, esc(sub2))
	b.WriteString(grid.String())
	b.WriteString(body.String())
	for i, n := range notes {
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" class="t2" font-size="12">%s</text>`+"\n", pad, bottom+56+float64(i)*18, esc(n))
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}
