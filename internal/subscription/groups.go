package subscription

import (
	"regexp"
	"strings"
	"sync"
)

// Policy group types.
const (
	GroupSelect      = "select"
	GroupURLTest     = "url-test"
	GroupFallback    = "fallback"
	GroupLoadBalance = "load-balance"
)

// Defaults for health-checked groups without url/interval.
const (
	DefaultTestURL      = "http://www.gstatic.com/generate_204"
	DefaultTestInterval = 300
)

// MemberKind tells what a group member refers to.
type MemberKind uint8

// Member kinds.
const (
	MemberProxy MemberKind = iota
	MemberGroup
	MemberDirect
	MemberReject
)

// Member is one entry of a resolved policy group.
type Member struct {
	Name string
	Kind MemberKind
}

// ResolvedGroup is a policy group with its include tokens resolved to proxy names, other groups, DIRECT and
// REJECT (see docs/RULESETS.md "Template and group").
type ResolvedGroup struct {
	Name     string
	Type     string
	URL      string
	Interval int
	Members  []Member
}

// IsHealthChecked reports whether the group probes its members (url-test, fallback, load-balance).
func (g *ResolvedGroup) IsHealthChecked() bool { return g.Type != GroupSelect }

// SplitInclude splits an include list on commas, trimming blanks.
func SplitInclude(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ResolveGroups resolves policy groups against proxies:
//   - a token naming another group adds that group, DIRECT/REJECT add the built-in policy;
//   - "*" adds every proxy (info pseudo nodes only to the Proxy select group, where users look for them);
//   - any other token is an unanchored RE2 expression matched against proxy names (invalid expressions match
//     literally); info nodes are never matched.
//
// Members are de-duplicated in order, references that would form a cycle are dropped, and a group left empty
// gets Proxy (or DIRECT when that would loop, and always for Proxy itself).
func ResolveGroups(groups []PolicyGroup, proxies []Proxy) []ResolvedGroup {
	index := make(map[string]int, len(groups))
	for i, g := range groups {
		if _, dup := index[g.Name]; !dup {
			index[g.Name] = i
		}
	}
	out := make([]ResolvedGroup, 0, len(groups))
	for i, g := range groups {
		if index[g.Name] != i {
			continue // duplicate name: first wins
		}
		rg := ResolvedGroup{Name: g.Name, Type: g.Type, URL: g.URL, Interval: g.Interval}
		if rg.Type == "" {
			rg.Type = GroupSelect
		}
		if rg.IsHealthChecked() {
			if rg.URL == "" {
				rg.URL = DefaultTestURL
			}
			if rg.Interval <= 0 {
				rg.Interval = DefaultTestInterval
			}
		}
		seen := map[string]bool{}
		add := func(m Member) {
			if !seen[m.Name] {
				seen[m.Name] = true
				rg.Members = append(rg.Members, m)
			}
		}
		for _, tok := range SplitInclude(g.Include) {
			switch tok {
			case g.Name:
			case PolicyDirect:
				add(Member{Name: PolicyDirect, Kind: MemberDirect})
			case PolicyReject:
				add(Member{Name: PolicyReject, Kind: MemberReject})
			case "*":
				for j := range proxies {
					if proxies[j].Info && (rg.IsHealthChecked() || rg.Name != GroupProxy) {
						continue
					}
					add(Member{Name: proxies[j].Name})
				}
			default:
				if _, ok := index[tok]; ok {
					add(Member{Name: tok, Kind: MemberGroup})
					continue
				}
				re := includeRegexp(tok)
				for j := range proxies {
					if !proxies[j].Info && re.MatchString(proxies[j].Name) {
						add(Member{Name: proxies[j].Name})
					}
				}
			}
		}
		out = append(out, rg)
	}
	breakCycles(out)
	fillEmpty(out)
	return out
}

// includeRegexps caches compiled include tokens: every subscription render resolves every group, and the
// tokens of the default template and of saved profiles repeat across users. Bounded by clearing when full.
var includeRegexps struct {
	sync.Mutex
	m map[string]*regexp.Regexp
}

const includeRegexpCacheSize = 4096

// includeRegexp compiles tok as an unanchored RE2 expression (literally when it is not valid), cached.
func includeRegexp(tok string) *regexp.Regexp {
	includeRegexps.Lock()
	re, ok := includeRegexps.m[tok]
	includeRegexps.Unlock()
	if ok {
		return re
	}
	re, err := regexp.Compile(tok)
	if err != nil {
		re = regexp.MustCompile(regexp.QuoteMeta(tok))
	}
	includeRegexps.Lock()
	if includeRegexps.m == nil || len(includeRegexps.m) >= includeRegexpCacheSize {
		includeRegexps.m = make(map[string]*regexp.Regexp, 64)
	}
	includeRegexps.m[tok] = re
	includeRegexps.Unlock()
	return re
}

// reaches reports whether group from reaches group to through group members.
func reaches(groups []ResolvedGroup, byName map[string]int, from, to string) bool {
	seen := map[string]bool{}
	stack := []string{from}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == to {
			return true
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		if i, ok := byName[cur]; ok {
			for _, m := range groups[i].Members {
				if m.Kind == MemberGroup {
					stack = append(stack, m.Name)
				}
			}
		}
	}
	return false
}

func groupIndex(groups []ResolvedGroup) map[string]int {
	m := make(map[string]int, len(groups))
	for i, g := range groups {
		m[g.Name] = i
	}
	return m
}

// breakCycles removes group references that close a loop, scanning groups and members in order.
func breakCycles(groups []ResolvedGroup) {
	byName := groupIndex(groups)
	for i := range groups {
		kept := groups[i].Members[:0]
		for _, m := range groups[i].Members {
			if m.Kind == MemberGroup && reaches(groups, byName, m.Name, groups[i].Name) {
				continue
			}
			kept = append(kept, m)
		}
		groups[i].Members = kept
	}
}

func fillEmpty(groups []ResolvedGroup) {
	byName := groupIndex(groups)
	_, hasProxy := byName[GroupProxy]
	for i := range groups {
		if len(groups[i].Members) > 0 {
			continue
		}
		if groups[i].Name != GroupProxy && hasProxy && !reaches(groups, byName, GroupProxy, groups[i].Name) {
			groups[i].Members = []Member{{Name: GroupProxy, Kind: MemberGroup}}
			continue
		}
		groups[i].Members = []Member{{Name: PolicyDirect, Kind: MemberDirect}}
	}
}
