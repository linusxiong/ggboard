package logx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"regexp"
	"strings"
	"testing"
	"time"
)

var fixedTime = time.Date(2026, 10, 23, 9, 5, 7, 123456789, time.UTC)

// handle sends one record with a fixed time through the handler of l.
func handle(t *testing.T, l *slog.Logger, level slog.Level, msg string, args ...any) {
	t.Helper()
	if !l.Handler().Enabled(context.Background(), level) {
		return
	}
	r := slog.NewRecord(fixedTime, level, msg, 0)
	r.Add(args...)
	if err := l.Handler().Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
}

// JSON and plain text output must stay byte-for-byte what slog's own handlers write (log shippers parse them),
// whatever Pretty says.
func TestPlainFormatsUnchanged(t *testing.T) {
	args := []any{"addr", "127.0.0.1:8080", "status", 503, "err", errors.New("boom\x1b[31m"), "latency", 1500 * time.Microsecond}
	for _, format := range []string{"json", "text"} {
		var got, want bytes.Buffer
		opts := []Option(nil)
		if format == "json" {
			opts = append(opts, Pretty(true))
		}
		l, err := New("debug", format, &got, NewRing(4), opts...)
		if err != nil {
			t.Fatal(err)
		}
		ref := slog.New(slog.NewTextHandler(&want, &slog.HandlerOptions{Level: slog.LevelDebug}))
		if format == "json" {
			ref = slog.New(slog.NewJSONHandler(&want, &slog.HandlerOptions{Level: slog.LevelDebug}))
		}
		handle(t, l.With("component", "http"), slog.LevelWarn, "ggboard listening", args...)
		handle(t, ref.With("component", "http"), slog.LevelWarn, "ggboard listening", args...)
		if got.String() != want.String() {
			t.Errorf("%s output changed:\n got %q\nwant %q", format, got.String(), want.String())
		}
	}
}

func TestPrettyFormat(t *testing.T) {
	var out bytes.Buffer
	ring := NewRing(8)
	l, err := New("info", "text", &out, ring, Pretty(false))
	if err != nil {
		t.Fatal(err)
	}
	handle(t, l.With("component", "mail"), slog.LevelInfo, "ggboard listening", "addr", ":8080", "db", "sqlite")
	handle(t, l, slog.LevelError, "request failed", "status", 500, "err", errors.New("db: closed"))
	handle(t, l, slog.LevelDebug, "hidden")
	want := "Oct 23 09:05:07.123 INF ggboard listening component=mail addr=:8080 db=sqlite\n" +
		"Oct 23 09:05:07.123 ERR request failed status=500 err=\"db: closed\"\n"
	if out.String() != want {
		t.Fatalf("pretty without colours:\n got %q\nwant %q", out.String(), want)
	}
	if e := ring.Entries(slog.LevelDebug, 0); len(e) != 2 || e[1].Attrs["component"] != "mail" || e[0].Attrs["err"] != "db: closed" {
		t.Fatalf("ring = %+v", e)
	}
}

