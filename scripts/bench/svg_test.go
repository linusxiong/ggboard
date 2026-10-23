package main

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func sampleResults() *Results {
	return &Results{
		GeneratedAt: "2026-10-23T12:00:00Z",
		Commit:      "abc1234",
		Machine:     Machine{CPU: "Test CPU", Cores: 8, MemoryBytes: 16 << 30, OS: "testOS"},
		Server:      ServerInfo{Version: "abc1234", GoVersion: "go1.27", DBDriver: "sqlite"},
		Dataset:     DatasetInfo{Users: 10000, Nodes: 50},
		Settings:    RunSettings{Concurrency: 64, LoginConcurrency: 16, Duration: "30s", Warmup: "3s", PerTokenBudget: 25},
		Scenarios: []ScenarioResult{
			{ID: "a", Group: groupSub, Title: `subscribe <v2ray> & "friends"`, Request: "GET /a", Detail: "GET /a · x", Concurrency: 64,
				Requests: 1000, OK: 1000, RPS: 14496.4, P50Ms: 3.3, P95Ms: 12, P99Ms: 17.2},
			{ID: "b", Group: groupNode, Title: "node", Request: "GET /b", Concurrency: 64, Requests: 5000, OK: 5000, RPS: 62900, P99Ms: 4.7},
			{ID: "c", Group: groupLogin, Title: "login", Request: "POST /c", Concurrency: 16, Requests: 300, OK: 298, Errors: 2,
				RPS: 116.2, P99Ms: 179},
			{ID: "d", Group: groupStatic, Title: "zero", Request: "GET /d", Concurrency: 64},
		},
	}
}

// wellFormed decodes every token of b as XML.
func wellFormed(t *testing.T, b []byte) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(string(b)))
	for {
		_, err := d.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("SVG is not well-formed XML: %v", err)
		}
	}
}

var barWidth = regexp.MustCompile(`<path class="bar" d="M[0-9.]+ [0-9.]+h([0-9.]+)a([0-9.]+)`)

func TestRenderSVG(t *testing.T) {
	r := sampleResults()
	svg := renderSVG(r)
	wellFormed(t, svg)
	s := string(svg)

	for _, want := range []string{
		`role="img" aria-labelledby="title desc"`, `<title id="title">`, `<desc id="desc">`,
		"@media (prefers-color-scheme:dark)", "14,496 req/s", "62,900 req/s", "116 req/s", "· p99 17 ms", "· 2 errors",
		"订阅 · Subscription", "登录 · Login", "GET /a · x", "POST /c · 16 conn", "Test CPU · 8 cores · 16 GB RAM · testOS",
		"subscribe &lt;v2ray&gt; &amp; &quot;friends&quot;",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("SVG misses %q", want)
		}
	}
	if strings.Contains(s, "<v2ray>") {
		t.Error("titles must be escaped")
	}
	// Self-contained: no scripts, no external resources, no web fonts.
	for _, banned := range []string{"<script", "href=", "@import", "url(", "@font-face"} {
		if strings.Contains(s, banned) {
			t.Errorf("SVG contains %q", banned)
		}
	}

	// One bar per non-zero scenario; lengths follow the values on a linear scale from zero.
	m := barWidth.FindAllStringSubmatch(s, -1)
	if len(m) != 3 {
		t.Fatalf("got %d bars, want 3 (the zero scenario draws none)", len(m))
	}
	w := make([]float64, len(m))
	for i, g := range m {
		h, _ := strconv.ParseFloat(g[1], 64)
		rad, _ := strconv.ParseFloat(g[2], 64)
		w[i] = h + rad
	}
	if !(w[1] > w[0] && w[0] > w[2] && w[2] >= 2) {
		t.Fatalf("bar widths %v do not follow 62,900 > 14,496 > 116 (min 2px)", w)
	}
	if got, want := w[0]/w[1], 14496.4/62900; got < want*0.98 || got > want*1.02 {
		t.Fatalf("bar ratio %.4f, want %.4f (linear scale)", got, want)
	}
}

func TestAssetName(t *testing.T) {
	for in, want := range map[string]string{
		"/assets/react-B--GpZa_.js":            "react",
		"/assets/rolldown-runtime-BWaJ3C7D.js": "rolldown-runtime",
		"/assets/boot--9NTfdqn.js":             "boot",
		"/assets/plain.js":                     "plain",
	} {
		if got := assetName(in); got != want {
			t.Errorf("assetName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBarPathTinyBar(t *testing.T) {
	d := barPath(10, 20, 2, 18)
	if strings.Contains(d, "NaN") || strings.Contains(d, "v-") || !strings.HasSuffix(d, "h-1.0z") {
		t.Fatalf("tiny bar path %q", d)
	}
	if !strings.HasPrefix(d, "M10.0 20.0h1.0a1.0 1.0") {
		t.Fatalf("2px bar should have a 1px radius: %q", d)
	}
}

func TestWriteOutputsAndRender(t *testing.T) {
	dir := t.TempDir()
	r := sampleResults()
	if err := writeOutputs(dir, r, true); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"results.json", "results.md", "benchmark.svg"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "results.json"))
	if err != nil {
		t.Fatal(err)
	}
	var back Results
	if err := json.Unmarshal(b, &back); err != nil || len(back.Scenarios) != 4 || back.Scenarios[1].RPS != 62900 {
		t.Fatalf("results.json round trip: %v %+v", err, back.Scenarios)
	}
	md, err := os.ReadFile(filepath.Join(dir, "results.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "| Node API · node | `GET /b` | 64 | 5,000 | 62,900 |") {
		t.Fatalf("results.md row missing:\n%s", md)
	}

	// -render rewrites the SVG from the JSON alone.
	if err := os.Remove(filepath.Join(dir, "benchmark.svg")); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadResults(filepath.Join(dir, "results.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeOutputs(dir, loaded, false); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(filepath.Join(dir, "benchmark.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(renderSVG(r)) {
		t.Fatal("re-rendering from results.json must give the same SVG")
	}
}
