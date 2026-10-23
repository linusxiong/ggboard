package rulesets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/linusxiong/ggboard/internal/settings"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// MirrorDefault is the RuleProfile.Mirror value meaning "use settings.ruleset_mirror".
const MirrorDefault = "default"

// Defaults are the admin's profile defaults (settings ruleset_default_template, ruleset_default_source and
// ruleset_mirror).
type Defaults struct {
	Template string
	Source   string
	Mirror   string // settings.ruleset_mirror: prefix URL, mirror id or ""
}

// DefaultsFrom reads Defaults from settings.
func DefaultsFrom(s *settings.Service) Defaults {
	return Defaults{
		Template: s.String("ruleset_default_template"),
		Source:   s.String("ruleset_default_source"),
		Mirror:   s.String("ruleset_mirror"),
	}
}

// Overrides are the per-request query overrides of docs/SUBSCRIPTION.md (profile, source, cats, final, mirror).
type Overrides struct {
	Template string   // profile=<template>: template defaults replace the stored categories, groups and final
	Source   string   // source=<id>
	Cats     []string // cats=a,b: enable only these categories (stored/template targets, else default_target)
	HasCats  bool     // cats was present (even empty: no categories)
	Final    string   // final=<policy>
	Mirror   string   // mirror=0 (direct) | 1 (profile mirror, else settings) | <mirror id>
}

// IsZero reports whether no override is set.
func (o Overrides) IsZero() bool {
	return o.Template == "" && o.Source == "" && !o.HasCats && o.Final == "" && o.Mirror == ""
}

// pickTemplate returns the template id, falling back to the defaults, "standard" and the first template.
func pickTemplate(c *subscription.Catalog, ids ...string) (subscription.Template, bool) {
	for _, id := range append(ids, "standard") {
		if t, ok := c.Template(id); ok && id != "" {
			return t, true
		}
	}
	if len(c.Templates) > 0 {
		return c.Templates[0], true
	}
	return subscription.Template{}, false
}

// pickSource returns the first id that exists, else the source with the lowest order.
func pickSource(c *subscription.Catalog, ids ...string) string {
	for _, id := range ids {
		if _, ok := c.Sources[id]; ok && id != "" {
			return id
		}
	}
	if ordered := SourcesByOrder(c); len(ordered) > 0 {
		return ordered[0]
	}
	return ""
}

// SourcesByOrder returns source ids sorted by order, then id.
func SourcesByOrder(c *subscription.Catalog) []string {
	ids := sortedKeys(c.Sources)
	sort.SliceStable(ids, func(i, j int) bool { return c.Sources[ids[i]].Order < c.Sources[ids[j]].Order })
	return ids
}

// FromTemplate returns the profile a template defines (source and mirror from def, dns "default").
func FromTemplate(c *subscription.Catalog, t subscription.Template, def Defaults) subscription.RuleProfile {
	return subscription.RuleProfile{
		Template:      t.ID,
		Source:        pickSource(c, def.Source),
		Categories:    maps.Clone(subscription.CategoryTargets(t.Categories)),
		Groups:        slices.Clone(t.Groups),
		Final:         t.Final,
		GeoIPCNDirect: t.GeoIPCNDirect,
		Mirror:        MirrorDefault,
		DNS:           subscription.DNSDefault,
		Nodes:         subscription.NodeOps{}.Normalized(),
	}
}

// DefaultProfile is the profile of a user without a stored one.
func DefaultProfile(c *subscription.Catalog, def Defaults) subscription.RuleProfile {
	t, _ := pickTemplate(c, def.Template)
	return FromTemplate(c, t, def)
}

