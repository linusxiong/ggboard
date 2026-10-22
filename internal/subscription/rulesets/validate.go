package rulesets

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/linusxiong/ggboard/internal/subscription"
)

// FormatKeys are the catalog format keys renderers understand (docs/RULESETS.md "Format keys").
var FormatKeys = []string{
	"clash", "clash_classical", "clash_domain", "clash_ipcidr", "clash_mrs_domain", "clash_mrs_ipcidr",
	"singbox", "singbox_ipcidr", "singbox_source", "singbox_source_ipcidr",
	"surge", "surge_domain", "quantumultx", "shadowrocket", "shadowrocket_domain", "loon", "loon_domain",
}

// Limits for user-defined profiles and admin catalogs.
const (
	MaxGroups       = 32
	MaxGroupName    = 64
	MaxIncludeToken = 256
	MinInterval     = 10
	MaxInterval     = 86400
)

var (
	idPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	groupTypes  = []string{subscription.GroupSelect, subscription.GroupURLTest, subscription.GroupFallback, subscription.GroupLoadBalance}
	behaviors   = []string{subscription.BehaviorDomain, subscription.BehaviorIPCIDR, subscription.BehaviorClassical}
	fileFormats = []string{"yaml", "text", "mrs", "binary", "source"}
	refFields   = []string{"name", "geosite", "geoip"}
	dnsModes    = []string{subscription.DNSDefault, subscription.DNSFakeIP, subscription.DNSNone}
)

// ValidGroupName reports why name cannot be a policy group name ("" when it can).
func ValidGroupName(name string) string {
	switch {
	case strings.TrimSpace(name) == "":
		return "is required"
	case name != strings.TrimSpace(name):
		return "must not start or end with spaces"
	case utf8.RuneCountInString(name) > MaxGroupName:
		return fmt.Sprintf("must be at most %d characters", MaxGroupName)
	case strings.ContainsAny(name, ",=*\"'"):
		return `must not contain , = * " or '`
	case subscription.IsBuiltinPolicy(name) || subscription.IsReservedName(name):
		return "is a reserved policy name"
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "must not contain control characters"
		}
	}
	return ""
}

// validateGroups checks a group list and returns the set of group names. Field names are prefixed with
// prefix ("groups" or "templates.standard.groups").
func validateGroups(groups []subscription.PolicyGroup, prefix string, errs FieldErrors) map[string]bool {
	names := map[string]bool{}
	if len(groups) == 0 {
		errs.add(prefix, "at least the Proxy group is required")
		return names
	}
	if len(groups) > MaxGroups {
		errs.add(prefix, fmt.Sprintf("at most %d groups", MaxGroups))
	}
	for i, g := range groups {
		f := prefix + "[" + strconv.Itoa(i) + "]"
		if msg := ValidGroupName(g.Name); msg != "" {
			errs.add(f+".name", msg)
		} else if names[g.Name] {
			errs.add(f+".name", "duplicate group name")
		}
		names[g.Name] = true
	}
	if !names[subscription.GroupProxy] {
		errs.add(prefix, `the "Proxy" group is required`)
	}
	for i, g := range groups {
		f := prefix + "[" + strconv.Itoa(i) + "]"
		if !slices.Contains(groupTypes, g.Type) {
			errs.add(f+".type", "must be one of: "+strings.Join(groupTypes, ", "))
		}
		if g.Type != subscription.GroupSelect && slices.Contains(groupTypes, g.Type) {
			if !validHTTPURL(g.URL) {
				errs.add(f+".url", "must be an http(s) URL")
			}
			if g.Interval < MinInterval || g.Interval > MaxInterval {
				errs.add(f+".interval", fmt.Sprintf("must be between %d and %d seconds", MinInterval, MaxInterval))
			}
		} else if g.URL != "" && !validHTTPURL(g.URL) {
			errs.add(f+".url", "must be an http(s) URL")
		}
		tokens := subscription.SplitInclude(g.Include)
		if len(tokens) == 0 {
			errs.add(f+".include", "is required")
		}
		for _, t := range tokens {
			switch {
			case t == g.Name:
				errs.add(f+".include", "a group cannot include itself")
			case t == "*" || subscription.IsBuiltinPolicy(t) || names[t]:
			case len(t) > MaxIncludeToken:
				errs.add(f+".include", fmt.Sprintf("tokens must be at most %d bytes", MaxIncludeToken))
			default:
				if _, err := regexp.Compile(t); err != nil {
					errs.add(f+".include", fmt.Sprintf("%q is neither a group nor a valid regular expression", t))
				}
			}
		}
	}
	if cyc := groupCycle(groups, names); cyc != "" {
		errs.add(prefix, "groups include each other in a loop: "+cyc)
	}
	return names
}

