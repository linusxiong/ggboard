import { compact, optEnum, optInt, optString, page, pageSize } from '@/features/admin/orders/search'
import type { ID } from '@/lib/api/types'

export const TICKET_VIEWS = ['awaiting', 'open', 'closed', 'all'] as const
export type TicketView = (typeof TICKET_VIEWS)[number]

/** URL state of `/admin/tickets` (`id` = ticket open in the thread pane). */
export interface TicketSearch {
  page: number
  page_size: number
  view: TicketView
  q?: string
  id?: ID
}

export function parseTicketSearch(raw: Record<string, unknown>): TicketSearch {
  return compact({
    page: page(raw.page),
    page_size: pageSize(raw.page_size, 30),
    view: optEnum(raw.view, TICKET_VIEWS) ?? 'awaiting',
    q: optString(raw.q),
    id: optInt(raw.id, 1),
  })
}
