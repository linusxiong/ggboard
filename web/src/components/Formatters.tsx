/**
 * Display components for money / traffic / dates that read the current locale from the store.
 * They render semantic elements (`<data>`, `<time>`) with machine-readable values.
 */
import type { Locale } from '@/lib/api/types'
import { useLocale } from '@/i18n'
import {
  type DateStyle,
  formatBytes,
  formatDate,
  formatMoney,
  formatRelative,
  gbToBytes,
  toDate,
} from '@/lib/format'
import { cx } from '@/lib/cn'

interface MoneyProps {
  /** Integer cents. */
  cents: number
  /** ISO currency (site `currency`). Default CNY. */
  currency?: string
  /** Site `currency_symbol` overrides the Intl symbol. */
  symbol?: string
  trimZeros?: boolean
  className?: string
  locale?: Locale
}
export function Money({ cents, currency = 'CNY', symbol, trimZeros, className, locale }: MoneyProps) {
  const current = useLocale()
  return (
    <data value={(cents / 100).toFixed(2)} className={cx('tabular', className)}>
      {formatMoney(cents, currency, locale ?? current, {
        ...(symbol !== undefined ? { symbol } : {}),
        ...(trimZeros ? { trimZeros } : {}),
      })}
    </data>
  )
}

interface TrafficProps {
  /** Bytes (or GB when `unit="gb"`, e.g. `Plan.transfer_enable`). */
  value: number
  unit?: 'bytes' | 'gb'
  digits?: number
  className?: string
}
export function Traffic({ value, unit = 'bytes', digits, className }: TrafficProps) {
  const locale = useLocale()
  const bytes = unit === 'gb' ? gbToBytes(value) : value
  return (
    <data value={bytes} className={cx('tabular', className)}>
      {formatBytes(bytes, locale, digits !== undefined ? { digits } : {})}
    </data>
  )
}

interface DateTimeProps {
  value: string | number | Date | null | undefined
  /** `relative` → "3 天后". */
  format?: DateStyle | 'relative'
  /** Text when the value is null (e.g. "永不过期"). */
  fallback?: string
  className?: string
}
export function DateTime({ value, format = 'datetime', fallback = '—', className }: DateTimeProps) {
  const locale = useLocale()
  const date = toDate(value)
  if (!date) return <span className={className}>{fallback}</span>
  return (
    <time
      dateTime={date.toISOString()}
      title={formatDate(date, locale, 'long')}
      className={cx('tabular', className)}
    >
      {format === 'relative' ? formatRelative(date, locale) : formatDate(date, locale, format)}
    </time>
  )
}
