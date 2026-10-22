/**
 * One-click import deep links for proxy clients, and query overrides for the subscription URL.
 * Pure functions (unit tested in importLinks.test.ts).
 *
 * The server detects the client from the User-Agent (docs/SUBSCRIPTION.md), so the links carry the plain
 * subscription URL; the user's saved rule profile applies automatically. `withSubscribeQuery` adds the
 * documented per-request overrides (`profile=`, `source=`, …) when the user picks "use template without saving".
 */

export type ImportClient = 'clash' | 'stash' | 'singbox' | 'shadowrocket' | 'surge' | 'quantumultx' | 'loon'

export interface ImportClientInfo {
  id: ImportClient
  /** Display name (product names are not translated). */
  name: string
  /** Platforms, for the hint under the button. */
  platforms: string
}

export const IMPORT_CLIENTS: readonly ImportClientInfo[] = [
  { id: 'clash', name: 'Clash / mihomo', platforms: 'Clash Verge · FlClash · CMFA' },
  { id: 'singbox', name: 'sing-box', platforms: 'SFA · SFI · SFM · Hiddify' },
  { id: 'shadowrocket', name: 'Shadowrocket', platforms: 'iOS' },
  { id: 'stash', name: 'Stash', platforms: 'iOS · macOS' },
  { id: 'surge', name: 'Surge', platforms: 'iOS · macOS' },
  { id: 'quantumultx', name: 'Quantumult X', platforms: 'iOS' },
  { id: 'loon', name: 'Loon', platforms: 'iOS' },
]

/** Clients that only take a pasted link (base64 share links, `v2ray` format). */
export const COPY_ONLY_CLIENTS = ['v2rayN', 'v2rayNG', 'NekoBox', 'Karing', 'Passwall'] as const

/** RFC 4648 §5 base64url without padding, UTF-8 safe. */
export function base64Url(text: string): string {
  const bytes = new TextEncoder().encode(text)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

/**
 * Node-list resources: Quantumult X `[server_remote]` and Loon node subscriptions (`loon://import?sub=`) expect
 * proxy lines only, while `/client/subscribe` renders a full config for those clients unless `list=1`
 * (docs/SUBSCRIPTION.md → overrides). The flag pins the format whatever User-Agent the client sends.
 */
export function nodeListUrl(url: string, flag: 'quantumultx' | 'loon'): string {
  return withSubscribeQuery(url, { flag, list: 1 })
}

/**
 * Builds the deep link that opens `client` and imports `url`.
 * `name` labels the profile where the client supports it (Clash, sing-box, Shadowrocket, Quantumult X).
 */
export function importLink(client: ImportClient, url: string, name = ''): string {
  const u = encodeURIComponent(url)
  const n = encodeURIComponent(name)
  switch (client) {
    case 'clash':
      return `clash://install-config?url=${u}${name ? `&name=${n}` : ''}`
    case 'stash':
      return `stash://install-config?url=${u}`
    case 'singbox':
      return `sing-box://import-remote-profile?url=${u}${name ? `#${n}` : ''}`
    case 'shadowrocket':
      return `shadowrocket://add/sub://${base64Url(url)}?remark=${n}`
    case 'surge':
      return `surge:///install-config?url=${u}`
    case 'quantumultx': {
      const list = nodeListUrl(url, 'quantumultx')
      const resource = { server_remote: [name ? `${list}, tag=${name}` : list] }
      return `quantumult-x:///add-resource?remote-resource=${encodeURIComponent(JSON.stringify(resource))}`
    }
    case 'loon':
      return `loon://import?sub=${encodeURIComponent(nodeListUrl(url, 'loon'))}`
  }
}

/** Per-request overrides understood by `/client/subscribe` (docs/SUBSCRIPTION.md → Rule profiles). */
export interface SubscribeOverrides {
  /** Template id (`none` | `minimal` | `standard` | `full`). */
  profile?: string
  source?: string
  /** Enable only these categories (template targets). */
  cats?: readonly string[]
  final?: string
  mirror?: 0 | 1
  /** Force the output format (`quantumultx`, `loon`, …) instead of User-Agent detection. */
  flag?: string
  /** `1` = proxy lines only (surge, surfboard, loon, quantumultx). */
  list?: 0 | 1
}

/** Adds/replaces override params on the subscription URL (keeps `token` and anything else). */
export function withSubscribeQuery(url: string, overrides: SubscribeOverrides = {}): string {
  const entries = Object.entries(overrides).filter(
    ([, v]) => v !== undefined && v !== '' && !(Array.isArray(v) && v.length === 0),
  )
  if (entries.length === 0) return url
  let parsed: URL
  try {
    parsed = new URL(url, typeof window === 'undefined' ? 'http://localhost' : window.location.origin)
  } catch {
    return url
  }
  for (const [key, value] of entries) {
    parsed.searchParams.set(key, Array.isArray(value) ? value.join(',') : String(value as string | number))
  }
  // Keep relative inputs relative (tests / same-origin short links).
  return /^[a-z][a-z\d+.-]*:/i.test(url)
    ? parsed.toString()
    : `${parsed.pathname}${parsed.search}${parsed.hash}`
}
