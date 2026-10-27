package user_test

import (
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/notify"
	"github.com/linusxiong/ggboard/internal/notify/notifytest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// The user changing their own link (rotate-token, reset) is not told that "the system" reset it: no
// subscription_reset mail or event.
func TestSelfRotationSendsNoResetNotice(t *testing.T) {
	a, r := setup(t, testutil.WithSettings(map[string]any{"app_url": "https://panel.example.com"}))
	rec := notifytest.Attach(t, a)
	u := testutil.CreateUser(t, a, "self@example.com")
	tok := testutil.Login(t, a, u)

	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/subscription/rotate-token", nil, tok), http.StatusOK)
	testutil.ExpectStatus(t, testutil.Do(t, r, http.MethodPost, base+"/subscription/reset", nil, tok), http.StatusOK)
	if cur := reload(t, a, u.ID); cur.Token == u.Token || cur.UUID == u.UUID {
		t.Fatalf("not rotated: %+v", cur)
	}
	for _, d := range rec.Deliveries(t) {
		if d.Event.Type == notify.TypeSubscriptionReset {
			t.Fatalf("unexpected delivery %+v", d)
		}
	}
	for _, m := range testutil.Mails(t, a) {
		if m.Template == mail.TplSubscriptionReset {
			t.Fatalf("unexpected mail %+v", m)
		}
	}
}
