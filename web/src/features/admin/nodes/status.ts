import type { NodeLoadValue, NodeStatus, NodeStatusState } from '@/lib/api/types'

/** Default window W = 2 × max(push, pull) with the 60 s defaults (NODE_PROTOCOLS §5.1). */
export const STATUS_WINDOW_MS = 120_000

/** `status.state` when the backend sends it, otherwise derived from the timestamps. */
export function nodeState(status: NodeStatus | null | undefined, now = Date.now()): NodeStatusState {
  if (!status) return 'offline'
  if (status.state) return status.state
  const within = (t: string | null) => t !== null && now - new Date(t).getTime() <= STATUS_WINDOW_MS
  if (within(status.last_push_at)) return 'online'
  if (within(status.last_check_at)) return 'idle'
  return 'offline'
}

/** Load entry → 0..1 (`cpu` percentage 0–100, or `{total, used}` byte pairs); `null` when unknown. */
export function loadRatio(value: NodeLoadValue | null | undefined): number | null {
  if (value === null || value === undefined) return null
  if (typeof value === 'number') return Math.max(0, Math.min(1, value / 100))
  if (typeof value === 'object' && value.total > 0) return Math.max(0, Math.min(1, value.used / value.total))
  return null
}
