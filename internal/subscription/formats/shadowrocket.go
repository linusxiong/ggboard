package formats

import (
	"strconv"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// shadowrocket: base64 share links preceded by STATUS= (traffic and expiry, shown above the node list) and
// REMARKS= (subscription name). Account information replaces the info pseudo nodes; rules are configured
// in the app, not by the subscription. Shadowrocket has no mieru.
type shadowrocketFormat struct{}

func (shadowrocketFormat) Name() string { return subscription.FormatShadowrocket }
func (shadowrocketFormat) Supports(t string) bool {
	return t != model.NodeMieru && allTypes[t]
}

func (shadowrocketFormat) SupportsProxy(p *subscription.Proxy) bool {
	return !p.Info && (p.SS == nil || ssPlugin(p).Kind != "unknown")
}

func gb2(b int64) string { return strconv.FormatFloat(float64(b)/(1<<30), 'f', 2, 64) }

func (shadowrocketFormat) Render(ctx *subscription.RenderContext) ([]byte, string, error) {
	u := ctx.User
	lines := make([]string, 0, len(ctx.Proxies)+2)
	expires := "N/A"
	if u.ExpiredAt != nil {
		loc := ctx.Location
		if loc == nil {
			loc = ctx.Now.Location()
		}
		expires = u.ExpiredAt.In(loc).Format("2006-01-02")
	}
	lines = append(lines,
		"STATUS=🚀↑:"+gb2(u.U)+"GB,↓:"+gb2(u.D)+"GB,TOT:"+gb2(u.TransferEnable)+"GB💡Expires:"+expires,
		"REMARKS="+strings.ReplaceAll(ctx.AppName, "\n", " "))
	for i := range ctx.Proxies {
		if l := shareLink(&ctx.Proxies[i]); l != "" {
			lines = append(lines, l)
		}
	}
	return linksBody(lines), subscription.ContentTypePlain, nil
}

func init() { subscription.RegisterFormat(shadowrocketFormat{}) }
