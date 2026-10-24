/**
 * Admin user edit / create forms ↔ API. Units: the form edits traffic in GB and money in
 * currency units; `PATCH /admin/users/:id` takes bytes and cents (User object units), while
 * `POST /admin/users` takes `transfer_enable` in GB (docs/API.md).
 *
 * The PATCH payload only contains fields that changed, so editing e.g. the remarks never
 * overwrites `u`/`d` that nodes keep incrementing in the meantime.
 */
import type { AdminUser, AdminUserCreateInput, AdminUserUpdateInput, CommissionType } from '@/lib/api/types'
import { isoToLocalInput, localInputToIso } from '../shared/datetime'
import { bytesToGb, centsToUnits, gbToBytes, unitsToCents } from '../shared/units'

/** Select key for "no plan / no group" (collection keys cannot be empty). */
export const NONE = '0'

export interface UserFormValues {
  email: string
  /** `''` keeps the current password. */
  password: string
  plan_id: string
  group_id: string
  /** GB */
  transfer_enable: number | null
  /** GB used (upload) */
  u: number | null
  /** GB used (download) */
  d: number | null
  /** `datetime-local` value; `''` = never expires. */
  expired_at: string
  speed_limit: number | null
  device_limit: number | null
  conn_limit: number | null
  /** currency units */
  balance: number | null
  /** currency units */
  commission_balance: number | null
  commission_type: '0' | '1' | '2'
  commission_rate: number | null
  discount: number | null
  invite_user_id: number | null
  remarks: string
  is_admin: boolean
  is_staff: boolean
  banned: boolean
  remind_expire: boolean
  remind_traffic: boolean
}

export function userToValues(user: AdminUser, currency: string): UserFormValues {
  return {
    email: user.email,
    password: '',
    plan_id: user.plan_id ? String(user.plan_id) : NONE,
    group_id: user.group_id ? String(user.group_id) : NONE,
    transfer_enable: bytesToGb(user.transfer_enable),
    u: bytesToGb(user.u),
    d: bytesToGb(user.d),
    expired_at: isoToLocalInput(user.expired_at),
    speed_limit: user.speed_limit,
    device_limit: user.device_limit,
    conn_limit: user.conn_limit ?? null,
    balance: centsToUnits(user.balance, currency),
    commission_balance: centsToUnits(user.commission_balance, currency),
    commission_type: String(user.commission_type ?? 0) as UserFormValues['commission_type'],
    commission_rate: user.commission_rate,
    discount: user.discount,
    invite_user_id: user.invite_user_id,
    remarks: user.remarks ?? '',
    is_admin: user.is_admin,
    is_staff: user.is_staff,
    banned: user.banned,
    remind_expire: user.remind_expire,
    remind_traffic: user.remind_traffic,
  }
}

const idOrNull = (key: string) => (key && key !== NONE ? Number(key) : null)
const intOrNull = (v: number | null) => (v === null || Number.isNaN(v) ? null : Math.trunc(v))

/** Changed fields only, converted to API units. Empty object = nothing to save. */
export function valuesToPatch(
  values: UserFormValues,
  initial: UserFormValues,
  currency: string,
): AdminUserUpdateInput {
  const patch: AdminUserUpdateInput = {}
  const changed = <K extends keyof UserFormValues>(key: K) => values[key] !== initial[key]

  if (changed('email')) patch.email = values.email.trim()
  if (values.password) patch.password = values.password
  if (changed('plan_id')) patch.plan_id = idOrNull(values.plan_id)
  if (changed('group_id')) patch.group_id = idOrNull(values.group_id)
  if (changed('transfer_enable')) patch.transfer_enable = gbToBytes(values.transfer_enable ?? 0)
  if (changed('u')) patch.u = gbToBytes(values.u ?? 0)
  if (changed('d')) patch.d = gbToBytes(values.d ?? 0)
  if (changed('expired_at')) patch.expired_at = localInputToIso(values.expired_at)
  if (changed('speed_limit')) patch.speed_limit = intOrNull(values.speed_limit)
  if (changed('device_limit')) patch.device_limit = intOrNull(values.device_limit)
  if (changed('conn_limit')) patch.conn_limit = intOrNull(values.conn_limit)
  if (changed('balance')) patch.balance = unitsToCents(values.balance ?? 0, currency)
  if (changed('commission_balance'))
    patch.commission_balance = unitsToCents(values.commission_balance ?? 0, currency)
  if (changed('commission_type')) patch.commission_type = Number(values.commission_type) as CommissionType
  if (changed('commission_rate')) patch.commission_rate = intOrNull(values.commission_rate)
  if (changed('discount')) patch.discount = intOrNull(values.discount)
  if (changed('invite_user_id')) patch.invite_user_id = intOrNull(values.invite_user_id)
  if (changed('remarks')) patch.remarks = values.remarks
  for (const flag of ['is_admin', 'is_staff', 'banned', 'remind_expire', 'remind_traffic'] as const) {
    if (changed(flag)) patch[flag] = values[flag]
  }
  return patch
}

export interface CreateUserValues {
  email: string
  password: string
  plan_id: string
  group_id: string
  /** GB */
  transfer_enable: number | null
  expired_at: string
  speed_limit: number | null
  device_limit: number | null
  conn_limit: number | null
  /** currency units */
  balance: number | null
  is_admin: boolean
  is_staff: boolean
  remarks: string
}

export const EMPTY_CREATE: CreateUserValues = {
  email: '',
  password: '',
  plan_id: NONE,
  group_id: NONE,
  transfer_enable: null,
  expired_at: '',
  speed_limit: null,
  device_limit: null,
  conn_limit: null,
  balance: null,
  is_admin: false,
  is_staff: false,
  remarks: '',
}

export function createValuesToInput(values: CreateUserValues, currency: string): AdminUserCreateInput {
  const input: AdminUserCreateInput = { email: values.email.trim(), password: values.password }
  const plan = idOrNull(values.plan_id)
  const group = idOrNull(values.group_id)
  if (plan !== null) input.plan_id = plan
  if (group !== null) input.group_id = group
  if (values.transfer_enable !== null) input.transfer_enable = values.transfer_enable
  const expired = localInputToIso(values.expired_at)
  if (expired) input.expired_at = expired
  if (values.speed_limit !== null) input.speed_limit = Math.trunc(values.speed_limit)
  if (values.device_limit !== null) input.device_limit = Math.trunc(values.device_limit)
  if (values.conn_limit !== null) input.conn_limit = Math.trunc(values.conn_limit)
  if (values.balance !== null) input.balance = unitsToCents(values.balance, currency)
  if (values.is_admin) input.is_admin = true
  if (values.is_staff) input.is_staff = true
  if (values.remarks.trim()) input.remarks = values.remarks.trim()
  return input
}

/** 16-char random password (letters + digits, no look-alikes). */
export function randomPassword(length = 16): string {
  const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789'
  const bytes = crypto.getRandomValues(new Uint8Array(length))
  return Array.from(bytes, (b) => alphabet[b % alphabet.length]).join('')
}
