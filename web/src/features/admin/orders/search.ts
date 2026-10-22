/**
 * Tiny, dependency-free search-param parsers for the admin list routes (orders, coupons, tickets).
 * TanStack Router hands `validateSearch` the JSON-parsed query (`?page=2` → 2, `?q=abc` → "abc").
 * Every parser drops invalid values instead of throwing, so a hand-edited URL never breaks a page.
 * Keep this module tiny and dependency-free: route `validateSearch` functions import it eagerly
 * (they live in the entry chunk). URL → API mapping lives in `./query.ts`.
 */
import type { CommissionStatus, ID, ISODate, OrderStatus } from '@/lib/api/types'

export function optString(v: unknown): string | undefined {
  if (typeof v === 'number' && Number.isFinite(v)) return String(v)
  if (typeof v !== 'string') return undefined
  const s = v.trim()
  return s === '' ? undefined : s
}

export function optInt(v: unknown, min = 0, max = Number.MAX_SAFE_INTEGER): number | undefined {
  const n = typeof v === 'string' && v.trim() !== '' ? Number(v) : v
  if (typeof n !== 'number' || !Number.isInteger(n) || n < min || n > max) return undefined
  return n
}

export function optEnum<T extends string | number>(v: unknown, allowed: readonly T[]): T | undefined {
  const candidates: unknown[] = [v]
  if (typeof v === 'string' && v.trim() !== '' && !Number.isNaN(Number(v))) candidates.push(Number(v))
  for (const c of candidates) if ((allowed as readonly unknown[]).includes(c)) return c as T
  return undefined
}

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/
export function optDate(v: unknown): ISODate | undefined {
  return typeof v === 'string' && DATE_RE.test(v) && !Number.isNaN(Date.parse(v)) ? v : undefined
}

export function page(v: unknown): number {
  return optInt(v, 1) ?? 1
}

export const PAGE_SIZES = [10, 20, 50, 100] as const
export function pageSize(v: unknown, fallback = 20): number {
  return optInt(v, 1, 200) ?? fallback
}

/** Drops `undefined` keys so URLs stay short and query keys stay stable. */
export function compact<T extends object>(o: T): T {
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(o)) if (v !== undefined && v !== '') out[k] = v
  return out as T
}

// ---------------------------------------------------------------- orders

export const ORDER_STATUSES: readonly OrderStatus[] = [0, 1, 2, 3, 4]
export const COMMISSION_STATUSES: readonly CommissionStatus[] = [0, 1, 2, 3]

/** URL state of `/admin/orders`. `q` is "email or user id" as typed. */
export interface OrderSearch {
  page: number
  page_size: number
  sort?: string
  trade_no?: string
  q?: string
  status?: OrderStatus
  commission_status?: CommissionStatus
  plan_id?: ID
  payment_id?: ID
  date_from?: ISODate
  date_to?: ISODate
}

export function parseOrderSearch(raw: Record<string, unknown>): OrderSearch {
  return compact({
    page: page(raw.page),
    page_size: pageSize(raw.page_size),
    sort: optString(raw.sort),
    trade_no: optString(raw.trade_no),
    q: optString(raw.q),
    status: optEnum(raw.status, ORDER_STATUSES),
    commission_status: optEnum(raw.commission_status, COMMISSION_STATUSES),
    plan_id: optInt(raw.plan_id, 1),
    payment_id: optInt(raw.payment_id, 1),
    date_from: optDate(raw.date_from),
    date_to: optDate(raw.date_to),
  })
}