// groupCycle returns a description of the first include loop between groups, or "".
func groupCycle(groups []subscription.PolicyGroup, names map[string]bool) string {
	edges := map[string][]string{}
	for _, g := range groups {
		for _, t := range subscription.SplitInclude(g.Include) {
			if names[t] && t != g.Name {
				edges[g.Name] = append(edges[g.Name], t)
			}
		}
	}
	const (
		white = iota
		gray
		black
	)
	color := map[string]int{}
	var path []string
	var found string
	var visit func(n string) bool
	visit = func(n string) bool {
		color[n] = gray
		path = append(path, n)
		for _, m := range edges[n] {
			switch color[m] {
			case gray:
				i := slices.Index(path, m)
				found = strings.Join(append(slices.Clone(path[i:]), m), " -> ")
				return true
			case white:
				if visit(m) {
					return true
				}
			}
		}
		path = path[:len(path)-1]
		color[n] = black
		return false
	}
	for _, g := range groups {
		if color[g.Name] == white && visit(g.Name) {
			return found
		}
	}
	return ""
}

func validHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && !strings.ContainsAny(s, " ,\t\n")
}

// validTarget reports whether t is DIRECT, REJECT or one of groups.
func validTarget(t string, groups map[string]bool) bool {
	return subscription.IsBuiltinPolicy(t) || groups[t]
}

// ValidMirrorPrefix reports whether p is usable as a raw-URL mirror prefix (http(s) URL ending with "/").
func ValidMirrorPrefix(p string) bool {
	return validHTTPURL(p) && strings.HasSuffix(p, "/") && !strings.ContainsAny(p, "?#")
}

