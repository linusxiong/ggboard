// Package cli builds the ggboard command tree. Built-in commands: serve, migrate, admin, db, version.
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
	"log/slog"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/auth"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/console"
	"github.com/linusxiong/ggboard/internal/logx"
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
	pf.String("color", "", "colours and terminal layout: auto|always|never (default auto; NO_COLOR disables auto)")
	pf.String("db-driver", "", "database driver: sqlite|postgres|mysql")
	pf.String("db-dsn", "", "database DSN")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if f := cmd.Flags().Lookup("color"); f != nil && f.Changed && !console.ValidColor(f.Value.String()) {
			return fmt.Errorf("--color %q must be auto, always or never", f.Value.String())
		}
		if !needsApp(cmd) || a.Cfg != nil {
			return nil
		}
		file, _ := cmd.Flags().GetString("config")
		// config.Load may log (a generated jwt.secret) before the configured logger exists: hold those records and
		// replay them through it, so they get the configured format and reach the ring buffer.
		early := logx.NewBuffer()
		cfg, err := config.Load(config.Options{File: file, Flags: cmd.Flags(), Logger: slog.New(early)})
		if err == nil {
			auth.SetConcurrency(cfg.Auth.Argon2Concurrency) // before any command can hash or verify a password
			err = a.Init(cfg, app.WithVersion(version))
		}
		replayEarly(cmd, a, early)
		if err != nil {
			return err
		}
		// Packages that log through slog.Default (and the standard log package) now share the configured format
		// and the ring buffer instead of printing in the standard library's own format.
		slog.SetDefault(a.Log)
		return nil
	}

	root.AddCommand(serveCmd(a), migrateCmd(a), adminCmd(a), dbCmd(a), versionCmd(version))
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
	cmd, err := root.ExecuteContextC(ctx)
	if a.Cfg != nil {
		if cerr := a.Close(context.Background()); cerr != nil && !errors.Is(cerr, context.Canceled) {
			err = errors.Join(err, cerr)
		}
	}
	if err != nil {
		p := console.New(stderr, console.Detect(colorMode(cmd, a), stderr))
		p.Failure(p.Bad("error:") + " " + p.Text(err.Error()))
		return 1
	}
	return 0
}

// Output returns a printer for cmd's standard output in the style chosen by --color / log.color (package
// console): plain lines for pipes and files, the pretty layout on a terminal. Commands print their results with it.
func Output(cmd *cobra.Command, a *app.App) *console.Printer {
	w := cmd.OutOrStdout()
	return console.New(w, console.Detect(colorMode(cmd, a), w))
}

// colorMode is log.color once the configuration is loaded; before that (version, help, a configuration error)
// it is the --color flag, else GGBOARD_LOG_COLOR.
func colorMode(cmd *cobra.Command, a *app.App) string {
	if a != nil && a.Cfg != nil {
		return a.Cfg.Log.Color
	}
	if cmd != nil {
		if f := cmd.Flags().Lookup("color"); f != nil && f.Changed {
			return f.Value.String()
		}
	}
	return os.Getenv(config.EnvPrefix + "_LOG_COLOR")
}

// replayEarly sends the records logged during config.Load to the App's logger, or, when initialization failed
// before it existed, to a text logger on stderr.
func replayEarly(cmd *cobra.Command, a *app.App, early *logx.Buffer) {
	if early.Len() == 0 {
		return
	}
	log := a.Log
	if log == nil {
		var opts []logx.Option
		if st := console.Detect(colorMode(cmd, nil), os.Stderr); st.Pretty {
			opts = append(opts, logx.Pretty(st.Color))
		}
		var err error
		if log, err = logx.New("info", "text", os.Stderr, nil, opts...); err != nil {
			return
		}
	}
	_ = early.Replay(context.Background(), log.Handler())
}
