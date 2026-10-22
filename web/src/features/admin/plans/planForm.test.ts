import { describe, expect, it } from 'vitest'
import type { Plan } from '@/lib/api/types'
import { planToValues, valuesToPlanInput } from './planForm'

const plan: Plan = {
  id: 2,
  group_id: 3,
  name: 'Pro',
  content: '# Pro',
  tags: ['hot', 'new'],
  transfer_enable: 500,
  speed_limit: null,
  device_limit: 5,
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
})
