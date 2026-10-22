package guard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/httpx"
	"github.com/linusxiong/ggboard/internal/model"
)

// Access log limits (docs/SUBSCRIPTION.md "Access log and leak detection").
const (
	// MaxBuffered bounds the rows waiting for the next flush; requests never block on the log, rows beyond
	// it are dropped (and counted in the flush log line).
	MaxBuffered = 10000
	// KeepPerUser is how many rows per user the daily prune keeps; KeepFor is the maximum age of a row.
	KeepPerUser = 100
	KeepFor     = 30 * 24 * time.Hour
	// UserLogLimit / AdminLogLimit are the rows returned by the access-log endpoints.
	UserLogLimit  = 50
	AdminLogLimit = 100
	// LeakWindow is the rolling window of leak detection.
	LeakWindow = 24 * time.Hour

	flushBatch = 500
)

// Job names (registered in jobs.go).
const (
	JobAccessFlush = "subscription_access_flush"
	JobAccessPrune = "subscription_access_prune"
)

// Access is one successful subscription fetch.
type Access struct {
	UserID    uint64
	Token     string // the token used (only its tag is stored)
	IP        string // client IP (c.ClientIP())
	UserAgent string
	Format    string
	At        time.Time // zero = now
}

// recorder buffers access rows of one App until the next flush.
type recorder struct {
	mu      sync.Mutex
	buf     []model.SubscriptionAccess
	dropped int
}

const recorderKey = "subguard:recorder"

// recorderOf returns the App's recorder (kept in a.KV without expiry, so it lives as long as the App).
func recorderOf(a *app.App) *recorder {
	v := a.KV.Update(recorderKey, -1, true, func(old any, ok bool) any {
		if r, isRec := old.(*recorder); ok && isRec {
			return r
		}
		return &recorder{}
	})
	return v.(*recorder) //nolint:forcetypeassert // only this package writes the key
}

// TokenTag returns the first 8 hex characters of SHA-256(token): enough to tell tokens of one user apart in
// the log, useless for recovering the token.
func TokenTag(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:4])
}

// normalizeIP returns ip in canonical form (IPv4-mapped IPv6 addresses as IPv4, no zone); unparsable values
// are kept as they are (truncated to 45 bytes).
func normalizeIP(ip string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return truncateASCII(ip, 45)
	}
	return addr.Unmap().WithZone("").String()
}

