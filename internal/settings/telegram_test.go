package settings

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestTelegramSettings(t *testing.T) {
	d := newDB(t)
	s, err := New(d)
	if err != nil {
		t.Fatal(err)
	}
	if s.String("telegram_bot_token") != "" || !s.Bool("telegram_user_notify") || s.String("telegram_discuss_link") != "" ||
		len(s.Strings("telegram_admin_chat_ids")) != 0 {
		t.Fatal("telegram defaults")
	}
	if got := s.Strings("telegram_events"); strings.Join(got, ",") != "order_paid,admin_login,ticket_new,withdraw,subscription_leak" {
		t.Fatalf("telegram_events default = %v", got)
	}
	def, _ := Lookup("telegram_events")
	if strings.Join(def.Options, " ") != "order_new order_paid order_cancel admin_login ticket_new ticket_reply withdraw subscription_leak" {
		t.Fatalf("options = %v", def.Options)
	}
	for _, k := range []string{"telegram_bot_token", "telegram_webhook_secret"} {
		if !IsSecret(k) {
			t.Errorf("%s must be secret", k)
		}
	}
	if d, _ := Lookup("telegram_bot_token"); d.Group != GroupTelegram {
		t.Fatal("group")
	}

	// The webhook secret is generated once, like server_token, and regenerated when reset to null.
	secret := s.String("telegram_webhook_secret")
	if len(secret) != 32 {
		t.Fatalf("webhook secret = %q", secret)
	}
	s2, err := New(d)
	if err != nil || s2.String("telegram_webhook_secret") != secret {
		t.Fatal("the webhook secret must be generated only once")
	}
	if err := s.Set(map[string]any{"telegram_webhook_secret": nil}); err != nil {
		t.Fatal(err)
	}
	if n := s.String("telegram_webhook_secret"); len(n) != 32 || n == secret {
		t.Fatalf("regenerated secret = %q", n)
	}
	if err := s.Set(map[string]any{"telegram_webhook_secret": "short"}); err == nil {
		t.Fatal("a short webhook secret must be refused")
	}

	// Every element of telegram_events must be a known option; strings settings without options stay free.
	err = s.Set(map[string]any{"telegram_events": []any{"order_new", "nope"}})
	var fe FieldErrors
	if !errors.As(err, &fe) || len(fe["telegram_events"]) != 1 || !strings.Contains(fe["telegram_events"][0], `"nope"`) {
		t.Fatalf("unknown event = %v", err)
	}
	if err := s.Set(map[string]any{"telegram_events": "order_new, ticket_reply", "telegram_admin_chat_ids": "123\n-100456,@ops"}); err != nil {
		t.Fatal(err)
	}
	if got := s.Strings("telegram_events"); !slices.Equal(got, []string{"order_new", "ticket_reply"}) {
		t.Fatalf("events = %v", got)
	}
	if got := s.Strings("telegram_admin_chat_ids"); !slices.Equal(got, []string{"123", "-100456", "@ops"}) {
		t.Fatalf("chat ids = %v", got)
	}
	if err := s.Set(map[string]any{"telegram_events": []string{}}); err != nil || len(s.Strings("telegram_events")) != 0 {
		t.Fatalf("empty events = %v", err)
	}
}
