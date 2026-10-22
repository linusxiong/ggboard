package xboardimport

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/cli"
)

func init() { cli.Register(newImportCommand) }

// newImportCommand builds `ggboard import` with the `xboard` subcommand.
func newImportCommand(a *app.App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import data from another panel",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newXboardCommand(a))
	return cmd
}

func newXboardCommand(a *app.App) *cobra.Command {
	var (
		opts Options
		tz   string
	)
	cmd := &cobra.Command{ //nolint:gosec // G101: the Example DSNs are documentation placeholders, not credentials
		Use:   "xboard",
		Short: "Import an Xboard database (legacy or upstream schema); orders are never imported",
		Long: `Import users, plans, server groups, routes, nodes, payments, coupons, invite codes, commission logs,
tickets, notices, knowledge and settings from an Xboard database into ggboard (docs/IMPORT_XBOARD.md).

The source is opened read-only. Every target write runs in one transaction: a fatal error leaves the target
unchanged. Run it right after "ggboard migrate" on an empty database (or pass --force to replace the imported
tables). --dry-run performs every read and write and then rolls back; it skips only the PostgreSQL sequence
reset (setval is not transactional) and the reload of the running settings.`,
		Example: `  ggboard import xboard --dsn "mysql://xboard:secret@127.0.0.1:3306/xboard" --dry-run
  ggboard import xboard --dsn "mysql://xboard:secret@tcp(127.0.0.1:3306)/xboard" --with-stats
  ggboard import xboard --dsn "postgres://xboard:secret@127.0.0.1:5432/xboard?sslmode=disable"
  ggboard import xboard --dsn "sqlite:///srv/xboard/database/database.sqlite" --force`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			loc, err := time.LoadLocation(tz)
			if err != nil {
				return usageErr("--source-tz %q: %v", tz, err)
			}
			opts.SourceTZ = loc
			rep, err := Run(cmd.Context(), a, opts)
			if err != nil {
				return err
			}
			rep.WriteSummary(cmd.OutOrStdout())
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.DSN, "dsn", "", `source database: mysql://…, postgres://… or sqlite:///path (required)`)
	f.StringVar(&opts.Driver, "driver", "", "source driver for a DSN without a scheme: mysql, postgres or sqlite")
	f.BoolVar(&opts.DryRun, "dry-run", false, "read and write everything, then roll back")
	f.BoolVar(&opts.Force, "force", false, "delete the existing ggboard data (users, plans, nodes, …) before importing")
	f.BoolVar(&opts.WithStats, "with-stats", false, "also import daily traffic and business statistics")
	f.IntVar(&opts.StatsDays, "stats-days", DefaultStatsDays, "with --with-stats: number of days of statistics to import")
	f.StringVar(&tz, "source-tz", DefaultSourceTZ, "time zone of the Xboard installation (Laravel app.timezone)")
	_ = cmd.MarkFlagRequired("dsn")
	return cmd
}
