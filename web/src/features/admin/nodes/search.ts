/** `/admin/nodes` URL filters (kept tiny: route search validation ships with the route tree). */
import type { NodeStatusState, NodeType } from '@/lib/api/types'

export interface NodeSearch {
  type?: NodeType
  group?: number
  status?: NodeStatusState
  q?: string
}

const TYPES = new Set([
  'vmess',
  'vless',
  'shadowsocks',
  'shadowsocksr',
  'trojan',
  'hysteria',
  'anytls',
  'mieru',
  'tuic',
])
const STATES = new Set(['online', 'idle', 'offline'])

export function parseNodeSearch(raw: Record<string, unknown>): NodeSearch {
  const out: NodeSearch = {}
  if (typeof raw.type === 'string' && TYPES.has(raw.type)) out.type = raw.type as NodeType
  const group = Number(raw.group)
  if (Number.isInteger(group) && group > 0) out.group = group
  if (typeof raw.status === 'string' && STATES.has(raw.status)) out.status = raw.status as NodeStatusState
  if (typeof raw.q === 'string' && raw.q.trim()) out.q = raw.q.trim().slice(0, 100)
  return out
}
