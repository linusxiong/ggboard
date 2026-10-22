package subscription

import "strings"

// detectRules is the ordered detection table of docs/SUBSCRIPTION.md (first match wins).
var detectRules = []struct {
	format  string
	needles []string
}{
	{FormatClashMeta, []string{"clash.meta", "mihomo", "clashmeta", "clash-verge", "clash verge", "verge", "flclash", "clash nyanpasu", "clashx meta"}},
	{FormatStash, []string{"stash"}},
	{FormatClash, []string{"clashforandroid", "clashx", "clash"}},
	{FormatSingbox, []string{"sing-box", "singbox", "sfa", "sfi", "sfm", "nekobox", "hiddify"}},
	{FormatSurfboard, []string{"surfboard"}},
	{FormatSurge, []string{"surge"}},
	{FormatQuantumultX, []string{"quantumult x", "quantumult%20x", "quanx"}},
	{FormatShadowrocket, []string{"shadowrocket"}},
	{FormatLoon, []string{"loon"}},
	{FormatV2ray, []string{"v2rayn", "v2rayng", "nekoray", "passwall", "ssrplus", "sagernet", "karing", "v2box", "shadowsocks"}},
}

// formatNames are accepted verbatim in ?flag=.
var formatNames = map[string]bool{
	FormatClashMeta: true, FormatStash: true, FormatClash: true, FormatSingbox: true, FormatSurge: true,
	FormatSurfboard: true, FormatQuantumultX: true, FormatShadowrocket: true, FormatLoon: true,
	FormatV2ray: true, FormatSSR: true,
}

func match(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	for _, r := range detectRules {
		for _, n := range r.needles {
			if strings.Contains(s, n) {
				return r.format
			}
		}
	}
	return ""
}

// DetectFormat picks the output format from the ?flag= value (an exact format name, or a client name
// matched like a User-Agent) and then the User-Agent, defaulting to v2ray.
func DetectFormat(flag, userAgent string) string {
	if f := strings.ToLower(strings.TrimSpace(flag)); formatNames[f] {
		return f
	}
	if f := match(flag); f != "" {
		return f
	}
	if f := match(userAgent); f != "" {
		return f
	}
	return FormatV2ray
}