// DecodeProfile decodes a stored or submitted profile and fills omitted parts from its template: missing
// categories, groups, final, geoip_cn_direct, source, mirror and dns take the template / default values, so
// the result is complete. Unknown JSON fields are rejected.
func DecodeProfile(c *subscription.Catalog, raw []byte, def Defaults) (subscription.RuleProfile, error) {
	var p subscription.RuleProfile
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, FieldErrors{"profile": {"invalid JSON: " + err.Error()}}
	}
	var present map[string]json.RawMessage
	_ = json.Unmarshal(raw, &present)
	tplID := p.Template
	if tplID == "" {
		tplID = def.Template
	}
	t, ok := c.Template(tplID)
	if !ok {
		if _, given := present["template"]; given && p.Template != "" {
			return p, FieldErrors{"template": {"unknown template"}}
		}
		t, _ = pickTemplate(c, def.Template)
	}
	base := FromTemplate(c, t, def)
	p.Template = t.ID
	if _, ok := present["categories"]; !ok {
		p.Categories = base.Categories
	} else if p.Categories == nil {
		p.Categories = subscription.CategoryTargets{}
	}
	if len(p.Groups) == 0 {
		p.Groups = base.Groups
	}
	if p.Final == "" {
		p.Final = base.Final
	}
	if _, ok := present["geoip_cn_direct"]; !ok {
		p.GeoIPCNDirect = base.GeoIPCNDirect
	}
	if p.Source == "" {
		p.Source = base.Source
	}
	if _, ok := present["mirror"]; !ok {
		p.Mirror = base.Mirror
	}
	if p.DNS == "" {
		p.DNS = base.DNS
	}
	p.Nodes = p.Nodes.Normalized()
	return p, nil
}

// ValidateProfile checks a complete profile strictly: at most MaxProfileBytes as stored, known template, source
// and categories, valid groups, targets that are DIRECT, REJECT or a defined group, a valid final policy, mirror
// and dns mode. The size is checked first: an oversized profile gets only that error and nothing is compiled.
func ValidateProfile(c *subscription.Catalog, p subscription.RuleProfile) error {
	if b, err := json.Marshal(p); err != nil {
		return err
	} else if len(b) > MaxProfileBytes {
		return FieldErrors{"profile": {fmt.Sprintf("must be at most %d KiB as stored", MaxProfileBytes>>10)}}
	}
	errs := FieldErrors{}
	if _, ok := c.Template(p.Template); !ok {
		errs.add("template", "unknown template")
	}
	if _, ok := c.Sources[p.Source]; !ok {
		errs.add("source", "unknown source")
	}
	groups := validateGroups(p.Groups, "groups", errs)
	for _, id := range sortedKeys(p.Categories) {
		if _, ok := c.Category(id); !ok {
			errs.add("categories."+id, "unknown category")
		} else if !validTarget(p.Categories[id], groups) {
			errs.add("categories."+id, "target must be DIRECT, REJECT or one of the groups")
		}
	}
	if !validTarget(p.Final, groups) {
		errs.add("final", "must be DIRECT, REJECT or one of the groups")
	}
	if msg := validMirror(c, p.Mirror); msg != "" {
		errs.add("mirror", msg)
	}
	if !slices.Contains(dnsModes, p.DNS) {
		errs.add("dns", "must be one of: "+strings.Join(dnsModes, ", "))
	}
	validateNodeOps(p.Nodes, errs)
	return errs.err()
}

func validMirror(c *subscription.Catalog, m string) string {
	switch {
	case m == "" || m == MirrorDefault:
		return ""
	case strings.Contains(m, "://"):
		// https only (docs/SUBSCRIPTION.md): rule sets fetched over plain http can be rewritten in transit.
		if !strings.HasPrefix(m, "https://") || len(m) > MaxURLLen || !ValidMirrorPrefix(m) {
			return `must be an https URL ending with "/"`
		}
		return ""
	}
	if _, ok := c.Mirror(m); !ok {
		return `must be "", "default", a mirror id or an http(s) prefix`
	}
	return ""
}

