import type { TrafficLog } from '@/lib/api/types'

export interface DayTraffic {
  /** `YYYY-MM-DD` */
  date: string
  u: number
  d: number
  total: number
}

/** Local calendar date as `YYYY-MM-DD`. */
export function isoDay(date: Date): string {
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

function shiftDay(day: string, delta: number): string {
  const [y = 0, m = 1, d = 1] = day.split('-').map(Number)
  return isoDay(new Date(y, m - 1, d + delta))
}

/**
 * Sums per-node traffic logs (`record_at` = `YYYY-MM-DD`) into the last `days` calendar days, oldest
 * first, with missing days as zero. The window ends today, or on the newest record when the server's
 * date is ahead of the browser's (time zones).
 */
export function lastDays(logs: readonly TrafficLog[], days = 7, now = new Date()): DayTraffic[] {
  const today = isoDay(now)
  const newest = logs.reduce((max, l) => (l.record_at > max ? l.record_at : max), today)
  const end = newest > today ? newest : today
  const sums = new Map<string, { u: number; d: number }>()
  for (const log of logs) {
    const day = log.record_at.slice(0, 10)
    const sum = sums.get(day) ?? { u: 0, d: 0 }
    sum.u += log.u
    sum.d += log.d
    sums.set(day, sum)
  }
  const out: DayTraffic[] = []
  for (let i = days - 1; i >= 0; i--) {
    const date = shiftDay(end, -i)
    const sum = sums.get(date) ?? { u: 0, d: 0 }
    out.push({ date, u: sum.u, d: sum.d, total: sum.u + sum.d })
  }
  return out
}

/** A "nice" axis maximum (1, 2, 2.5, 5 × 1024ⁿ steps) at or above `value`. */
export function niceByteMax(value: number): number {
  if (value <= 0) return 1024 ** 3
  const unit = 1024 ** Math.floor(Math.log(value) / Math.log(1024))
  const scaled = value / unit
  const step = [1, 2, 2.5, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000, 1024].find((s) => s >= scaled) ?? 1024
  return step * unit
}
