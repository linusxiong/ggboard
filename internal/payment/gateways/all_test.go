package gateways

import (
	"slices"
	"testing"

	"github.com/linusxiong/ggboard/internal/payment"
)

func TestAllRegistered(t *testing.T) {
	names := payment.Names()
	for _, want := range []string{"alipay_f2f", "btcpay", "coinbase", "epay", "epusdt", "stripe"} {
		if !slices.Contains(names, want) {
			t.Fatalf("gateway %s not registered (have %v)", want, names)
		}
		gw, _ := payment.Get(want)
		for _, f := range gw.Fields() {
			switch f.Type {
			case payment.FieldText, payment.FieldPassword, payment.FieldNumber, payment.FieldSelect, payment.FieldTextarea:
			default:
				t.Fatalf("%s.%s: bad field type %q", want, f.Key, f.Type)
			}
		}
	}
}
