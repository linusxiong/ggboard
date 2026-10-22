package subscription

import "strings"

// Rule-set behaviors (mihomo / sing-box vocabulary).
const (
	BehaviorDomain    = "domain"
	BehaviorIPCIDR    = "ipcidr"
	BehaviorClassical = "classical"
)

// RuleSet is one remote rule-set file a client downloads, already resolved for one output format: the source
// and format key were chosen, the URL expanded (mirror applied) and the target policy resolved.
type RuleSet struct {
	Tag       string // "<category>[-<n>]-<domain|ip|classical>", unique within a plan
	Category  string // category id
	Source    string // source id
	Key       string // catalog format key (clash_mrs_domain, singbox, surge_domain, ...)
	Behavior  string // domain | ipcidr | classical
	Format    string // yaml | text | mrs | binary | source
	URL       string
	Target    string // group name, DIRECT or REJECT
	NoResolve bool   // IP set outside lan/cn: clients should not resolve domains to match it
}

// Ext returns the file extension of the rule-set URL ("mrs", "yaml", "txt", "list", "srs", "json"), or the
// format name when the URL has none.
func (r RuleSet) Ext() string {
	u := r.URL
	if i := strings.IndexByte(u, '?'); i >= 0 {
		u = u[:i]
	}
	slash := strings.LastIndexByte(u, '/')
	dot := strings.LastIndexByte(u, '.')
	if dot > slash && dot < len(u)-1 {
		return u[dot+1:]
	}
	return r.Format
}

// RulePlan is the routing part of a subscription for one format: rule sets in emission order (categories by
// order; within a category domain/classical sets before IP sets), the GEOIP,CN,DIRECT switch and the final
// policy. GeoIPCN is set for clients without a GeoIP database (sing-box) when GeoIPCNDirect is on.
type RulePlan struct {
	Sets          []RuleSet
	GeoIPCNDirect bool
	GeoIPCN       *RuleSet
	Final         string
	MirrorPrefix  string // applied to every GitHub URL ("" = direct)
}

// CategoryGroups returns the sets grouped by category in plan order (sing-box emits one rule per category).
func (p *RulePlan) CategoryGroups() [][]RuleSet {
	if p == nil {
		return nil
	}
	var out [][]RuleSet
	for i, s := range p.Sets {
		if i == 0 || p.Sets[i-1].Category != s.Category || p.Sets[i-1].Target != s.Target {
			out = append(out, nil)
		}
		out[len(out)-1] = append(out[len(out)-1], s)
	}
	return out
}
