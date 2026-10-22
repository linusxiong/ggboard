package formats

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// Review 2026-10-22: U+FFFE/U+FFFF were written raw, and yaml.v3 rejects the whole document over them.
func TestReviewYAMLQuotesNoncharacters(t *testing.T) {
	for _, s := range []string{"a￾b", "a￿b", "a b", "x\x00y"} {
		var out map[string]string
		if err := yaml.Unmarshal([]byte("k: "+dquote(s)), &out); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if out["k"] != s {
			t.Fatalf("%q round-tripped as %q", s, out["k"])
		}
	}
}
