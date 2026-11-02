package nodesvc

import (
	"slices"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
)

func TestCleanMatchAcceptsPastedLists(t *testing.T) {
	want := []string{"geosite:falungong", "geosite:bbc", "domain:rfa.org", "domain:rsf.org"}
	cases := map[string][]string{
		"plain":          {"geosite:falungong", "geosite:bbc", "domain:rfa.org", "domain:rsf.org"},
		"quoted + comma": {`"geosite:falungong",`, `"geosite:bbc",`, `"domain:rfa.org",`, `"domain:rsf.org"`},
		"single quotes":  {`'geosite:falungong',`, `'geosite:bbc',`, `'domain:rfa.org',`, `'domain:rsf.org'`},
		"curly quotes":   {`“geosite:falungong”，`, `‘geosite:bbc’,`, `“domain:rfa.org”,`, `“domain:rsf.org”`},
		"unquoted comma": {"geosite:falungong,", "geosite:bbc,", "domain:rfa.org,", "domain:rsf.org"},
		"indented":       {"\t\"geosite:falungong\",  ", "  geosite:bbc ,", "\"domain:rfa.org\"", "domain:rsf.org,"},
		"one blob":       {"\"geosite:falungong\",\r\n\"geosite:bbc\",\r\n\"domain:rfa.org\",\r\n\"domain:rsf.org\"\r\n"},
		"one line":       {`"geosite:falungong", "geosite:bbc","domain:rfa.org" ,"domain:rsf.org"`},
		"json array":     {"[", `"geosite:falungong",`, `"geosite:bbc",`, `"domain:rfa.org",`, `"domain:rsf.org"`, "]"},
		"json one line":  {`["geosite:falungong","geosite:bbc","domain:rfa.org","domain:rsf.org"]`},
		"blanks":         {"geosite:falungong", "", `"geosite:bbc",`, "domain:rfa.org", ",", " ", "domain:rsf.org"},
	}
	for name, in := range cases {
		if got := CleanMatch(model.RouteActionBlock, in); !slices.Equal(got, want) {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
		if got := CleanMatch(model.RouteActionDNS, in); !slices.Equal(got, want) {
			t.Errorf("%s (dns): got %q, want %q", name, got, want)
		}
	}
}

func TestCleanMatchKeepsRegexps(t *testing.T) {
	in := []string{`(.*\.)?doubleclick\.net`, `[a-z]+\.example\.com`, `[abc]`, `a{1,3}\.example\.org`, `regexp:.*\.cn$`, `protocol:bittorrent`}
	if got := CleanMatch(model.RouteActionBlock, in); !slices.Equal(got, in) {
		t.Fatalf("got %q, want %q", got, in)
	}
	// An apostrophe inside an entry is not a quote pair.
	if got := CleanMatch(model.RouteActionBlock, []string{`it's.example.com`}); !slices.Equal(got, []string{`it's.example.com`}) {
		t.Fatalf("got %q", got)
	}
	if got := CleanMatch(model.RouteActionBlock, []string{"[]"}); len(got) != 0 {
		t.Fatalf("empty array: %q", got)
	}
}

func TestCleanMatchKeepsRawDNSJSON(t *testing.T) {
	in := []string{"main", "{", `"servers": [`, `"1.1.1.1",`, `"8.8.8.8"`, "]", "}", "}", ""}
	want := []string{"main", "{", `"servers": [`, `"1.1.1.1",`, `"8.8.8.8"`, "]", "}", "}"}
	if got := CleanMatch(model.RouteActionDNS, in); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	// "main" only means raw JSON on a dns route; order and repeats are kept everywhere.
	if got := CleanMatch(model.RouteActionBlock, []string{"main", `"a.com",`, "main"}); !slices.Equal(got, []string{"main", "a.com", "main"}) {
		t.Fatalf("block: %q", got)
	}
}

func TestNormalizeRouteBulkPaste(t *testing.T) {
	var lines []string
	for i := 0; i < 21; i++ {
		lines = append(lines, `"domain:site`+strings.Repeat("x", i)+`.example.com",`)
	}
	r := &model.NodeRoute{Action: " Block ", Match: []string{strings.Join(lines, "\n")}}
	NormalizeRoute(r)
	if err := ValidateRoute(r); err != nil || len(r.Match) != 21 || r.Match[0] != "domain:site.example.com" || r.Match[20] != "domain:sitexxxxxxxxxxxxxxxxxxxx.example.com" {
		t.Fatalf("err=%v match=%q", err, r.Match)
	}
}
