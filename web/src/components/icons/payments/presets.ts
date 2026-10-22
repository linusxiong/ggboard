/**
 * Preset payment-method icons and the `Payment.icon` value format.
 *
 * `Payment.icon` stays a free string (docs/API.md → payments):
 *   ''                    no icon (neutral fallback glyph)
 *   'preset:<id>'         a built-in mark from PAYMENT_ICON_PRESETS (id = [a-z0-9_-]{1,32})
 *   'https://…', '/…'     an image URL (absolute or same-origin path; `//host/…` too)
 *   'data:image/…'        an inline image
 *   '💳'                  a single emoji / symbol
 * Unknown presets and anything else render the fallback glyph, so old values never break.
 */
import type { ComponentType } from 'react'
import type { Locale, PaymentIconValue } from '@/lib/api/types'
import {
  AlipayGlyph,
  BankGlyph,
  BitcoinGlyph,
  BtcpayGlyph,
  CardGlyph,
  CoinbaseGlyph,
  CryptoGlyph,
  EpayGlyph,
  EpusdtGlyph,
  EthGlyph,
  type PaymentGlyphProps,
  PaypalGlyph,
  QrcodeGlyph,
  StripeGlyph,
  UsdtGlyph,
  WalletGlyph,
  WechatGlyph,
} from './glyphs'

export const PAYMENT_ICON_IDS = [
  'alipay',
  'wechat',
  'stripe',
  'card',
  'paypal',
  'crypto',
  'btc',
  'usdt',
  'eth',
  'bank',
  'qrcode',
  'wallet',
  'epay',
  'epusdt',
  'btcpay',
  'coinbase',
] as const

export type PaymentIconId = (typeof PAYMENT_ICON_IDS)[number]

export interface PaymentIconPreset {
  id: PaymentIconId
  /** Display name per locale (the admin picker caption). */
  name: Readonly<Record<Locale, string>>
  Component: ComponentType<PaymentGlyphProps>
  /** Gateway ids (`Payment.gateway`) that suggest this icon when an admin adds a payment. */
  defaultFor?: readonly string[]
}

/** In display order (the admin grid). */
export const PAYMENT_ICON_PRESETS: readonly PaymentIconPreset[] = [
  {
    id: 'alipay',
    name: { 'zh-CN': '支付宝', en: 'Alipay' },
    Component: AlipayGlyph,
    defaultFor: ['alipay_f2f'],
  },
  { id: 'wechat', name: { 'zh-CN': '微信支付', en: 'WeChat Pay' }, Component: WechatGlyph },
  { id: 'stripe', name: { 'zh-CN': 'Stripe', en: 'Stripe' }, Component: StripeGlyph, defaultFor: ['stripe'] },
  { id: 'card', name: { 'zh-CN': '银行卡', en: 'Card' }, Component: CardGlyph },
  { id: 'paypal', name: { 'zh-CN': 'PayPal', en: 'PayPal' }, Component: PaypalGlyph },
  { id: 'crypto', name: { 'zh-CN': '加密货币', en: 'Crypto' }, Component: CryptoGlyph },
  { id: 'btc', name: { 'zh-CN': '比特币', en: 'Bitcoin' }, Component: BitcoinGlyph },
  { id: 'usdt', name: { 'zh-CN': 'USDT', en: 'USDT' }, Component: UsdtGlyph, defaultFor: ['epusdt'] },
  { id: 'eth', name: { 'zh-CN': '以太坊', en: 'Ethereum' }, Component: EthGlyph },
  { id: 'bank', name: { 'zh-CN': '银行转账', en: 'Bank transfer' }, Component: BankGlyph },
  { id: 'qrcode', name: { 'zh-CN': '扫码支付', en: 'QR code' }, Component: QrcodeGlyph },
  { id: 'wallet', name: { 'zh-CN': '余额 / 钱包', en: 'Wallet' }, Component: WalletGlyph },
  { id: 'epay', name: { 'zh-CN': '易支付', en: 'EPay' }, Component: EpayGlyph, defaultFor: ['epay'] },
  { id: 'epusdt', name: { 'zh-CN': 'Epusdt', en: 'Epusdt' }, Component: EpusdtGlyph },
  { id: 'btcpay', name: { 'zh-CN': 'BTCPay', en: 'BTCPay' }, Component: BtcpayGlyph, defaultFor: ['btcpay'] },
  {
    id: 'coinbase',
    name: { 'zh-CN': 'Coinbase', en: 'Coinbase' },
    Component: CoinbaseGlyph,
    defaultFor: ['coinbase'],
  },
]

