/**
 * API barrel. Import endpoint groups as namespaces (tree-shakeable):
 *   import { userApi, adminApi } from '@/lib/api'
 *   userApi.orders({ page: 1 })
 */
export * as guestApi from './guest'
export * as passportApi from './passport'
export * as userApi from './user'
export * as adminApi from './admin'
export { ApiError, isApiError, api, apiUrl, buildQuery, setUnauthorizedHandler } from './client'
export type * from './types'
export { PLAN_PERIODS, NODE_TYPES, ORDER_STATUS, ORDER_TYPE } from './types'
