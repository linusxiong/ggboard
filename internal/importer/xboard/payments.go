package xboardimport

import (
	"fmt"
	"sort"
	"strings"

	"github.com/linusxiong/ggboard/internal/model"
	"github.com/linusxiong/ggboard/internal/settings"
)

// payments ← v2_payment (§4.9). Config keys follow docs/PAYMENTS_GATEWAYS.md.

// xboardPluginCodes maps Xboard payment class names to U's payment plugin codes (v2_plugins.code).
var xboardPluginCodes = map[string]string{
	"EPay": "epay", "AlipayF2F": "alipay_f2f", "BTCPay": "btcpay", "Coinbase": "coinbase",
	"CoinPayments": "coin_payments", "MGate": "mgate",
}

// stripeClasses are StripeALLInOne and the older single-method Stripe classes, with the method they imply.
var stripeClasses = map[string]string{
	"StripeALLInOne": "", "StripeAlipay": "alipay", "StripeWepay": "wechat_pay", "StripeCredit": "card", "StripeCheckout": "card",
}

func (im *importer) importPayments() error {
	stat := im.rep.Table("payments")
	disabledPlugins, err := im.disabledPaymentPlugins()
	if err != nil {
		return err
	}
	seenUUID := map[string]uint64{}
	type gwCount struct{ total, enabled int }
	counts := map[string]*gwCount{}
	b := newBatcher[model.Payment](im.tx)
	err = im.src.each(im.ctx, "v2_payment", "", nil, func(r row) error {
		stat.Source++
		id := r.id()
		scope := scopeID("payments", id)
		class := strings.TrimSpace(r.str("payment"))
		p := model.Payment{
			ID:           id,
			UUID:         strings.TrimSpace(r.str("uuid")),
			Name:         im.fit(scope, "name", r.str("name"), 191),
			Icon:         im.fitURL(scope, "icon", r.str("icon"), 255),
			NotifyDomain: im.fitURL(scope, "notify_domain", strings.TrimSpace(r.str("notify_domain")), 255),
			Enabled:      r.bool("enable"),
			Sort:         int(r.i64("sort")),
			CreatedAt:    im.ts(scope, r["created_at"]),
			UpdatedAt:    im.ts(scope, r["updated_at"]),
		}
		p.HandlingFeeFixed = r.i64("handling_fee_fixed")
		if f, ok := flexFloat(r["handling_fee_percent"]); ok {
			p.HandlingFeePercent = f
		}
		if prev, dup := seenUUID[strings.ToLower(p.UUID)]; p.UUID == "" || dup || len(p.UUID) > 32 {
			old := p.UUID
			p.UUID = settings.RandomToken(8)
			if dup {
				im.warn(scope, "uuid %q duplicates payment #%d → new uuid %s: the notify URL changes", old, prev, p.UUID)
			} else {
				im.warn(scope, "uuid %q is empty or invalid → new uuid %s: the notify URL changes", old, p.UUID)
			}
		}
		seenUUID[strings.ToLower(p.UUID)] = id

		cfg := map[string]any{}
		if obj, err := jsonObject(r["config"]); err != nil {
			im.warn(scope, "config: %v → {}", err)
		} else if obj != nil {
			cfg = obj
		}
		p.Gateway, p.Config = im.mapGateway(scope, class, cfg, &p)

		if code, ok := xboardPluginCodes[class]; ok && disabledPlugins[code] && p.Enabled {
			p.Enabled = false
			im.warn(scope, "Xboard payment plugin %q is disabled → payment imported disabled", code)
		}
		if !p.Enabled {
			im.rep.DisabledPays = append(im.rep.DisabledPays, fmt.Sprintf("%s #%d %q", p.Gateway, id, p.Name))
		}
		c := counts[p.Gateway]
		if c == nil {
			c = &gwCount{}
			counts[p.Gateway] = c
		}
		c.total++
		if p.Enabled {
			c.enabled++
		}
		return b.add(p)
	})
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	stat.Imported = b.n
	names := make([]string, 0, len(counts))
	for g := range counts {
		names = append(names, g)
	}
	sort.Strings(names)
	var parts []string
	for _, g := range names {
		parts = append(parts, fmt.Sprintf("%s %d (enabled %d)", g, counts[g].total, counts[g].enabled))
	}
	stat.Note = strings.Join(parts, " · ")
	return nil
}

// disabledPaymentPlugins returns U's payment plugin codes with is_enabled = 0.
func (im *importer) disabledPaymentPlugins() (map[string]bool, error) {
	out := map[string]bool{}
	if !im.src.has("v2_plugins") {
		return out, nil
	}
	err := im.src.each(im.ctx, "v2_plugins", "", nil, func(r row) error {
		if !r.bool("is_enabled") {
			out[strings.TrimSpace(r.str("code"))] = true
		}
		return nil
	})
	return out, err
}

