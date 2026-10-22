package formats

import (
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// clash: the classic Clash core (ss without 2022 ciphers, ssr, vmess, trojan); no rule providers, only
// GEOIP,CN,DIRECT and MATCH (documented limitation).
func init() {
	subscription.RegisterFormat(clashFormat{name: subscription.FormatClash, flavor: flavorClassic, types: typeSet(
		model.NodeShadowsocks, model.NodeShadowsocksR, model.NodeVmess, model.NodeTrojan)})
}
