/** Authenticated user endpoints (`/user`, 🔑) — docs/API.md §User. */
import { api, requestText } from './client'
import type {
  CheckoutResult,
  CommissionLog,
  CouponCheck,
  ID,
  InviteInfo,
  KnowledgeArticle,
  KnowledgeIndex,
  ListQuery,
  Notice,
  Order,
  OrderStatus,
  PaymentMethod,
  Plan,
  PlanPeriod,
  RuleProfile,
  RulesetCatalog,
  SubscribeQuery,
  SubscriptionReset,
  Ticket,
  TicketLevel,
  TicketStatus,
  TrafficLog,
  User,
  UserNode,
  UserStats,
  UserSubscription,
} from './types'

const enc = encodeURIComponent

// ---------------------------------------------------------------- account

/** `GET /user/me` */
export const me = (signal?: AbortSignal) => api.get<User>('/user/me', undefined, { signal })

/** `PATCH /user/me` */
export const updateMe = (input: { remind_expire?: boolean; remind_traffic?: boolean }) =>
  api.patch<User>('/user/me', input)

/** `POST /user/password` (bumps password_version → other sessions die). */
export const changePassword = (input: { old_password: string; new_password: string }) =>
  api.post<true>('/user/password', input)

/** `GET /user/stats` */
export const stats = (signal?: AbortSignal) => api.get<UserStats>('/user/stats', undefined, { signal })

/** `GET /user/subscription` */
export const subscription = (signal?: AbortSignal) =>
  api.get<UserSubscription>('/user/subscription', undefined, { signal })

/** `POST /user/subscription/reset` → new uuid + token. */
export const resetSubscription = () => api.post<SubscriptionReset>('/user/subscription/reset')

/** `GET /user/subscription/preview?client=clash&…` → generated config as text. */
export const previewSubscription = (query: SubscribeQuery & { client?: string } = {}, signal?: AbortSignal) =>
  requestText('GET', '/user/subscription/preview', { query, ...(signal ? { signal } : {}) })

/** `GET /user/traffic-logs` */
export const trafficLogs = (query: ListQuery = {}, signal?: AbortSignal) =>
  api.list<TrafficLog>('/user/traffic-logs', query, { signal })

/** `GET /user/nodes` */
export const nodes = (signal?: AbortSignal) => api.get<UserNode[]>('/user/nodes', undefined, { signal })

/** `POST /user/logout` (server no-op; the client discards its token). */
export const logout = () => api.post<true>('/user/logout')

// ---------------------------------------------------------------- rule profile

/** `GET /user/rule-profile` (defaults if none). */
export const ruleProfile = (signal?: AbortSignal) =>
  api.get<RuleProfile>('/user/rule-profile', undefined, { signal })

/** `PUT /user/rule-profile` */
export const saveRuleProfile = (profile: RuleProfile) => api.put<RuleProfile>('/user/rule-profile', profile)

/** `GET /user/rulesets/catalog` */
export const rulesetCatalog = (signal?: AbortSignal) =>
  api.get<RulesetCatalog>('/user/rulesets/catalog', undefined, { signal })

// ---------------------------------------------------------------- shop

/** `GET /user/plans` */
export const plans = (signal?: AbortSignal) => api.get<Plan[]>('/user/plans', undefined, { signal })

/** `GET /user/plans/:id` */
export const plan = (id: ID, signal?: AbortSignal) =>
  api.get<Plan>(`/user/plans/${id}`, undefined, { signal })

/** `GET /user/payment-methods` */
export const paymentMethods = (signal?: AbortSignal) =>
  api.get<PaymentMethod[]>('/user/payment-methods', undefined, { signal })

/** `POST /user/coupons/check` → coupon or 400. */
export const checkCoupon = (input: { code: string; plan_id: ID; period: PlanPeriod }) =>
  api.post<CouponCheck>('/user/coupons/check', input)

