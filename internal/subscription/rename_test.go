package subscription

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// renameStep must give exactly what ReplaceAllString gives, cut to the cap: same matches (empty matches next to a
// previous match are skipped), same template expansion ($1, ${1}x, ${name}, $$, malformed $), same text between.
func TestRenameStepMatchesReplaceAll(t *testing.T) {
	exprs := []string{`x*`, `a*`, `\b`, `^`, `$`, `(?m)^`, `a|`, `(a)(b)?`, `(?i)hk`, `(?P<n>\d+)`, `.`, ``, `香`, `\s+`,
		`^(.*)$`, `(\S+) (\d+)`, `[^\x00-\x7f]`, `(x)?`}
	inputs := []string{"", "a", "baaac", "HK 01", "香港 HK 01", "aaa ab b", "\xffa\xfe", "hk,HK=Hk",
		strings.Repeat("香港 HK 01,", 80), strings.Repeat("ab", MaxProxyNameBytes/2)}
	repls := []string{"", "X", "$0$0", "${1}x", "$1x", "$$", "${n}", "$n$n", "$", "$ 1", "[$0]", "${2}-$1", strings.Repeat("$0", 128),
		strings.Repeat("香", 85), "$$$1", "$}$$", "$$$", "${a$b}", "$a$$b", "${1", "$1$", "x$$", "$香", "${0}${1}$$$2$", "-$0-$1$$-"}
	for _, e := range exprs {
		re := regexp.MustCompile(e)
		for _, in := range inputs {
			for _, repl := range repls {
				want := capName(re.ReplaceAllString(in, repl))
				if got := renameStep(re, in, splitTemplate(repl)); got != want {
					t.Fatalf("renameStep(%q, %.30q, %.20q) = %.60q (%d bytes), want %.60q (%d bytes)", e, in, repl, got, len(got), want, len(want))
				}
			}
		}
	}
}

func TestSplitTemplate(t *testing.T) {
	cases := map[string]string{
		"":                      "",
		"abc":                   "abc",
		"${1}-$2":               "${1}-|$2",
		"a$$b$1c":               "a$$b|$1c",
		"$$$1":                  "$$|$1",
		"$$$":                   "$$|$",
		strings.Repeat("$1", 3): "$1|$1|$1",
	}
	for in, want := range cases {
		if got := strings.Join(splitTemplate(in), "|"); got != want {
			t.Errorf("splitTemplate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCapName(t *testing.T) {
	ascii := strings.Repeat("a", MaxProxyNameBytes)
	if capName(ascii) != ascii || capName(ascii+"b") != ascii {
		t.Fatal("ascii")
	}
	for _, pad := range []int{0, 1, 2, 3} { // a 4-byte rune straddling the cap at every offset
		s := strings.Repeat("a", MaxProxyNameBytes-pad) + "🇭🇰"
		got := capName(s)
		if len(got) > MaxProxyNameBytes || !utf8.ValidString(got) || !strings.HasPrefix(s, got) || len(got) < MaxProxyNameBytes-3 {
			t.Fatalf("pad %d: %d bytes, valid %v", pad, len(got), utf8.ValidString(got))
		}
	}
	if got := capName(strings.Repeat("\x80", 2*MaxProxyNameBytes)); len(got) != MaxProxyNameBytes-3 { // invalid UTF-8: bounded back-off
		t.Fatalf("invalid = %d bytes", len(got))
	}
}
