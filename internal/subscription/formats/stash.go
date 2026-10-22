package formats

import (
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// stash: the mihomo subset Stash documents (no mieru/anytls; hysteria2 uses auth/up-speed/down-speed; rule
// providers without mrs).
func init() {
	subscription.RegisterFormat(clashFormat{name: subscription.FormatStash, flavor: flavorStash, types: typeSet(
		model.NodeShadowsocks, model.NodeShadowsocksR, model.NodeVmess, model.NodeVless, model.NodeTrojan,
		model.NodeHysteria, model.NodeTuic)})
}
