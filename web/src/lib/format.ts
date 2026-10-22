/**
 * Intl-based formatters (no dayjs/moment). All accept an explicit locale; components use the
 * store locale via `useLocale()`.
 *
 * Units: money in cents, traffic in bytes, plan traffic in GB, time as RFC3339 strings.
 */
import type { Locale } from '@/lib/api/types'

const cache = new Map<string, Intl.NumberFormat | Intl.DateTimeFormat | Intl.RelativeTimeFormat>()
function memo<T extends Intl.NumberFormat | Intl.DateTimeFormat | Intl.RelativeTimeFormat>(key: string, make: () => T): T {
  let fmt = cache.get(key) as T | undefined
  if (!fmt) {
    fmt = make()
    cache.set(key, fmt)
  }
  return fmt
}

/** Currencies without minor units in ISO 4217 that panels commonly use. */
const ZERO_DECIMAL = new Set(['JPY', 'KRW', 'VND', 'IDR', 'CLP', 'TWD'])

/**
 * Formats integer cents as currency, e.g. `formatMoney(1500, 'CNY', 'zh-CN')` → `¥15.00`.
 * Pass `symbol` to override the Intl symbol with the site's `currency_symbol`.
 */
export function formatMoney(
  cents: number,
  currency = 'CNY',
  locale: Locale = 'zh-CN',
  opts: { symbol?: string; compact?: boolean; trimZeros?: boolean } = {},
): string {
  const code = currency.toUpperCase()
  const value = ZERO_DECIMAL.has(code) ? cents : cents / 100
  const digits = ZERO_DECIMAL.has(code) ? 0 : 2
  const minimumFractionDigits = opts.trimZeros && Number.isInteger(value) ? 0 : digits
  let fmt: Intl.NumberFormat
  try {
    fmt = memo(`money:${locale}:${code}:${minimumFractionDigits}:${opts.compact ? 1 : 0}`, () =>
      new Intl.NumberFormat(locale, {
        style: 'currency',
        currency: code,
        minimumFractionDigits,
        maximumFractionDigits: digits,
        ...(opts.compact ? { notation: 'compact' as const } : {}),
      }),
    )
  } catch {
    // Unknown currency code: fall back to plain number + symbol.
    return `${opts.symbol ?? ''}${formatNumber(value, locale, { minimumFractionDigits, maximumFractionDigits: digits })}`
  }
  if (opts.symbol === undefined) return fmt.format(value)
  return fmt
    .formatToParts(value)
    .map((p) => (p.type === 'currency' ? opts.symbol : p.value))
    .join('')
}

export function formatNumber(value: number, locale: Locale = 'zh-CN', opts: Intl.NumberFormatOptions = {}): string {
  return memo(`num:${locale}:${JSON.stringify(opts)}`, () => new Intl.NumberFormat(locale, opts)).format(value)
}

export function formatPercent(ratio: number, locale: Locale = 'zh-CN', digits = 0): string {
  return formatNumber(ratio, locale, { style: 'percent', maximumFractionDigits: digits })
}

const BYTE_UNITS = ['byte', 'kilobyte', 'megabyte', 'gigabyte', 'terabyte', 'petabyte'] as const

/**
 * Binary (1024) traffic formatting with Intl unit names, e.g. `formatBytes(1610612736)` → `1.5 GB`.
 * Proxy panels conventionally use 1024-based units labelled KB/MB/GB.
 */
export function formatBytes(bytes: number, locale: Locale = 'zh-CN', opts: { digits?: number } = {}): string {
  if (!Number.isFinite(bytes)) return '—'
  const sign = bytes < 0 ? -1 : 1
  let value = Math.abs(bytes)
  let unit = 0
  while (value >= 1024 && unit < BYTE_UNITS.length - 1) {
    value /= 1024
    unit++
  }
  const digits = opts.digits ?? (unit === 0 ? 0 : value >= 100 ? 0 : value >= 10 ? 1 : 2)
  return memo(`bytes:${locale}:${unit}:${digits}`, () =>
    new Intl.NumberFormat(locale, {
      style: 'unit',
      unit: BYTE_UNITS[unit],
      unitDisplay: 'short',
      maximumFractionDigits: digits,
    }),
  ).format(sign * value)
}

/** Plan traffic is stored in GB (as entered). */
export const gbToBytes = (gb: number) => gb * 1024 ** 3

export function toDate(value: string | number | Date | null | undefined): Date | null {
  if (value === null || value === undefined || value === '') return null
  const date = value instanceof Date ? value : new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

export type DateStyle = 'date' | 'datetime' | 'time' | 'short' | 'long'
const DATE_OPTIONS: Record<DateStyle, Intl.DateTimeFormatOptions> = {
  date: { year: 'numeric', month: '2-digit', day: '2-digit' },
  datetime: { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false },
  time: { hour: '2-digit', minute: '2-digit', hour12: false },
  short: { month: 'short', day: 'numeric' },
  long: { dateStyle: 'long', timeStyle: 'short' },
}

export function formatDate(
  value: string | number | Date | null | undefined,
  locale: Locale = 'zh-CN',
  style: DateStyle = 'datetime',
  timeZone?: string,
): string {
  const date = toDate(value)
  if (!date) return '—'
  return memo(`date:${locale}:${style}:${timeZone ?? ''}`, () =>
    new Intl.DateTimeFormat(locale, { ...DATE_OPTIONS[style], ...(timeZone ? { timeZone } : {}) }),
  ).format(date)
}

const RELATIVE_STEPS: Array<[Intl.RelativeTimeFormatUnit, number]> = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
  ['second', 1],
]

/** "3 天后" / "2 hours ago". */
export function formatRelative(
  value: string | number | Date | null | undefined,
  locale: Locale = 'zh-CN',
  now: number = Date.now(),
): string {
  const date = toDate(value)
  if (!date) return '—'
  const diff = (date.getTime() - now) / 1000
  const rtf = memo(`rel:${locale}`, () => new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }))
  for (const [unit, seconds] of RELATIVE_STEPS) {
    if (Math.abs(diff) >= seconds || unit === 'second') return rtf.format(Math.round(diff / seconds), unit)
  }
  return rtf.format(0, 'second')
}

/** Whole days until `value` (negative when past). */
export function daysUntil(value: string | Date | null | undefined, now: number = Date.now()): number | null {
  const date = toDate(value)
  return date ? Math.ceil((date.getTime() - now) / 86_400_000) : null
}
