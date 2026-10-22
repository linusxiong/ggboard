package subscription

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Node operation modes.
const (
	EmojiKeep  = "keep"
	EmojiAdd   = "add"
	EmojiStrip = "strip"

	SortDefault = "default"
	SortName    = "name"
	SortRegion  = "region"
)

// NodeOps are the Sub-Store-style node operations of a rule profile (docs/SUBSCRIPTION.md "Node operations").
// They apply to node entries only (info entries are never touched), after the query filters and before every
// format renders, in this order: filters (types, regions, include, exclude) → dedupe → rename rules → emoji →
// prefix/suffix → sort → limit. Regular expressions are RE2 matched against the node name (`(?i)` for
// case-insensitive); rename replacements expand `$1`, `${1}` and `${name}`.
type NodeOps struct {
	Include []string     `json:"include"` // keep nodes whose name matches any expression (empty = all)
	Exclude []string     `json:"exclude"` // drop nodes whose name matches any expression
	Types   []string     `json:"types"`   // node types or the ?types= aliases (ss, ssr, hy, hy2, ...); empty = all
	Regions []string     `json:"regions"` // region codes (HK, TW, JP, SG, US, KR, DE, UK/GB, ...); empty = all
	Rename  []RenameRule `json:"rename"`  // applied in order to every name
	Prefix  string       `json:"prefix"`
	Suffix  string       `json:"suffix"`
	Emoji   string       `json:"emoji"`  // keep | add (prepend the region flag when the name has none) | strip
	Sort    string       `json:"sort"`   // default (sort, id) | name (natural, case-insensitive) | region
	Dedupe  bool         `json:"dedupe"` // keep only the first node per (type, host, port)
	Limit   int          `json:"limit"`  // at most this many nodes (0 = all)
}

// RenameRule replaces every match of Match (RE2) with Replace.
type RenameRule struct {
	Match   string `json:"match"`
	Replace string `json:"replace"`
}

