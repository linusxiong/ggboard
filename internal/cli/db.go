package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/config"
	"github.com/linusxiong/ggboard/internal/console"
	"github.com/linusxiong/ggboard/internal/db"
)

func dbCmd(a *app.App) *cobra.Command {
	cmd := &cobra.Command{Use: "db", Short: "Database maintenance", Args: cobra.NoArgs}
	cmd.AddCommand(dbCopyCmd(a))
	return cmd
}

func dbCopyCmd(a *app.App) *cobra.Command {
	var (
		to   config.DatabaseConfig
		opts db.CopyOptions
	)
	cmd := &cobra.Command{ //nolint:gosec // G101: the Example DSNs are documentation placeholders, not credentials
		Use:   "copy",
		Short: "Copy every table into another database (e.g. SQLite to PostgreSQL) and verify the copy",
		Long: `Copy the whole ggboard database into another one: the source is the database this command is configured
with (database.driver / database.dsn, GGBOARD_DATABASE_* or --db-driver / --db-dsn, exactly like serve), the
target is --to-driver / --to-dsn. Stop the server first: rows written during the copy are not copied, and traffic
is buffered in memory until a graceful shutdown (SIGTERM) writes it.

The target gets the schema that serve would create (migrate), then every row with its primary key and every
column as stored, in one transaction. The copy is verified before it commits: the same tables, row counts and a
SHA-256 of every row read back from both sides; on PostgreSQL every id sequence is moved past the copied ids. Any
error rolls the target back. Refused: the same database on both sides, a table or column that ggboard does not
create on either side, and a target that already holds rows (unless --overwrite).

Timestamps keep the precision of the target (PostgreSQL microseconds, MySQL milliseconds). Then switch
database.driver / database.dsn to the target and start the server (docs/DEPLOY.md §5).`,
		Example: `  ggboard db copy --to-driver postgres --to-dsn 'postgres://ggboard:secret@127.0.0.1:5432/ggboard?sslmode=disable' --dry-run
  ggboard db copy --to-driver postgres --to-dsn 'postgres://ggboard:secret@db.example.com:5432/ggboard?sslmode=require'
  GGBOARD_DATABASE_DRIVER=postgres GGBOARD_DATABASE_DSN='postgres://…' ggboard db copy --to-driver sqlite --to-dsn /data/ggboard-from-postgres.db`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			to.Driver = strings.ToLower(strings.TrimSpace(to.Driver))
			switch to.Driver {
			case config.DriverSQLite, config.DriverPostgres, config.DriverMySQL:
			default:
				return fmt.Errorf("--to-driver %q must be sqlite, postgres or mysql", to.Driver)
			}
			if strings.TrimSpace(to.DSN) == "" {
				return errors.New("--to-dsn is required")
			}
			target, err := db.Open(to, a.Log)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close(target) }()
			opts.Log = a.Log
			rep, err := db.Copy(cmd.Context(), a.DB, target, opts)
			if err != nil {
				return err
			}
			printCopyReport(Output(cmd, a), a.Cfg.Database, to, rep)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&to.Driver, "to-driver", "", "target driver: postgres, sqlite or mysql (required)")
	f.StringVar(&to.DSN, "to-dsn", "", "target DSN, in the format of database.dsn for that driver (required)")
	f.BoolVar(&opts.Overwrite, "overwrite", false, "delete every row of the target first (same transaction as the copy)")
	f.BoolVar(&opts.DryRun, "dry-run", false, "copy and verify everything, then roll the target back")
	f.IntVar(&opts.BatchSize, "batch-size", db.DefaultCopyBatch, "rows per SELECT and INSERT")
	_ = cmd.MarkFlagRequired("to-driver")
	_ = cmd.MarkFlagRequired("to-dsn")
	return cmd
}

// printCopyReport prints the outcome of db copy: a result line, the source/target/mode block and one line per table.
// Plain output (pipes, files) keeps the same lines without styles.
func printCopyReport(p *console.Printer, from, to config.DatabaseConfig, r *db.CopyReport) {
	what := fmt.Sprintf("%d tables, %d rows from %s to %s", len(r.Tables), r.TotalRows(), r.Source, r.Target)
	if r.DryRun {
		p.Warning("dry run: copied and verified " + what + ", then rolled back")
	} else {
		p.Success("copied and verified " + what)
	}
	mode := "committed"
	if r.DryRun {
		mode = "dry run (rolled back; only the schema was created)"
	}
	if r.Overwrite {
		mode += " · --overwrite (target rows deleted first)"
	}
	fields := []console.Field{
		{Key: "source", Value: p.Text(databaseSummary(from))},
		{Key: "target", Value: p.Text(databaseSummary(to))},
		{Key: "mode", Value: mode},
		{Key: "verified", Value: "same tables, row counts and SHA-256 of every row on both sides"},
	}
	if r.TimePrecision > time.Nanosecond {
		// The coarser side sets the precision: MySQL (milliseconds) over PostgreSQL (microseconds) over SQLite.
		limit := r.Target
		if r.Source == config.DriverMySQL || r.Target == config.DriverSQLite {
			limit = r.Source
		}
		fields = append(fields, console.Field{Key: "timestamps", Value: precisionName(r.TimePrecision) + " precision (" + limit + ")"})
	}
	if r.Target == config.DriverPostgres {
		seq := strconv.Itoa(r.Sequences) + " moved past the copied ids"
		if r.DryRun {
			seq = "not moved (dry run)"
		}
		fields = append(fields, console.Field{Key: "sequences", Value: seq})
	}
	fields = append(fields, console.Field{Key: "took", Value: r.Took.Round(time.Millisecond).String()})
	p.Fields(2, fields)
	p.Println()

	headers := []string{"table", "rows", "sha256"}
	right := []int{1}
	if r.Overwrite {
		headers = []string{"table", "rows", "deleted", "sha256"}
		right = []int{1, 2}
	}
	rows := make([][]string, 0, len(r.Tables))
	for _, t := range r.Tables {
		row := []string{t.Name, strconv.FormatInt(t.Rows, 10)}
		if r.Overwrite {
			row = append(row, strconv.FormatInt(t.Cleared, 10))
		}
		rows = append(rows, append(row, p.Faint(t.Digest[:16])))
	}
	p.Table(headers, rows, console.TableOptions{Indent: 2, Right: right})
	if !r.DryRun {
		p.Println()
		p.Println("  next: set database.driver / database.dsn (GGBOARD_DATABASE_DRIVER, GGBOARD_DATABASE_DSN) to the target, keep jwt.secret, start the server")
	}
}

func precisionName(d time.Duration) string {
	switch d {
	case time.Microsecond:
		return "microseconds"
	case time.Millisecond:
		return "milliseconds"
	case time.Second:
		return "seconds"
	}
	return d.String()
}
