import type { GatewayField, Payment, PaymentGateway } from '@/lib/api/types'

/** Gateways whose provider must be configured with our notify URL as a webhook endpoint. */
export const WEBHOOK_GATEWAYS: ReadonlySet<string> = new Set(['stripe', 'coinbase', 'btcpay'])

/** Built-in gateways that have a help text (`admin-payments:help.<name>`). */
export const KNOWN_GATEWAYS = ['epay', 'epusdt', 'alipay_f2f', 'stripe', 'coinbase', 'btcpay'] as const

/**
 * Suggestions for free-text fields when the backend sends no `options` (docs/PAYMENTS_GATEWAYS.md:
 * EPay `type` and Epusdt `currency_type` accept any provider value).
 */
const SUGGESTIONS: Record<string, readonly string[]> = {
  'epay.type': ['alipay', 'wxpay', 'qqpay', 'usdt.tron'],
  'epusdt.currency_type': [
    'usdt.trc20',
    'usdc.trc20',
    'tron.trx',
    'usdt.erc20',
    'usdt.polygon',
    'usdt.bep20',
    'usdt.solana',
    'usdt.ton',
    'usdt.arbitrum',
    'usdc.base',
  ],
  'stripe.payment_methods': ['auto', 'card', 'alipay', 'wechat_pay'],
  'alipay_f2f.sandbox': ['0', '1'],
}

export function fieldSuggestions(gateway: string, field: GatewayField): readonly string[] {
  return field.options?.length ? field.options : (SUGGESTIONS[`${gateway}.${field.key}`] ?? [])
}

/** Default config for a newly selected gateway (`Field.default`). */
export function defaultConfig(gateway: PaymentGateway | undefined): Record<string, string> {
  const out: Record<string, string> = {}
  for (const f of gateway?.fields ?? []) out[f.key] = f.default ?? ''
  return out
}

/** Payment config values are strings on the wire (`JSONObject[string]`). */
export function configToStrings(config: Record<string, unknown> | null | undefined): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(config ?? {})) {
    out[k] = v === null || v === undefined ? '' : typeof v === 'string' ? v : JSON.stringify(v)
  }
  return out
}

/** Problems with a gateway config before sending it (required + numeric fields). */
export function validateConfig(
  fields: readonly GatewayField[],
  config: Record<string, string>,
): Record<string, 'required' | 'number'> {
  const errors: Record<string, 'required' | 'number'> = {}
  for (const f of fields) {
    const v = (config[f.key] ?? '').trim()
    if (f.required && !v) errors[f.key] = 'required'
    else if (f.type === 'number' && v && !Number.isFinite(Number(v))) errors[f.key] = 'number'
  }
  return errors
}

/** `<base>/api/v1/guest/payment/notify/<gateway>/<uuid>` (docs/PAYMENTS_GATEWAYS.md §1). */
export function notifyUrlFor(
  payment: Pick<Payment, 'notify_url' | 'notify_domain' | 'gateway' | 'uuid'>,
  appUrl: string,
) {
  if (payment.notify_url) return payment.notify_url
  const base = (
    payment.notify_domain ||
    appUrl ||
    (typeof window !== 'undefined' ? window.location.origin : '')
  ).replace(/\/+$/, '')
  return `${base}/api/v1/guest/payment/notify/${payment.gateway}/${payment.uuid}`
}
