/**
 * Site-wide settings the commerce pages need (currency, symbol, public URL), read from
 * `/guest/config` through React Query (authenticated area only).
 */
import { useQuery } from '@tanstack/react-query'
import { useCallback } from 'react'
import { useLocale } from '@/i18n'
import { guestConfigQuery } from '@/lib/api/queries'
import { formatMoney } from '@/lib/format'

export interface SiteInfo {
  currency: string
  symbol: string | undefined
  /** Public panel URL without a trailing slash (`app_url`, else the current origin). */
  appUrl: string
  appName: string
}

export function useSite(): SiteInfo {
  const { data } = useQuery(guestConfigQuery())
  const appUrl = (data?.app_url || window.location.origin).replace(/\/+$/, '')
  return {
    currency: data?.currency || 'CNY',
    symbol: data?.currency_symbol || undefined,
    appUrl,
    appName: data?.app_name || 'GG Bond',
  }
}

/** `(cents) => "¥12.00"` in the site currency and the current locale (for strings / aria labels). */
export function useFormatMoney(): (cents: number) => string {
  const { currency, symbol } = useSite()
  const locale = useLocale()
  return useCallback(
    (cents: number) => formatMoney(cents, currency, locale, symbol !== undefined ? { symbol } : {}),
    [currency, symbol, locale],
  )
}
