package xboardimport

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/console"
)

// Warning is one "W" line of docs/IMPORT_XBOARD.md: a lossy conversion or something the operator must act on.
type Warning struct {
	Scope   string // "users#88", "settings", "nodes#5", …
	Message string
}

func (w Warning) String() string { return "[" + w.Scope + "] " + w.Message }

// TableStat is one summary line.
type TableStat struct {
	Name     string
	Source   int // rows read from the source
	Imported int // rows written to the target
	Skipped  int // rows dropped
	Note     string
}

// addNote prepends s to the line's note (notes are joined with " · ").
func (t *TableStat) addNote(s string) {
	if t.Note == "" {
		t.Note = s
		return
	}
	t.Note = s + " · " + t.Note
}

// BannedSkip counts what --skip-banned left out or rewrote (docs/IMPORT_XBOARD.md §3.5).
type BannedSkip struct {
	Users          int // banned v2_user rows not imported
	AdminsOrStaff  int // of them, admins or staff
	InviteCodes    int // codes owned by a banned user
	CommissionLogs int // logs earned by a banned inviter
	Tickets        int // tickets opened by a banned user
	TicketMessages int // messages of those tickets
	StatUsers      int // v2_stat_user rows of banned users (--with-stats)

	// References from imported rows to a banned user, rewritten so that none dangles.
	InviteUserIDNulled     int // users.invite_user_id → NULL
	CommissionUserIDZeroed int // commission_logs.user_id (a banned invitee) → 0
	LastReplyUserIDNulled  int // tickets.last_reply_user_id → NULL
	MessageUserIDZeroed    int // ticket_messages.user_id (a banned author in a kept ticket) → 0
}

// SkippedTable is a source table that is never imported (§4.15).
type SkippedTable struct {
	Table  string
	Rows   int64
	Reason string
}

// Report is the outcome of Run. It is identical for a dry run and a real run of the same data.
type Report struct {
	Source        string
	Target        string
	Flavor        Flavor
	Migrations    int64
	LastMigration string
	DryRun        bool
	Force         bool
	SkipBanned    bool
	Banned        BannedSkip // with SkipBanned
	WithStats     bool
	StatsDays     int
	SourceTZ      string
	Tables        []*TableStat
	Warnings      []Warning
	SkippedTables []SkippedTable

	// Inputs of the post-import checklist.
	RenumberedNodes []string // "vmess #3 "HK-01" → node 12"
	CodeNodes       []string // upstream nodes whose backend may use NodeID=<code>
	Webhooks        []string // "stripe #3 (uuid abcd1234)": webhook URL to update in the gateway dashboard
	DisabledPays    []string // payments imported disabled
	DisabledCoupons []string // "CODE (#id)": coupons imported disabled because a limit list lost every element
	UnusableHashes  int
	LegacyHashes    int
	AccessArticles  int
	AppURL          string
	ServerTokenKept bool // Xboard's server_token was copied
	// Xboard's server_token was shorter than ggboard's minimum and not imported (ggboard's own token is kept).
	ServerTokenShort bool
}

// Table returns the summary line of a table (creating it on first use, in call order).
func (r *Report) Table(name string) *TableStat {
	for _, t := range r.Tables {
		if t.Name == name {
			return t
		}
	}
	t := &TableStat{Name: name}
	r.Tables = append(r.Tables, t)
	return t
}

