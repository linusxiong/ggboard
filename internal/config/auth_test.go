package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/logx"
)

// auth.argon2_concurrency: 0 (automatic) by default, set by the file or GGBOARD_AUTH_ARGON2_CONCURRENCY, never negative.
func TestArgon2Concurrency(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GGBOARD_DATA_DIR", t.TempDir())
	t.Setenv("GGBOARD_JWT_SECRET", testSecret)

	if cfg := load(t, Options{}); cfg.Auth.Argon2Concurrency != 0 {
		t.Fatalf("default = %d, want 0 (automatic)", cfg.Auth.Argon2Concurrency)
	}

	file := filepath.Join(t.TempDir(), "ggboard.yaml")
	if err := os.WriteFile(file, []byte("auth:\n  argon2_concurrency: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg := load(t, Options{File: file}); cfg.Auth.Argon2Concurrency != 2 {
		t.Fatalf("file value = %d, want 2", cfg.Auth.Argon2Concurrency)
	}
	t.Setenv("GGBOARD_AUTH_ARGON2_CONCURRENCY", "3")
	if cfg := load(t, Options{File: file}); cfg.Auth.Argon2Concurrency != 3 {
		t.Fatalf("env value = %d, want 3 (env overrides the file)", cfg.Auth.Argon2Concurrency)
	}

	t.Setenv("GGBOARD_AUTH_ARGON2_CONCURRENCY", "-1")
	_, err := Load(Options{Logger: logx.Discard()})
	if err == nil || !strings.Contains(err.Error(), "auth.argon2_concurrency") {
		t.Fatalf("negative value: err = %v", err)
	}
}
