package rulesets

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// Limits of the node operations of a profile (docs/SUBSCRIPTION.md "Node operations"). Every expression is
// matched against every node on every render, so they share the group include limits on size and complexity.
const (
	MaxNodeRegex      = 32 // include + exclude expressions
	MaxRenameRules    = 64
	MaxReplaceLen     = 256 // bytes of one replacement
	MaxAffixLen       = 32  // runes of prefix / suffix
	MaxNodeTypes      = 16
	MaxNodeRegions    = 64
	MaxNodeLimit      = 1000
	nodeTypeAliasList = "ss, ssr, hy, hy1, hysteria1, hy2, hysteria2"
)

var (
	emojiModes = []string{subscription.EmojiKeep, subscription.EmojiAdd, subscription.EmojiStrip}
	sortModes  = []string{subscription.SortDefault, subscription.SortName, subscription.SortRegion}
	typeNames  = append(model.NodeTypes(), "ss", "ssr", "hy", "hy1", "hysteria1", "hy2", "hysteria2")
)

// validateNodeOps checks the node operations of a profile; field names start with "nodes.".
func validateNodeOps(o subscription.NodeOps, errs FieldErrors) {
	regexes := 0
	checkRegex := func(field, expr string) {
		switch {
		case strings.TrimSpace(expr) == "":
			errs.add(field, "is required")
		case len(expr) > MaxIncludeToken:
			errs.add(field, fmt.Sprintf("must be at most %d bytes", MaxIncludeToken))
		default:
			if _, err := regexp.Compile(expr); err != nil {
				errs.add(field, "is not a valid regular expression (RE2): "+err.Error())
			} else if regexTooComplex(expr) {
				errs.add(field, "is too complex")
			}
		}
	}
	for _, list := range []struct {
		name  string
		exprs []string
	}{{"include", o.Include}, {"exclude", o.Exclude}} {
		for i, e := range list.exprs {
			if regexes++; regexes == MaxNodeRegex+1 {
				errs.add("nodes."+list.name, fmt.Sprintf("at most %d expressions across include and exclude", MaxNodeRegex))
			}
			checkRegex("nodes."+list.name+"["+strconv.Itoa(i)+"]", e)
		}
	}
	if len(o.Types) > MaxNodeTypes {
		errs.add("nodes.types", fmt.Sprintf("at most %d entries", MaxNodeTypes))
	}
	for i, t := range o.Types {
		if !slices.Contains(typeNames, strings.ToLower(t)) {
			errs.add("nodes.types["+strconv.Itoa(i)+"]", "must be a node type ("+strings.Join(model.NodeTypes(), ", ")+") or one of "+nodeTypeAliasList)
		}
	}
	if len(o.Regions) > MaxNodeRegions {
		errs.add("nodes.regions", fmt.Sprintf("at most %d entries", MaxNodeRegions))
	}
	for i, r := range o.Regions {
		if _, ok := subscription.RegionCode(r); !ok {
			errs.add("nodes.regions["+strconv.Itoa(i)+"]", "unknown region code")
		}
	}
	if len(o.Rename) > MaxRenameRules {
		errs.add("nodes.rename", fmt.Sprintf("at most %d rules", MaxRenameRules))
	}
	for i, r := range o.Rename {
		f := "nodes.rename[" + strconv.Itoa(i) + "]"
		checkRegex(f+".match", r.Match)
		if len(r.Replace) > MaxReplaceLen {
			errs.add(f+".replace", fmt.Sprintf("must be at most %d bytes", MaxReplaceLen))
		} else if !safeText(r.Replace) {
			errs.add(f+".replace", "must not contain control or invisible characters")
		}
	}
	for _, a := range []struct{ field, v string }{{"nodes.prefix", o.Prefix}, {"nodes.suffix", o.Suffix}} {
		if utf8.RuneCountInString(a.v) > MaxAffixLen {
			errs.add(a.field, fmt.Sprintf("must be at most %d characters", MaxAffixLen))
		} else if !safeText(a.v) {
			errs.add(a.field, "must not contain control or invisible characters")
		}
	}
	if !slices.Contains(emojiModes, o.Emoji) {
		errs.add("nodes.emoji", "must be one of: "+strings.Join(emojiModes, ", "))
	}
	if !slices.Contains(sortModes, o.Sort) {
		errs.add("nodes.sort", "must be one of: "+strings.Join(sortModes, ", "))
	}
	if o.Limit < 0 || o.Limit > MaxNodeLimit {
		errs.add("nodes.limit", fmt.Sprintf("must be between 0 and %d", MaxNodeLimit))
	}
}

func safeText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if subscription.UnsafeRune(r) {
			return false
		}
	}
	return true
}
