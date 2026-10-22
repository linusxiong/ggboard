/**
 * Client-side price helpers for the shop. The server is authoritative (docs/PAYMENTS.md §Order
 * lifecycle: price → user discount → coupon → surplus → balance); these only power previews.
 * Money is integer cents.
 */
import { type CouponCheck, PLAN_PERIODS, type Plan, type PlanPeriod } from '@/lib/api/types'

/** Months covered by each recurring period (`onetime` / `reset_traffic` have none). */
export const PERIOD_MONTHS: Readonly<Partial<Record<PlanPeriod, number>>> = {
  monthly: 1,
  quarterly: 3,
  half_yearly: 6,
  yearly: 12,
  two_yearly: 24,
  three_yearly: 36,
}

export interface PriceOffer {
  period: PlanPeriod
  cents: number
}

/** Periods a plan is sold for (in canonical order), without the `reset_traffic` add-on. */
export function offeredPeriods(plan: Pick<Plan, 'prices'>): PriceOffer[] {
  const offers: PriceOffer[] = []
  for (const period of PLAN_PERIODS) {
    if (period === 'reset_traffic') continue
    const cents = plan.prices[period]
    if (typeof cents === 'number' && Number.isFinite(cents) && cents >= 0) offers.push({ period, cents })
  }
  return offers
}

/** Per-month equivalent in cents (rounded), `null` for non-recurring periods. */
export function perMonth(cents: number, period: PlanPeriod): number | null {
  const months = PERIOD_MONTHS[period]
  return months ? Math.round(cents / months) : null
}

/**
 * Saving against paying monthly for the same span, as a whole percent (`null` when there is no
 * monthly price, the period is not recurring, or there is no saving).
 */
export function savingPercent(plan: Pick<Plan, 'prices'>, offer: PriceOffer): number | null {
  const monthly = plan.prices.monthly
  const months = PERIOD_MONTHS[offer.period]
  if (typeof monthly !== 'number' || monthly <= 0 || !months || months === 1) return null
  const full = monthly * months
  const saving = Math.floor(((full - offer.cents) / full) * 100)
  return saving > 0 ? saving : null
}

/**
 * `capacity_limit` on `/user/plans` is the remaining capacity when the server exposes it (Xboard
 * semantics); `0` or less means sold out, `null`/absent means unlimited.
 */
export function isSoldOut(plan: Pick<Plan, 'capacity_limit'>): boolean {
  return typeof plan.capacity_limit === 'number' && plan.capacity_limit <= 0
}

/** Discount a checked coupon gives on `price` (type 1 fixed cents, type 2 percent), capped at the price. */
export function couponDiscount(coupon: Pick<CouponCheck, 'type' | 'value'>, price: number): number {
  const raw = coupon.type === 2 ? Math.round((price * coupon.value) / 100) : coupon.value
  return Math.max(0, Math.min(price, raw))
}

export interface CheckoutEstimate {
  price: number
  /** User-level percent discount (`User.discount`). */
  userDiscount: number
  couponDiscount: number
  /** After discounts, before balance. */
  subtotal: number
  /** Balance the server will apply automatically. */
  balanceUsed: number
  /** Estimated amount left to pay (before upgrade credit and gateway fees). */
  payable: number
}

export function estimateCheckout(
  price: number,
  opts: { userDiscountPercent?: number | null; coupon?: CouponCheck | null; balance?: number } = {},
): CheckoutEstimate {
  const percent = opts.userDiscountPercent ?? 0
  const userDiscount = percent > 0 ? Math.min(price, Math.round((price * percent) / 100)) : 0
  const coupon = opts.coupon ? couponDiscount(opts.coupon, price) : 0
  const subtotal = Math.max(0, price - userDiscount - coupon)
  const balanceUsed = Math.min(subtotal, Math.max(0, opts.balance ?? 0))
  return {
    price,
    userDiscount,
    couponDiscount: Math.min(coupon, price - userDiscount),
    subtotal,
    balanceUsed,
    payable: subtotal - balanceUsed,
  }
}
