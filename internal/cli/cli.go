// Package cli builds the ggboard command tree. Built-in commands: serve, migrate, admin, version.
// Other packages add subcommands without editing main.go:
//
//	// cmd/ggboard/import_xboard.go
//	import _ "github.com/linusxiong/ggboard/internal/importer/xboard"
//
//	// internal/importer/xboard/cli.go
//	func init() { cli.Register(func(a *app.App) *cobra.Command { return &cobra.Command{Use: "import", RunE: ...} }) }
//
// The *app.App passed to factories is initialized (config loaded, database migrated) before any RunE runs,
// unless the command carries the AnnotationNoApp annotation.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/config"
)

// AnnotationNoApp marks commands that run without an initialized App (set it to "true").
const AnnotationNoApp = "ggboard/no-app"

// Factory builds a subcommand bound to the (lazily initialized) App.
type Factory func(a *app.App) *cobra.Command

var (
	mu        sync.Mutex
	factories []Factory
)

// Register adds a top-level subcommand factory (call from init).
func Register(f func(*app.App) *cobra.Command) {
	mu.Lock()
	factories = append(factories, f)
	mu.Unlock()
}

func registered() []Factory {
	mu.Lock()
	defer mu.Unlock()
	return append([]Factory(nil), factories...)
}

func needsApp(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations[AnnotationNoApp] == "true" {
			return false
		}
		if c.Name() == "help" || c.Name() == "completion" || c.Name() == cobra.ShellCompRequestCmd || c.Name() == cobra.ShellCompNoDescRequestCmd {
			return false
		}
	}
	return true
}

// NewRoot returns the root command and the App it initializes on demand. The caller must Close the App
// when a.Cfg != nil after execution.
func NewRoot(version string) (*cobra.Command, *app.App) {
	a := &app.App{}
	root := &cobra.Command{
		Use:           "ggboard",
		Short:         "GG Bond - a fast, simple proxy panel",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	pf := root.PersistentFlags()
	pf.StringP("config", "c", "", "config file (default: $GGBOARD_CONFIG or ./config.yaml when present)")
	pf.String("data-dir", "", "data directory (default ./data)")
	pf.String("log-level", "", "log level: debug|info|warn|error")
	pf.String("log-format", "", "log format: text|json")
	pf.String("db-driver", "", "database driver: sqlite|postgres|mysql")
	pf.String("db-dsn", "", "database DSN")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if !needsApp(cmd) || a.Cfg != nil {
			return nil
		}
		file, _ := cmd.Flags().GetString("config")
		cfg, err := config.Load(config.Options{File: file, Flags: cmd.Flags()})
		if err != nil {
			return err
		}
		return a.Init(cfg, app.WithVersion(version))
	}

	root.AddCommand(serveCmd(a), migrateCmd(a), adminCmd(a), versionCmd(version))
	for _, f := range registered() {
		if c := f(a); c != nil {
			root.AddCommand(c)
		}
	}
	return root, a
}

// Execute runs the CLI with os.Args and returns the process exit code.
func Execute(version string) int {
	return Run(context.Background(), version, os.Args[1:], os.Stdout, os.Stderr)
}

// Run executes the CLI with explicit arguments and outputs (used by tests).
func Run(ctx context.Context, version string, args []string, stdout, stderr io.Writer) int {
	root, a := NewRoot(version)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.ExecuteContext(ctx)
	if a.Cfg != nil {
		if cerr := a.Close(context.Background()); cerr != nil && !errors.Is(cerr, context.Canceled) {
			err = errors.Join(err, cerr)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}