// Lookup returns the summary line of a table, or nil.
func (r *Report) Lookup(name string) *TableStat {
	for _, t := range r.Tables {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// WarningsFor returns the warnings whose scope starts with prefix.
func (r *Report) WarningsFor(prefix string) []Warning {
	var out []Warning
	for _, w := range r.Warnings {
		if strings.HasPrefix(w.Scope, prefix) {
			out = append(out, w)
		}
	}
	return out
}

func (r *Report) finish(im *importer) {
	r.AppURL = im.appURL
}

// summaryOrder is the display order of the table lines; pairs are printed on one line.
var summaryOrder = [][]string{
	{"server_groups"}, {"plans"}, {"node_routes"}, {"nodes"}, {"users"}, {"invite_codes"}, {"commission_logs"},
	{"coupons"}, {"payments"}, {"tickets", "ticket_messages"}, {"notices", "knowledge"}, {"settings"},
	{"stats"}, {"stat_nodes"}, {"stat_users"},
}

// summaryRow is one display line of the table: a table, or a pair of tables ("tickets / messages", "97/402").
type summaryRow struct {
	name, source, imported string
	skipped                int
	note                   string
}

func (r *Report) rows() []summaryRow {
	var out []summaryRow
	for _, names := range summaryOrder {
		var lines []*TableStat
		for _, n := range names {
			if t := r.Lookup(n); t != nil {
				lines = append(lines, t)
			}
		}
		switch len(lines) {
		case 0:
			continue
		case 1:
			t := lines[0]
			out = append(out, summaryRow{t.Name, strconv.Itoa(t.Source), strconv.Itoa(t.Imported), t.Skipped, t.Note})
		default:
			a, b := lines[0], lines[1]
			var notes []string
			for _, t := range lines {
				if t.Note != "" {
					notes = append(notes, t.Note)
				}
			}
			out = append(out, summaryRow{
				a.Name + " / " + trimPlural(b.Name),
				fmt.Sprintf("%d/%d", a.Source, b.Source), fmt.Sprintf("%d/%d", a.Imported, b.Imported),
				a.Skipped + b.Skipped, strings.Join(notes, " · "),
			})
		}
	}
	return out
}

func (r *Report) migrationsLine() string {
	mig := fmt.Sprintf("migrations %d", r.Migrations)
	if r.LastMigration != "" {
		mig += " (last " + r.LastMigration + ")"
	}
	return mig
}

func (r *Report) statsLine() string {
	if r.WithStats {
		return fmt.Sprintf("last %d days", r.StatsDays)
	}
	return "off"
}

// notImported lists the skipped source tables worth mentioning (§4.15): every one with rows, and v2_order always.
func (r *Report) notImported() []string {
	var parts []string
	for _, s := range r.SkippedTables {
		if s.Rows > 0 || s.Table == "v2_order" {
			parts = append(parts, fmt.Sprintf("%s %d rows", s.Table, s.Rows))
		}
	}
	return parts
}

const dryRunNote = "dry run: nothing was written. Read every warning above, then run again without --dry-run."

const skipBannedMode = "--skip-banned (banned users left out)"

// WriteSummary prints the plain-text summary of §6 followed by the post-import checklist of §7.
func (r *Report) WriteSummary(w io.Writer) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	p("Xboard import — source %s · flavor %s · %s\n", r.Source, r.Flavor, r.migrationsLine())
	mode := "COMMITTED"
	if r.DryRun {
		mode = "DRY-RUN (rolled back)"
	}
	if r.Force {
		mode += " · --force (target tables cleared first)"
	}
	if r.SkipBanned {
		mode += " · " + skipBannedMode
	}
	p("mode: %s · target %s · source-tz %s · stats: %s\n", mode, r.Target, r.SourceTZ, r.statsLine())
	p("%-20s %8s %9s %8s\n", "table", "source", "imported", "skipped")
	for _, row := range r.rows() {
		p("%-20s %8s %9s %8d", row.name, row.source, row.imported, row.skipped)
		if row.note != "" {
			p("   %s", row.note)
		}
		p("\n")
	}
	if parts := r.notImported(); len(parts) > 0 {
		p("not imported (§4.15): %s\n", strings.Join(parts, ", "))
	}
	p("warnings (%d):\n", len(r.Warnings))
	for _, wn := range r.Warnings {
		p("  %s\n", wn.String())
	}
	p("\n")
	if r.DryRun {
		p("%s\n\n", dryRunNote)
	}
	p("post-import checklist (docs/IMPORT_XBOARD.md §7):\n")
	for i, item := range r.checklist() {
		p("  %d. %s\n", i+1, item)
	}
}

// Print writes the summary to out: WriteSummary's plain text for pipes and files (scripts and the docs rely on
// it), a styled report with the same content on a terminal.
func (r *Report) Print(out *console.Printer) {
	if !out.Pretty() {
		r.WriteSummary(out.Writer())
		return
	}
	mode := out.Good("COMMITTED")
	if r.DryRun {
		mode = out.Caution("DRY RUN") + " " + out.Faint("rolled back")
	}
	out.Println()
	out.Println("  " + out.Bold("Xboard import") + "  " + mode)
	out.Println()
	fields := []console.Field{
		{Key: "Source", Value: out.Text(r.Source) + out.Faint(" · flavor ") + out.Text(string(r.Flavor)) + out.Faint(" · ") + out.Text(r.migrationsLine())},
		{Key: "Target", Value: out.Text(r.Target) + out.Faint(" · source-tz ") + out.Text(r.SourceTZ) + out.Faint(" · stats ") + r.statsLine()},
	}
	if r.Force {
		fields = append(fields, console.Field{Key: "Force", Value: out.Caution("target tables were cleared first")})
	}
	if r.SkipBanned {
		fields = append(fields, console.Field{Key: "Skip banned", Value: out.Caution(skipBannedMode)})
	}
	out.Fields(2, fields)
	out.Println()

	var rows [][]string
	var notes []string
	for _, row := range r.rows() {
		skipped := strconv.Itoa(row.skipped)
		if row.skipped > 0 {
			skipped = out.Caution(skipped)
		}
		rows = append(rows, []string{row.name, row.source, row.imported, skipped})
		notes = append(notes, out.Faint(row.note))
	}
	out.Table([]string{"table", "source", "imported", "skipped"}, rows,
		console.TableOptions{Indent: 2, Right: []int{1, 2, 3}, Notes: notes})
	if parts := r.notImported(); len(parts) > 0 {
		out.Println()
		out.Println("  " + out.Faint("Not imported (§4.15): ") + out.Text(strings.Join(parts, ", ")))
	}

	out.Println()
	if len(r.Warnings) == 0 {
		out.Println("  " + out.Good(console.SymbolSuccess) + " No warnings")
	} else {
		out.Println("  " + out.Caution(console.SymbolWarning) + " " + out.Bold(fmt.Sprintf("Warnings (%d)", len(r.Warnings))))
		for _, wn := range r.Warnings {
			out.Println("    " + out.Faint("["+wn.Scope+"]") + " " + out.Text(wn.Message))
		}
	}
	out.Println()
	if r.DryRun {
		out.Println("  " + out.Caution("Dry run: nothing was written.") + " Read every warning above, then run again without --dry-run.")
		out.Println()
	}
	out.Println("  " + out.Bold("Post-import checklist") + " " + out.Faint("(docs/IMPORT_XBOARD.md §7)"))
	items := r.checklist()
	width := len(strconv.Itoa(len(items)))
	for i, item := range items {
		out.Println("    " + out.Accent(fmt.Sprintf("%*d.", width, i+1)) + " " + out.Text(item))
	}
	out.Println()
}

func trimPlural(s string) string {
	if s == "ticket_messages" {
		return "messages"
	}
	return s
}

// checklist returns the post-import checklist of §7 filled with this run's specifics.
func (r *Report) checklist() []string {
	var items []string
	item := func(format string, args ...any) { items = append(items, fmt.Sprintf(format, args...)) }
	host := strings.TrimRight(r.AppURL, "/")
	if host == "" {
		host = "https://<host>"
	}
	if r.AppURL == "" {
		item("Set app_url in Admin → Settings → Site: Xboard had none, and subscription links and payment notify URLs are built from it.")
	}
	switch {
	case r.ServerTokenKept:
		item("Backends: server_token was copied, so XrayR ApiKey, V2bX and soga keys stay valid. Point ApiHost / webapi_url at %s (or move the old domain).", host)
	case r.ServerTokenShort:
		item("Backends: Xboard's server_token is shorter than %d characters and was not imported; ggboard kept its own generated token. Copy ggboard's server_token (Admin → Settings → Node connection) into every XrayR/V2bX/soga config and point ApiHost / webapi_url at %s.", serverTokenMinLen(), host)
	default:
		item("Backends: Xboard had no server_token; copy ggboard's server_token (Admin → Settings → Node connection) into every XrayR/V2bX/soga config and point ApiHost / webapi_url at %s.", host)
	}
	for _, s := range r.RenumberedNodes {
		item("Node renumbered: %s. Change NodeID in that backend's config.", s)
	}
	for _, s := range r.CodeNodes {
		item("Node %s.", s)
	}
	if len(r.Webhooks) > 0 {
		sorted := append([]string(nil), r.Webhooks...)
		sort.Strings(sorted)
		for _, s := range sorted {
			item("Payment %s: update the webhook URL in the gateway dashboard, then enable it in Admin → Payments.", s)
		}
	}
	if len(r.DisabledPays) > 0 {
		item("Review the payments imported as disabled: %s.", strings.Join(r.DisabledPays, ", "))
	}
	if len(r.DisabledCoupons) > 0 {
		item("Coupons imported disabled because none of their limited plans or periods could be imported (an empty limit would allow everything): %s. Set the limits again in Admin → Coupons, then enable them.", strings.Join(r.DisabledCoupons, ", "))
	}
	item(`Subscriptions: curl -sI "%s/api/v1/client/subscribe?token=<a real token>" must answer 200 with subscription-userinfo; check that subscribe_url lists the domains users already have.`, host)
	if r.LegacyHashes > 0 || r.UnusableHashes > 0 {
		item("Logins: %d md5/sha256 password hashes upgrade to argon2id on the next login; %d users with unusable hashes must use \"forgot password\".", r.LegacyHashes, r.UnusableHashes)
	}
	item("Settings: send a test mail (Admin → Settings → Mail), re-check captcha keys and re-set the dropped settings you need (theme, Telegram). Xboard's secure_path no longer applies: restrict /admin at the proxy if you relied on it.")
	if r.AccessArticles > 0 {
		item("Content: %d knowledge articles use <!--access start--> blocks; ggboard hides them for users without an active plan (review the wording).", r.AccessArticles)
	}
	if r.SkipBanned && r.Banned.Users > 0 {
		item("Banned users: %d were not imported (--skip-banned), with their invite codes, commission logs, tickets and traffic statistics. Their e-mail addresses are free again (they can register anew), and the Xboard database is now their only copy.", r.Banned.Users)
	}
	item("Keep the Xboard database as a read-only backup; the importer never modified it.")
	return items
}
