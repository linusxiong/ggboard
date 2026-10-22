package coinbase

import "testing"

// Review 2026-10-22: the form default exchange_rate "1" passed validation on a CNY site, charging 100 CNY as
// 100 USDC.
func TestReviewRateRequiredOffDollar(t *testing.T) {
	_, sec1PEM, _ := ecKeyPEM(t)
	gw := &Gateway{}
	for rate, ok := range map[string]bool{"": false, "1": false, "1.00": false, "0.14": true} {
		err := gw.ValidateConfig(map[string]string{"api_key_secret": sec1PEM, "exchange_rate": rate}, "CNY")
		if (err == nil) != ok {
			t.Errorf("CNY rate %q: err=%v", rate, err)
		}
	}
	for _, cur := range []string{"USD", "usdt", "USDC"} {
		if err := gw.ValidateConfig(map[string]string{"api_key_secret": sec1PEM, "exchange_rate": ""}, cur); err != nil {
			t.Errorf("%s with the default rate: %v", cur, err)
		}
	}
}
