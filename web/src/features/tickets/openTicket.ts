import { userApi } from '@/lib/api'
import type { ID } from '@/lib/api/types'

/**
 * The user's open ticket (one at a time, Xboard rule). Used after a 409 from "new ticket" or
 * "withdraw commission", whose error body does not carry the id.
 */
export async function findOpenTicket(): Promise<ID | null> {
  try {
    const page = await userApi.tickets({ status: 0, page: 1, page_size: 1 })
    return page.items[0]?.id ?? null
  } catch {
    return null
  }
}