// mapGateway returns the ggboard gateway id and config for an Xboard payment class (table of §4.9) and may
// disable p.
func (im *importer) mapGateway(scope, class string, cfg map[string]any, p *model.Payment) (string, model.JSONObject[string]) {
	get := func(k string) string { return strings.TrimSpace(str(cfg[k])) }
	out := model.JSONObject[string]{}
	notify := func(gw string) string {
		host := strings.TrimRight(im.appURL, "/")
		if p.NotifyDomain != "" {
			host = strings.TrimRight(p.NotifyDomain, "/")
		}
		if host == "" {
			host = "https://<host>"
		}
		return fmt.Sprintf("%s/api/v1/guest/payment/notify/%s/%s", host, gw, p.UUID)
	}
	switch {
	case class == "EPay":
		out["url"] = strings.TrimRight(get("url"), "/")
		out["pid"] = get("pid")
		out["key"] = get("key")
		out["type"] = get("type")
		switch out["type"] {
		case "", "alipay", "wxpay", "qqpay":
		default:
			im.warn(scope, "EPay type %q is not one of alipay/wxpay/qqpay — kept (provider-specific)", out["type"])
		}
		return "epay", out

	case class == "EpusdtPay":
		out["api_url"] = strings.TrimRight(get("epusdt_pay_url"), "/")
		out["api_token"] = get("epusdt_pay_apitoken")
		out["currency_type"] = get("currency_types")
		if out["currency_type"] == "" {
			out["currency_type"] = "usdt.trc20"
		}
		return "epusdt", out

	case class == "AlipayF2F":
		out["app_id"] = get("app_id")
		out["private_key"] = get("private_key")
		out["alipay_public_key"] = get("public_key")
		out["sandbox"] = "0"
		if get("product_name") != "" {
			im.warn(scope, "AlipayF2F product_name %q dropped: ggboard sends \"<app_name> - <plan>\" as the subject", get("product_name"))
		}
		return "alipay_f2f", out

	case isStripeClass(class):
		out["secret_key"] = get("stripe_sk_live")
		out["webhook_secret"] = get("stripe_webhook_key")
		out["currency"] = strings.ToLower(get("currency"))
		method := stripeClasses[class]
		if method == "" {
			switch strings.ToLower(get("payment_method")) {
			case "cards", "card":
				method = "card"
			case "alipay":
				method = "alipay"
			case "wechat_pay", "wechat":
				method = "wechat_pay"
			default:
				method = "auto"
			}
		}
		out["payment_methods"] = method
		if p.Enabled {
			p.Enabled = false
		}
		im.warn(scope, "Stripe imported disabled: point the webhook at %s, subscribe it to checkout.session.completed and checkout.session.async_payment_succeeded, then enable it", notify("stripe"))
		im.rep.Webhooks = append(im.rep.Webhooks, fmt.Sprintf("stripe #%d → %s", p.ID, notify("stripe")))
		if cur := out["currency"]; cur != "" && !strings.EqualFold(cur, im.currency) {
			im.warn(scope, "Stripe currency %s differs from the site currency %s: set exchange_rate (Xboard used a live FX rate)", strings.ToUpper(cur), im.currency)
		}
		return "stripe", out

	case class == "Coinbase":
		p.Enabled = false
		im.warn(scope, "Coinbase Commerce closed on 2026-03-31 and its API keys no longer work: imported disabled; create a Coinbase Business CDP key and a webhook to %s", notify("coinbase"))
		im.rep.Webhooks = append(im.rep.Webhooks, fmt.Sprintf("coinbase #%d → %s (new Coinbase Business credentials required)", p.ID, notify("coinbase")))
		return "coinbase", out

	case class == "BTCPay":
		out["server_url"] = strings.TrimRight(get("btcpay_url"), "/")
		out["store_id"] = get("btcpay_storeId")
		out["api_key"] = get("btcpay_api_key")
		out["webhook_secret"] = get("btcpay_webhook_key")
		im.warn(scope, "BTCPay webhook URL changes: update it to %s", notify("btcpay"))
		im.rep.Webhooks = append(im.rep.Webhooks, fmt.Sprintf("btcpay #%d → %s", p.ID, notify("btcpay")))
		return "btcpay", out
	}
	// CoinPayments, MGate and third-party plugins: kept for reference, disabled.
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out[k] = str(cfg[k])
	}
	gw := strings.ToLower(class)
	if gw == "" {
		gw = "unknown"
	}
	if len(gw) > 32 {
		gw = gw[:32]
	}
	p.Enabled = false
	im.warn(scope, "gateway %q is not available in ggboard; kept for reference, disabled", class)
	return gw, out
}

func isStripeClass(class string) bool {
	_, ok := stripeClasses[class]
	return ok
}