// ClientFamily reduces a User-Agent to its first product token ("clash-verge/v2.2.3", "mihomo/1.19.27"):
// printable ASCII, at most 64 bytes, "unknown" when empty.
func ClientFamily(ua string) string {
	ua = strings.TrimSpace(ua)
	if i := strings.IndexAny(ua, " ;("); i >= 0 {
		ua = ua[:i]
	}
	var b strings.Builder
	for i := 0; i < len(ua) && b.Len() < 64; i++ {
		if c := ua[i]; c > 0x20 && c < 0x7f {
			b.WriteByte(c)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func truncateASCII(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Record queues one access row; it never blocks and never fails (rows beyond MaxBuffered are dropped).
func Record(a *app.App, acc Access) {
	at := acc.At
	if at.IsZero() {
		at = time.Now()
	}
	ip := normalizeIP(acc.IP)
	row := model.SubscriptionAccess{
		UserID: acc.UserID, IP: ip, IPBucket: truncateASCII(httpx.IPKey(ip), 64), TokenTag: TokenTag(acc.Token),
		Client: ClientFamily(acc.UserAgent), Format: truncateASCII(acc.Format, 16), At: at.UTC(),
	}
	r := recorderOf(a)
	r.mu.Lock()
	if len(r.buf) < MaxBuffered {
		r.buf = append(r.buf, row)
	} else {
		r.dropped++
	}
	r.mu.Unlock()
}

// Pending returns the number of buffered rows.
func Pending(a *app.App) int {
	r := recorderOf(a)
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.buf)
}

// FlushAccess writes the buffered rows in batches and runs leak detection for them. It runs every 10 s
// (JobAccessFlush) and before the access-log endpoints answer.
func FlushAccess(ctx context.Context, a *app.App) error {
	r := recorderOf(a)
	r.mu.Lock()
	rows, dropped := r.buf, r.dropped
	r.buf, r.dropped = nil, 0
	r.mu.Unlock()
	if dropped > 0 {
		a.Log.Warn("subscription access log: buffer full, rows dropped", "dropped", dropped)
	}
	if len(rows) == 0 {
		return nil
	}
	if err := a.DB.WithContext(ctx).CreateInBatches(&rows, flushBatch).Error; err != nil {
		return err
	}
	return detectLeaks(ctx, a, rows)
}

// PruneAccess deletes rows older than KeepFor and keeps the newest KeepPerUser rows of every user.
func PruneAccess(ctx context.Context, a *app.App, now time.Time) error {
	db := a.DB.WithContext(ctx)
	if err := db.Where("at < ?", now.Add(-KeepFor)).Delete(&model.SubscriptionAccess{}).Error; err != nil {
		return err
	}
	var users []uint64
	if err := db.Model(&model.SubscriptionAccess{}).Group("user_id").Having("COUNT(*) > ?", KeepPerUser).
		Pluck("user_id", &users).Error; err != nil {
		return err
	}
	for _, uid := range users {
		var cutoff []uint64
		if err := db.Model(&model.SubscriptionAccess{}).Where("user_id = ?", uid).Order("id DESC").
			Offset(KeepPerUser-1).Limit(1).Pluck("id", &cutoff).Error; err != nil {
			return err
		}
		if len(cutoff) == 0 {
			continue
		}
		if err := db.Where("user_id = ? AND id < ?", uid, cutoff[0]).Delete(&model.SubscriptionAccess{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// AccessEntry is one row of GET /user/subscription/access-log (IP masked) and of the admin variant
// (unmasked, with ip_bucket and token_tag).
type AccessEntry struct {
	IP       string    `json:"ip"`
	IPBucket string    `json:"ip_bucket,omitempty"`
	TokenTag string    `json:"token_tag,omitempty"`
	Client   string    `json:"client"`
	Format   string    `json:"format"`
	At       time.Time `json:"at"`
}

// ListAccess returns the newest rows of a user (pending rows are flushed first). masked hides the last IPv4
// octet and the last 64 bits of IPv6 addresses and leaves out ip_bucket and token_tag.
func ListAccess(ctx context.Context, a *app.App, userID uint64, limit int, masked bool) ([]AccessEntry, error) {
	if err := FlushAccess(ctx, a); err != nil {
		a.Log.Warn("subscription access log: flush failed", "err", err)
	}
	var rows []model.SubscriptionAccess
	if err := a.DB.WithContext(ctx).Where("user_id = ?", userID).Order("at DESC, id DESC").Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AccessEntry, 0, len(rows))
	for _, r := range rows {
		e := AccessEntry{IP: r.IP, Client: r.Client, Format: r.Format, At: r.At.UTC()}
		if masked {
			e.IP = MaskIP(r.IP)
		} else {
			e.IPBucket, e.TokenTag = r.IPBucket, r.TokenTag
		}
		out = append(out, e)
	}
	return out, nil
}

// MaskIP hides the host part shown to users: "203.0.113.*" for IPv4, "2001:db8:1:2:*" (the /64 prefix) for
// IPv6, "*" for anything unparsable.
func MaskIP(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "*"
	}
	addr = addr.Unmap()
	if addr.Is4() {
		b := addr.As4()
		return strconv.Itoa(int(b[0])) + "." + strconv.Itoa(int(b[1])) + "." + strconv.Itoa(int(b[2])) + ".*"
	}
	b := addr.As16()
	parts := make([]string, 0, 5)
	for i := 0; i < 8; i += 2 {
		parts = append(parts, strconv.FormatUint(uint64(b[i])<<8|uint64(b[i+1]), 16))
	}
	return strings.Join(append(parts, "*"), ":")
}
