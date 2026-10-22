import type { AdminLogEntry, AdminLogQuery } from '@/lib/api/types'

export const LOG_LEVELS = ['all', 'debug', 'info', 'warn', 'error'] as const
export type LogLevelFilter = (typeof LOG_LEVELS)[number]
export const LOG_LIMITS = [100, 200, 500, 1000] as const

/** Query sent to `GET /admin/system/logs` for the toolbar state. */
export function logsQuery(level: LogLevelFilter, limit: number): AdminLogQuery {
  return level === 'all' ? { limit } : { level, limit }
}

/** Client-side text filter over message and attributes. */
export function filterLogs(entries: readonly AdminLogEntry[], text: string | undefined): AdminLogEntry[] {
  const q = text?.trim().toLowerCase()
  if (!q) return [...entries]
  return entries.filter(
    (e) =>
      e.msg.toLowerCase().includes(q) ||
      (e.attrs ? JSON.stringify(e.attrs).toLowerCase().includes(q) : false),
  )
}
