/**
 * React Query key factory + ready-made `queryOptions` for common reads.
 *
 * Usage:
 *   const { data } = useQuery(meQuery())                                            // in a component
 *   loader: ({ context }) => context.queryClient.query({ ...userPlansQuery(), staleTime: 'static' })  // route loader
 *   (TanStack Query 5.10x deprecates ensureQueryData/fetchQuery/prefetchQuery → `queryClient.query()`)
 *   queryClient.invalidateQueries({ queryKey: qk.user.orders() })              // after a mutation
 *
 * Every option is a factory function so unused ones tree-shake away.
 */
import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import * as adminApi from './admin'
import * as guestApi from './guest'
import * as userApi from './user'
import type {
  AdminOrderFilters,
  AdminTicketFilters,
  AdminUserFilters,
  ID,
  ListQuery,
  StatsMetric,
  StatsRange,
} from './types'

const MINUTE = 60_000

// Keys used on the public pages are standalone so the rest of the key objects tree-shake away.
const guestConfigKey = () => ['guest', 'config'] as const
const guestPlansKey = () => ['guest', 'plans'] as const
const meKey = () => ['user', 'me'] as const

const guestKeys = {
  all: ['guest'] as const,
  config: guestConfigKey,
  plans: guestPlansKey,
  catalog: () => ['guest', 'catalog'] as const,
}

const userKeys = {
  all: ['user'] as const,
  me: meKey,
  stats: () => ['user', 'stats'] as const,
  subscription: () => ['user', 'subscription'] as const,
  trafficLogs: (q: ListQuery = {}) => ['user', 'traffic-logs', q] as const,
  nodes: () => ['user', 'nodes'] as const,
  ruleProfile: () => ['user', 'rule-profile'] as const,
  catalog: () => ['user', 'catalog'] as const,
  plans: () => ['user', 'plans'] as const,
  plan: (id: ID) => ['user', 'plans', id] as const,
  paymentMethods: () => ['user', 'payment-methods'] as const,
  orders: (q: userApi.OrdersQuery = {}) => ['user', 'orders', q] as const,
  order: (tradeNo: string) => ['user', 'order', tradeNo] as const,
  invite: () => ['user', 'invite'] as const,
  commissions: (q: ListQuery = {}) => ['user', 'commissions', q] as const,
  tickets: (q: userApi.TicketsQuery = {}) => ['user', 'tickets', q] as const,
  ticket: (id: ID) => ['user', 'ticket', id] as const,
  notices: (q: ListQuery = {}) => ['user', 'notices', q] as const,
  knowledge: (q: { language?: string; keyword?: string } = {}) => ['user', 'knowledge', q] as const,
  article: (id: ID) => ['user', 'knowledge', 'article', id] as const,
}

const adminKeys = {
  all: ['admin'] as const,
  overview: () => ['admin', 'overview'] as const,
  series: (metric: StatsMetric, range: StatsRange) => ['admin', 'series', metric, range] as const,
  users: (q: AdminUserFilters = {}) => ['admin', 'users', q] as const,
  user: (id: ID) => ['admin', 'user', id] as const,
  plans: () => ['admin', 'plans'] as const,
  groups: () => ['admin', 'groups'] as const,
  nodes: () => ['admin', 'nodes'] as const,
  node: (id: ID) => ['admin', 'node', id] as const,
  routes: () => ['admin', 'routes'] as const,
  orders: (q: AdminOrderFilters = {}) => ['admin', 'orders', q] as const,
  order: (id: ID) => ['admin', 'order', id] as const,
  coupons: (q: ListQuery = {}) => ['admin', 'coupons', q] as const,
  payments: () => ['admin', 'payments'] as const,
  gateways: () => ['admin', 'gateways'] as const,
  tickets: (q: AdminTicketFilters = {}) => ['admin', 'tickets', q] as const,
  ticket: (id: ID) => ['admin', 'ticket', id] as const,
  notices: (q: ListQuery = {}) => ['admin', 'notices', q] as const,
  knowledge: (q: ListQuery = {}) => ['admin', 'knowledge', q] as const,
  article: (id: ID) => ['admin', 'knowledge', 'article', id] as const,
  settings: () => ['admin', 'settings'] as const,
  rulesets: () => ['admin', 'rulesets'] as const,
  system: () => ['admin', 'system'] as const,
  logs: (q: { level?: string; limit?: number } = {}) => ['admin', 'logs', q] as const,
}

/**
 * Hierarchical keys: invalidate `qk.user.all` to refetch everything user-scoped.
 * (Split into per-area objects internally so public pages don't ship the admin keys.)
 */
