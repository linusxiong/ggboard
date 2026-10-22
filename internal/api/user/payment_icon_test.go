package user_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/payment/gateways/epay"
	"github.com/linusxiong/ggboard/internal/testutil"
)

// TestPaymentMethodsIconPassThrough: GET /user/payment-methods returns Payment.icon verbatim — preset
// references, URLs and emoji alike — and the web app decides how to draw it.
func TestPaymentMethodsIconPassThrough(t *testing.T) {
	a, r := shopFixture(t)
	tok := testutil.Login(t, a, testutil.CreateUser(t, a, "icons@example.com"))
	icons := []string{"preset:alipay", "https://cdn.example.com/wechat.svg", "💳", ""}
	for i, icon := range icons {
		shopCreate(t, a, &model.Payment{UUID: strings.Repeat(string(rune('a'+i)), 32), Gateway: epay.Name,
			Name: "Pay " + string(rune('A'+i)), Icon: icon, Enabled: true, Sort: i})
	}
	type method struct {
		Name string `json:"name"`
		Icon string `json:"icon"`
	}
	methods, total := testutil.DecodeList[method](t, testutil.Do(t, r, http.MethodGet, "/api/v1/user/payment-methods", nil, tok))
	if total != int64(len(icons)) {
		t.Fatalf("methods = %+v", methods)
	}
	for i, m := range methods {
		if m.Icon != icons[i] {
			t.Fatalf("method %d icon = %q, want %q", i, m.Icon, icons[i])
		}
	}
}
