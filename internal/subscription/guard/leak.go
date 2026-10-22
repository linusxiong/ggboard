package guard

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	usersvc "github.com/linusxiong/ggboard/internal/service/user"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// Leak detection (settings subscribe_leak_max_ips, subscribe_leak_action): after each flush, every (user,
// token) that was fetched from an IP bucket it was not seen from in the last 24 hours (per process) is
// re-counted: distinct ip_bucket values of that token within LeakWindow. Above the limit, the token is rotated
// when the action is "rotate" and subscription.LeakHook runs, at most once per token and window. Rows carry
// the token tag, so a rotated (or reset) token starts a fresh count even after a restart.

const (
	seenPrefix  = "subguard:seen:"
	alertPrefix = "subguard:alert:"
)

type tokenKey struct {
	user uint64
	tag  string
}

func detectLeaks(ctx context.Context, a *app.App, rows []model.SubscriptionAccess) error {
	maxIPs := a.Settings.Int("subscribe_leak_max_ips")
	if maxIPs <= 0 {
		return nil
	}
	check := map[tokenKey]bool{}
	for _, r := range rows {
		k := seenPrefix + strconv.FormatUint(r.UserID, 10) + ":" + r.TokenTag + ":" + r.IPBucket
		if _, seen := a.KV.Get(k); seen {
			continue
		}
		a.KV.Set(k, true, LeakWindow)
		check[tokenKey{r.UserID, r.TokenTag}] = true
	}
	since := time.Now().Add(-LeakWindow)
	var errs []error
	for k := range check {
		var n int64
		if err := a.DB.WithContext(ctx).Model(&model.SubscriptionAccess{}).
			Where("user_id = ? AND token_tag = ? AND at >= ?", k.user, k.tag, since).
			Distinct("ip_bucket").Count(&n).Error; err != nil {
			errs = append(errs, err)
			continue
		}
		if n <= int64(maxIPs) {
			continue
		}
		alert := alertPrefix + strconv.FormatUint(k.user, 10) + ":" + k.tag
		if _, done := a.KV.Get(alert); done {
			continue
		}
		a.KV.Set(alert, true, LeakWindow)
		if err := handleLeak(ctx, a, k, int(n)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// handleLeak rotates the token (action rotate) and runs subscription.LeakHook. Nothing happens when the
// token was changed meanwhile (the leaked one is already dead).
func handleLeak(ctx context.Context, a *app.App, k tokenKey, distinct int) error {
	users := usersvc.New(a)
	u, err := users.FindByID(ctx, k.user)
	if errors.Is(err, usersvc.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if TokenTag(u.Token) != k.tag {
		return nil
	}
	if a.Settings.String("subscribe_leak_action") == subscription.LeakActionRotate {
		if err := users.RotateToken(ctx, u); err != nil {
			return err
		}
		a.Log.Warn("subscription token rotated after a suspected leak", "user_id", u.ID, "distinct_ips", distinct)
	}
	if hook := subscription.LeakHook; hook != nil {
		hook(a, u, distinct)
	}
	return nil
}
