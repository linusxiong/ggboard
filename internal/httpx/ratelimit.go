package httpx

import (
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/linusxiong/ggboard/internal/cache"
)

// KeyFunc extracts the rate-limit key from a request ("" skips limiting).
type KeyFunc func(c *gin.Context) string

// KeyByIP keys by client IP (honoring trusted proxies). IPv6 clients are keyed by their /64: a single host
// usually controls a whole /64, so per-address buckets would let it rotate addresses past every limit.
func KeyByIP(c *gin.Context) string { return IPKey(c.ClientIP()) }

// IPKey returns the rate-limit key of ip: IPv4 addresses as they are, IPv6 addresses as their /64 prefix.
func IPKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil || addr.Unmap().Is4() {
		return ip
	}
	p, err := addr.WithZone("").Prefix(64)
	if err != nil {
		return ip
	}
	return p.String()
}

// KeyByQuery returns a KeyFunc keying by a query parameter (e.g. the subscription token).
func KeyByQuery(name string) KeyFunc {
	return func(c *gin.Context) string { return c.Query(name) }
}

// Limiter is an in-memory keyed token-bucket limiter; idle keys are evicted.
type Limiter struct {
	r     rate.Limit
	burst int
	idle  time.Duration
	m     *cache.TTLMap[string, *rate.Limiter]
}

// NewLimiter allows r events per second with the given burst per key.
func NewLimiter(r rate.Limit, burst int) *Limiter {
	if burst < 1 {
		burst = 1
	}
	idle := 10 * time.Minute
	if r > 0 {
		if refill := time.Duration(float64(burst) / float64(r) * float64(time.Second)); refill > idle {
			idle = refill
		}
	}
	return &Limiter{r: r, burst: burst, idle: idle, m: cache.NewTTLMap[string, *rate.Limiter](idle)}
}

// Per returns the rate of n events per d (e.g. Per(10, time.Hour)).
func Per(n int, d time.Duration) rate.Limit { return rate.Limit(float64(n) / d.Seconds()) }

func (l *Limiter) get(key string) *rate.Limiter {
	return l.m.Update(key, l.idle, false, func(old *rate.Limiter, ok bool) *rate.Limiter {
		if ok {
			return old
		}
		return rate.NewLimiter(l.r, l.burst)
	})
}

// Allow consumes one token for key.
func (l *Limiter) Allow(key string) bool { return l.get(key).Allow() }

// Exhausted reports whether key has no token left, without consuming one: for buckets charged only after the work
// (misses), checked before it.
func (l *Limiter) Exhausted(key string) bool { return key != "" && l.get(key).Tokens() < 1 }

// RetryAfter estimates when the next token for key becomes available (without consuming it).
func (l *Limiter) RetryAfter(key string) time.Duration {
	lim := l.get(key)
	r := lim.Reserve()
	d := r.Delay()
	r.Cancel()
	return d
}

// Keys returns the number of tracked keys.
func (l *Limiter) Keys() int { return l.m.Len() }

// RateLimit returns middleware allowing r events/second (burst) per key. Each call creates its own buckets;
// share a Limiter between routes with Limit. name is used in logs.
func RateLimit(name string, r rate.Limit, burst int, keyFn KeyFunc) gin.HandlerFunc {
	return Limit(name, NewLimiter(r, burst), keyFn)
}

// Limit returns middleware enforcing l keyed by keyFn; denied requests get 429 with Retry-After.
func Limit(name string, l *Limiter, keyFn KeyFunc) gin.HandlerFunc {
	if keyFn == nil {
		keyFn = KeyByIP
	}
	return func(c *gin.Context) {
		if l.Check(c, name, keyFn(c)) {
			c.Next()
		}
	}
}

// Check consumes one token for key ("" is never limited). When none is left it aborts the request with 429 and
// Retry-After and returns false. name is used in logs; the logged path is the route pattern (LogPath), so
// secrets carried in path parameters (/s/:token) never reach the log.
func (l *Limiter) Check(c *gin.Context, name, key string) bool {
	if key == "" || l.Allow(key) {
		return true
	}
	l.Deny(c, name, key)
	return false
}

// Deny aborts the request with 429 and the Retry-After of key.
func (l *Limiter) Deny(c *gin.Context, name, key string) {
	secs := int(math.Ceil(l.RetryAfter(key).Seconds()))
	if secs < 1 {
		secs = 1
	}
	c.Header("Retry-After", strconv.Itoa(secs))
	Logger(c).Info("rate limited", "limiter", name, "path", LogPath(c))
	Fail(c, NewError(http.StatusTooManyRequests, CodeRateLimited, "too many requests, retry later"))
}
