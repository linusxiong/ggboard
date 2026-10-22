package formats

import (
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// ssr: base64 of ssr:// links for ShadowsocksR clients (SSR nodes plus plain shadowsocks nodes expressed as
// protocol origin / obfs plain).
type ssrFormat struct{}

func (ssrFormat) Name() string { return subscription.FormatSSR }
func (ssrFormat) Supports(t string) bool {
	return t == model.NodeShadowsocksR || t == model.NodeShadowsocks
}

func (ssrFormat) SupportsProxy(p *subscription.Proxy) bool {
	return p.SS == nil || (!p.SS.Is2022() && ssPlugin(p).Kind == "")
}

func (ssrFormat) Render(ctx *subscription.RenderContext) ([]byte, string, error) {
	lines := make([]string, 0, len(ctx.Proxies))
	for i := range ctx.Proxies {
		lines = append(lines, ssrLink(&ctx.Proxies[i], ctx.AppName))
	}
	return linksBody(lines), subscription.ContentTypePlain, nil
}

func init() { subscription.RegisterFormat(ssrFormat{}) }
