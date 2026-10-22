package formats

import (
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// surfboard: the Surge subset Surfboard understands (ss with AEAD ciphers, vmess, trojan), same layout.
func init() {
	subscription.RegisterFormat(surgeFormat{name: subscription.FormatSurfboard, surfboard: true, types: typeSet(
		model.NodeShadowsocks, model.NodeVmess, model.NodeTrojan)})
}