// Normalized returns ops with empty lists instead of nil, trimmed regions/types and default modes, so stored
// profiles always carry the complete object.
func (o NodeOps) Normalized() NodeOps {
	norm := func(in []string, upper bool) []string {
		out := make([]string, 0, len(in))
		for _, s := range in {
			s = strings.TrimSpace(s)
			if upper {
				s = strings.ToUpper(s)
			}
			out = append(out, s)
		}
		return out
	}
	o.Include = nonNil(o.Include)
	o.Exclude = nonNil(o.Exclude)
	o.Types = norm(o.Types, false)
	o.Regions = norm(o.Regions, true)
	if o.Rename == nil {
		o.Rename = []RenameRule{}
	}
	if o.Emoji == "" {
		o.Emoji = EmojiKeep
	}
	if o.Sort == "" {
		o.Sort = SortDefault
	}
	return o
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// IsZero reports whether ops change nothing.
func (o NodeOps) IsZero() bool {
	return len(o.Include) == 0 && len(o.Exclude) == 0 && len(o.Types) == 0 && len(o.Regions) == 0 &&
		len(o.Rename) == 0 && o.Prefix == "" && o.Suffix == "" && (o.Emoji == "" || o.Emoji == EmojiKeep) &&
		(o.Sort == "" || o.Sort == SortDefault) && !o.Dedupe && o.Limit <= 0
}

// TypeMatches reports whether p matches one of the type names or ?types= aliases (empty = every type).
func TypeMatches(p *Proxy, want []string) bool { return typeMatches(p, want) }

// ApplyNodeOps applies ops to the node entries of proxies and returns the result: info entries keep their
// position in front, node entries are filtered, renamed and reordered. The input slice is not modified.
func ApplyNodeOps(proxies []Proxy, ops NodeOps) []Proxy {
	if ops.IsZero() {
		return proxies
	}
	var info []Proxy
	type item struct {
		p      Proxy
		region string
	}
	nodes := make([]item, 0, len(proxies))
	for _, p := range proxies {
		if p.Info {
			info = append(info, p)
			continue
		}
		nodes = append(nodes, item{p: p, region: DetectRegion(p.Name, p.Tags)})
	}

	// Filters.
	regions := make(map[string]int, len(ops.Regions))
	for i, r := range ops.Regions {
		if code, ok := RegionCode(r); ok {
			if _, dup := regions[code]; !dup {
				regions[code] = i
			}
		}
	}
	kept := nodes[:0:0]
	for _, it := range nodes {
		p := &it.p
		if len(ops.Types) > 0 && !typeMatches(p, ops.Types) {
			continue
		}
		if len(ops.Regions) > 0 {
			if _, ok := regions[it.region]; !ok {
				continue
			}
		}
		if len(ops.Include) > 0 && !anyMatch(ops.Include, p.Name) {
			continue
		}
		if anyMatch(ops.Exclude, p.Name) {
			continue
		}
		kept = append(kept, it)
	}

	// Dedupe on the endpoint, before renames and sorting: the first node in the admin's order wins.
	if ops.Dedupe {
		seen := make(map[string]bool, len(kept))
		uniq := kept[:0]
		for _, it := range kept {
			k := it.p.ProtoName() + "|" + strings.ToLower(it.p.Host) + "|" + strconv.Itoa(it.p.Port)
			if seen[k] {
				continue
			}
			seen[k] = true
			uniq = append(uniq, it)
		}
		kept = uniq
	}

	// Names.
	for i := range kept {
		name := kept[i].p.Name
		for _, r := range ops.Rename {
			name = includeRegexp(r.Match).ReplaceAllString(name, r.Replace)
		}
		switch ops.Emoji {
		case EmojiStrip:
			name = DisplayName(name, true)
		case EmojiAdd:
			if !HasFlag(name) {
				if flag := RegionFlag(kept[i].region); flag != "" {
					name = flag + " " + strings.TrimSpace(name)
				}
			}
		}
		kept[i].p.Name = ops.Prefix + name + ops.Suffix
	}

	switch ops.Sort {
	case SortName:
		sort.SliceStable(kept, func(i, j int) bool { return NaturalLess(kept[i].p.Name, kept[j].p.Name) })
	case SortRegion:
		rank := func(code string) int {
			if len(regions) > 0 {
				if r, ok := regions[code]; ok {
					return r
				}
				return len(ops.Regions) + len(regionTable)
			}
			if r, ok := regionIndex[code]; ok {
				return r
			}
			return len(regionTable)
		}
		sort.SliceStable(kept, func(i, j int) bool { return rank(kept[i].region) < rank(kept[j].region) })
	}
	if ops.Limit > 0 && len(kept) > ops.Limit {
		kept = kept[:ops.Limit]
	}

	out := make([]Proxy, 0, len(info)+len(kept))
	out = append(out, info...)
	for _, it := range kept {
		out = append(out, it.p)
	}
	return out
}

func anyMatch(exprs []string, s string) bool {
	return slices.ContainsFunc(exprs, func(e string) bool { return includeRegexp(e).MatchString(s) })
}

// NaturalLess compares names case-insensitively with digit runs compared as numbers ("HK 2" < "HK 10").
func NaturalLess(a, b string) bool {
	for a != "" && b != "" {
		ra, na := utf8.DecodeRuneInString(a)
		rb, nb := utf8.DecodeRuneInString(b)
		if unicode.IsDigit(ra) && unicode.IsDigit(rb) && ra < utf8.RuneSelf && rb < utf8.RuneSelf {
			da, db := digitRun(a), digitRun(b)
			ta, tb := strings.TrimLeft(a[:da], "0"), strings.TrimLeft(b[:db], "0")
			if len(ta) != len(tb) {
				return len(ta) < len(tb)
			}
			if ta != tb {
				return ta < tb
			}
			a, b = a[da:], b[db:]
			continue
		}
		la, lb := unicode.ToLower(ra), unicode.ToLower(rb)
		if la != lb {
			return la < lb
		}
		a, b = a[na:], b[nb:]
	}
	return len(a) < len(b)
}

func digitRun(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i
}
