package rulesets

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/linusxiong/ggboard/internal/testutil"
)

var update = flag.Bool("update", false, "rewrite testdata/public-catalog.json")

// publicSource is the rule builder's algorithm (docs/SUBSCRIPTION.md, public catalog): the client's preferred source,
// the profile's source, then the category's sources by order; the first whose source_clients lists the client wins.
func publicSource(pub PublicCatalog, pc PublicCategory, profileSource, client string) string {
	for _, sid := range append([]string{pub.PreferredSources[client], profileSource}, pc.Sources...) {
		if slices.Contains(pc.SourceClients[sid], client) {
			return sid
		}
	}
	return ""
}

// Whatever source a profile chooses, every client gets every enabled category some source publishes for it, and the
// public catalog reproduces BuildPlan's choice exactly (that is what the rule builder shows per client).
func TestChosenSourceKeepsCoverage(t *testing.T) {
	a := testutil.NewTestApp(t)
	c := Builtin()
	pub := PublicView(c, a.Settings)
	full, ok := c.Template("full")
	if !ok || len(full.Categories) != len(c.Categories) {
		t.Fatalf("full template has %d of %d categories", len(full.Categories), len(c.Categories))
	}
	for _, sid := range SourcesByOrder(c) {
		p := FromTemplate(c, full, Defaults{Source: sid})
		for _, client := range RuleClients {
			got := map[string]string{}
			for _, s := range BuildPlan(c, p, client, "").Sets {
				if prev, seen := got[s.Category]; seen && prev != s.Source {
					t.Errorf("source %s, %s: category %s mixes %s and %s", sid, client, s.Category, prev, s.Source)
				}
				got[s.Category] = s.Source
			}
			for _, pc := range pub.Categories {
				want := publicSource(pub, pc, sid, client)
				if got[pc.ID] != want {
					t.Errorf("source %s, %s, %s: plan uses %q, public catalog says %q", sid, client, pc.ID, got[pc.ID], want)
				}
				if (want != "") != slices.Contains(pc.Clients, client) {
					t.Errorf("source %s, %s, %s: served=%v but category clients = %v", sid, client, pc.ID, want != "", pc.Clients)
				}
			}
		}
	}
}

// testdata/public-catalog.json is the public catalog with default settings; the web tests render the rule builder
// from it (go test ./internal/subscription/rulesets -run TestPublicCatalogFixture -update rewrites it).
func TestPublicCatalogFixture(t *testing.T) {
	a := testutil.NewTestApp(t)
	got, err := json.MarshalIndent(PublicView(Builtin(), a.Settings), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "public-catalog.json")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale; rerun with -update", path)
	}
}
