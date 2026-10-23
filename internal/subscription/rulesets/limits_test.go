package rulesets

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/subscription"
)

// hugeRegex is 250 bytes but compiles to 240000 instructions (about 10 ms and 10 MB per compile).
var hugeRegex = `(?:` + strings.Repeat("abcdefghij", 24) + `){1000}`

// fullProgramTooComplex is the complexity check before the parse-tree lower bound: always simplify and compile.
func fullProgramTooComplex(t string) bool {
	re, err := syntax.Parse(t, syntax.Perl)
	if err != nil {
		return false
	}
	prog, err := syntax.Compile(re.Simplify())
	if err != nil || len(prog.Inst) > MaxRegexInsts {
		return true
	}
	runes := 0
	for _, in := range prog.Inst {
		runes += len(in.Rune)
	}
	return runes > MaxRegexRunes
}

// The cheap pre-check must not change any verdict: checkRegex accepts and refuses exactly what regexp.Compile
// plus the full program check did, and the parse-tree bounds never exceed the compiled program's sizes.
func TestCheckRegexSameVerdictAsFullCompile(t *testing.T) {
	corpus := []string{
		"香港|HK", "(?i)japan", `^(\S+) (\d+)$`, "(?i)expire|剩余", "US|SG|JP", `[\pL\pN]{2}`, `[\pL\pN]{20}`,
		`[\pL\pN]{1000}`, `(?:ab){127}`, `(?:ab){128}`, `(a){100}`, `(?:)`, `(?:){1000}`, `x{0}`, `x{0,0}`,
		`(?:a{10}){10}`, `(?:a?){200}`, `[^\x00-\x{10FFFF}]{300}`, `\b{300}`, `.{300}`, `(?s).{100}`, `(?i)abc{100}`,
		`(?:[a-c]|[x-z]){128}`, `a{2,}`, `a{300,}`, `a{0,}`, `a{1,}`, `a{1}`, `(?:abc|def)+`, `((((a))))`, `(?U)a+?`,
		`(?:a*)*`, `(?:(?:a*)*){500}`, `(?P<n>a)\d{3}`, `[[:alpha:]]{100}`, `\p{Han}{3}`, `(?i)[\p{Greek}]{50}`,
		"([", "a**", "x{1001}", `(?:(?:a{100}){100}){100}`, `\Q` + strings.Repeat("a.", 120) + `\E`, hugeRegex,
	}
	for _, n := range []int{1, 50, 127, 128, 200, 253, 254, 255, 256, 300, 1000} {
		corpus = append(corpus, fmt.Sprintf("a{%d}", n), fmt.Sprintf("(?:ab){%d}", n), fmt.Sprintf("[a-z]{%d}", n),
			fmt.Sprintf(`\pL{%d}`, n), fmt.Sprintf("(?i)k{%d}", n), strings.Repeat("a", min(n, 256)))
	}
	for _, tpl := range Builtin().Templates {
		for _, g := range tpl.Groups {
			corpus = append(corpus, subscription.SplitInclude(g.Include)...)
		}
	}
	for _, expr := range corpus {
		_, compileErr := regexp.Compile(expr)
		err := checkRegex(expr)
		switch {
		case compileErr != nil:
			if err == nil || errors.Is(err, errRegexTooComplex) || err.Error() != compileErr.Error() {
				t.Errorf("%q: checkRegex = %v, regexp.Compile = %v", expr, err, compileErr)
			}
			continue
		case fullProgramTooComplex(expr) != errors.Is(err, errRegexTooComplex) || err != nil && !errors.Is(err, errRegexTooComplex):
			t.Errorf("%q: checkRegex = %v, full check too complex = %v", expr, err, fullProgramTooComplex(expr))
		}
		re, _ := syntax.Parse(expr, syntax.Perl)
		prog, _ := syntax.Compile(re.Simplify())
		runes := 0
		for _, in := range prog.Inst {
			runes += len(in.Rune)
		}
		if li, lr := progLowerBound(re); li > len(prog.Inst) || lr > runes {
			t.Errorf("%q: lower bound (%d insts, %d runes) above the program (%d, %d)", expr, li, lr, len(prog.Inst), runes)
		}
	}
	// The expensive ones are refused from the parse tree alone, without being compiled.
	for _, expr := range []string{hugeRegex, `[\pL\pN]{1000}`, `(?:(?:a{10}){10}){10}`, "a{1000}"} {
		re, err := syntax.Parse(expr, syntax.Perl)
		if err != nil {
			t.Fatal(err)
		}
		if li, lr := progLowerBound(re); li <= MaxRegexInsts && lr <= MaxRegexRunes {
			t.Errorf("%q: lower bound (%d, %d) does not refuse it", expr, li, lr)
		}
	}
}

func renameRules(n int, match string) []subscription.RenameRule {
	out := make([]subscription.RenameRule, n)
	for i := range out {
		out[i] = subscription.RenameRule{Match: match, Replace: "x"}
	}
	return out
}

func storedSize(t *testing.T, p subscription.RuleProfile) int {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return len(b)
}

