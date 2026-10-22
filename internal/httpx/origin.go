package httpx

import (
	"net/netip"
	"strings"

	"github.com/gin-gonic/gin"
)

const ctxTrustedPeer = "httpx.trusted_peer"

// parsePrefixes parses server.trusted_proxies entries (IPs or CIDRs, as gin accepts them); invalid entries are
// skipped (gin's SetTrustedProxies already reported them).
func parsePrefixes(list []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		s = strings.TrimSpace(s)
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
		} else if a, err := netip.ParseAddr(s); err == nil {
			out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
		}
	}
	return out
}

// trustedPeer records whether the direct peer of the request is one of the trusted proxies.
func trustedPeer(nets []netip.Prefix) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(nets) > 0 {
			if a, err := netip.ParseAddr(c.RemoteIP()); err == nil {
				a = a.Unmap()
				for _, n := range nets {
					if n.Contains(a) {
						c.Set(ctxTrustedPeer, true)
						break
					}
				}
			}
		}
		c.Next()
	}
}

// RequestScheme is the scheme the client used: https for TLS connections, otherwise X-Forwarded-Proto, but only
// when the direct peer is a trusted proxy (server.trusted_proxies); anyone else could claim https to make the
// panel build https links to a host it does not serve. Defaults to http.
func RequestScheme(c *gin.Context) string {
	if c.Request.TLS != nil {
		return "https"
	}
	if c.GetBool(ctxTrustedPeer) {
		if p := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))); p == "https" || p == "http" {
			return p
		}
	}
	return "http"
}

// RequestBase is RequestScheme://Host of the request (for links when settings.app_url is empty).
func RequestBase(c *gin.Context) string { return RequestScheme(c) + "://" + c.Request.Host }