export const qk = { guest: guestKeys, user: userKeys, admin: adminKeys }

// ---------------------------------------------------------------- guest

export const guestConfigQuery = () =>
  queryOptions({ queryKey: guestConfigKey(), queryFn: ({ signal }) => guestApi.config(signal), staleTime: 10 * MINUTE })

export const guestPlansQuery = () =>
  queryOptions({ queryKey: guestPlansKey(), queryFn: ({ signal }) => guestApi.plans(signal), staleTime: 5 * MINUTE })

export const guestCatalogQuery = () =>
  queryOptions({
    queryKey: guestKeys.catalog(),
    queryFn: ({ signal }) => guestApi.rulesetCatalog(signal),
    staleTime: 30 * MINUTE,
  })

// ---------------------------------------------------------------- user

export const meQuery = () =>
  queryOptions({ queryKey: meKey(), queryFn: ({ signal }) => userApi.me(signal), staleTime: MINUTE })

export const userStatsQuery = () =>
  queryOptions({ queryKey: userKeys.stats(), queryFn: ({ signal }) => userApi.stats(signal) })

export const subscriptionQuery = () =>
  queryOptions({ queryKey: userKeys.subscription(), queryFn: ({ signal }) => userApi.subscription(signal) })

export const trafficLogsQuery = (q: ListQuery = {}) =>
  queryOptions({
    queryKey: userKeys.trafficLogs(q),
    queryFn: ({ signal }) => userApi.trafficLogs(q, signal),
    placeholderData: keepPreviousData,
  })

export const userNodesQuery = () =>
  queryOptions({ queryKey: userKeys.nodes(), queryFn: ({ signal }) => userApi.nodes(signal) })

export const ruleProfileQuery = () =>
  queryOptions({ queryKey: userKeys.ruleProfile(), queryFn: ({ signal }) => userApi.ruleProfile(signal) })

export const userCatalogQuery = () =>
  queryOptions({
    queryKey: userKeys.catalog(),
    queryFn: ({ signal }) => userApi.rulesetCatalog(signal),
    staleTime: 30 * MINUTE,
  })

export const userPlansQuery = () =>
  queryOptions({ queryKey: userKeys.plans(), queryFn: ({ signal }) => userApi.plans(signal), staleTime: 5 * MINUTE })

export const userPlanQuery = (id: ID) =>
  queryOptions({ queryKey: userKeys.plan(id), queryFn: ({ signal }) => userApi.plan(id, signal) })

export const paymentMethodsQuery = () =>
  queryOptions({
    queryKey: userKeys.paymentMethods(),
    queryFn: ({ signal }) => userApi.paymentMethods(signal),
    staleTime: 5 * MINUTE,
  })

export const ordersQuery = (q: userApi.OrdersQuery = {}) =>
  queryOptions({
    queryKey: userKeys.orders(q),
    queryFn: ({ signal }) => userApi.orders(q, signal),
    placeholderData: keepPreviousData,
  })

export const orderQuery = (tradeNo: string) =>
  queryOptions({ queryKey: userKeys.order(tradeNo), queryFn: ({ signal }) => userApi.order(tradeNo, signal) })

export const inviteQuery = () =>
  queryOptions({ queryKey: userKeys.invite(), queryFn: ({ signal }) => userApi.invite(signal) })

export const commissionsQuery = (q: ListQuery = {}) =>
  queryOptions({
    queryKey: userKeys.commissions(q),
    queryFn: ({ signal }) => userApi.commissions(q, signal),
    placeholderData: keepPreviousData,
  })

export const ticketsQuery = (q: userApi.TicketsQuery = {}) =>
  queryOptions({
    queryKey: userKeys.tickets(q),
    queryFn: ({ signal }) => userApi.tickets(q, signal),
    placeholderData: keepPreviousData,
  })

export const ticketQuery = (id: ID) =>
  queryOptions({ queryKey: userKeys.ticket(id), queryFn: ({ signal }) => userApi.ticket(id, signal) })

export const noticesQuery = (q: ListQuery = {}) =>
  queryOptions({
    queryKey: userKeys.notices(q),
    queryFn: ({ signal }) => userApi.notices(q, signal),
    placeholderData: keepPreviousData,
  })

export const knowledgeIndexQuery = (q: { language?: string; keyword?: string } = {}) =>
  queryOptions({ queryKey: userKeys.knowledge(q), queryFn: ({ signal }) => userApi.knowledgeIndex(q, signal) })

export const knowledgeQuery = (id: ID) =>
  queryOptions({ queryKey: userKeys.article(id), queryFn: ({ signal }) => userApi.knowledge(id, signal) })

