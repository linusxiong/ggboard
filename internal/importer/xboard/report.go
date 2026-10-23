package xboardimport

import (
	"fmt"
	"io"
	"sort"
	"strings"
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

// WriteSummary prints the summary of §6 followed by the post-import checklist of §7.
func (r *Report) WriteSummary(w io.Writer) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	mig := fmt.Sprintf("migrations %d", r.Migrations)
	if r.LastMigration != "" {
		mig += " (last " + r.LastMigration + ")"
	}
	p("Xboard import — source %s · flavor %s · %s\n", r.Source, r.Flavor, mig)
	mode := "COMMITTED"
	if r.DryRun {
		mode = "DRY-RUN (rolled back)"
	}
	if r.Force {
		mode += " · --force (target tables cleared first)"
	}
	stats := "off"
	if r.WithStats {
		stats = fmt.Sprintf("last %d days", r.StatsDays)
	}
	p("mode: %s · target %s · source-tz %s · stats: %s\n", mode, r.Target, r.SourceTZ, stats)
	p("%-20s %8s %9s %8s\n", "table", "source", "imported", "skipped")
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
			p("%-20s %8d %9d %8d", t.Name, t.Source, t.Imported, t.Skipped)
			if t.Note != "" {
				p("   %s", t.Note)
			}
			p("\n")
		default:
			a, b := lines[0], lines[1]
			p("%-20s %8s %9s %8d", a.Name+" / "+trimPlural(b.Name),
				fmt.Sprintf("%d/%d", a.Source, b.Source), fmt.Sprintf("%d/%d", a.Imported, b.Imported), a.Skipped+b.Skipped)
			var notes []string
			for _, t := range lines {
				if t.Note != "" {
					notes = append(notes, t.Note)
				}
			}
			if len(notes) > 0 {
				p("   %s", strings.Join(notes, " · "))
			}
			p("\n")
		}
	}
	if len(r.SkippedTables) > 0 {
		parts := make([]string, 0, len(r.SkippedTables))
		for _, s := range r.SkippedTables {
			if s.Rows > 0 || s.Table == "v2_order" {
				parts = append(parts, fmt.Sprintf("%s %d rows", s.Table, s.Rows))
			}
		}
		if len(parts) > 0 {
			p("not imported (§4.15): %s\n", strings.Join(parts, ", "))
		}
	}
	p("warnings (%d):\n", len(r.Warnings))
	for _, wn := range r.Warnings {
		p("  %s\n", wn.String())
	}
	p("\n")
	r.writeChecklist(w)
}

func trimPlural(s string) string {
	if s == "ticket_messages" {
		return "messages"
	}
	return s
}

func (r *Report) writeChecklist(w io.Writer) {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
	if r.DryRun {
		p("dry run: nothing was written. Read every warning above, then run again without --dry-run.\n\n")
	}
	host := strings.TrimRight(r.AppURL, "/")
	if host == "" {
		host = "https://<host>"
	}
	p("post-import checklist (docs/IMPORT_XBOARD.md §7):\n")
	n := 0
	item := func(format string, args ...any) {
		n++
		p("  %d. %s\n", n, fmt.Sprintf(format, args...))
	}
	if r.AppURL == "" {
		item("Set app_url in Admin → Settings → Site: Xboard had none, and subscription links and payment notify URLs are built from it.")
	}
	switch {
	case r.ServerTokenKept:
		item("Backends: server_token was copied, so XrayR ApiKey, V2bX and soga keys stay valid. Point ApiHost / webapi_url at %s (or move the old domain).", host)
	case r.ServerTokenShort:
		item("Backends: Xboard's server_token is shorter than %d characters and was not imported; ggboard kept its own generated token. Copy ggboard's server_token (Admin → Settings → Server) into every XrayR/V2bX/soga config and point ApiHost / webapi_url at %s.", serverTokenMinLen(), host)
	default:
		item("Backends: Xboard had no server_token; copy ggboard's server_token (Admin → Settings → Server) into every XrayR/V2bX/soga config and point ApiHost / webapi_url at %s.", host)
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
	item("Keep the Xboard database as a read-only backup; the importer never modified it.")
}
