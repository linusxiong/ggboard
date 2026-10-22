// Package epusdt implements the legacy Epusdt protocol served by epusdt <= v1 and the compatible BEpusdt fork
// (docs/PAYMENTS_GATEWAYS.md §3): MD5-signed JSON create-transaction and JSON notify. GM Pay v2 users
// configure the epay gateway instead.
package epusdt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/linusxiong/ggboard/internal/payment"
	"github.com/linusxiong/ggboard/internal/payment/gateways/gwutil"
)

// Name is the registry id.
const Name = "epusdt"

// DefaultTradeType is sent as trade_type when currency_type is empty.
const DefaultTradeType = "usdt.trc20"

// maxCents is the largest accepted amount: BEpusdt's %v formatting diverges from strconv at 1e6.
const maxCents = 100_000_000

func init() { payment.Register(&Gateway{}) }

// Gateway is the Epusdt plugin. Client overrides the shared HTTP client (tests).
type Gateway struct {
	Client *http.Client
}

// Name implements payment.Gateway.
func (*Gateway) Name() string { return Name }

// Label implements payment.Gateway.
func (*Gateway) Label() string { return "Epusdt / BEpusdt" }

// Fields implements payment.Gateway.
func (*Gateway) Fields() []payment.Field {
	return []payment.Field{
		{Key: "api_url", Label: "API 地址", Type: payment.FieldText, Required: true, Description: "Base URL, e.g. https://usdt.example.com"},
		{Key: "api_token", Label: "API Token", Type: payment.FieldPassword, Required: true, Description: "BEpusdt: 系统管理 → 基本设置 → API 设置 → 对接令牌"},
		{Key: "currency_type", Label: "交易类型", Type: payment.FieldText, Default: DefaultTradeType,
			Options:     []string{"usdt.trc20", "usdc.trc20", "tron.trx", "usdt.erc20", "usdt.polygon", "usdt.bep20", "usdt.solana", "usdt.ton", "usdt.arbitrum", "usdc.base"},
			Description: "Sent as trade_type (BEpusdt); legacy epusdt ignores it"},
	}
}

// ValidateConfig checks the API URL.
func (*Gateway) ValidateConfig(cfg map[string]string, _ string) error {
	_, err := gwutil.BaseURL(cfg["api_url"], Name, true)
	return err
}

// Canonical renders a decoded JSON value the way the servers sign it; ok=false skips the key
// (null and empty strings).
func canonical(v any, floatFallback bool) (string, bool, error) {
	switch x := v.(type) {
	case nil:
		return "", false, nil
	case string:
		return x, x != "", nil
	case json.Number:
		if floatFallback {
			f, err := x.Float64()
			if err != nil {
				return "", false, err
			}
			return fmt.Sprint(f), true, nil
		}
		return x.String(), true, nil
	case bool:
		if x {
			return "true", true, nil
		}
		return "false", true, nil
	default:
		return "", false, fmt.Errorf("epusdt: unsupported value type %T", v)
	}
}

// Sign returns md5(signString + token) over params except "signature".
func Sign(params map[string]string, token string) string {
	return gwutil.MD5Hex(gwutil.SignString(params, "signature") + token)
}

type createResponse struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       *struct {
		PaymentURL string          `json:"payment_url"`
		TradeID    json.RawMessage `json:"trade_id"`
	} `json:"data"`
}

// Pay implements payment.Gateway.
func (g *Gateway) Pay(ctx context.Context, cfg map[string]string, req payment.PayRequest) (*payment.PayResult, error) {
	if err := gwutil.Required(cfg, Name, "api_url", "api_token"); err != nil {
		return nil, err
	}
	if err := gwutil.RequireAmount(Name, req.TotalAmount); err != nil {
		return nil, err
	}
	if req.TotalAmount >= maxCents {
		return nil, errors.New("epusdt: amount must be below 1000000")
	}
	base, err := gwutil.BaseURL(cfg["api_url"], Name, true)
	if err != nil {
		return nil, err
	}
	tradeType := strings.TrimSpace(cfg["currency_type"])
	if tradeType == "" {
		tradeType = DefaultTradeType
	}
	amount := gwutil.TrimmedAmount(req.TotalAmount)
	params := map[string]string{
		"amount":       amount,
		"order_id":     req.TradeNo,
		"notify_url":   req.NotifyURL,
		"redirect_url": req.ReturnURL,
		"trade_type":   tradeType,
	}
	if cur := strings.ToUpper(strings.TrimSpace(req.Currency)); cur != "" && cur != "CNY" {
		params["fiat"] = cur
	}
	body := map[string]any{"amount": json.Number(amount)}
	for k, v := range params {
		if k != "amount" {
			body[k] = v
		}
	}
	body["signature"] = Sign(params, cfg["api_token"])
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/order/create-transaction", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "application/json")
	status, raw, err := gwutil.Do(ctx, g.Client, hreq, Name)
	if err != nil {
		return nil, err
	}
	var res createResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("epusdt: invalid response (HTTP %d)", status)
	}
	if res.StatusCode != http.StatusOK || res.Data == nil || res.Data.PaymentURL == "" {
		return nil, fmt.Errorf("epusdt: create transaction failed: %s", gwutil.Truncate(res.Message, 200))
	}
	return &payment.PayResult{Type: payment.PayTypeURL, Data: res.Data.PaymentURL}, nil
}

// Notify implements payment.Gateway.
func (*Gateway) Notify(_ context.Context, cfg map[string]string, r *http.Request) (*payment.NotifyResult, *payment.NotifyResponse, error) {
	if err := gwutil.Required(cfg, Name, "api_token"); err != nil {
		return nil, nil, err
	}
	raw, err := gwutil.ReadBody(r.Body)
	if err != nil {
		return nil, nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, nil, fmt.Errorf("epusdt: invalid JSON: %w", err)
	}
	sig, _ := m["signature"].(string)
	if sig == "" {
		return nil, nil, errors.New("epusdt: missing signature")
	}
	exact, fallback := map[string]string{}, map[string]string{}
	for k, v := range m {
		if k == "signature" {
			continue
		}
		s, ok, err := canonical(v, false)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			exact[k] = s
		}
		if s, ok, err = canonical(v, true); err != nil {
			return nil, nil, err
		} else if ok {
			fallback[k] = s
		}
	}
	if !gwutil.EqualHex(sig, Sign(exact, cfg["api_token"])) && !gwutil.EqualHex(sig, Sign(fallback, cfg["api_token"])) {
		return nil, nil, errors.New("epusdt: signature mismatch")
	}
	ok := gwutil.TextResponse(http.StatusOK, "ok")
	if exact["status"] != "2" {
		return nil, ok, nil
	}
	tradeNo := exact["order_id"]
	if tradeNo == "" {
		return nil, nil, errors.New("epusdt: missing order_id")
	}
	amount, err := gwutil.ParseCents(exact["amount"])
	if err != nil {
		return nil, nil, err
	}
	slog.Info("epusdt payment confirmed", "trade_no", tradeNo, "trade_id", exact["trade_id"], "block_transaction_id", exact["block_transaction_id"])
	return &payment.NotifyResult{TradeNo: tradeNo, CallbackNo: exact["trade_id"], Amount: amount}, ok, nil
}
