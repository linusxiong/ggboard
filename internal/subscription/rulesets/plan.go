package rulesets

import (
	"sort"
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/subscription"
)

// familyKeys lists, per rule family, the key sets tried for a RuleRef: the first set with at least one key in
// the ref's formats wins, and every present key of that set is used (docs/RULESETS.md, step 3).
var familyKeys = map[string][][]string{
	subscription.FormatClashMeta:    {{"clash_mrs_domain", "clash_mrs_ipcidr"}, {"clash", "clash_domain", "clash_ipcidr"}},
	subscription.FormatStash:        {{"clash", "clash_domain", "clash_ipcidr"}},
	subscription.FormatSingbox:      {{"singbox", "singbox_ipcidr"}, {"singbox_source", "singbox_source_ipcidr"}},
	subscription.FormatSurge:        {{"surge", "surge_domain"}},
	subscription.FormatSurfboard:    {{"surge", "surge_domain"}},
	subscription.FormatShadowrocket: {{"shadowrocket", "shadowrocket_domain"}},
	subscription.FormatLoon:         {{"loon", "loon_domain"}},
	subscription.FormatQuantumultX:  {{"quantumultx"}},
}

// preferredSource is tried before the profile's source (mrs files are the fastest for mihomo).
var preferredSource = map[string]string{subscription.FormatClashMeta: "metacubex"}

// noResolveExempt lists categories whose IP sets are matched with DNS resolution (local targets).
var noResolveExempt = map[string]bool{"lan": true, "cn": true}

// HasFamily reports whether family consumes catalog rule sets.
func HasFamily(family string) bool { _, ok := familyKeys[family]; return ok }

// keysFor returns the format keys of ref used by family.
func keysFor(family string, ref subscription.RuleRef) []string {
	for _, set := range familyKeys[family] {
		var keys []string
		for _, k := range set {
			if ref.HasFormat(k) {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			return keys
		}
	}
	return nil
}

// sourceFor picks the source serving category cat for family: the preferred source of the family, then the
// profile's source, then every other source by order. The first source with a usable RuleRef wins.
func sourceFor(c *subscription.Catalog, cat subscription.Category, family, profileSource string, ordered []string) string {
	candidates := make([]string, 0, len(ordered)+2)
	if s := preferredSource[family]; s != "" {
		candidates = append(candidates, s)
	}
	candidates = append(candidates, profileSource)
	candidates = append(candidates, ordered...)
	for _, sid := range candidates {
		if _, ok := c.Sources[sid]; !ok {
			continue
		}
		for _, ref := range cat.Rules[sid] {
			if len(keysFor(family, ref)) > 0 {
				return sid
			}
		}
	}
	return ""
}

var behaviorSuffix = map[string]string{
	subscription.BehaviorDomain:    "domain",
	subscription.BehaviorIPCIDR:    "ip",
	subscription.BehaviorClassical: "classical",
}

// IsMirrorable reports whether a URL is served by GitHub and may be prefixed with a raw-URL mirror.
func IsMirrorable(u string) bool {
	return strings.HasPrefix(u, "https://raw.githubusercontent.com/") || strings.HasPrefix(u, "https://github.com/") ||
		strings.HasPrefix(u, "https://gist.githubusercontent.com/")
}

// WithMirror prefixes a GitHub URL with mirror (other URLs are returned unchanged).
func WithMirror(mirror, u string) string {
	if mirror == "" || !IsMirrorable(u) {
		return u
	}
	return mirror + u
}

// geoIPCN is the sing-box rule set used for GEOIP,CN,DIRECT (sing-box >= 1.12 has no GeoIP database).
const geoIPCNSource, geoIPCNKey, geoIPCNName = "metacubex", "singbox_ipcidr", "cn"

// BuildPlan turns an effective profile into the rule plan of one rule family (a format name; "" or a family
// without catalog keys yields a plan with only GEOIP,CN,DIRECT and the final policy). mirror is the resolved
// prefix (MirrorPrefix).
func BuildPlan(c *subscription.Catalog, p subscription.RuleProfile, family, mirror string) *subscription.RulePlan {
	groups := map[string]bool{}
	for _, g := range p.Groups {
		groups[g.Name] = true
	}
	final := p.Final
	if !validTarget(final, groups) {
		final = fallbackFinal(groups)
	}
	plan := &subscription.RulePlan{GeoIPCNDirect: p.GeoIPCNDirect, Final: final, MirrorPrefix: mirror}
	if !HasFamily(family) {
		return plan
	}
	cats := make([]subscription.Category, 0, len(p.Categories))
	for id := range p.Categories {
		if cat, ok := c.Category(id); ok {
			cats = append(cats, cat)
		}
	}
	sort.SliceStable(cats, func(i, j int) bool {
		if cats[i].Order != cats[j].Order {
			return cats[i].Order < cats[j].Order
		}
		return cats[i].ID < cats[j].ID
	})
	ordered := SourcesByOrder(c)
	used := map[string]bool{}
	urls := map[string]bool{}
	for _, cat := range cats {
		target := p.Categories[cat.ID]
		if !validTarget(target, groups) {
			target = final
		}
		sid := sourceFor(c, cat, family, p.Source, ordered)
		if sid == "" {
			continue
		}
		src := c.Sources[sid]
		refs := cat.Rules[sid]
		var sets []subscription.RuleSet
		for i, ref := range refs {
			for _, key := range keysFor(family, ref) {
				tpl, meta, ok := src.Template(key)
				if !ok {
					continue
				}
				val := ref.Field(meta.Field)
				if val == "" {
					continue
				}
				url := WithMirror(mirror, strings.ReplaceAll(tpl, "{name}", val))
				if urls[url] {
					continue
				}
				urls[url] = true
				tag := cat.ID
				if len(refs) > 1 {
					tag += "-" + strconv.Itoa(i+1)
				}
				tag += "-" + behaviorSuffix[meta.Behavior]
				base := tag
				for n := 2; used[tag]; n++ {
					tag = base + "-" + strconv.Itoa(n)
				}
				used[tag] = true
				sets = append(sets, subscription.RuleSet{
					Tag: tag, Category: cat.ID, Source: sid, Key: key, Behavior: meta.Behavior, Format: meta.Format,
					URL: url, Target: target,
					NoResolve: meta.Behavior == subscription.BehaviorIPCIDR && !noResolveExempt[cat.ID],
				})
			}
		}
		sort.SliceStable(sets, func(i, j int) bool {
			return sets[i].Behavior != subscription.BehaviorIPCIDR && sets[j].Behavior == subscription.BehaviorIPCIDR
		})
		plan.Sets = append(plan.Sets, sets...)
	}
	if p.GeoIPCNDirect && family == subscription.FormatSingbox {
		if src, ok := c.Sources[geoIPCNSource]; ok {
			if tpl, meta, ok := src.Template(geoIPCNKey); ok {
				url := WithMirror(mirror, strings.ReplaceAll(tpl, "{name}", geoIPCNName))
				if !urls[url] {
					plan.GeoIPCN = &subscription.RuleSet{
						Tag: "geoip-cn", Category: "geoip-cn", Source: geoIPCNSource, Key: geoIPCNKey,
						Behavior: meta.Behavior, Format: meta.Format, URL: url, Target: subscription.PolicyDirect,
					}
				}
			}
		}
	}
	return plan
}
