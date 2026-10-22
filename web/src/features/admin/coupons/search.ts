import { compact, optString, page, pageSize } from '@/features/admin/orders/search'

export interface CouponSearch {
  page: number
  page_size: number
  sort?: string
}

export function parseCouponSearch(raw: Record<string, unknown>): CouponSearch {
  return compact({ page: page(raw.page), page_size: pageSize(raw.page_size), sort: optString(raw.sort) })
}
