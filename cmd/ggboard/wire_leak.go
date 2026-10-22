package main

import (
	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/subscription"
)

// Subscription leak alerts go to the user by mail (DefaultLeakHook) and to the admins / the bound user
// through the notification drivers (Telegram).
func init() {
	subscription.LeakHook = func(a *app.App, u *model.User, distinct int) {
		subscription.DefaultLeakHook(a, u, distinct)
		notify.EmitSubscriptionLeak(a, u, distinct)
	}
}