const BY_ID: ReadonlyMap<string, PaymentIconPreset> = new Map(PAYMENT_ICON_PRESETS.map((p) => [p.id, p]))

export const PRESET_PREFIX = 'preset:'
const PRESET_RE = /^preset:[a-z0-9_-]{1,32}$/

export function findPaymentIconPreset(id: string): PaymentIconPreset | undefined {
  return BY_ID.get(id)
}

/** `'alipay'` → `'preset:alipay'` (the stored `Payment.icon`). */
export function presetIconValue(id: PaymentIconId): PaymentIconValue {
  return `${PRESET_PREFIX}${id}`
}

/** The preset a gateway suggests by default (`defaultFor`), if any. */
export function presetForGateway(gateway: string): PaymentIconPreset | undefined {
  return PAYMENT_ICON_PRESETS.find((p) => p.defaultFor?.includes(gateway))
}

export type ParsedPaymentIcon =
  | { kind: 'none' }
  /** `preset` is undefined for a well-formed id this build does not know (fallback glyph). */
  | { kind: 'preset'; id: string; preset: PaymentIconPreset | undefined }
  | { kind: 'url'; src: string }
  | { kind: 'emoji'; text: string }

const URL_RE = /^(?:https?:)?\/\//i

/** Classifies a `Payment.icon` value (see the module comment). */
export function parsePaymentIcon(value: PaymentIconValue | null | undefined): ParsedPaymentIcon {
  const icon = (value ?? '').trim()
  if (!icon) return { kind: 'none' }
  if (icon.startsWith(PRESET_PREFIX)) {
    if (!PRESET_RE.test(icon)) return { kind: 'none' }
    const id = icon.slice(PRESET_PREFIX.length)
    return { kind: 'preset', id, preset: BY_ID.get(id) }
  }
  if (URL_RE.test(icon) || icon.startsWith('/') || /^data:image\//i.test(icon)) {
    return /\s/.test(icon) ? { kind: 'none' } : { kind: 'url', src: icon }
  }
  if (isSingleSymbol(icon)) return { kind: 'emoji', text: icon }
  return { kind: 'none' }
}

/** One grapheme (an emoji incl. ZWJ sequences / flags, or a symbol such as ¥), no letters. */
function isSingleSymbol(text: string): boolean {
  if (/[A-Za-z\s]/.test(text) || /\p{Cc}/u.test(text)) return false
  if (typeof Intl !== 'undefined' && 'Segmenter' in Intl) {
    return [...new Intl.Segmenter(undefined, { granularity: 'grapheme' }).segment(text)].length === 1
  }
  return Array.from(text).length <= 2
}

/**
 * Whether a value is accepted by the backend (`internal/api/admin/payments.go` → `validPaymentIcon`):
 * empty, `preset:<id>`, an http(s) URL with a host, a `/path`, a `data:image/` URI, or a short symbol
 * (≤ 16 code points, no ASCII letters, no whitespace / control characters; here also a single grapheme so
 * the preview matches what users see). At most 255 characters.
 */
export function isValidPaymentIcon(value: string): boolean {
  const icon = value.trim()
  if (!icon) return true
  if (icon.length > 255) return false
  if (icon.startsWith(PRESET_PREFIX)) return PRESET_RE.test(icon)
  if (/\s/.test(icon) || /\p{Cc}/u.test(icon)) return false
  if (/^https?:\/\//i.test(icon)) return /^https?:\/\/[^/?#\s]+/i.test(icon) && canParseUrl(icon)
  if (icon.startsWith('/') || /^data:image\//i.test(icon)) return true
  // The backend allows up to 16 code points; the admin UI only offers what renders (one grapheme).
  return Array.from(icon).length <= 16 && isSingleSymbol(icon)
}

function canParseUrl(value: string): boolean {
  try {
    return new URL(value).host !== ''
  } catch {
    return false
  }
}
