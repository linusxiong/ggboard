package subscription

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
)

// Info pseudo-node endpoint (never reachable; clients only show the names).
const (
	InfoHost   = "127.0.0.1"
	InfoPort   = 1080
	InfoCipher = "aes-128-gcm"
)

type infoText struct {
	remaining, expires, never, reset, banned, expired, exhausted string
}

var infoTexts = map[string]infoText{
	"zh-CN": {
		remaining: "剩余流量：%s", expires: "套餐到期：%s", never: "长期有效", reset: "距离下次重置剩余：%d 天",
		banned: "账户已封禁", expired: "套餐已过期", exhausted: "流量已用尽",
	},
	"en": {
		remaining: "Traffic left: %s", expires: "Expires: %s", never: "never", reset: "Next reset in: %d days",
		banned: "Account banned", expired: "Plan expired", exhausted: "Traffic exhausted",
	},
}

// InfoLines returns the account information lines for u (localized; zh-CN for unknown locales): a status line
// when u is unavailable, remaining traffic, expiry and days until the next traffic reset.
func InfoLines(u *model.User, locale string, now time.Time, loc *time.Location) []string {
	t, ok := infoTexts[locale]
	if !ok {
		t = infoTexts["zh-CN"]
	}
	if loc == nil {
		loc = time.Local
	}
	var lines []string
	switch {
	case u.Banned:
		lines = append(lines, t.banned)
	case u.IsExpired(now):
		lines = append(lines, t.expired)
	case u.Used() >= u.TransferEnable:
		lines = append(lines, t.exhausted)
	}
	lines = append(lines, fmt.Sprintf(t.remaining, FormatGB(u.Remaining())))
	if u.ExpiredAt == nil {
		lines = append(lines, fmt.Sprintf(t.expires, t.never))
	} else {
		lines = append(lines, fmt.Sprintf(t.expires, u.ExpiredAt.In(loc).Format("2006-01-02")))
	}
	if d := usersvc.ResetDays(u.NextResetAt, now); d != nil && !u.IsExpired(now) {
		lines = append(lines, fmt.Sprintf(t.reset, *d))
	}
	return lines
}

// InfoProxies turns InfoLines into shadowsocks pseudo nodes pointing at 127.0.0.1:1080.
func InfoProxies(u *model.User, locale string, now time.Time, loc *time.Location) []Proxy {
	lines := InfoLines(u, locale, now, loc)
	out := make([]Proxy, 0, len(lines))
	for _, l := range lines {
		ss := model.ShadowsocksSettings{Cipher: InfoCipher, Obfs: "plain"}
		out = append(out, Proxy{
			Name: DisplayName(l, false), Type: model.NodeShadowsocks, Host: InfoHost, Port: InfoPort, Info: true,
			UUID: u.UUID, Password: u.UUID, SS: &ss,
		})
	}
	return out
}

// FormatGB formats bytes as GiB with up to two decimals ("12.5 GB", "0 GB").
func FormatGB(b int64) string {
	s := strconv.FormatFloat(float64(b)/(1<<30), 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s + " GB"
}
