package cli

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"runtime"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/console"
)

func TestColorFlag(t *testing.T) {
	t.Setenv(console.EnvNoColor, "")
	t.Setenv("GGBOARD_LOG_COLOR", "")
	out, code := run(t, "version")
	if want := "ggboard 9.9.9 (" + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH + ")\n"; code != 0 || out != want {
		t.Fatalf("plain version = %d %q, want %q", code, out, want)
	}
	out, code = run(t, "version", "--color", "always")
	if code != 0 || !strings.Contains(out, "\x1b[") || !strings.Contains(out, "9.9.9") {
		t.Fatalf("--color=always version = %d %q", code, out)
	}
	t.Setenv("GGBOARD_LOG_COLOR", "always")
	if out, _ = run(t, "version"); !strings.Contains(out, "\x1b[") {
		t.Fatalf("GGBOARD_LOG_COLOR=always version = %q", out)
	}
	if out, _ = run(t, "version", "--color", "never"); strings.Contains(out, "\x1b") {
		t.Fatalf("--color=never must win over the environment: %q", out)
	}
	t.Setenv("GGBOARD_LOG_COLOR", "")
	if out, code = run(t, "version", "--color", "rainbow"); code == 0 || !strings.Contains(out, "error: --color \"rainbow\" must be auto, always or never") {
		t.Fatalf("invalid --color = %d %q", code, out)
	}
	// Errors keep the plain "error: …" line on a pipe and get the failure symbol when styled.
	if out, code = run(t, "version", "--color", "always", "--nope"); code == 0 || !strings.Contains(out, "✗") || !strings.Contains(out, "unknown flag: --nope") {
		t.Fatalf("styled error = %d %q", code, out)
	}
}

// The generated jwt.secret warning is logged while the configuration is read, before the configured logger exists;
// it must still reach the App's logger (and so the ring buffer of /admin/system/logs).
func TestEarlyLogsReplayed(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GGBOARD_DATA_DIR", t.TempDir())
	t.Setenv("GGBOARD_LOG_LEVEL", "warn")
	root, a := NewRoot("test")
	root.SetArgs([]string{"migrate"})
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	var found bool
	for _, e := range a.Logs.Entries(slog.LevelWarn, 0) {
		found = found || strings.Contains(e.Msg, "jwt.secret not configured")
	}
	if !found {
		t.Fatalf("early warning not replayed: %+v", a.Logs.Entries(slog.LevelDebug, 0))
	}
	if slog.Default().Handler() != a.Log.Handler() {
		t.Error("slog.Default must use the configured logger")
	}
	if out.String() != "database migrated (sqlite)\n" {
		t.Errorf("migrate output = %q", out.String())
	}
}

func TestStartupSummary(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GGBOARD_DATA_DIR", t.TempDir())
	t.Setenv("GGBOARD_LOG_LEVEL", "error")
	root, a := NewRoot("v1.2.3")
	root.SetArgs([]string{"migrate"})
	root.SetOut(&bytes.Buffer{})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	addr := &net.TCPAddr{IP: net.IPv6unspecified, Port: 18080}

	var plain bytes.Buffer
	printSummary(context.Background(), console.New(&plain, console.Style{}), a, addr)
	got := plain.String()
	for _, s := range []string{
		"GG Bond v1.2.3 ready in ",
		"\n  Local     http://localhost:18080/ (listening on [::]:18080)\n",
		"\n  Site      not set (Admin → Settings → Site)\n",
		"\n  Database  sqlite · ",
		"\n  Logs      error · text\n",
		"\n  Payments  ",
		"\n  Notify    off\n",
		" scheduled\n",
		"\n  No administrator yet. Create one with: ggboard admin create --email you@example.com\n",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("plain summary lacks %q:\n%s", s, got)
		}
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "➜") {
		t.Errorf("plain summary must have no escapes or symbols:\n%q", got)
	}

	var pretty bytes.Buffer
	printSummary(context.Background(), console.New(&pretty, console.Style{Pretty: true, Color: true}), a, addr)
	if got := pretty.String(); !strings.Contains(got, "➜") || !strings.Contains(got, "\x1b[36mhttp://localhost:18080/") {
		t.Errorf("pretty summary:\n%q", got)
	}
}

func TestDatabaseSummaryHidesCredentials(t *testing.T) {
	cases := map[string]config.DatabaseConfig{
		"sqlite · in memory":                    {Driver: "sqlite", DSN: ":memory:"},
		"mysql · db.example.com:3306/ggboard":   {Driver: "mysql", DSN: "ggboard:s3cret@tcp(db.example.com:3306)/ggboard?parseTime=true"},
		"postgres · db.example.com:5432/panel":  {Driver: "postgres", DSN: "postgres://ggboard:s3cret@db.example.com:5432/panel?sslmode=disable"},
		"postgres · db.example.com:6543/panel2": {Driver: "postgres", DSN: "host=db.example.com port=6543 user=ggboard password=s3cret dbname=panel2"},
		"postgres":                              {Driver: "postgres", DSN: "postgres://ggboard:s3cret@[bad"},
		"mysql":                                 {Driver: "mysql", DSN: "not a dsn s3cret"},
	}
	for want, c := range cases {
		got := databaseSummary(c)
		if got != want || strings.Contains(got, "s3cret") || strings.Contains(got, "ggboard:") {
			t.Errorf("databaseSummary(%s %q) = %q, want %q", c.Driver, c.DSN, got, want)
		}
	}
	if got := databaseSummary(config.DatabaseConfig{Driver: "sqlite", DSN: "file:data/ggboard.db?_pragma=x"}); !strings.HasSuffix(got, "/data/ggboard.db") {
		t.Errorf("sqlite path = %q", got)
	}
}
