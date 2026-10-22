/**
 * `/admin/users` URL search ⇄ API filters (docs/API.md Admin › Users). Kept dependency-free:
 * the route's `validateSearch` ships with the route tree.
 */
import type { AdminUserFilters, ExpiredFilter, ListQuery } from '@/lib/api/types'

export type UserRole = 'admin' | 'staff' | 'user'

export interface UserSearch {
  page: number
  page_size: number
  sort?: string
  email?: string
  plan_id?: number
  group_id?: number
  banned?: boolean
  role?: UserRole
  expired?: ExpiredFilter
  invite_user_id?: number
  id?: number
  uuid?: string
  token?: string
  remarks?: string
}

export const PAGE_SIZES = [20, 50, 100] as const
const ROLES = new Set<UserRole>(['admin', 'staff', 'user'])
const EXPIRED = new Set<ExpiredFilter>(['active', 'expired', 'never'])
export const SORTABLE = new Set(['id', 'balance', 'expired_at', 'created_at', 'last_login_at'])

const positiveInt = (v: unknown) => {
  const n = typeof v === 'number' ? v : typeof v === 'string' && v.trim() ? Number(v) : Number.NaN
  return Number.isInteger(n) && n > 0 ? n : undefined
}
const text = (v: unknown, max = 191) =>
  typeof v === 'string' && v.trim() ? v.trim().slice(0, max) : undefined
const bool = (v: unknown) =>
  v === true || v === 'true' || v === '1'
    ? true
    : v === false || v === 'false' || v === '0'
      ? false
      : undefined

/** Sanitises raw search params (unknown keys dropped, invalid values ignored). */
export function parseUserSearch(raw: Record<string, unknown>): UserSearch {
  const size = positiveInt(raw.page_size)
  const out: UserSearch = {
    page: positiveInt(raw.page) ?? 1,
    page_size: size && size <= 200 ? size : 20,
  }
  const sort = text(raw.sort, 40)
  if (sort && SORTABLE.has(sort.replace(/^-/, ''))) out.sort = sort
  const email = text(raw.email)
  if (email) out.email = email
  const planId = positiveInt(raw.plan_id)
  if (planId) out.plan_id = planId
  const groupId = positiveInt(raw.group_id)
  if (groupId) out.group_id = groupId
  const banned = bool(raw.banned)
  if (banned !== undefined) out.banned = banned
  if (typeof raw.role === 'string' && ROLES.has(raw.role as UserRole)) out.role = raw.role as UserRole
  if (typeof raw.expired === 'string' && EXPIRED.has(raw.expired as ExpiredFilter))
    out.expired = raw.expired as ExpiredFilter
  const inviter = positiveInt(raw.invite_user_id)
  if (inviter) out.invite_user_id = inviter
  const id = positiveInt(raw.id)
  if (id) out.id = id
  const uuid = text(raw.uuid, 36)
  if (uuid) out.uuid = uuid
  const token = text(raw.token, 64)
  if (token) out.token = token
  const remarks = text(raw.remarks)
  if (remarks) out.remarks = remarks
  return out
}

/** Filter part only (export / bulk mail take the list filters without paging). */
export type UserFilterParams = Omit<AdminUserFilters, keyof ListQuery>

export function toFilterParams(s: Partial<UserSearch>): UserFilterParams {
  const f: UserFilterParams = {}
  if (s.email) f.email = s.email
  if (s.plan_id) f.plan_id = s.plan_id
  if (s.group_id) f.group_id = s.group_id
  if (s.banned !== undefined) f.banned = s.banned
  if (s.role === 'admin') f.is_admin = true
  if (s.role === 'staff') f.is_staff = true
  if (s.role === 'user') {
    f.is_admin = false
    f.is_staff = false
  }
  if (s.expired) f.expired = s.expired
  if (s.invite_user_id) f.invite_user_id = s.invite_user_id
  if (s.id) f.id = s.id
  if (s.uuid) f.uuid = s.uuid
  if (s.token) f.token = s.token
  if (s.remarks) f.remarks = s.remarks
  return f
}

/** URL search → `GET /admin/users` query (paging + sort + filters). */
export function toUserFilters(s: UserSearch): AdminUserFilters {
  return { page: s.page, page_size: s.page_size, ...(s.sort ? { sort: s.sort } : {}), ...toFilterParams(s) }
}

const FILTER_KEYS = [
  'email',
  'plan_id',
  'group_id',
  'banned',
  'role',
  'expired',
  'invite_user_id',
  'id',
  'uuid',
  'token',
  'remarks',
] as const
export type UserFilterKey = (typeof FILTER_KEYS)[number]

export function activeFilterCount(s: Partial<UserSearch>): number {
  return FILTER_KEYS.filter((k) => s[k] !== undefined).length
}

/** Search with every filter cleared (keeps page size and sort). */
export function clearFilters(s: UserSearch): UserSearch {
  return { page: 1, page_size: s.page_size, ...(s.sort ? { sort: s.sort } : {}) }
}
