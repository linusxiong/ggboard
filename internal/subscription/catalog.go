package subscription

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Catalog is the rule-set catalog (internal/subscription/rulesets/catalog.json merged with the admin's
// rulesets_custom setting). The loader lives in internal/subscription/rulesets; docs/RULESETS.md is the schema.
type Catalog struct {
	Sources    map[string]Source `json:"sources,omitempty"`
	Categories []Category        `json:"categories,omitempty"`
	Templates  []Template        `json:"templates,omitempty"`
	Mirrors    []Mirror          `json:"mirrors,omitempty"`
	Meta       *CatalogMeta      `json:"meta,omitempty"`
}

// CatalogMeta describes the catalog file.
type CatalogMeta struct {
	GeneratedAt string `json:"generated_at,omitempty"`
	Version     int    `json:"version"`
}

// LocalizedText maps locales (zh-CN, en) to text.
type LocalizedText map[string]string

// Get returns the text for locale, falling back to en and then any value (deterministically: the smallest key).
func (l LocalizedText) Get(locale string) string {
	if s, ok := l[locale]; ok {
		return s
	}
	if s, ok := l["en"]; ok {
		return s
	}
	best, found := "", false
	var out string
	for k, s := range l {
		if !found || k < best {
			best, out, found = k, s, true
		}
	}
	return out
}

// Source is a rule-set provider with raw-URL templates per format key ({name} placeholder).
type Source struct {
	Name        string             `json:"name"`
	Homepage    string             `json:"homepage,omitempty"`
	License     string             `json:"license,omitempty"`
	Order       int                `json:"order"`
	Description LocalizedText      `json:"description,omitempty"`
	URLs        map[string]string  `json:"urls"`
	URLMeta     map[string]URLMeta `json:"url_meta,omitempty"`
	Aliases     map[string]string  `json:"aliases,omitempty"`
}

// URLMeta describes one URL template of a source: which RuleRef field fills {name}, the rule behavior and the
// file format.
type URLMeta struct {
	Field    string `json:"field"`    // name | geosite | geoip
	Behavior string `json:"behavior"` // domain | ipcidr | classical
	Format   string `json:"format"`   // yaml | text | mrs | binary | source
}

// Template returns the URL template and its metadata for format key k (resolving aliases).
func (s Source) Template(k string) (string, URLMeta, bool) {
	if u, ok := s.URLs[k]; ok {
		return u, s.meta(k), true
	}
	if a, ok := s.Aliases[k]; ok {
		if u, ok := s.URLs[a]; ok {
			return u, s.meta(a), true
		}
	}
	return "", URLMeta{}, false
}

func (s Source) meta(k string) URLMeta {
	if m, ok := s.URLMeta[k]; ok {
		return m
	}
	return URLMeta{Field: "name", Behavior: "classical", Format: "text"}
}

// Category is a routing category (ads, openai, cn, ...) with per-source rule references.
type Category struct {
	ID            string              `json:"id"`
	Name          LocalizedText       `json:"name,omitempty"`
	DefaultTarget string              `json:"default_target"`
	Order         int                 `json:"order"`
	Rules         map[string]RuleRefs `json:"rules"`
}

// RuleRef points at a rule list inside a source.
type RuleRef struct {
	Name     string   `json:"name,omitempty"`
	Behavior string   `json:"behavior,omitempty"` // classical | domain | ipcidr
	GeoSite  string   `json:"geosite,omitempty"`
	GeoIP    string   `json:"geoip,omitempty"`
	Split    bool     `json:"split,omitempty"`   // list is split into _Domain/_IP parts (blackmatrix7)
	Formats  []string `json:"formats,omitempty"` // format keys verified to exist for this ref
}

// Field returns the value of the RuleRef field named by URLMeta.Field.
func (r RuleRef) Field(name string) string {
	switch name {
	case "geosite":
		return r.GeoSite
	case "geoip":
		return r.GeoIP
	default:
		return r.Name
	}
}

// HasFormat reports whether key is listed in Formats.
func (r RuleRef) HasFormat(key string) bool {
	for _, f := range r.Formats {
		if f == key {
			return true
		}
	}
	return false
}

// RuleRefs is one or more RuleRefs. In JSON it is either a single object or an array; a single element is
// written back as an object so the catalog round-trips unchanged.
type RuleRefs []RuleRef

// UnmarshalJSON accepts an object or an array of objects.
func (r *RuleRefs) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '[' {
		var many []RuleRef
		if err := json.Unmarshal(b, &many); err != nil {
			return err
		}
		*r = many
		return nil
	}
	if bytes.Equal(b, []byte("null")) {
		*r = nil
		return nil
	}
	var one RuleRef
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*r = RuleRefs{one}
	return nil
}

// MarshalJSON writes a single ref as an object and several as an array.
func (r RuleRefs) MarshalJSON() ([]byte, error) {
	if len(r) == 1 {
		return json.Marshal(r[0])
	}
	return json.Marshal([]RuleRef(r))
}

// Template is a preset RuleProfile.
type Template struct {
	ID            string            `json:"id"`
	Name          LocalizedText     `json:"name,omitempty"`
	Description   LocalizedText     `json:"description,omitempty"`
	Categories    map[string]string `json:"categories"`
	Groups        []PolicyGroup     `json:"groups"`
	Final         string            `json:"final"`
	GeoIPCNDirect bool              `json:"geoip_cn_direct"`
}

// Mirror is a GitHub raw-URL proxy prefix.
type Mirror struct {
	ID     string        `json:"id"`
	Name   LocalizedText `json:"name,omitempty"`
	Prefix string        `json:"prefix"`
}

// Template returns the template with id.
func (c *Catalog) Template(id string) (Template, bool) {
	if c == nil {
		return Template{}, false
	}
	for _, t := range c.Templates {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

// Category returns the category with id.
func (c *Catalog) Category(id string) (Category, bool) {
	if c == nil {
		return Category{}, false
	}
	for _, cat := range c.Categories {
		if cat.ID == id {
			return cat, true
		}
	}
	return Category{}, false
}

// Source returns the source with id.
func (c *Catalog) Source(id string) (Source, bool) {
	if c == nil {
		return Source{}, false
	}
	s, ok := c.Sources[id]
	return s, ok
}

// Mirror returns the mirror with id.
func (c *Catalog) Mirror(id string) (Mirror, bool) {
	if c == nil {
		return Mirror{}, false
	}
	for _, m := range c.Mirrors {
		if m.ID == id {
			return m, true
		}
	}
	return Mirror{}, false
}

// Built-in policy names every client understands.
const (
	PolicyDirect = "DIRECT"
	PolicyReject = "REJECT"
	GroupProxy   = "Proxy"
)

// IsBuiltinPolicy reports whether name is DIRECT or REJECT.
func IsBuiltinPolicy(name string) bool { return name == PolicyDirect || name == PolicyReject }

// reservedNames may never be used as node or group names (they are built-in policies of some client).
var reservedNames = map[string]bool{
	"direct": true, "reject": true, "reject-drop": true, "reject-tinygif": true, "pass": true, "global": true,
	"match": true, "final": true, "proxy-direct": true, "block": true, "dns-out": true,
}

// IsReservedName reports whether name collides (case-insensitively) with a built-in policy of any client.
func IsReservedName(name string) bool { return reservedNames[strings.ToLower(strings.TrimSpace(name))] }