func TestPrettyColor(t *testing.T) {
	var out bytes.Buffer
	l, err := New("debug", "text", &out, nil, Pretty(true))
	if err != nil {
		t.Fatal(err)
	}
	handle(t, l, slog.LevelError, "boom", "err", errors.New("disk full"), "status", 502)
	handle(t, l, slog.LevelInfo, "http", "status", 404)
	handle(t, l, slog.LevelWarn, "careful", "n", 3)
	got := out.String()
	for _, s := range []string{
		"\x1b[2mOct 23 09:05:07.123\x1b[0m",          // faint time
		"\x1b[91mERR\x1b[0m",                         // red level
		"\x1b[93mWRN\x1b[0m",                         // yellow level
		"\x1b[2;91merr=\x1b[22m\"disk full\"\x1b[0m", // red error value
		"\x1b[2;91mstatus=\x1b[22m502",               // 5xx red
		"\x1b[2;93mstatus=\x1b[22m404",               // 4xx yellow
		"\x1b[2mn=\x1b[0m3",                          // faint key
	} {
		if !strings.Contains(got, s) {
			t.Errorf("colour output lacks %q:\n%q", s, got)
		}
	}
}

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Request data must not reach the terminal as escape sequences: not as raw ESC, not as the literal text `\x1b`
// that tint's colour mode turns back into ESC, and not through a message with control characters.
func TestPrettyDefusesEscapes(t *testing.T) {
	for _, color := range []bool{true, false} {
		var out bytes.Buffer
		l, err := New("info", "text", &out, nil, Pretty(color))
		if err != nil {
			t.Fatal(err)
		}
		osc := "\x1b]52;c;cGF5bG9hZA==\x07"
		handle(t, l.With("ua", "curl "+osc), slog.LevelInfo, "login\r\x1b[2Kforged",
			"subject", `quoted \x1b]0;title\x1b\\ text`,
			"raw", "esc "+osc,
			"err", errors.New("bad input "+osc),
			"bytes", []byte("b "+osc),
			"addr", netip.MustParseAddr("192.0.2.1"),
			"list", []string{"a " + osc},
		)
		got := out.String()
		if color {
			// Drop tint's own SGR colour codes; any other escape (OSC, cursor movement, erase) must not exist.
			got = sgr.ReplaceAllString(got, "")
		}
		if strings.Contains(got, "\x1b") || strings.Contains(got, "\a") || strings.Contains(got, "\r") {
			t.Errorf("color=%v: escape reached the output: %q", color, got)
		}
		for _, s := range []string{`"login\r\x1b[2Kforged"`, `\\x1B]0;title\\x1B\\\\`, "addr=192.0.2.1", "\u241b]52"} {
			if !strings.Contains(got, s) {
				t.Errorf("color=%v: output lacks %q: %q", color, s, got)
			}
		}
	}
}

func TestBufferReplay(t *testing.T) {
	buf := NewBuffer()
	early := slog.New(buf)
	early.Warn("jwt.secret not configured; generated a random secret", "file", "data/jwt.secret")
	early.With("component", "config").WithGroup("req").Debug("dropped by level", "id", 1)
	early.WithGroup("g").With("k", "v").Info("grouped", "id", 2)
	if buf.Len() != 3 {
		t.Fatalf("held %d records", buf.Len())
	}

	var out bytes.Buffer
	ring := NewRing(8)
	l, err := New("info", "text", &out, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := buf.Replay(context.Background(), l.Handler()); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatal("Replay must empty the buffer")
	}
	got := out.String()
	if !strings.Contains(got, `level=WARN msg="jwt.secret not configured; generated a random secret" file=data/jwt.secret`) ||
		!strings.Contains(got, `msg=grouped g.k=v g.id=2`) || strings.Contains(got, "dropped") {
		t.Fatalf("replayed:\n%s", got)
	}
	if e := ring.Entries(slog.LevelDebug, 0); len(e) != 2 || e[0].Attrs["g.k"] != "v" {
		t.Fatalf("ring = %+v", e)
	}
}

// The pretty handler must stay in the same cost class as slog.TextHandler (it is only used on a terminal, but the
// access log runs through it there). go test -bench Handlers -benchmem ./internal/logx/
func BenchmarkHandlers(b *testing.B) {
	for _, c := range []struct {
		name string
		opts []Option
	}{{"text", nil}, {"pretty", []Option{Pretty(false)}}, {"pretty-color", []Option{Pretty(true)}}} {
		b.Run(c.name, func(b *testing.B) {
			l, err := New("info", "text", io.Discard, nil, c.opts...)
			if err != nil {
				b.Fatal(err)
			}
			l = l.With("request_id", "0f3c2a")
			b.ReportAllocs()
			for b.Loop() {
				l.Info("http", "method", "GET", "path", "/api/v1/user/info", "status", 200,
					"latency", "1.2ms", "ip", "192.0.2.1", "bytes", 512)
			}
		})
	}
}