// ValidateCatalog checks a merged catalog (built-in plus custom overrides). It is used by PUT /admin/rulesets
// and by the tests that lint the embedded catalog.
func ValidateCatalog(c *subscription.Catalog) error {
	errs := FieldErrors{}
	if len(c.Sources) == 0 {
		errs.add("sources", "at least one source is required")
	}
	for _, id := range sortedKeys(c.Sources) {
		s := c.Sources[id]
		f := "sources." + id
		if !idPattern.MatchString(id) {
			errs.add(f, "id must match "+idPattern.String())
		}
		if strings.TrimSpace(s.Name) == "" {
			errs.add(f+".name", "is required")
		}
		if len(s.URLs) == 0 {
			errs.add(f+".urls", "at least one URL template is required")
		}
		for _, k := range sortedKeys(s.URLs) {
			if !slices.Contains(FormatKeys, k) {
				errs.add(f+".urls."+k, "unknown format key")
			}
			if !validHTTPURL(strings.ReplaceAll(s.URLs[k], "{name}", "x")) || !strings.Contains(s.URLs[k], "{name}") {
				errs.add(f+".urls."+k, "must be an http(s) URL template containing {name}")
			}
			if m, ok := s.URLMeta[k]; ok {
				if !slices.Contains(refFields, m.Field) {
					errs.add(f+".url_meta."+k+".field", "must be one of: "+strings.Join(refFields, ", "))
				}
				if !slices.Contains(behaviors, m.Behavior) {
					errs.add(f+".url_meta."+k+".behavior", "must be one of: "+strings.Join(behaviors, ", "))
				}
				if !slices.Contains(fileFormats, m.Format) {
					errs.add(f+".url_meta."+k+".format", "must be one of: "+strings.Join(fileFormats, ", "))
				}
			}
		}
		for _, k := range sortedKeys(s.URLMeta) {
			if _, ok := s.URLs[k]; !ok {
				errs.add(f+".url_meta."+k, "has no matching URL template")
			}
		}
		for _, k := range sortedKeys(s.Aliases) {
			if _, ok := s.URLs[s.Aliases[k]]; !ok {
				errs.add(f+".aliases."+k, "points at a missing URL template")
			}
		}
	}
	catIDs := map[string]bool{}
	for i, cat := range c.Categories {
		f := "categories[" + strconv.Itoa(i) + "]"
		if !idPattern.MatchString(cat.ID) {
			errs.add(f+".id", "must match "+idPattern.String())
		} else if catIDs[cat.ID] {
			errs.add(f+".id", "duplicate category id")
		}
		catIDs[cat.ID] = true
		if strings.TrimSpace(cat.DefaultTarget) == "" {
			errs.add(f+".default_target", "is required")
		}
		if len(cat.Rules) == 0 {
			errs.add(f+".rules", "at least one source is required")
		}
		for _, sid := range sortedKeys(cat.Rules) {
			src, ok := c.Sources[sid]
			if !ok {
				errs.add(f+".rules."+sid, "unknown source")
				continue
			}
			for j, ref := range cat.Rules[sid] {
				rf := f + ".rules." + sid + "[" + strconv.Itoa(j) + "]"
				if len(ref.Formats) == 0 {
					errs.add(rf+".formats", "at least one format key is required")
				}
				for _, k := range ref.Formats {
					_, meta, ok := src.Template(k)
					if !ok {
						errs.add(rf+".formats", fmt.Sprintf("source %s has no %q URL template", sid, k))
						continue
					}
					if ref.Field(meta.Field) == "" {
						errs.add(rf+"."+meta.Field, fmt.Sprintf("is required for format %q", k))
					}
				}
			}
		}
	}
	if len(c.Templates) == 0 {
		errs.add("templates", "at least one template is required")
	}
	tplIDs := map[string]bool{}
	for i, t := range c.Templates {
		f := "templates[" + strconv.Itoa(i) + "]"
		if !idPattern.MatchString(t.ID) {
			errs.add(f+".id", "must match "+idPattern.String())
		} else if tplIDs[t.ID] {
			errs.add(f+".id", "duplicate template id")
		}
		tplIDs[t.ID] = true
		groups := validateGroups(t.Groups, f+".groups", errs)
		for _, cid := range sortedKeys(t.Categories) {
			if !catIDs[cid] {
				errs.add(f+".categories."+cid, "unknown category")
			} else if !validTarget(t.Categories[cid], groups) {
				errs.add(f+".categories."+cid, "target must be DIRECT, REJECT or a group of the template")
			}
		}
		if !validTarget(t.Final, groups) {
			errs.add(f+".final", "must be DIRECT, REJECT or a group of the template")
		}
	}
	mirrorIDs := map[string]bool{}
	for i, m := range c.Mirrors {
		f := "mirrors[" + strconv.Itoa(i) + "]"
		if !idPattern.MatchString(m.ID) {
			errs.add(f+".id", "must match "+idPattern.String())
		} else if mirrorIDs[m.ID] {
			errs.add(f+".id", "duplicate mirror id")
		}
		mirrorIDs[m.ID] = true
		if m.Prefix != "" && !ValidMirrorPrefix(m.Prefix) {
			errs.add(f+".prefix", `must be empty or an http(s) URL ending with "/"`)
		}
	}
	return errs.err()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
