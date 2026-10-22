package settings

import "testing"

// Review 2026-10-22: server_token authenticates every node backend and node endpoints are not rate limited, so
// an admin-chosen short token could be brute-forced online. Xboard requires 16 characters as well.
func TestReviewServerTokenMinLength(t *testing.T) {
	s, err := New(newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(map[string]any{"server_token": "short-token"}); err == nil {
		t.Fatal("an 11-character server_token was accepted")
	}
	for _, ok := range []string{"0123456789abcdef", ""} {
		if err := s.Set(map[string]any{"server_token": ok}); err != nil {
			t.Fatalf("%q rejected: %v", ok, err)
		}
	}
}

// Review 2026-10-22: PUT /admin/settings {"server_token": null} reset it to "" until the next boot, rejecting every
// node. A reset now generates a new token at once.
func TestReviewServerTokenResetRegenerates(t *testing.T) {
	s, err := New(newDB(t))
	if err != nil {
		t.Fatal(err)
	}
	old := s.String("server_token")
	if err := s.Set(map[string]any{"server_token": nil}); err != nil {
		t.Fatal(err)
	}
	if got := s.String("server_token"); len(got) != 32 || got == old {
		t.Fatalf("server_token after reset = %q (old %q)", got, old)
	}
}