export interface OrdersQuery extends ListQuery {
  status?: OrderStatus
}
/** `GET /user/orders` */
export const orders = (query: OrdersQuery = {}, signal?: AbortSignal) =>
  api.list<Order>('/user/orders', query, { signal })

/** `POST /user/orders` → `{trade_no}` (409 if a pending order exists). */
export const createOrder = (input: { plan_id: ID; period: PlanPeriod; coupon_code?: string }) =>
  api.post<{ trade_no: string }>('/user/orders', input)

/** `GET /user/orders/:trade_no` */
export const order = (tradeNo: string, signal?: AbortSignal) =>
  api.get<Order>(`/user/orders/${enc(tradeNo)}`, undefined, { signal })

/** `POST /user/orders/:trade_no/checkout` — omit `payment_id` when total is 0 / balance covers it. */
export const checkout = (tradeNo: string, input: { payment_id?: ID } = {}) =>
  api.post<CheckoutResult>(`/user/orders/${enc(tradeNo)}/checkout`, input)

/** `GET /user/orders/:trade_no/status` — poll while paying. */
export const orderStatus = (tradeNo: string, signal?: AbortSignal) =>
  api.get<{ status: OrderStatus }>(`/user/orders/${enc(tradeNo)}/status`, undefined, { signal })

/** `POST /user/orders/:trade_no/cancel` */
export const cancelOrder = (tradeNo: string) => api.post<true>(`/user/orders/${enc(tradeNo)}/cancel`)

// ---------------------------------------------------------------- invite & commission

/** `GET /user/invite` */
export const invite = (signal?: AbortSignal) => api.get<InviteInfo>('/user/invite', undefined, { signal })

/** `POST /user/invite/codes` (respects `invite_gen_limit`). */
export const createInviteCode = () => api.post<{ code: string }>('/user/invite/codes')

/** `GET /user/invite/commissions` */
export const commissions = (query: ListQuery = {}, signal?: AbortSignal) =>
  api.list<CommissionLog>('/user/invite/commissions', query, { signal })

/** `POST /user/commission/transfer` — commission → balance (cents). */
export const transferCommission = (amount: number) => api.post<User>('/user/commission/transfer', { amount })

/** `POST /user/commission/withdraw` → creates a ticket. */
export const withdrawCommission = (input: { method: string; account: string }) =>
  api.post<{ ticket_id: ID }>('/user/commission/withdraw', input)

// ---------------------------------------------------------------- support & content

export interface TicketsQuery extends ListQuery {
  status?: TicketStatus
}
/** `GET /user/tickets` (no messages). */
export const tickets = (query: TicketsQuery = {}, signal?: AbortSignal) =>
  api.list<Ticket>('/user/tickets', query, { signal })

/** `POST /user/tickets` */
export const createTicket = (input: { subject: string; level: TicketLevel; message: string }) =>
  api.post<Ticket>('/user/tickets', input)

/** `GET /user/tickets/:id` (with messages). */
export const ticket = (id: ID, signal?: AbortSignal) =>
  api.get<Ticket>(`/user/tickets/${id}`, undefined, { signal })

/** `POST /user/tickets/:id/reply` */
export const replyTicket = (id: ID, message: string) =>
  api.post<Ticket>(`/user/tickets/${id}/reply`, { message })

/** `POST /user/tickets/:id/close` */
export const closeTicket = (id: ID) => api.post<true>(`/user/tickets/${id}/close`)

/** `GET /user/notices` */
export const notices = (query: ListQuery = {}, signal?: AbortSignal) =>
  api.list<Notice>('/user/notices', query, { signal })

/** `GET /user/knowledge` → `{category: [{id,title,updated_at}]}` */
export const knowledgeIndex = (query: { language?: string; keyword?: string } = {}, signal?: AbortSignal) =>
  api.get<KnowledgeIndex>('/user/knowledge', query, { signal })

/** `GET /user/knowledge/:id` (placeholders substituted server-side). */
export const knowledge = (id: ID, signal?: AbortSignal) =>
  api.get<KnowledgeArticle>(`/user/knowledge/${id}`, undefined, { signal })
