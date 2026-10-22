package rulesets

import (
	"slices"

	"github.com/linusxiong/ggboard/internal/settings"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// RuleClients are the formats that emit catalog rule sets, in display order.
var RuleClients = []string{
	subscription.FormatClashMeta, subscription.FormatStash, subscription.FormatSingbox, subscription.FormatSurge,
	subscription.FormatSurfboard, subscription.FormatLoon, subscription.FormatQuantumultX,
}

// PublicCatalog is the catalog shown to users and guests: no URL templates, aliases or per-ref format keys,
// plus which clients can use each source/category and the admin defaults.
type PublicCatalog struct {
	Sources    map[string]PublicSource `json:"sources"`
	Categories []PublicCategory        `json:"categories"`
	Templates  []subscription.Template `json:"templates"`
	Mirrors    []subscription.Mirror   `json:"mirrors"`
	Clients    []string                `json:"clients"` // formats that receive rule sets
	Defaults   PublicDefaults          `json:"defaults"`
	Editable   bool                    `json:"editable"` // settings.allow_user_rule_profile
}

// PublicSource is a rule-set source without its URL templates.
type PublicSource struct {
	Name        string                     `json:"name"`
	Homepage    string                     `json:"homepage,omitempty"`
	License     string                     `json:"license,omitempty"`
	Order       int                        `json:"order"`
	Description subscription.LocalizedText `json:"description,omitempty"`
	Clients     []string                   `json:"clients"` // formats this source has files for
}

// PublicCategory is a category with the sources that carry it.
type PublicCategory struct {
	ID            string                     `json:"id"`
	Name          subscription.LocalizedText `json:"name,omitempty"`
	DefaultTarget string                     `json:"default_target"`
	Order         int                        `json:"order"`
	Sources       []string                   `json:"sources"` // source ids by order
	Clients       []string                   `json:"clients"` // formats that can get the category (any source)
}

// PublicDefaults are the admin defaults for new profiles.
type PublicDefaults struct {
	Template     string `json:"template"`
	Source       string `json:"source"`
	Mirror       string `json:"mirror"`        // always "default" (= settings.ruleset_mirror)
	MirrorPrefix string `json:"mirror_prefix"` // what "default" resolves to ("" = direct)
}

func refsServe(refs subscription.RuleRefs, family string) bool {
	for _, r := range refs {
		if len(keysFor(family, r)) > 0 {
			return true
		}
	}
	return false
}

// PublicView builds the public catalog from c and the settings.
func PublicView(c *subscription.Catalog, s *settings.Service) PublicCatalog {
	def := DefaultsFrom(s)
	ordered := SourcesByOrder(c)
	out := PublicCatalog{
		Sources:   make(map[string]PublicSource, len(c.Sources)),
		Templates: slices.Clone(c.Templates),
		Mirrors:   slices.Clone(c.Mirrors),
		Clients:   slices.Clone(RuleClients),
		Editable:  s.Bool("allow_user_rule_profile"),
	}
	if out.Templates == nil {
		out.Templates = []subscription.Template{}
	}
	if out.Mirrors == nil {
		out.Mirrors = []subscription.Mirror{}
	}
	tpl, _ := pickTemplate(c, def.Template)
	out.Defaults = PublicDefaults{
		Template: tpl.ID, Source: pickSource(c, def.Source), Mirror: MirrorDefault,
		MirrorPrefix: MirrorPrefix(c, MirrorDefault, def),
	}
	srcClients := map[string][]string{}
	for _, cat := range c.Categories {
		pc := PublicCategory{ID: cat.ID, Name: cat.Name, DefaultTarget: cat.DefaultTarget, Order: cat.Order,
			Sources: []string{}, Clients: []string{}}
		for _, sid := range ordered {
			if refs, ok := cat.Rules[sid]; ok {
				pc.Sources = append(pc.Sources, sid)
				for _, f := range RuleClients {
					if refsServe(refs, f) && !slices.Contains(srcClients[sid], f) {
						srcClients[sid] = append(srcClients[sid], f)
					}
				}
			}
		}
		for _, f := range RuleClients {
			for _, sid := range pc.Sources {
				if refsServe(cat.Rules[sid], f) {
					pc.Clients = append(pc.Clients, f)
					break
				}
			}
		}
		out.Categories = append(out.Categories, pc)
	}
	if out.Categories == nil {
		out.Categories = []PublicCategory{}
	}
	for id, src := range c.Sources {
		clients := []string{}
		for _, f := range RuleClients {
			if slices.Contains(srcClients[id], f) {
				clients = append(clients, f)
			}
		}
		out.Sources[id] = PublicSource{Name: src.Name, Homepage: src.Homepage, License: src.License, Order: src.Order,
			Description: src.Description, Clients: clients}
	}
	return out
}
