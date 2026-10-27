package adminuser

import (
	"strings"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/notify"
)

// notifySubscriptionReset tells the user that an administrator changed their subscription credentials: before
// and after are the user before and after the change. Nothing is sent unless the token or the uuid changed. It
// queues the subscription_reset mail and the user-only subscription_reset notify event (Telegram when bound);
// neither carries the new link or token, only the subscription page (app_url + "/subscription"). A mail that
// cannot be queued is logged at WARN and never fails the caller; delivery happens on the queues' workers.
func (s *Service) notifySubscriptionReset(before, after *model.User) {
	r := notify.Reset{
		UserID:    after.ID,
		Token:     before.Token != after.Token,
		UUID:      before.UUID != after.UUID,
		SignedOut: before.PasswordVersion != after.PasswordVersion,
	}
	if !r.Token && !r.UUID {
		return
	}
	if base := strings.TrimRight(s.a.Settings.String("app_url"), "/"); base != "" {
		r.URL = base + "/subscription"
	}
	notify.Emit(s.a, notify.SubscriptionReset(s.a, r))
	if after.Email == "" || s.a.Mailer == nil {
		return
	}
	data := mail.Data{"TokenReset": r.Token, "UUIDReset": r.UUID, "SignedOut": r.SignedOut}
	if r.URL != "" {
		data["URL"] = r.URL
	}
	if err := s.a.Mailer.SendTemplate(after.Email, mail.TplSubscriptionReset, data); err != nil {
		s.a.Log.Warn("subscription reset: mail not queued", "user_id", after.ID, "err", err)
	}
}