// ResolveProfile computes the effective profile of a request: the stored profile (or the default template
// when stored is nil), with ov applied. Stale references in a stored profile (a template, source or category
// the admin removed) fall back to the defaults silently so subscriptions keep working; invalid overrides are
// reported as FieldErrors (400). Category targets that are not a defined group are kept: the rule plan routes
// them to the final policy.
func ResolveProfile(c *subscription.Catalog, stored *subscription.RuleProfile, def Defaults, ov Overrides) (subscription.RuleProfile, error) {
	errs := FieldErrors{}
	var p subscription.RuleProfile
	switch {
	case ov.Template != "":
		t, ok := c.Template(ov.Template)
		if !ok {
			errs.add("profile", "unknown template")
			return p, errs
		}
		p = FromTemplate(c, t, def)
		if stored != nil {
			p.Source, p.Mirror, p.DNS, p.Nodes = stored.Source, stored.Mirror, stored.DNS, stored.Nodes
		}
	case stored != nil:
		p = *stored
		p.Categories = maps.Clone(stored.Categories)
		p.Groups = slices.Clone(stored.Groups)
		t, ok := c.Template(p.Template)
		if !ok {
			t, _ = pickTemplate(c, def.Template)
			p.Template = t.ID
		}
		if len(p.Groups) == 0 {
			p.Groups = slices.Clone(t.Groups)
		}
		if p.Categories == nil {
			p.Categories = maps.Clone(subscription.CategoryTargets(t.Categories))
		}
		if p.Final == "" {
			p.Final = t.Final
		}
	default:
		p = DefaultProfile(c, def)
	}
	p.Source = pickSource(c, p.Source, def.Source)
	if p.DNS == "" || !slices.Contains(dnsModes, p.DNS) {
		p.DNS = subscription.DNSDefault
	}
	if validMirror(c, p.Mirror) != "" {
		p.Mirror = MirrorDefault
	}
	for id := range p.Categories {
		if _, ok := c.Category(id); !ok {
			delete(p.Categories, id)
		}
	}

	if ov.Source != "" {
		if _, ok := c.Sources[ov.Source]; ok {
			p.Source = ov.Source
		} else {
			errs.add("source", "unknown source")
		}
	}
	if ov.HasCats {
		tpl, _ := c.Template(p.Template)
		cats := subscription.CategoryTargets{}
		for _, id := range ov.Cats {
			cat, ok := c.Category(id)
			if !ok {
				errs.add("cats", "unknown category "+id)
				continue
			}
			switch {
			case p.Categories[id] != "":
				cats[id] = p.Categories[id]
			case tpl.Categories[id] != "":
				cats[id] = tpl.Categories[id]
			default:
				cats[id] = cat.DefaultTarget
			}
		}
		p.Categories = cats
	}
	groups := map[string]bool{}
	for _, g := range p.Groups {
		groups[g.Name] = true
	}
	if ov.Final != "" {
		if validTarget(ov.Final, groups) {
			p.Final = ov.Final
		} else {
			errs.add("final", "must be DIRECT, REJECT or one of the groups")
		}
	}
	if !validTarget(p.Final, groups) {
		p.Final = fallbackFinal(groups)
	}
	switch ov.Mirror {
	case "":
	case "0", "false":
		p.Mirror = ""
	case "1", "true":
		if p.Mirror == "" {
			p.Mirror = MirrorDefault
		}
	default:
		if _, ok := c.Mirror(ov.Mirror); ok {
			p.Mirror = ov.Mirror
		} else {
			errs.add("mirror", `must be 0, 1 or a mirror id`)
		}
	}
	return p, errs.err()
}

func fallbackFinal(groups map[string]bool) string {
	if groups[subscription.GroupProxy] {
		return subscription.GroupProxy
	}
	return subscription.PolicyDirect
}

// MirrorPrefix resolves a profile mirror value to a URL prefix: "" -> none, "default" -> settings
// ruleset_mirror (itself a prefix or a mirror id), a mirror id -> its prefix, a URL -> itself.
func MirrorPrefix(c *subscription.Catalog, profileMirror string, def Defaults) string {
	m := profileMirror
	if m == MirrorDefault {
		m = def.Mirror
		if m == MirrorDefault {
			return ""
		}
	}
	switch {
	case m == "":
		return ""
	case strings.Contains(m, "://"):
		if !strings.HasSuffix(m, "/") {
			m += "/"
		}
		if ValidMirrorPrefix(m) {
			return m
		}
		return ""
	}
	if mi, ok := c.Mirror(m); ok {
		return mi.Prefix
	}
	return ""
}
