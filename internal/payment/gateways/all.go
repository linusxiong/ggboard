// Package gateways links every built-in payment gateway: importing it (blank) runs each gateway's init(),
// which registers it with internal/payment. cmd/ggboard imports it from plugins_payment.go.
package gateways

import (
	_ "github.com/linusxiong/ggboard/internal/payment/gateways/alipay_f2f" // alipay_f2f
	_ "github.com/linusxiong/ggboard/internal/payment/gateways/btcpay"     // btcpay
	_ "github.com/linusxiong/ggboard/internal/payment/gateways/coinbase"   // coinbase
	_ "github.com/linusxiong/ggboard/internal/payment/gateways/epay"       // epay
	_ "github.com/linusxiong/ggboard/internal/payment/gateways/epusdt"     // epusdt
	_ "github.com/linusxiong/ggboard/internal/payment/gateways/stripe"     // stripe
)
