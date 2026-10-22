import { useMemo } from 'react'
import { useLocale } from '@/i18n'
import { formatBytes, formatDate, formatMoney, formatNumber } from '@/lib/format'
import { useSite } from './useSite'

/** Locale + site-currency bound formatters for admin tables and charts. */
export function useFormatters() {
  const locale = useLocale()
  const { currency, symbol } = useSite()
  return useMemo(
    () => ({
      locale,
      currency,
      symbol,
      money: (cents: number) => formatMoney(cents, currency, locale, symbol ? { symbol } : {}),
      moneyCompact: (cents: number) =>
        formatMoney(cents, currency, locale, {
          compact: true,
          trimZeros: true,
          ...(symbol ? { symbol } : {}),
        }),
      count: (n: number) => formatNumber(n, locale, { maximumFractionDigits: 0 }),
      countCompact: (n: number) => formatNumber(n, locale, { notation: 'compact', maximumFractionDigits: 1 }),
      bytes: (b: number) => formatBytes(b, locale),
      bytesCompact: (b: number) => formatBytes(b, locale, { digits: 0 }),
      /** `YYYY-MM-DD` → "10月22日" / "Oct 22" (parsed as a local calendar date). */
      shortDate: (iso: string) => formatDate(`${iso}T00:00:00`, locale, 'short'),
      date: (iso: string) => formatDate(iso.length === 10 ? `${iso}T00:00:00` : iso, locale, 'date'),
      dateTime: (iso: string | null | undefined) => formatDate(iso, locale, 'datetime'),
    }),
    [locale, currency, symbol],
  )
}

export type Formatters = ReturnType<typeof useFormatters>
