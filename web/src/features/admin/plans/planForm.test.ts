import { describe, expect, it } from 'vitest'
import type { Plan } from '@/lib/api/types'
import { planToValues, valuesToPlanInput, valuesToPlanPatch } from './planForm'

const plan: Plan = {
  id: 2,
  group_id: 3,
  name: 'Pro',
  content: '# Pro',
  tags: ['hot', 'new'],
  transfer_enable: 500,
  speed_limit: null,
  device_limit: 5,
  conn_limit: 128,
  capacity_limit: null,
  prices: { monthly: 3500, yearly: 36000 },
  reset_traffic_method: null,
  show: true,
  sell: false,
  renew: true,
  sort: 1,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
}

describe('plan form', () => {
  it('round-trips prices (cents ↔ units) and empty periods', () => {
    const values = planToValues(plan, 'CNY')
    expect(values.prices.monthly).toBe(35)
    expect(values.prices.quarterly).toBeNull()
    expect(values.reset_traffic_method).toBe('follow')
    expect(valuesToPlanInput(values, 'CNY')).toEqual({
      name: 'Pro',
      group_id: 3,
      transfer_enable: 500,
      speed_limit: null,
      device_limit: 5,
      conn_limit: 128,
      capacity_limit: null,
      prices: { monthly: 3500, yearly: 36000 },
      reset_traffic_method: null,
      tags: ['hot', 'new'],
      content: '# Pro',
      show: true,
      sell: false,
      renew: true,
    })
  })

  it('converts edited values', () => {
    const values = planToValues(plan, 'CNY')
    const input = valuesToPlanInput(
      {
        ...values,
        prices: { ...values.prices, monthly: null, quarterly: 99.5 },
        reset_traffic_method: '4',
        tags: ['a', ' b ', 'a'],
      },
      'CNY',
    )
    expect(input.prices).toEqual({ quarterly: 9950, yearly: 36000 })
    expect(input.reset_traffic_method).toBe(4)
    expect(input.tags).toEqual(['a', 'b'])
  })

  it('round-trips the connection limit: new plan, edit, clear to null', () => {
    // New plan: the field starts empty (= unlimited) and is sent as null.
    const blank = planToValues(null, 'CNY')
    expect(blank.conn_limit).toBeNull()
    expect(valuesToPlanInput({ ...blank, name: 'New', group_id: '1' }, 'CNY').conn_limit).toBeNull()
    expect(
      valuesToPlanInput({ ...blank, name: 'New', group_id: '1', conn_limit: 64 }, 'CNY').conn_limit,
    ).toBe(64)

    // Edit: the stored value is loaded, a new value is sent (fractions truncated).
    const values = planToValues(plan, 'CNY')
    expect(values.conn_limit).toBe(128)
    expect(valuesToPlanInput({ ...values, conn_limit: 256.7 }, 'CNY').conn_limit).toBe(256)

    // Clearing the field sends null, which PATCH /admin/plans/:id treats as "clear".
    const cleared = valuesToPlanInput({ ...values, conn_limit: null }, 'CNY')
    expect('conn_limit' in cleared).toBe(true)
    expect(cleared.conn_limit).toBeNull()
    expect(valuesToPlanInput({ ...values, conn_limit: Number.NaN }, 'CNY').conn_limit).toBeNull()

    // A plan from an older backend without the field loads as unlimited.
    const legacy = { ...plan, conn_limit: undefined } as unknown as Plan
    expect(planToValues(legacy, 'CNY').conn_limit).toBeNull()
  })

  it('adds force_update to the PATCH body only when asked', () => {
    const values = planToValues(plan, 'CNY')
    const input = valuesToPlanInput(values, 'CNY')
    expect(valuesToPlanPatch(values, 'CNY', true)).toEqual({ ...input, force_update: true })
    const plain = valuesToPlanPatch(values, 'CNY', false)
    expect(plain).toEqual(input)
    expect(plain).not.toHaveProperty('force_update')
  })
})
