package cli

import (
	"testing"

	"github.com/linusxiong/ggboard/internal/auth"
)

// Every command that loads the configuration applies auth.argon2_concurrency before it can hash a password.
func TestArgon2ConcurrencyApplied(t *testing.T) {
	t.Cleanup(func() { auth.SetConcurrency(0) })
	t.Chdir(t.TempDir())
	t.Setenv("GGBOARD_DATA_DIR", t.TempDir())
	t.Setenv("GGBOARD_LOG_LEVEL", "error")

	t.Setenv("GGBOARD_AUTH_ARGON2_CONCURRENCY", "3")
	if out, code := run(t, "migrate"); code != 0 {
		t.Fatalf("migrate = %d %q", code, out)
	}
	if got := auth.Concurrency(); got != 3 {
		t.Fatalf("argon2 concurrency = %d, want 3", got)
	}

	t.Setenv("GGBOARD_AUTH_ARGON2_CONCURRENCY", "0")
	if out, code := run(t, "migrate"); code != 0 {
		t.Fatalf("migrate = %d %q", code, out)
	}
	if got, want := auth.Concurrency(), auth.DefaultConcurrency(); got != want {
		t.Fatalf("argon2 concurrency = %d, want the default %d", got, want)
	}
}
