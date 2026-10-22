import type { AdminCoupon, CouponCreateInput, CouponType, PlanPeriod } from '@/lib/api/types'
import { fromLocalInput, toLocalInput } from './datetime'

export interface CouponValues {
  name: string
  code: string
  count: number | null
  type: string
  /** cents (fixed) or percent (1–100). */
  value: number | null
  limit_use: number | null
  limit_use_with_user: number | null
  limit_plan_ids: string[]
  limit_periods: string[]
  started_at: string
  ended_at: string
  show: boolean
}

export function couponDefaults(coupon?: AdminCoupon | null): CouponValues {
  const now = new Date()
  const in30 = new Date(now.getTime() + 30 * 86_400_000)
  return {
    name: coupon?.name ?? '',
    code: coupon?.code ?? '',
    count: 1,
    type: String(coupon?.type ?? 1),
    value: coupon?.value ?? null,
    limit_use: coupon?.limit_use ?? null,
    limit_use_with_user: coupon?.limit_use_with_user ?? null,
    limit_plan_ids: (coupon?.limit_plan_ids ?? []).map(String),
    limit_periods: [...(coupon?.limit_periods ?? [])],
    started_at: toLocalInput(coupon?.started_at ?? now.toISOString()),
    ended_at: toLocalInput(coupon?.ended_at ?? in30.toISOString()),
    show: coupon?.show ?? false,
  }
}

/** Form values → `POST/PATCH /admin/coupons` body. Empty limits become `null` (unlimited). */
export function toCouponInput(v: CouponValues, isEdit: boolean): CouponCreateInput {
  const count = v.count ?? 1
  return {
    name: v.name.trim(),
    ...(v.code.trim() && (isEdit || count === 1) ? { code: v.code.trim() } : {}),
    ...(isEdit ? {} : { count }),
    type: Number(v.type) as CouponType,
    value: v.value ?? 0,
    limit_use: v.limit_use,
    limit_use_with_user: v.limit_use_with_user,
    limit_plan_ids: v.limit_plan_ids.length ? v.limit_plan_ids.map(Number) : null,
    limit_periods: v.limit_periods.length ? (v.limit_periods as PlanPeriod[]) : null,
    started_at: fromLocalInput(v.started_at) ?? '',
    ended_at: fromLocalInput(v.ended_at) ?? '',
    show: v.show,
  }
}