// maxedProfile sits at every per-field limit of a user profile: 32 groups with 64-character names, 2048-byte
// include lists and 512-byte URLs, 32 group expressions, 32 node expressions, 64 rename rules with 256-byte
// replacements, 16 types, 64 regions and 32-character affixes.
func maxedProfile(c *subscription.Catalog) subscription.RuleProfile {
	p := DefaultProfile(c, def)
	literal := strings.Repeat("n", 254) // 254 + fail + match = 256 instructions
	p.Groups = nil
	for i := range MaxGroups {
		name := subscription.GroupProxy
		if i > 0 {
			name = fmt.Sprintf("组%02d", i) + strings.Repeat("名", MaxGroupName-3)
		}
		include := literal
		for len(include)+len(", DIRECT") <= MaxIncludeLen {
			include += ", DIRECT"
		}
		p.Groups = append(p.Groups, subscription.PolicyGroup{Name: name, Type: subscription.GroupSelect, Include: include,
			URL: "https://example.com/" + strings.Repeat("u", MaxURLLen-len("https://example.com/"))})
	}
	p.Categories = subscription.CategoryTargets{}
	for _, cat := range c.Categories {
		p.Categories[cat.ID] = subscription.GroupProxy
	}
	p.Final = subscription.GroupProxy
	p.Nodes = subscription.NodeOps{
		Include: slices.Repeat([]string{literal}, MaxNodeRegex/2),
		Exclude: slices.Repeat([]string{literal}, MaxNodeRegex/2),
		Types:   slices.Repeat([]string{"vless"}, MaxNodeTypes),
		Regions: slices.Repeat([]string{"HK"}, MaxNodeRegions),
		Rename:  slices.Repeat([]subscription.RenameRule{{Match: literal, Replace: strings.Repeat("x", MaxReplaceLen)}}, MaxRenameRules),
		Prefix:  strings.Repeat("前", MaxAffixLen),
		Suffix:  strings.Repeat("后", MaxAffixLen),
		Limit:   MaxNodeLimit,
	}.Normalized()
	return p
}

// Boundary: a profile at every per-field limit is valid and fits the stored-size limit, so the size cap never
// refuses a profile the other limits accept; one more rename rule beyond the size cap is refused by size alone.
func TestValidateProfileSizeLimit(t *testing.T) {
	c := Builtin()
	p := maxedProfile(c)
	if err := ValidateProfile(c, p); err != nil {
		t.Fatalf("maxed profile: %v", err)
	}
	if n := storedSize(t, p); n > MaxProfileBytes {
		t.Fatalf("maxed profile is %d bytes, over MaxProfileBytes %d", n, MaxProfileBytes)
	} else {
		t.Logf("maxed profile: %d bytes", n)
	}

	// 1 MiB of rename rules with a huge expression each: before the limits, validating it compiled all ~3700
	// expressions twice (about 50 s of CPU for one request).
	big := DefaultProfile(c, def)
	big.Nodes.Rename = renameRules(3700, hugeRegex)
	start := time.Now()
	fe, ok := AsFieldErrors(ValidateProfile(c, big))
	if !ok || len(fe) != 1 || len(fe["profile"]) != 1 {
		t.Fatalf("oversized profile: %v", fe)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("refusing an oversized profile took %v", d)
	}
}

// Expressions over the count limits are refused without being compiled: no per-expression error follows the
// count error, and a profile just under the size limit full of huge expressions is refused quickly.
func TestValidateProfileStopsAtCountLimits(t *testing.T) {
	c := Builtin()
	check := func(name string, p subscription.RuleProfile, want map[string]int) {
		t.Helper()
		start := time.Now()
		fe, _ := AsFieldErrors(ValidateProfile(c, p))
		if d := time.Since(start); d > 3*time.Second {
			t.Errorf("%s: validation took %v", name, d)
		}
		got := map[string]int{}
		for f, msgs := range fe {
			got[f] = len(msgs)
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s: errors %v, want counts %v", name, fe, want)
		}
	}

	// Group include: 32 expressions, then invalid and huge ones that are never compiled.
	tokens := make([]string, 0, 40)
	for i := range MaxIncludeRegex {
		tokens = append(tokens, fmt.Sprintf("n%02d", i))
	}
	tokens = append(tokens, "([", hugeRegex, "(?:")
	p := DefaultProfile(c, def)
	p.Groups = []subscription.PolicyGroup{{Name: subscription.GroupProxy, Type: subscription.GroupSelect, Include: strings.Join(tokens, ", ")}}
	p.Categories = subscription.CategoryTargets{}
	p.Final = subscription.GroupProxy
	check("group include", p, map[string]int{"groups[0].include": 1})

	// Node include + exclude: 32 expressions, then invalid ones.
	p = DefaultProfile(c, def)
	p.Nodes.Include = slices.Repeat([]string{"a"}, MaxNodeRegex)
	p.Nodes.Exclude = []string{"([", hugeRegex}
	check("node expressions", p, map[string]int{"nodes.exclude": 1})

	// Rename: the first 64 rules are checked, the rest only counted.
	p = DefaultProfile(c, def)
	p.Nodes.Rename = append(renameRules(MaxRenameRules, "a"), renameRules(6, "(")...)
	check("rename count", p, map[string]int{"nodes.rename": 1})

	// Just under the size limit: ~800 huge rename rules. Before the limits each was compiled twice (~15 s).
	p = DefaultProfile(c, def)
	p.Nodes.Rename = renameRules(800, hugeRegex)
	if n := storedSize(t, p); n > MaxProfileBytes {
		t.Fatalf("test profile is %d bytes, over the size limit", n)
	}
	want := map[string]int{"nodes.rename": 1}
	for i := range MaxRenameRules {
		want[fmt.Sprintf("nodes.rename[%d].match", i)] = 1
	}
	check("huge rename rules", p, want)
}
