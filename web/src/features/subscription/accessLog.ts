/** Subscription access log helpers (user security card + admin drawer). */
import type { SubscriptionAccess, SubscriptionFormatInfo } from '@/lib/api/types'

/** Newest first (the server already sorts; this keeps the UI stable if it does not). */
export const sortAccess = (log: readonly SubscriptionAccess[]) =>
  [...log].sort((a, b) => Date.parse(b.at) - Date.parse(a.at))

/** Distinct IPs that fetched the subscription within `hours` before `now`. */
export function recentIpCount(log: readonly SubscriptionAccess[], now: number, hours = 24): number {
  const since = now - hours * 3_600_000
  return new Set(log.filter((e) => Date.parse(e.at) >= since).map((e) => e.ip)).size
}

/** Short client label: product token of the User-Agent (`clash-verge/v2.2.3` → `clash-verge/v2.2.3`). */
export function clientLabel(ua: string): string {
  const trimmed = ua.trim()
  if (!trimmed || trimmed === 'unknown') return ''
  return trimmed.length > 48 ? `${trimmed.slice(0, 47)}…` : trimmed
}

/** Format id → display name for the formats that can carry age (mihomo YAML). */
const FORMAT_NAMES: Record<string, string> = { 'clash-meta': 'Clash Meta (mihomo)' }
const DEFAULT_AGE_FORMATS = ['clash-meta']

/**
 * Display names of the formats served encrypted: the age view's `formats` ids, else `GET /user/subscription` →
 * `formats[].supports_age`, else the contract default (mihomo only).
 */
export function ageClients(
  ids: readonly string[] | undefined,
  formats?: readonly SubscriptionFormatInfo[],
): string[] {
  if (ids && ids.length > 0)
    return ids.map((id) => FORMAT_NAMES[id] ?? formats?.find((f) => f.id === id)?.name ?? id)
  if (formats) return formats.filter((f) => f.supports_age).map((f) => f.name || f.id)
  return DEFAULT_AGE_FORMATS.map((id) => FORMAT_NAMES[id] ?? id)
}

/**
 * Minimal mihomo profile that downloads the subscription through an encrypted `proxy-providers` entry (Clash Verge
 * Rev, plain mihomo). Providers import the `proxies` only, so it carries its own group and catch-all rule.
 */
export function ageSnippet(url: string | undefined): string {
  return [
    'proxy-providers:',
    '  ggboard:',
    '    type: http',
    `    url: "${url ?? 'https://…'}"`,
    '    interval: 3600',
    '    age-secret-key: AGE-SECRET-KEY-1…',
    'proxy-groups:',
    '  - name: Proxy',
    '    type: select',
    '    use: [ggboard]',
    'rules:',
    '  - MATCH,Proxy',
  ].join('\n')
}
