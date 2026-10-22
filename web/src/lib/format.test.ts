import { describe, expect, it } from 'vitest'
import {
  daysUntil,
  formatBytes,
  formatDate,
  formatMoney,
  formatPercent,
  formatRelative,
  gbToBytes,
} from './format'

describe('formatters', () => {
  it('formats cents as currency', () => {
    expect(formatMoney(1500, 'CNY', 'zh-CN')).toBe('¥15.00')
    expect(formatMoney(1500, 'CNY', 'zh-CN', { trimZeros: true })).toBe('¥15')
    expect(formatMoney(123456, 'USD', 'en')).toBe('$1,234.56')
    expect(formatMoney(1000, 'JPY', 'en')).toBe('¥1,000')
    expect(formatMoney(990, 'USD', 'en', { symbol: 'US$' })).toBe('US$9.90')
  })

  it('formats traffic with binary units', () => {
    expect(formatBytes(0, 'en')).toBe('0 byte')
    expect(formatBytes(1536, 'en')).toBe('1.5 kB')
    expect(formatBytes(gbToBytes(1.5), 'en')).toBe('1.5 GB')
    expect(formatBytes(gbToBytes(500), 'en')).toBe('500 GB')
    expect(formatBytes(Number.NaN, 'en')).toBe('—')
  })

  it('formats dates in a fixed time zone', () => {
    expect(formatDate('2027-01-01T00:00:00Z', 'en', 'date', 'UTC')).toBe('01/01/2027')
    expect(formatDate('2027-01-01T08:30:00Z', 'zh-CN', 'datetime', 'UTC')).toBe('2027/01/01 08:30')
    expect(formatDate(null)).toBe('—')
    expect(formatDate('garbage')).toBe('—')
  })

  it('formats relative time and percentages', () => {
    const now = Date.parse('2026-10-21T00:00:00Z')
    expect(formatRelative('2026-10-24T00:00:00Z', 'en', now)).toBe('in 3 days')
    expect(formatRelative('2026-10-20T22:00:00Z', 'en', now)).toBe('2 hours ago')
    expect(daysUntil('2026-10-31T00:00:00Z', now)).toBe(10)
    expect(daysUntil(null, now)).toBeNull()
    expect(formatPercent(0.256, 'en')).toBe('26%')
  })
})
