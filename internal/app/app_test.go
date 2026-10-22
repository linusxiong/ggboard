package app_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/logx"
)

func init() {
	app.RegisterJob("test_job", "@daily", func(ctx context.Context, a *app.App) error { return nil })
}

func testConfig(t *testing.T) *config.Config {
	dir := t.TempDir()
	return &config.Config{
		Server:   config.ServerConfig{Addr: "127.0.0.1:0"},
		Database: config.DatabaseConfig{Driver: config.DriverSQLite, DSN: filepath.Join(dir, "a.db")},
		JWT:      config.JWTConfig{Secret: "0123456789abcdef0123456789abcdef", TTL: time.Hour},
		Log:      config.LogConfig{Level: "info", Format: "text"},
		DataDir:  dir,
	}
}

func TestNewStartClose(t *testing.T) {
	a, err := app.New(testConfig(t), app.WithLogger(logx.Discard(), nil), app.WithVersion("1.2.3"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Version != "1.2.3" || a.DBDriver() != "sqlite" || a.Settings.String("server_token") == "" || a.Logs == nil {
		t.Fatal("app not initialized")
	}
	names := map[string]bool{}
	for _, st := range a.Scheduler.Status() {
		names[st.Name] = true
	}
	for _, n := range []string{app.JobOnlineCleanup, app.JobTrafficFlush, "test_job"} {
		if !names[n] {
			t.Errorf("job %s not registered", n)
		}
	}
	a.Traffic.Add(1, 1, 1, 10, 10)
	if err := a.Scheduler.RunNow(app.JobTrafficFlush); err != nil {
		t.Fatal(err)
	}
	if err := a.Scheduler.RunNow(app.JobOnlineCleanup); err != nil {
		t.Fatal(err)
	}
	a.Start()
	a.Traffic.Add(1, 1, 1, 1, 1)
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Fatal("Close must be idempotent")
	}
	if a.Traffic.Pending() != 0 {
		t.Fatal("Close must flush traffic")
	}
}

func TestAliveTTLFollowsSettings(t *testing.T) {
	a, err := app.New(testConfig(t), app.WithLogger(logx.Discard(), nil))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	if err := a.Settings.Set(map[string]any{"server_push_interval": 600}); err != nil {
		t.Fatal(err)
	}
	a.Online.SetAliveIPs(1, 1, []string{"1.1.1.1"})
	if a.Online.AliveCount(1) != 1 {
		t.Fatal("alive entry missing")
	}
}

func TestInitErrors(t *testing.T) {
	if _, err := app.New(nil); err == nil {
		t.Fatal("nil config must fail")
	}
	cfg := testConfig(t)
	cfg.Database.DSN = filepath.Join(t.TempDir(), "missing-dir", "x", "a.db")
	if _, err := app.New(cfg, app.WithLogger(logx.Discard(), nil)); err == nil {
		t.Fatal("unopenable database must fail")
	}
}
