/**
 * Plan form model ↔ API `Plan` / `PlanInput`.
 * Prices: the form edits currency units (`15.5`), the API stores integer cents (`1550`); an empty
 * price means the period is not offered (key absent). Traffic stays in GB (`Plan.transfer_enable`).
 */
import {
  PLAN_PERIODS,
  type Plan,
  type PlanInput,
  type PlanPeriod,
  type ResetTrafficMethod,
} from '@/lib/api/types'
import { centsToUnits, parseList, unitsToCents } from '../shared/units'

/** `follow` = null (use the `reset_traffic_method` setting). */
export type ResetChoice = 'follow' | '0' | '1' | '2' | '3' | '4'
export const RESET_CHOICES: readonly ResetChoice[] = ['follow', '0', '1', '2', '3', '4']

export interface PlanFormValues {
  name: string
  group_id: string | null
  /** GB */
  transfer_enable: number | null
  speed_limit: number | null
  device_limit: number | null
  capacity_limit: number | null
  /** Currency units per period; `null` = not offered. */
  prices: Record<PlanPeriod, number | null>
  reset_traffic_method: ResetChoice
  /** Edited as chips (`TagsField`). */
  tags: string[]
  content: string
  show: boolean
  sell: boolean
  renew: boolean
}

export function planToValues(plan: Plan | null | undefined, currency: string): PlanFormValues {
  const prices = Object.fromEntries(
    PLAN_PERIODS.map((period) => {
      const cents = plan?.prices[period]
      return [period, typeof cents === 'number' ? centsToUnits(cents, currency) : null]
    }),
  ) as Record<PlanPeriod, number | null>
  return {
    name: plan?.name ?? '',
    group_id: plan ? String(plan.group_id) : null,
    transfer_enable: plan?.transfer_enable ?? null,
    speed_limit: plan?.speed_limit ?? null,
    device_limit: plan?.device_limit ?? null,
    capacity_limit: plan?.capacity_limit ?? null,
    prices,
    reset_traffic_method:
      plan?.reset_traffic_method === null || plan?.reset_traffic_method === undefined
        ? 'follow'
        : (String(plan.reset_traffic_method) as ResetChoice),
    tags: [...(plan?.tags ?? [])],
    content: plan?.content ?? '',
    show: plan?.show ?? true,
    sell: plan?.sell ?? true,
    renew: plan?.renew ?? true,
  }
}

const intOrNull = (v: number | null) => (v === null || Number.isNaN(v) ? null : Math.trunc(v))

export function valuesToPlanInput(values: PlanFormValues, currency: string): PlanInput {
  const prices: Partial<Record<PlanPeriod, number>> = {}
  for (const period of PLAN_PERIODS) {
    const units = values.prices[period]
    if (units !== null && units !== undefined && !Number.isNaN(units))
      prices[period] = unitsToCents(units, currency)
  }
  return {
    name: values.name.trim(),
    group_id: Number(values.group_id),
    transfer_enable: values.transfer_enable ?? 0,
    speed_limit: intOrNull(values.speed_limit),
    device_limit: intOrNull(values.device_limit),
    capacity_limit: intOrNull(values.capacity_limit),
    prices,
    reset_traffic_method:
      values.reset_traffic_method === 'follow'
        ? null
        : (Number(values.reset_traffic_method) as ResetTrafficMethod),
    tags: parseList(values.tags.join('\n')),
    content: values.content,
    show: values.show,
    sell: values.sell,
    renew: values.renew,
  }
}
