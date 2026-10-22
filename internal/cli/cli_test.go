package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/model"
)

func run(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(context.Background(), "9.9.9", args, &out, &errOut)
	return out.String() + errOut.String(), code
}

func TestVersionNeedsNoApp(t *testing.T) {
	t.Setenv("GGBOARD_DATABASE_DRIVER", "invalid") // would fail if the App were initialized
	out, code := run(t, "version")
	if code != 0 || !strings.Contains(out, "ggboard 9.9.9") {
		t.Fatalf("version = %d %q", code, out)
	}
	if _, code := run(t, "help"); code != 0 {
		t.Fatal("help")
	}
	if out, code := run(t, "migrate"); code == 0 || !strings.Contains(out, "database.driver") {
		t.Fatalf("invalid config must fail commands that need the app: %q", out)
	}
}

func TestMigrateAndAdminCommands(t *testing.T) {
	auth.SetParams(auth.Params{Memory: 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16})
	t.Cleanup(func() { auth.SetParams(auth.DefaultParams) })
	t.Chdir(t.TempDir())
	dir := t.TempDir()
	t.Setenv("GGBOARD_DATA_DIR", dir)
	t.Setenv("GGBOARD_LOG_LEVEL", "error")

	if out, code := run(t, "migrate"); code != 0 || !strings.Contains(out, "database migrated (sqlite)") {
		t.Fatalf("migrate = %d %q", code, out)
	}
	if out, code := run(t, "admin", "create", "--email", "root@example.com", "--password", "password123"); code != 0 || !strings.Contains(out, "created administrator root@example.com") {
		t.Fatalf("create = %d %q", code, out)
	}
	if out, code := run(t, "admin", "create", "--email", "root@example.com", "--password", "password123"); code == 0 {
		t.Fatalf("duplicate create must fail: %q", out)
	}
	out, code := run(t, "admin", "create", "--email", "helper@example.com", "--staff")
	if code != 0 || !strings.Contains(out, "created staff") || !strings.Contains(out, "generated password: ") {
		t.Fatalf("staff = %d %q", code, out)
	}
	if out, code := run(t, "admin", "create", "--email", "bad-email", "--password", "password123"); code == 0 {
		t.Fatalf("invalid email accepted: %q", out)
	}
	if out, code := run(t, "admin", "create", "--email", "x@example.com", "--password", "short"); code == 0 {
		t.Fatalf("short password accepted: %q", out)
	}
	if out, code := run(t, "admin", "create"); code == 0 || !strings.Contains(out, "email") {
		t.Fatalf("missing --email = %q", out)
	}
	if out, code := run(t, "admin", "reset-password", "--email", "root@example.com", "--password", "new-password-1"); code != 0 || !strings.Contains(out, "password updated") {
		t.Fatalf("reset = %d %q", code, out)
	}
	if out, code := run(t, "admin", "reset-password", "--email", "ghost@example.com"); code == 0 {
		t.Fatalf("reset unknown = %q", out)
	}
	if out, code := run(t, "admin", "promote", "--email", "helper@example.com"); code != 0 || !strings.Contains(out, "now an administrator") {
		t.Fatalf("promote = %d %q", code, out)
	}

	// Verify the resulting state through a registered subcommand (the extension point used by importers).
	var checked bool
	Register(func(a *app.App) *cobra.Command {
		return &cobra.Command{Use: "zzz-check", RunE: func(cmd *cobra.Command, _ []string) error {
			var root, helper model.User
			a.DB.Where("email = ?", "root@example.com").First(&root)
			a.DB.Where("email = ?", "helper@example.com").First(&helper)
			checked = root.IsAdmin && auth.VerifyPassword(root.PasswordHash, "new-password-1") && root.PasswordVersion == 1 &&
				helper.IsAdmin && helper.IsStaff
			return nil
		}}
	})
	t.Cleanup(func() { mu.Lock(); factories = factories[:len(factories)-1]; mu.Unlock() })
	if out, code := run(t, "zzz-check"); code != 0 || !checked {
		t.Fatalf("registered command = %d %q checked=%v", code, out, checked)
	}
}

func TestServeShutsDownOnCancel(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GGBOARD_DATA_DIR", t.TempDir())
	t.Setenv("GGBOARD_LOG_LEVEL", "error")
	root, a := NewRoot("test")
	ctx, cancel := context.WithCancel(context.Background())
	root.SetArgs([]string{"migrate"})
	root.SetOut(&bytes.Buffer{})
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	a.Cfg.Server.Addr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, a) }()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	a.Cfg.Server.Addr = "256.0.0.1:99999"
	if err := Serve(context.Background(), a); err == nil {
		t.Fatal("invalid listen address must fail")
	}
}
