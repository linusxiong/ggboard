package console

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// fakeTerminal makes every writer count as a terminal for the duration of the test.
func fakeTerminal(t *testing.T, tty bool) {
	t.Helper()
	old := terminal
	terminal = func(io.Writer) bool { return tty }
	t.Cleanup(func() { terminal = old })
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		tty     bool
		noColor string
		term    string
		want    Style
	}{
		{"auto pipe", "", false, "", "xterm-256color", Style{}},
		{"auto tty", "auto", true, "", "xterm-256color", Style{Pretty: true, Color: true}},
		{"auto tty NO_COLOR", "auto", true, "1", "xterm-256color", Style{Pretty: true}},
		{"auto tty NO_COLOR any value", "", true, "false", "xterm-256color", Style{Pretty: true}},
		{"auto tty dumb", "auto", true, "", "dumb", Style{Pretty: true}},
		{"never tty", "never", true, "", "xterm", Style{Pretty: true}},
		{"never pipe", "NEVER", false, "", "xterm", Style{}},
		{"always pipe", "always", false, "", "", Style{Pretty: true, Color: true}},
		{"always beats NO_COLOR", " Always ", true, "1", "dumb", Style{Pretty: true, Color: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeTerminal(t, c.tty)
			t.Setenv(EnvNoColor, c.noColor)
			t.Setenv("TERM", c.term)
			if got := Detect(c.mode, io.Discard); got != c.want {
				t.Fatalf("Detect(%q) = %+v, want %+v", c.mode, got, c.want)
			}
		})
	}
	if IsTerminal(&bytes.Buffer{}) {
		t.Fatal("a buffer is not a terminal")
	}
}

func TestValidColor(t *testing.T) {
	for _, ok := range []string{"", "auto", "ALWAYS", " never "} {
		if !ValidColor(ok) {
			t.Errorf("ValidColor(%q) = false", ok)
		}
	}
	for _, bad := range []string{"yes", "true", "16"} {
		if ValidColor(bad) {
			t.Errorf("ValidColor(%q) = true", bad)
		}
	}
}

func TestClean(t *testing.T) {
	cases := map[string]string{
		"plain text":                  "plain text",
		"tab\tand\nnewline":           "tab\tand\nnewline",
		"中文 · emoji 🐷":                "中文 · emoji 🐷",
		"\x1b]52;c;aGk=\x07title":     "\ufffd]52;c;aGk=\ufffdtitle",
		"cr\rbs\bdel\x7f":             "cr\ufffdbs\ufffddel\ufffd",
		"c1 \u009b31m csi":            "c1 \ufffd31m csi",
		"bad \xff utf8":               "bad \ufffd utf8",
		"\x1b[31mred\x1b[0m after 中文": "\ufffd[31mred\ufffd[0m after 中文",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func render(s Style, fn func(p *Printer)) string {
	var buf bytes.Buffer
	fn(New(&buf, s))
	return buf.String()
}

func TestPrinterStyles(t *testing.T) {
	write := func(p *Printer) {
		p.Success("database migrated " + p.Faint("(sqlite)"))
		p.Warning("generated password: " + p.Bold("pw\x1b[2J"))
		p.Failure("error: first\nsecond")
		p.Fields(2, []Field{{Key: "Local", Value: p.Accent("http://localhost:8080/"), Mark: SymbolArrow}, {Key: "Database", Value: "sqlite"}})
		p.Table([]string{"table", "source"}, [][]string{{"users", "12"}, {"plans", p.Caution("3")}},
			TableOptions{Indent: 2, Right: []int{1}, Notes: []string{"", p.Faint("a long note that must not widen the rule")}})
	}

	plain := render(Style{}, write)
	want := "database migrated (sqlite)\ngenerated password: pw\ufffd[2J\nerror: first\nsecond\n  Local     http://localhost:8080/\n  Database  sqlite\n"
	if !strings.HasPrefix(plain, want) || strings.Contains(plain, "\x1b") {
		t.Fatalf("plain output:\n%q\nwant prefix\n%q", plain, want)
	}

	pretty := render(Style{Pretty: true}, write)
	if strings.Contains(pretty, "\x1b") {
		t.Fatalf("pretty output without colours has escapes: %q", pretty)
	}
	for _, s := range []string{"✓ database migrated (sqlite)\n", "! generated password: pw\ufffd[2J\n", "✗ error: first\n  second\n",
		"  ➜  Local     http://localhost:8080/\n", "     Database  sqlite\n", "  table  source\n", "  ─────────────\n", "  users      12\n", "  plans       3  a long note that must not widen the rule\n"} {
		if !strings.Contains(pretty, s) {
			t.Errorf("pretty output lacks %q:\n%s", s, pretty)
		}
	}

	color := render(Style{Pretty: true, Color: true}, write)
	if !strings.Contains(color, "\x1b[") || strings.Contains(color, "\x1b[2J") {
		t.Fatalf("colour output must style but never pass untrusted escapes: %q", color)
	}
	if strings.Contains(color, "\x1b[38;") {
		t.Fatalf("colour output must stay within the 16 basic colours: %q", color)
	}
	t.Logf("plain:\n%s\npretty:\n%s\ncolor:\n%s", plain, pretty, color)
}
