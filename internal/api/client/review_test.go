package client_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/client"
)

// Review 2026-10-22: the token bucket was keyed by the raw, unchecked token, so every distinct string created a
// limiter entry (unbounded memory) and unknown tokens were never limited. Unknown tokens now drain a per-IP bucket.
func TestReviewUnknownTokensLimitedPerIP(t *testing.T) {
	e := setup(t, nil)
	codes := map[int]int{}
	for i := range client.MissBurst + 1 {
		path := fmt.Sprintf("/api/v1/client/subscribe?token=%032x", i)
		if i%2 == 1 {
			path = "/s/" + strings.Repeat("x", 100+i) // malformed tokens count as misses too
		}
		codes[e.get(t, path, nil).Code]++
	}
	if codes[http.StatusForbidden] != client.MissBurst || codes[http.StatusTooManyRequests] != 1 {
		t.Fatalf("codes = %v", codes)
	}
	// A valid token from the same address (e.g. behind NAT) is not blocked by the misses.
	if rec := e.get(t, e.sub("&flag=v2ray"), nil); rec.Code != http.StatusOK {
		t.Fatalf("valid token after misses = %d", rec.Code)
	}
}

// Review 2026-10-22: the 429 log line of the short link used the raw path and leaked the token.
func TestReviewRateLimitLogRedactsShortLink(t *testing.T) {
	e := setup(t, nil)
	for range client.SubscribeBurst + 2 {
		e.get(t, "/s/"+e.u.Token+"?flag=v2ray", nil)
	}
	limited := false
	for _, entry := range e.a.Logs.Entries(slog.LevelDebug, 0) {
		b, _ := json.Marshal(entry)
		if strings.Contains(string(b), e.u.Token) {
			t.Fatalf("token leaked into logs: %s", b)
		}
		if entry.Msg == "rate limited" {
			limited = true
		}
	}
	if !limited {
		t.Fatal("expected a rate limited log line")
	}
}
