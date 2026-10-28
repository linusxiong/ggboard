package xboardimport

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestSourceMetadataNamesAreIdentifiers(t *testing.T) {
	for _, name := range []string{`v2_user WHERE 1 = 1 --`, `v2_extra"quoted`, "v2_extra`quoted", "v2_extra.with.dot"} {
		t.Run(name, func(t *testing.T) {
			dsn := newSource(t, FlavorLegacy, `CREATE TABLE "`+strings.ReplaceAll(name, `"`, `""`)+`" (probe_only TEXT);`)
			d, err := ParseDSN(dsn, "")
			if err != nil {
				t.Fatal(err)
			}
			s, err := openSource(context.Background(), d)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.close)
			if err := s.begin(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := s.loadTables(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := s.loadColumns(context.Background()); err != nil {
				t.Fatal(err)
			}
			cols, err := s.probeColumns(context.Background(), name)
			if err != nil || !slices.Equal(cols, []string{"probe_only"}) {
				t.Fatalf("columns = %v, err = %v; want only the named table's probe_only column", cols, err)
			}
		})
	}
}