// ---------------------------------------------------------------- admin

export const adminOverviewQuery = () =>
  queryOptions({ queryKey: adminKeys.overview(), queryFn: ({ signal }) => adminApi.statsOverview(signal) })

export const adminSeriesQuery = (metric: StatsMetric, range: StatsRange = '30d') =>
  queryOptions({
    queryKey: adminKeys.series(metric, range),
    queryFn: ({ signal }) => adminApi.statsSeries(metric, range, signal),
  })

export const adminUsersQuery = (q: AdminUserFilters = {}) =>
  queryOptions({
    queryKey: adminKeys.users(q),
    queryFn: ({ signal }) => adminApi.users(q, signal),
    placeholderData: keepPreviousData,
  })

export const adminUserQuery = (id: ID) =>
  queryOptions({ queryKey: adminKeys.user(id), queryFn: ({ signal }) => adminApi.user(id, signal) })

export const adminPlansQuery = () =>
  queryOptions({ queryKey: adminKeys.plans(), queryFn: ({ signal }) => adminApi.plans(signal) })

export const adminGroupsQuery = () =>
  queryOptions({ queryKey: adminKeys.groups(), queryFn: ({ signal }) => adminApi.groups(signal) })

export const adminNodesQuery = () =>
  queryOptions({
    queryKey: adminKeys.nodes(),
    queryFn: ({ signal }) => adminApi.nodes(signal),
    refetchInterval: MINUTE,
  })

export const adminNodeQuery = (id: ID) =>
  queryOptions({ queryKey: adminKeys.node(id), queryFn: ({ signal }) => adminApi.node(id, signal) })

export const adminRoutesQuery = () =>
  queryOptions({ queryKey: adminKeys.routes(), queryFn: ({ signal }) => adminApi.routes(signal) })

export const adminOrdersQuery = (q: AdminOrderFilters = {}) =>
  queryOptions({
    queryKey: adminKeys.orders(q),
    queryFn: ({ signal }) => adminApi.orders(q, signal),
    placeholderData: keepPreviousData,
  })

export const adminOrderQuery = (id: ID) =>
  queryOptions({ queryKey: adminKeys.order(id), queryFn: ({ signal }) => adminApi.order(id, signal) })

export const adminCouponsQuery = (q: ListQuery = {}) =>
  queryOptions({
    queryKey: adminKeys.coupons(q),
    queryFn: ({ signal }) => adminApi.coupons(q, signal),
    placeholderData: keepPreviousData,
  })

export const adminPaymentsQuery = () =>
  queryOptions({ queryKey: adminKeys.payments(), queryFn: ({ signal }) => adminApi.payments(signal) })

export const adminGatewaysQuery = () =>
  queryOptions({
    queryKey: adminKeys.gateways(),
    queryFn: ({ signal }) => adminApi.paymentGateways(signal),
    staleTime: 30 * MINUTE,
  })

export const adminTicketsQuery = (q: AdminTicketFilters = {}) =>
  queryOptions({
    queryKey: adminKeys.tickets(q),
    queryFn: ({ signal }) => adminApi.tickets(q, signal),
    placeholderData: keepPreviousData,
  })

export const adminTicketQuery = (id: ID) =>
  queryOptions({ queryKey: adminKeys.ticket(id), queryFn: ({ signal }) => adminApi.ticket(id, signal) })

export const adminNoticesQuery = (q: ListQuery = {}) =>
  queryOptions({
    queryKey: adminKeys.notices(q),
    queryFn: ({ signal }) => adminApi.notices(q, signal),
    placeholderData: keepPreviousData,
  })

export const adminKnowledgeQuery = (q: ListQuery = {}) =>
  queryOptions({
    queryKey: adminKeys.knowledge(q),
    queryFn: ({ signal }) => adminApi.knowledgeList(q, signal),
    placeholderData: keepPreviousData,
  })

export const adminSettingsQuery = () =>
  queryOptions({ queryKey: adminKeys.settings(), queryFn: ({ signal }) => adminApi.settings(signal) })

export const adminRulesetsQuery = () =>
  queryOptions({ queryKey: adminKeys.rulesets(), queryFn: ({ signal }) => adminApi.rulesets(signal) })

export const systemStatusQuery = () =>
  queryOptions({
    queryKey: adminKeys.system(),
    queryFn: ({ signal }) => adminApi.systemStatus(signal),
    refetchInterval: 30_000,
  })

export const systemLogsQuery = (q: { level?: string; limit?: number } = {}) =>
  queryOptions({ queryKey: adminKeys.logs(q), queryFn: ({ signal }) => adminApi.systemLogs(q, signal) })
