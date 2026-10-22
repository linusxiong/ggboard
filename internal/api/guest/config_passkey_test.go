package guest_test

import (
	"net/http"
	"testing"

	"github.com/linusxiong/ggboard/internal/api/guest"
	"github.com/linusxiong/ggboard/internal/testutil"
)

func TestConfigPasskeyEnabled(t *testing.T) {
	a := testutil.NewTestApp(t)
	r := testutil.NewRouterWith(t, a, guest.Register)
	get := func() bool {
		return testutil.Decode[guest.Config](t, testutil.Do(t, r, http.MethodGet, "/api/v1/guest/config", nil, "")).PasskeyEnabled
	}
	if get() {
		t.Fatal("passkeys need app_url or passkey_rp_id")
	}
	for _, set := range []map[string]any{
		{"app_url": "https://panel.example.com"},
		{"app_url": nil, "passkey_rp_id": "example.com"},
	} {
		if err := a.Settings.Set(set); err != nil {
			t.Fatal(err)
		}
		if !get() {
			t.Fatalf("passkey_enabled must be true with %v", set)
		}
	}
}
