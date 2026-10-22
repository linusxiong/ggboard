import { useQuery } from '@tanstack/react-query'
import { guestConfigQuery } from '@/lib/api/queries'

/** Site currency (`settings.currency` / `currency_symbol`) for `<Money>` and money inputs. */
export function useCurrency(): { currency: string; symbol: string | undefined } {
  const { data } = useQuery(guestConfigQuery())
  return { currency: data?.currency || 'CNY', symbol: data?.currency_symbol || undefined }
}

/** Integer cents → decimal units (2 places). */
export const centsToUnits = (cents: number | null | undefined): number | null =>
  cents === null || cents === undefined ? null : Math.round(cents) / 100

/** Decimal units → integer cents (rounded half away from zero). */
export const unitsToCents = (units: number | null | undefined): number | null =>
  units === null || units === undefined || Number.isNaN(units) ? null : Math.round(units * 100)
