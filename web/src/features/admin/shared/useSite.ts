import { useQuery } from '@tanstack/react-query'
import { guestConfigQuery } from '@/lib/api/queries'

/** Site currency + URL from `/guest/config` (cached 10 min). */
export function useSite() {
  const { data } = useQuery(guestConfigQuery())
  return {
    currency: data?.currency || 'CNY',
    symbol: data?.currency_symbol || undefined,
    appUrl: (data?.app_url || (typeof window !== 'undefined' ? window.location.origin : '')).replace(
      /\/+$/,
      '',
    ),
  }
}
