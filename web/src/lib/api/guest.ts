/** Public endpoints (`/guest`, 🔓) — docs/API.md §Guest. */
import { API_BASE, api, buildQuery } from './client'
import type { GuestConfig, Plan, RulesetCatalog, SubscribeQuery } from './types'

/** `GET /guest/config` — site name, currency, register rules, captcha, downloads, locales. */
export const config = (signal?: AbortSignal) => api.get<GuestConfig>('/guest/config', undefined, { signal })

/** `GET /guest/plans` — plans with show=true & sell=true. */
export const plans = (signal?: AbortSignal) => api.get<Plan[]>('/guest/plans', undefined, { signal })

/** `GET /guest/rulesets/catalog` — rule-set catalog (docs/SUBSCRIPTION.md). */
export const rulesetCatalog = (signal?: AbortSignal) =>
  api.get<RulesetCatalog>('/guest/rulesets/catalog', undefined, { signal })

/**
 * `GET /guest/payment/return/:trade_no` — browser redirect target after paying (302 → `/orders/:trade_no`).
 * Not fetched by the SPA; exposed as a URL builder for completeness.
 */
export const paymentReturnUrl = (tradeNo: string) =>
  `${API_BASE}/guest/payment/return/${encodeURIComponent(tradeNo)}`

/**
 * `ANY /guest/payment/notify/:gateway/:uuid` — gateway → server callback (signature verified server-side).
 * Never called by the browser; the admin payments page shows this URL.
 */
export const paymentNotifyUrl = (gateway: string, uuid: string, origin = '') =>
  `${origin}${API_BASE}/guest/payment/notify/${encodeURIComponent(gateway)}/${encodeURIComponent(uuid)}`

/**
 * `GET /api/v1/client/subscribe?token=…` (🔓 token) — subscription URL builder for clients.
 * Also mounted at `/s/:token` (short link).
 */
export function clientSubscribeUrl(token: string, query: SubscribeQuery = {}, origin = ''): string {
  return `${origin}${API_BASE}/client/subscribe${buildQuery({ token, ...query })}`
}

/** `/s/:token` short subscription link. */
export const shortSubscribeUrl = (token: string, origin = '') => `${origin}/s/${encodeURIComponent(token)}`
