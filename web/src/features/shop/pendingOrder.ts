import { type ApiError, userApi } from '@/lib/api'
import { ORDER_STATUS } from '@/lib/api/types'

/**
 * Trade number of the pending order behind a 409 from `POST /user/orders`: the server puts it in
 * `errors.trade_no`; older servers don't, so fall back to the newest pending order in the list.
 */
export async function findPendingOrder(error: ApiError): Promise<string | null> {
  const fromServer = error.fieldError('trade_no')
  if (fromServer) return fromServer
  try {
    const page = await userApi.orders({ status: ORDER_STATUS.pending, page: 1, page_size: 1 })
    return page.items[0]?.trade_no ?? null
  } catch {
    return null
  }
}
