import { describe, expect, it } from 'vitest'
import {
  couponDiscount,
  estimateCheckout,
  isSoldOut,
  offeredPeriods,
  perMonth,
  savingPercent,
} from './pricing'

describe('shop pricing', () => {
  const plan = { prices: { yearly: 30000, monthly: 3000, reset_traffic: 500, quarterly: 8500 } }

  it('lists offered periods in canonical order without reset_traffic', () => {
    expect(offeredPeriods(plan).map((o) => o.period)).toEqual(['monthly', 'quarterly', 'yearly'])
  })

  it('computes per-month equivalents and savings', () => {
    expect(perMonth(8500, 'quarterly')).toBe(2833)
    expect(perMonth(1000, 'onetime')).toBeNull()
    expect(savingPercent(plan, { period: 'yearly', cents: 30000 })).toBe(16)
    expect(savingPercent(plan, { period: 'monthly', cents: 3000 })).toBeNull()
    expect(savingPercent({ prices: { yearly: 100 } }, { period: 'yearly', cents: 100 })).toBeNull()
  })

  it('treats a non-positive remaining capacity as sold out', () => {
    expect(isSoldOut({ capacity_limit: 0 })).toBe(true)
    expect(isSoldOut({ capacity_limit: 3 })).toBe(false)
    expect(isSoldOut({ capacity_limit: null })).toBe(false)
    expect(isSoldOut({})).toBe(false)
  })

  it('applies fixed and percent coupons capped at the price', () => {
    expect(couponDiscount({ type: 1, value: 500 }, 3000)).toBe(500)
    expect(couponDiscount({ type: 1, value: 5000 }, 3000)).toBe(3000)
    expect(couponDiscount({ type: 2, value: 15 }, 3000)).toBe(450)
  })

  it('estimates discounts and the automatic balance deduction', () => {
    const coupon = { id: 1, code: 'X', name: 'X', type: 2 as const, value: 10 }
    expect(estimateCheckout(3000, { userDiscountPercent: 5, coupon, balance: 1000 })).toEqual({
      price: 3000,
      userDiscount: 150,
      couponDiscount: 300,
      subtotal: 2550,
      balanceUsed: 1000,
      payable: 1550,
    })
    expect(estimateCheckout(3000, { balance: 5000 }).payable).toBe(0)
  })
})
