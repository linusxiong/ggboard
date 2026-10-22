// Package subscription defines subscription output formats (plugins registered from init in
// subscription/formats), the render context, the user rule profile, client detection, the node query and the
// proxy/group model shared by every format (docs/SUBSCRIPTION.md).
package subscription

import (
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/plugin"
	"github.com/linusxiong/ggboard/internal/settings"
)

// Format names.
const (
	FormatClashMeta    = "clash-meta"
	FormatStash        = "stash"
	FormatClash        = "clash"
	FormatSingbox      = "singbox"
	FormatSurge        = "surge"
	FormatSurfboard    = "surfboard"
	FormatQuantumultX  = "quantumultx"
	FormatShadowrocket = "shadowrocket"
	FormatLoon         = "loon"
	FormatV2ray        = "v2ray"
	FormatSSR          = "ssr"
)

// Content types of subscription bodies.
const (
	ContentTypeYAML  = "text/yaml; charset=utf-8"
	ContentTypeJSON  = "application/json; charset=utf-8"
	ContentTypePlain = "text/plain; charset=utf-8"
)

// Format renders a subscription for one client family.
type Format interface {
	Name() string
	Supports(nodeType string) bool
	Render(ctx *RenderContext) (body []byte, contentType string, err error)
}

// ProxyFilter is implemented by formats that support a node type only in some configurations (for example
// sing-box has no xhttp transport, Surge has no VLESS or chacha20 shadowsocks-2022). Proxies it rejects are
// dropped before groups are resolved.
type ProxyFilter interface {
	SupportsProxy(p *Proxy) bool
}

// RuleConsumer is implemented by formats that emit rule sets; RuleFamily is the format name used to pick
// catalog format keys (docs/RULESETS.md "How each format consumes the catalog"). Formats without it get no
// rule sets (only GEOIP,CN,DIRECT and the final policy where they support rules at all).
type RuleConsumer interface {
	RuleFamily() string
}

// RenderContext is everything a format needs. Proxies are already filtered to what the user may see and to
// what the format supports, carry the user's credentials and have unique names; Groups are resolved against
// them. Rules is nil when the effective profile disables rules for this format.
type RenderContext struct {
	User         *model.User
	Nodes        []*model.Node // the nodes behind Proxies (info pseudo nodes have none)
	Proxies      []Proxy
	Groups       []ResolvedGroup
	Rules        *RulePlan
	Profile      RuleProfile // effective profile
	Catalog      *Catalog
	Settings     *settings.Service
	SubscribeURL string // absolute URL of this subscription (managed-config headers)
	RequestHost  string // host the client used (forced DIRECT so refreshes never loop through a proxy)
	AppName      string
	Locale       string // settings.default_locale
	Mirror       string // resolved raw-URL mirror prefix ("" = none)
	UserAgent    string
	ListOnly     bool           // ?list=1: line-based formats emit proxy lines only (Surge policy-path, QX server_remote)
	UpdateHours  int            // settings.subscribe_update_interval
	Now          time.Time      // render time
	Location     *time.Location // time zone for dates shown to users
}

var formats = plugin.NewRegistry[Format]("subscription format")

// RegisterFormat adds a format (call from init; panics on duplicates).
func RegisterFormat(f Format) { formats.Register(f.Name(), f) }

// GetFormat returns the format registered under name.
func GetFormat(name string) (Format, bool) { return formats.Get(name) }

// FormatNames returns every registered format name sorted.
func FormatNames() []string { return formats.Names() }
