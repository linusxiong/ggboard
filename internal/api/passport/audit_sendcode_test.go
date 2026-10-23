package passport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linusxiong/ggboard/internal/app"
	"github.com/linusxiong/ggboard/internal/mail"
	"github.com/linusxiong/ggboard/internal/service/account"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// sendCodeOK posts send-code for email and asserts the plain `true` answer.
func sendCodeOK(t *testing.T, r http.Handler, email string) {
	t.Helper()
	if ok := testutil.Decode[bool](t, testutil.Do(t, r, http.MethodPost, base+"/email/send-code", map[string]any{"email": email}, "")); !ok {
		t.Fatalf("%s: send-code = false", email)
	}
}

func recipients(msgs []mail.Message) string {
	to := make([]string, 0, len(msgs))
	for _, m := range msgs {
		to = append(to, m.To)
	}
	return strings.Join(to, ",")
}

// Audit 2026-10-22 (send-code): anonymous send-code had no aggregate budget, so enough IP keys could spend the
// operator's SMTP quota without bound. Beyond the global budget the answer stays `true` but nothing is sent.
func TestAuditSendCodeGlobalBudget(t *testing.T) {
	a, r := setup(t, nil)
	if !account.SetCodeBudget(a, 0, 2) { // two codes, no refill
		t.Fatal("SetCodeBudget did not take effect")
	}
	for _, email := range []string{"one@example.com", "two@example.com", "three@example.com", "four@example.com"} {
		sendCodeOK(t, r, email)
	}
	if got := recipients(testutil.Mails(t, a)); got != "one@example.com,two@example.com" {
		t.Fatalf("delivered to %q, want only the first two", got)
	}
	// The dropped request behaves like a delivered one: the per-email cooldown applies.
	expectError(t, r, "/email/send-code", map[string]any{"email": "three@example.com"}, http.StatusTooManyRequests, "")
	var warned bool
	for _, e := range a.Logs.Entries(slog.LevelWarn, 0) {
		if strings.Contains(e.Msg, "global budget exhausted") {
			warned = true
		}
	}
	if !warned {
		t.Fatal("no WARN for the exhausted budget")
	}
	if account.SetCodeBudget(a, 100, 100) {
		t.Fatal("SetCodeBudget after the first code must not replace the budget")
	}
}

// A code nobody could use is not sent (same answer): no account, and stop_register or the whitelist refuses.
func TestAuditSendCodeSkipsUnusableCodes(t *testing.T) {
	a, r := setup(t, map[string]any{"stop_register": true})
	testutil.CreateUser(t, a, "member@example.com")
	sendCodeOK(t, r, "member@example.com")   // password reset stays possible
	sendCodeOK(t, r, "stranger@example.com") // could not register: nothing sent
	if got := recipients(testutil.Mails(t, a)); got != "member@example.com" {
		t.Fatalf("stop_register: delivered to %q", got)
	}

	b, rb := setup(t, map[string]any{"email_whitelist_enable": true, "email_whitelist_suffix": []string{"example.com"}})
	testutil.CreateUser(t, b, "legacy@example.org") // registered before the whitelist
	for _, email := range []string{"allowed@example.com", "blocked@example.org", "legacy@example.org"} {
		sendCodeOK(t, rb, email)
	}
	if got := recipients(testutil.Mails(t, b)); got != "allowed@example.com,legacy@example.org" {
		t.Fatalf("whitelist: delivered to %q", got)
	}
}

// replaceMailer swaps a's mail queue for an unstarted one of the given size (the old one is stopped).
func replaceMailer(t *testing.T, a *app.App, size int) (*mail.Queue, *mail.Memory) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Mailer.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	q, err := mail.NewQueue(a.DB, a.Settings, a.Log, mail.QueueOptions{Size: size})
	if err != nil {
		t.Fatal(err)
	}
	box := &mail.Memory{}
	q.SetMailer(box)
	a.Mailer = q
	return q, box
}

// Above the queue's anonymous high-water mark send-code answers 429 (it used to fill the queue and then 500),
// and transactional mail still fits.
func TestAuditSendCodeQueueHighWater(t *testing.T) {
	a, r := setup(t, nil)
	q, box := replaceMailer(t, a, 4)
	for i := range 3 { // 75 % of 4
		if err := q.Enqueue(mail.Message{To: fmt.Sprintf("order%d@example.com", i), Subject: "paid"}); err != nil {
			t.Fatal(err)
		}
	}
	rec := testutil.Do(t, r, http.MethodPost, base+"/email/send-code", map[string]any{"email": "late@example.com"}, "")
	if e := testutil.DecodeError(t, rec); e.Status != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("send-code above the high-water mark = %d %s (Retry-After %q)", rec.Code, rec.Body.String(), rec.Header().Get("Retry-After"))
	}
	if err := q.Enqueue(mail.Message{To: "ticket@example.com", Subject: "reply"}); err != nil {
		t.Fatalf("transactional mail refused: %v", err)
	}
	q.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	// The refused request left no cooldown behind: the user can ask again once there is room.
	sendCodeOK(t, r, "late@example.com")
	if err := q.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if got := recipients(box.Messages()); got != "order0@example.com,order1@example.com,order2@example.com,ticket@example.com,late@example.com" {
		t.Fatalf("delivered to %q", got)
	}
}

// sendCodeFrom posts send-code for email from the client address ip.
func sendCodeFrom(t *testing.T, r http.Handler, ip, email string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"email": email})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, base+"/email/send-code", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if strings.Contains(ip, ":") {
		req.RemoteAddr = "[" + ip + "]:40000"
	} else {
		req.RemoteAddr = ip + ":40000"
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

// IPv6 clients are also limited per /48, so rotating /64s inside one site prefix does not multiply the budget.
func TestAuditSendCodeIPv6SiteLimit(t *testing.T) {
	a, r := setup(t, nil)
	account.SetCodeBudget(a, 100_000, 100_000) // this test is about the route limits only
	n := 0
	email := func() string { n++; return fmt.Sprintf("u%d@example.com", n) }
	for sub := range 6 { // 6 × 10 = the /48's 60 per hour
		for range 10 {
			if code := sendCodeFrom(t, r, fmt.Sprintf("2001:db8:7:%x::1", sub), email()); code != http.StatusOK {
				t.Fatalf("/64 %d: status %d", sub, code)
			}
		}
	}
	if code := sendCodeFrom(t, r, "2001:db8:7:6::1", email()); code != http.StatusTooManyRequests {
		t.Fatalf("a fresh /64 in an exhausted /48 = %d, want 429", code)
	}
	if code := sendCodeFrom(t, r, "2001:db8:8::1", email()); code != http.StatusOK {
		t.Fatalf("another /48 = %d", code)
	}
	for host := range 7 { // IPv4 clients are not grouped: 70 requests from 7 addresses all pass
		for range 10 {
			if code := sendCodeFrom(t, r, fmt.Sprintf("198.51.100.%d", host+1), email()); code != http.StatusOK {
				t.Fatalf("IPv4 %d: status %d", host, code)
			}
		}
	}
}
