/** Pure dashboard helpers (kept out of component files for fast refresh). */
import type { NodeType, UserSubscription } from '@/lib/api/types'

/** Protocol names are product names (not translated). */
export const NODE_TYPE_LABEL: Record<NodeType, string> = {
  vmess: 'VMess',
  vless: 'VLESS',
  shadowsocks: 'Shadowsocks',
  shadowsocksr: 'ShadowsocksR',
  trojan: 'Trojan',
  hysteria: 'Hysteria',
  anytls: 'AnyTLS',
  mieru: 'Mieru',
  tuic: 'TUIC',
}

/** Plain-text excerpt of a Markdown notice (no renderer needed for a one-line preview). */
export function excerpt(markdown: string, max = 90): string {
  const text = markdown
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, ' ')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/<[^>]+>/g, ' ')
    .replace(/[#>*_`~|-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  return text.length > max ? `${text.slice(0, max).trimEnd()}…` : text
}

export type PlanState = 'active' | 'expired' | 'depleted' | 'none'

export function planState(sub: UserSubscription, now = Date.now()): PlanState {
  if (!sub.plan) return 'none'
  if (sub.expired_at && new Date(sub.expired_at).getTime() <= now) return 'expired'
  if (sub.transfer_enable > 0 && sub.u + sub.d >= sub.transfer_enable) return 'depleted'
  return 'active'
}
