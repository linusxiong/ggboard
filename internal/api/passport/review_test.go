package passport_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/captcha"
	"github.com/linusxiong/ggboard/internal/service/account"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// gateCaptcha holds every caller until want callers arrived (or a short timeout), widening the window between
// the lockout check and the password verification so the race is deterministic.
type gateCaptcha struct {
	want    int32
	arrived *atomic.Int32
	all     chan struct{}
	once    *sync.Once
}

func (g gateCaptcha) Verify(context.Context, string, string) error {
	if g.arrived.Add(1) == g.want {
		g.once.Do(func() { close(g.all) })
	}
	select {
	case <-g.all:
	case <-time.After(300 * time.Millisecond):
	}
	return nil
}

// Review 2026-10-22: the lockout was check-then-record, so N concurrent wrong passwords all passed the check
// before the first failure was recorded. Attempts are now reserved atomically before verification.
func TestReviewConcurrentLoginsRespectLockout(t *testing.T) {
	const n = 12
	gate := gateCaptcha{want: n, arrived: new(atomic.Int32), all: make(chan struct{}), once: new(sync.Once)}
	orig := account.CaptchaVerifier
	account.CaptchaVerifier = func(*app.App) captcha.Verifier { return gate }
	t.Cleanup(func() { account.CaptchaVerifier = orig })

	a, r := setup(t, map[string]any{"password_limit_count": 3, "password_limit_expire": 15,
		"captcha_provider": "turnstile", "captcha_site_key": "site", "captcha_secret": "secret"})
	u := testutil.CreateUser(t, a, "race@example.com")
	codes := make(chan int, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			rec := testutil.Do(t, r, http.MethodPost, base+"/login",
				map[string]any{"email": u.Email, "password": "wrong-password", "captcha": "x"}, "")
			codes <- rec.Code
		})
	}
	wg.Wait()
	close(codes)
	got := map[int]int{}
	for c := range codes {
		got[c]++
	}
	if got[http.StatusUnauthorized] != 3 || got[http.StatusTooManyRequests] != n-3 {
		t.Fatalf("codes = %v, want 3×401 and %d×429", got, n-3)
	}
}

// Review 2026-10-22: each new code reset the 5-guess budget, so guessing could go on indefinitely at one code a
// minute. Wrong codes are now also counted per email (MaxCodeFailures per CodeFailureWindow).
func TestReviewEmailCodeFailuresPerEmail(t *testing.T) {
	a, r := setup(t, nil)
	testutil.CreateUser(t, a, "victim@example.com")
	acc := account.New(a)
	for i := range account.MaxCodeFailures {
		if i%account.MaxCodeAttempts == 0 {
			a.KV.Delete("account:code_cooldown:victim@example.com") // a minute passes: a new code
			if err := acc.SendCode("victim@example.com"); err != nil {
				t.Fatal(err)
			}
		}
		code := wrongCode(lastCode(t, a, "victim@example.com"))
		expectError(t, r, "/forgot", map[string]any{"email": "victim@example.com", "email_code": code, "password": "new-password-1"}, 400, "email_code")
	}
	a.KV.Delete("account:code_cooldown:victim@example.com")
	if err := acc.SendCode("victim@example.com"); err != nil {
		t.Fatal(err)
	}
	rec := testutil.Do(t, r, http.MethodPost, base+"/forgot",
		map[string]any{"email": "victim@example.com", "email_code": lastCode(t, a, "victim@example.com"), "password": "new-password-1"}, "")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("after %d wrong codes the right one = %d %s", account.MaxCodeFailures, rec.Code, rec.Body.String())
	}
}
