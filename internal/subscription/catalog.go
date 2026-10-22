package subscription

// Catalog is the rule-set catalog (internal/subscription/rulesets/catalog.json merged with the admin's
// rulesets_custom setting). The loader lives in internal/subscription/rulesets.
type Catalog struct {
	Sources    map[string]Source `json:"sources"`
	Categories []Category        `json:"categories"`
	Templates  []Template        `json:"templates"`
}

// LocalizedText maps locales (zh-CN, en) to text.
type LocalizedText map[string]string

// Get returns the text for locale, falling back to en and then any value.
func (l LocalizedText) Get(locale string) string {
	if s, ok := l[locale]; ok {
		return s
	}
	if s, ok := l["en"]; ok {
		return s
	}
	for _, s := range l {
		return s
	}
	return ""
}

// Source is a rule-set provider with URL templates per client kind ({name} placeholder).
type Source struct {
	Name     string            `json:"name"`
	Homepage string            `json:"homepage,omitempty"`
	URLs     map[string]string `json:"urls"`
}

// Category is a routing category (ads, openai, cn, ...) with per-source rule references.
type Category struct {
	ID            string             `json:"id"`
	Name          LocalizedText      `json:"name,omitempty"`
	DefaultTarget string             `json:"default_target"`
	Order         int                `json:"order"`
	Rules         map[string]RuleRef `json:"rules"`
}

// RuleRef points at a rule list inside a source.
type RuleRef struct {
	Name     string `json:"name,omitempty"`
	Behavior string `json:"behavior,omitempty"` // classical | domain | ipcidr
	GeoSite  string `json:"geosite,omitempty"`
	GeoIP    string `json:"geoip,omitempty"`
	Split    bool   `json:"split,omitempty"` // list is split into _Domain/_IP parts (blackmatrix7)
}

// Template is a preset RuleProfile.
type Template struct {
	ID            string            `json:"id"`
	Name          LocalizedText     `json:"name,omitempty"`
	Categories    map[string]string `json:"categories"`
	Groups        []PolicyGroup     `json:"groups"`
	Final         string            `json:"final"`
	GeoIPCNDirect bool              `json:"geoip_cn_direct"`
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
