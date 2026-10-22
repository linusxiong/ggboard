import { describe, expect, it } from 'vitest'
import type { TrafficLog } from '@/lib/api/types'
import { isoDay, lastDays, niceByteMax } from './traffic'

const log = (record_at: string, u: number, d: number, node_name: string | null = 'HK'): TrafficLog => ({
  record_at,
  node_id: 1,
  node_name,
  rate: 1,
  u,
  d,
})

describe('lastDays', () => {
  const now = new Date(2026, 9, 22, 15, 0)

  it('sums nodes per day, oldest first, zero-filling gaps', () => {
    const days = lastDays(
      [log('2026-10-22', 1, 2), log('2026-10-22', 3, 4, null), log('2026-10-20', 10, 0)],
      7,
      now,
    )
    expect(days.map((d) => d.date)).toEqual([
      '2026-10-16',
      '2026-10-17',
      '2026-10-18',
      '2026-10-19',
      '2026-10-20',
      '2026-10-21',
      '2026-10-22',
    ])
    expect(days[6]).toEqual({ date: '2026-10-22', u: 4, d: 6, total: 10 })
    expect(days[4]?.total).toBe(10)
    expect(days[5]?.total).toBe(0)
  })

  it('ignores records outside the window and follows a server date ahead of the browser', () => {
    const days = lastDays([log('2026-10-01', 5, 5), log('2026-10-23', 1, 1)], 7, now)
    expect(days.at(-1)?.date).toBe('2026-10-23')
    expect(days.reduce((s, d) => s + d.total, 0)).toBe(2)
  })

  it('isoDay uses the local calendar date', () => {
    expect(isoDay(new Date(2026, 0, 5, 23, 59))).toBe('2026-01-05')
  })
})

describe('niceByteMax', () => {
  it('rounds up to a readable axis maximum', () => {
    expect(niceByteMax(0)).toBe(1024 ** 3)
    expect(niceByteMax(3 * 1024 ** 3)).toBe(5 * 1024 ** 3)
    expect(niceByteMax(700 * 1024 ** 2)).toBe(1000 * 1024 ** 2)
    expect(niceByteMax(26 * 1024 ** 3)).toBe(50 * 1024 ** 3)
  })
})
