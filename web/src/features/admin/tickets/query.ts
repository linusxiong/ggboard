import { splitUserQuery } from '@/features/admin/orders/query'
import { compact } from '@/features/admin/orders/search'
import type { AdminTicketQuery } from '@/lib/api/types'
import type { TicketSearch, TicketView } from './search'

/**
 * View → API filters. Mixed views sort by `reply_status` so tickets awaiting staff come first;
 * single-state views show the most recently active first.
 */
export function ticketSearchToQuery(s: TicketSearch): AdminTicketQuery {
  const byView: Record<TicketView, AdminTicketQuery> = {
    awaiting: { status: 0, reply_status: 0, sort: '-updated_at' },
    open: { status: 0, sort: 'reply_status' },
    closed: { status: 1, sort: '-updated_at' },
    all: { sort: 'reply_status' },
  }
  return compact({ page: s.page, page_size: s.page_size, ...byView[s.view], ...splitUserQuery(s.q) })
}

/** Chip colour per ticket level (0 low, 1 medium, 2 high). */
export const LEVEL_COLOR = { 0: 'default', 1: 'warning', 2: 'danger' } as const
