package formats

import "github.com/linusxiong/ggboard/internal/subscription"

// clash-meta: mihomo YAML with every node type, rule providers (mrs preferred), DNS and geox mirrors.
func init() {
	subscription.RegisterFormat(clashFormat{name: subscription.FormatClashMeta, flavor: flavorMeta, types: allTypes})
}
