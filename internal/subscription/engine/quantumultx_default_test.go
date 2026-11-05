package engine_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/subscription"
	"github.com/linusxiong/ggboard/internal/subscription/engine"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// Quantumult X adds a pasted link as a node resource and answers a whole configuration with "response invalid", so
// its requests get the node lines unless list=0 asks for the configuration. The panel preview keeps showing the
// configuration, and other clients are unchanged.
func TestQuantumultXNodeListByDefault(t *testing.T) {
	a := testutil.NewTestApp(t)
	if err := a.Settings.Set(map[string]any{"show_info_to_server_enable": true}); err != nil {
		t.Fatal(err)
	}
	u := testutil.CreateUser(t, a, "alice@example.com")
	const qx = "Quantumult%20X/1.9.0 (iPhone18,4; iOS 27.2)"
	cases := []struct {
		name, want string
		req        engine.Request
	}{
		{"plain link", "shadowsocks=", engine.Request{UserAgent: qx}},
		{"flag only", "shadowsocks=", engine.Request{Flag: "quantumultx"}},
		{"one-tap import", "shadowsocks=", engine.Request{UserAgent: qx, Flag: "quantumultx", Query: url.Values{"list": {"1"}}}},
		{"whole configuration", "[general]", engine.Request{UserAgent: qx, Query: url.Values{"list": {"0"}}}},
		{"preview", "[general]", engine.Request{Format: subscription.FormatQuantumultX}},
		{"preview of the list", "shadowsocks=", engine.Request{Format: subscription.FormatQuantumultX, Query: url.Values{"list": {"1"}}}},
		{"another client", "#!MANAGED-CONFIG", engine.Request{UserAgent: "Surge iOS/3867", SelfURL: "https://panel.example.com/s/x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := engine.Generate(context.Background(), a, u, tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if body := string(res.Body); !strings.HasPrefix(body, tc.want) {
				t.Fatalf("body starts %.60q, want %q", body, tc.want)
			}
		})
	}
}
