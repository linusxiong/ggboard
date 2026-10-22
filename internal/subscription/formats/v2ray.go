package formats

import (
	"github.com/linusxiong/ggboard/internal/subscription"
)

// v2ray: base64 of share links for every node type (v2rayN, v2rayNG, NekoRay, PassWall, Karing, ...).
// Clients do not take rules from this format.
type v2rayFormat struct{}

func (v2rayFormat) Name() string           { return subscription.FormatV2ray }
func (v2rayFormat) Supports(t string) bool { return allTypes[t] }

func (v2rayFormat) SupportsProxy(p *subscription.Proxy) bool {
	return p.SS == nil || ssPlugin(p).Kind != "unknown"
}

func (v2rayFormat) Render(ctx *subscription.RenderContext) ([]byte, string, error) {
	lines := make([]string, 0, len(ctx.Proxies))
	for i := range ctx.Proxies {
		if l := shareLink(&ctx.Proxies[i]); l != "" {
			lines = append(lines, l)
		}
	}
	return linksBody(lines), subscription.ContentTypePlain, nil
}

func init() { subscription.RegisterFormat(v2rayFormat{}) }
