import type { AdminOrderFilters } from '@/lib/api/types'
import { compact, type OrderSearch } from './search'

/** Splits the "email or user id" box: digits → `user_id`, anything else → `email` (substring match). */
export function splitUserQuery(q: string | undefined): Pick<AdminOrderFilters, 'email' | 'user_id'> {
  const v = q?.trim()
  if (!v) return {}
  if (/^\d+$/.test(v)) return { user_id: Number(v) }
  return { email: v }
}

/** `/admin/orders` URL state → API query (`GET /admin/orders`). */
export function orderSearchToQuery(s: OrderSearch): AdminOrderFilters {
  return compact({
    page: s.page,
    page_size: s.page_size,
    sort: s.sort ?? '-created_at',
    trade_no: s.trade_no,
    ...splitUserQuery(s.q),
    status: s.status,
    commission_status: s.commission_status,
    plan_id: s.plan_id,
    payment_id: s.payment_id,
    date_from: s.date_from,
    date_to: s.date_to,
  })
}

/** Number of active filters (badge on the "reset" button). */
export function activeOrderFilters(s: OrderSearch): number {
  const keys: Array<keyof OrderSearch> = [
    'trade_no',
    'q',
    'status',
    'commission_status',
    'plan_id',
    'payment_id',
    'date_from',
    'date_to',
  ]
  return keys.filter((k) => s[k] !== undefined).length
}
