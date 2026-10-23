package xboardimport

import (
	"bytes"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/console"
)

func printReport() *Report {
	r := &Report{Source: "mysql 127.0.0.1/xboard", Target: "sqlite", Flavor: FlavorLegacy, Migrations: 33,
		DryRun: true, SourceTZ: "Asia/Shanghai",
		Warnings:        []Warning{{"plans#2", "name \x1b]0;pwned\x07 truncated"}},
		SkippedTables:   []SkippedTable{{"v2_order", 120, "historical orders are never imported"}},
		DisabledCoupons: []string{"SPRING (#4)"}}
	for _, n := range []string{"plans", "users", "tickets", "ticket_messages"} {
		t := r.Table(n)
		t.Source, t.Imported = 5, 4
		t.Skipped = t.Source - t.Imported
	}
	return r
}

// A pipe or a file gets exactly WriteSummary; a terminal gets the styled report with the same content, and data
// from the source database never reaches it as escape sequences.
func TestReportPrint(t *testing.T) {
	r := printReport()
	var plain, want bytes.Buffer
	r.Print(console.New(&plain, console.Style{}))
	r.WriteSummary(&want)
	if plain.String() != want.String() {
		t.Fatalf("plain Print differs from WriteSummary:\n%s\n---\n%s", plain.String(), want.String())
	}

	for _, style := range []console.Style{{Pretty: true}, {Pretty: true, Color: true}} {
		var out bytes.Buffer
		r.Print(console.New(&out, style))
		got := out.String()
		for _, s := range []string{"Xboard import", "DRY RUN", "users", "tickets / messages", "5/5", "v2_order 120 rows",
			"Warnings (1)", "[plans#2]", "Dry run: nothing was written.", "Post-import checklist", "SPRING (#4)"} {
			if !strings.Contains(got, s) {
				t.Errorf("%+v: report lacks %q:\n%s", style, s, got)
			}
		}
		for i, item := range r.checklist() {
			if !strings.Contains(got, item) {
				t.Errorf("%+v: checklist item %d missing", style, i+1)
			}
		}
		if strings.Contains(got, "\x1b]") || strings.Contains(got, "\a") {
			t.Errorf("%+v: escape sequence from the source reached the terminal: %q", style, got)
		}
		if style.Color != strings.Contains(got, "\x1b[") {
			t.Errorf("%+v: colour escapes present = %v", style, !style.Color)
		}
	}
}
