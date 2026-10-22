package subscription

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// RuleProfile is a user's rule/policy choice (stored in rule_profiles.data).
type RuleProfile struct {
	Template      string          `json:"template"`        // catalog template id; "none" = no rules
	Source        string          `json:"source"`          // preferred rule-set source id
	Categories    CategoryTargets `json:"categories"`      // category id -> policy group name
	Groups        []PolicyGroup   `json:"groups"`          // policy groups in order ("Proxy" always present)
	Final         string          `json:"final"`           // policy for unmatched traffic
	GeoIPCNDirect bool            `json:"geoip_cn_direct"` // append GEOIP,CN,DIRECT
	Mirror        string          `json:"mirror"`          // "" direct | "default" settings.ruleset_mirror | custom prefix
	DNS           string          `json:"dns"`             // default | fakeip | none
	Nodes         NodeOps         `json:"nodes"`           // node operations (filters, renames, emoji, sort, dedupe, limit)
}

// PolicyGroup is one proxy group shown in the client.
type PolicyGroup struct {
	Name     string `json:"name"`
	Type     string `json:"type"`    // select | url-test | fallback | load-balance
	Include  string `json:"include"` // "auto, *", regex/glob on node names, other group names
	URL      string `json:"url,omitempty"`
	Interval int    `json:"interval,omitempty"`
}

// CategoryTargets maps category ids to targets. In JSON a value may also be false or null, meaning disabled
// (dropped on decode).
type CategoryTargets map[string]string

// UnmarshalJSON accepts string, false and null values.
func (c *CategoryTargets) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		*c = nil
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	out := make(CategoryTargets, len(raw))
	for k, v := range raw {
		switch x := v.(type) {
		case string:
			if x != "" {
				out[k] = x
			}
		case nil, bool:
			if x == true {
				return fmt.Errorf("category %q: true is not a target (use a policy name)", k)
			}
		default:
			return fmt.Errorf("category %q: target must be a string", k)
		}
	}
	*c = out
	return nil
}

// DNS modes.
const (
	DNSDefault = "default"
	DNSFakeIP  = "fakeip"
	DNSNone    = "none"
)
