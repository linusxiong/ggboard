// Package subscription defines subscription output formats (plugins registered from init in
// subscription/formats/<client>), the render context, the user rule profile and client detection
// (docs/SUBSCRIPTION.md).
package subscription

import (
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

// Format renders a subscription for one client family.
type Format interface {
	Name() string
	Supports(nodeType string) bool
	Render(ctx *RenderContext) (body []byte, contentType string, err error)
}

// RenderContext is everything a format needs. Nodes are already filtered to what the user may see (and to
// Format.Supports), sorted, with Credentials derived via model.UserCredentials.
type RenderContext struct {
	User         *model.User
	Nodes        []*model.Node
	Profile      RuleProfile
	Catalog      *Catalog
	Settings     *settings.Service
	SubscribeURL string
	AppName      string
	Locale       string // settings.default_locale
	Mirror       string // resolved raw-URL mirror prefix ("" = none)
	UserAgent    string
}

var formats = plugin.NewRegistry[Format]("subscription format")

// RegisterFormat adds a format (call from init; panics on duplicates).
func RegisterFormat(f Format) { formats.Register(f.Name(), f) }

// GetFormat returns the format registered under name.
func GetFormat(name string) (Format, bool) { return formats.Get(name) }

// FormatNames returns every registered format name sorted.
func FormatNames() []string { return formats.Names() }
