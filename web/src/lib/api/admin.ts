/** Admin endpoints (`/admin`, 🛡 admin; support & content 🧑‍💼 staff) — docs/API.md §Admin. */
import { api, requestBlob } from './client'
import type {
  AdminCoupon,
  AdminKnowledge,
  AdminKnowledgeDetail,
  AdminNotice,
  AdminOrder,
  AdminOrderCreateInput,
  AdminOrderFilters,
  AdminOverview,
  AdminPlan,
  AdminRulesets,
  AdminTicket,
  AdminTicketFilters,
  AdminUser,
  AdminUserCreateInput,
  AdminUserDetail,
  AdminUserFilters,
  AdminUserGenerateInput,
  AdminUserGenerateResult,
  AdminUserMailInput,
  AdminUserUpdateInput,
  CommissionStatus,
  Coupon,
  CouponCreateInput,
  ID,
  ISODate,
  KnowledgeInput,
  ListQuery,
  LogEntry,
  Node,
  NodeInput,
  NodeOnlineUser,
  NodeRanking,
  NodeRoute,
  NodeRouteInput,
  NodeTrafficPoint,
  NoticeInput,
  Order,
  Payment,
  PaymentGateway,
  PaymentInput,
  Plan,
  PlanInput,
  RulesetCatalog,
  SeriesPoint,
  ServerGroup,
  Settings,
  SettingsUpdate,
  StatsMetric,
  StatsRange,
  SystemStatus,
  Ticket,
  User,
  UserRanking,
} from './types'

type Sig = AbortSignal | undefined
const o = (signal: Sig) => ({ signal })

// ---------------------------------------------------------------- stats

/** `GET /admin/stats/overview` */
export const statsOverview = (signal?: AbortSignal) =>
  api.get<AdminOverview>('/admin/stats/overview', undefined, o(signal))

/** `GET /admin/stats/series?metric=&range=` */
export const statsSeries = (metric: StatsMetric, range: StatsRange = '30d', signal?: AbortSignal) =>
  api.get<SeriesPoint[]>('/admin/stats/series', { metric, range }, o(signal))

/** `GET /admin/stats/nodes/ranking?date=` */
export const nodeRanking = (date?: ISODate, signal?: AbortSignal) =>
  api.get<NodeRanking[]>('/admin/stats/nodes/ranking', { date }, o(signal))

/** `GET /admin/stats/users/ranking?date=&limit=` */
export const userRanking = (query: { date?: ISODate; limit?: number } = {}, signal?: AbortSignal) =>
  api.get<UserRanking[]>('/admin/stats/users/ranking', query, o(signal))

// ---------------------------------------------------------------- users

/** `GET /admin/users` */
export const users = (query: AdminUserFilters = {}, signal?: AbortSignal) =>
  api.list<AdminUser>('/admin/users', query, o(signal))

/** `POST /admin/users` */
export const createUser = (input: AdminUserCreateInput) => api.post<AdminUser>('/admin/users', input)

/** `GET /admin/users/:id` (+ stats). */
export const user = (id: ID, signal?: AbortSignal) =>
  api.get<AdminUserDetail>(`/admin/users/${id}`, undefined, o(signal))

/** `PATCH /admin/users/:id` */
export const updateUser = (id: ID, input: AdminUserUpdateInput) =>
  api.patch<AdminUser>(`/admin/users/${id}`, input)

/** `DELETE /admin/users/:id` (cascades orders/tickets/stat_users). */
export const deleteUser = (id: ID) => api.delete(`/admin/users/${id}`)

/** `POST /admin/users/:id/reset-secret` → new uuid/token. */
export const resetUserSecret = (id: ID) => api.post<AdminUser>(`/admin/users/${id}/reset-secret`)

/** `POST /admin/users/:id/login-token` → one-time login URL (`/login?token=…`). */
export const userLoginToken = (id: ID) => api.post<{ url: string }>(`/admin/users/${id}/login-token`)

/** `POST /admin/users/generate` → `{count, csv}` */
export const generateUsers = (input: AdminUserGenerateInput) =>
  api.post<AdminUserGenerateResult>('/admin/users/generate', input)

/** `POST /admin/users/export` (same filters as list) → CSV blob. */
export const exportUsers = (filters: Omit<AdminUserFilters, keyof ListQuery> = {}) =>
  requestBlob('POST', '/admin/users/export', { body: filters })

/** `POST /admin/users/mail` → `{queued}` */
export const mailUsers = (input: AdminUserMailInput) =>
  api.post<{ queued: number }>('/admin/users/mail', input)

/** `POST /admin/users/:id/traffic-reset` */
export const resetUserTraffic = (id: ID) => api.post<User>(`/admin/users/${id}/traffic-reset`)

// ---------------------------------------------------------------- plans

/** `GET /admin/plans` → `[Plan + user_count]` */
export const plans = (signal?: AbortSignal) => api.get<AdminPlan[]>('/admin/plans', undefined, o(signal))
/** `POST /admin/plans` */
export const createPlan = (input: PlanInput) => api.post<Plan>('/admin/plans', input)
/** `GET /admin/plans/:id` */
export const plan = (id: ID, signal?: AbortSignal) =>
  api.get<AdminPlan>(`/admin/plans/${id}`, undefined, o(signal))
/** `PATCH /admin/plans/:id` */
export const updatePlan = (id: ID, input: PlanInput) => api.patch<Plan>(`/admin/plans/${id}`, input)
/** `DELETE /admin/plans/:id` */
export const deletePlan = (id: ID) => api.delete(`/admin/plans/${id}`)
/** `POST /admin/plans/sort` */
export const sortPlans = (ids: ID[]) => api.post<true>('/admin/plans/sort', { ids })

// ---------------------------------------------------------------- server groups

/** `GET /admin/groups` */
export const groups = (signal?: AbortSignal) => api.get<ServerGroup[]>('/admin/groups', undefined, o(signal))
/** `POST /admin/groups` */
export const createGroup = (name: string) => api.post<ServerGroup>('/admin/groups', { name })
/** `PATCH /admin/groups/:id` */
export const updateGroup = (id: ID, name: string) => api.patch<ServerGroup>(`/admin/groups/${id}`, { name })
/** `DELETE /admin/groups/:id` */
export const deleteGroup = (id: ID) => api.delete(`/admin/groups/${id}`)

// ---------------------------------------------------------------- nodes

/** `GET /admin/nodes` (with status). */
export const nodes = (signal?: AbortSignal) => api.get<Node[]>('/admin/nodes', undefined, o(signal))
/** `POST /admin/nodes` */
export const createNode = (input: NodeInput) => api.post<Node>('/admin/nodes', input)
/** `GET /admin/nodes/:id` */
export const node = (id: ID, signal?: AbortSignal) =>
  api.get<Node>(`/admin/nodes/${id}`, undefined, o(signal))
/** `PATCH /admin/nodes/:id` */
export const updateNode = (id: ID, input: NodeInput) => api.patch<Node>(`/admin/nodes/${id}`, input)
/** `DELETE /admin/nodes/:id` */
export const deleteNode = (id: ID) => api.delete(`/admin/nodes/${id}`)
/** `POST /admin/nodes/:id/copy` */
export const copyNode = (id: ID) => api.post<Node>(`/admin/nodes/${id}/copy`)
/** `POST /admin/nodes/sort` */
export const sortNodes = (ids: ID[]) => api.post<true>('/admin/nodes/sort', { ids })
/** `GET /admin/nodes/:id/stats?range=` */
export const nodeStats = (id: ID, range: StatsRange = '7d', signal?: AbortSignal) =>
  api.get<NodeTrafficPoint[]>(`/admin/nodes/${id}/stats`, { range }, o(signal))
/** `GET /admin/nodes/:id/online` */
export const nodeOnline = (id: ID, signal?: AbortSignal) =>
  api.get<NodeOnlineUser[]>(`/admin/nodes/${id}/online`, undefined, o(signal))

// ---------------------------------------------------------------- node routes

/** `GET /admin/routes` */
export const routes = (signal?: AbortSignal) => api.get<NodeRoute[]>('/admin/routes', undefined, o(signal))
/** `POST /admin/routes` */
export const createRoute = (input: NodeRouteInput) => api.post<NodeRoute>('/admin/routes', input)
/** `PATCH /admin/routes/:id` */
export const updateRoute = (id: ID, input: Partial<NodeRouteInput>) =>
  api.patch<NodeRoute>(`/admin/routes/${id}`, input)
/** `DELETE /admin/routes/:id` */
export const deleteRoute = (id: ID) => api.delete(`/admin/routes/${id}`)

// ---------------------------------------------------------------- orders

/** `GET /admin/orders` → `[Order + user.email]` */
export const orders = (query: AdminOrderFilters = {}, signal?: AbortSignal) =>
  api.list<AdminOrder>('/admin/orders', query, o(signal))
/** `GET /admin/orders/:id` */
export const order = (id: ID, signal?: AbortSignal) =>
  api.get<AdminOrder>(`/admin/orders/${id}`, undefined, o(signal))
/** `POST /admin/orders` — assign a pending order. */
export const createOrder = (input: AdminOrderCreateInput) => api.post<Order>('/admin/orders', input)
/** `POST /admin/orders/:id/paid` — manual confirm → fulfil. */
export const markOrderPaid = (id: ID) => api.post<Order>(`/admin/orders/${id}/paid`)
/** `POST /admin/orders/:id/cancel` */
export const cancelOrder = (id: ID) => api.post<Order>(`/admin/orders/${id}/cancel`)
/** `PATCH /admin/orders/:id` */
export const updateOrder = (id: ID, input: { commission_status: CommissionStatus }) =>
  api.patch<Order>(`/admin/orders/${id}`, input)

// ---------------------------------------------------------------- coupons

/** `GET /admin/coupons` → `[Coupon + used_count]` */
export const coupons = (query: ListQuery = {}, signal?: AbortSignal) =>
  api.list<AdminCoupon>('/admin/coupons', query, o(signal))
/** `POST /admin/coupons` → `[Coupon]` (batch when `count` > 1). */
export const createCoupons = (input: CouponCreateInput) => api.post<Coupon[]>('/admin/coupons', input)
/** `PATCH /admin/coupons/:id` */
export const updateCoupon = (id: ID, input: Partial<CouponCreateInput>) =>
  api.patch<Coupon>(`/admin/coupons/${id}`, input)
/** `DELETE /admin/coupons/:id` */
export const deleteCoupon = (id: ID) => api.delete(`/admin/coupons/${id}`)

// ---------------------------------------------------------------- payments

/** `GET /admin/payments` (config included). */
export const payments = (signal?: AbortSignal) => api.get<Payment[]>('/admin/payments', undefined, o(signal))
/** `GET /admin/payments/gateways` — installed gateway plugins and their config fields. */
export const paymentGateways = (signal?: AbortSignal) =>
  api.get<PaymentGateway[]>('/admin/payments/gateways', undefined, o(signal))
/** `POST /admin/payments` → `Payment` + `notify_url` */
export const createPayment = (input: PaymentInput) => api.post<Payment>('/admin/payments', input)
/** `PATCH /admin/payments/:id` */
export const updatePayment = (id: ID, input: Partial<PaymentInput>) =>
  api.patch<Payment>(`/admin/payments/${id}`, input)
/** `DELETE /admin/payments/:id` */
export const deletePayment = (id: ID) => api.delete(`/admin/payments/${id}`)
/** `POST /admin/payments/sort` */
export const sortPayments = (ids: ID[]) => api.post<true>('/admin/payments/sort', { ids })

// ---------------------------------------------------------------- tickets (staff)

/** `GET /admin/tickets` → `[Ticket + user.email]` */
export const tickets = (query: AdminTicketFilters = {}, signal?: AbortSignal) =>
  api.list<AdminTicket>('/admin/tickets', query, o(signal))
/** `GET /admin/tickets/:id` (with messages). */
export const ticket = (id: ID, signal?: AbortSignal) =>
  api.get<AdminTicket>(`/admin/tickets/${id}`, undefined, o(signal))
/** `POST /admin/tickets/:id/reply` */
export const replyTicket = (id: ID, message: string) =>
  api.post<Ticket>(`/admin/tickets/${id}/reply`, { message })
/** `POST /admin/tickets/:id/close` */
export const closeTicket = (id: ID) => api.post<true>(`/admin/tickets/${id}/close`)

// ---------------------------------------------------------------- notices (staff)

/** `GET /admin/notices` */
export const notices = (query: ListQuery = {}, signal?: AbortSignal) =>
  api.list<AdminNotice>('/admin/notices', query, o(signal))
/** `POST /admin/notices` */
export const createNotice = (input: NoticeInput) => api.post<AdminNotice>('/admin/notices', input)
/** `PATCH /admin/notices/:id` */
export const updateNotice = (id: ID, input: Partial<NoticeInput>) =>
  api.patch<AdminNotice>(`/admin/notices/${id}`, input)
/** `DELETE /admin/notices/:id` */
export const deleteNotice = (id: ID) => api.delete(`/admin/notices/${id}`)
/** `POST /admin/notices/sort` */
export const sortNotices = (ids: ID[]) => api.post<true>('/admin/notices/sort', { ids })

// ---------------------------------------------------------------- knowledge (staff)

/** `GET /admin/knowledge` (no body). */
export const knowledgeList = (
  query: ListQuery & { language?: string; category?: string } = {},
  signal?: AbortSignal,
) => api.list<AdminKnowledge>('/admin/knowledge', query, o(signal))
/** `POST /admin/knowledge` */
export const createKnowledge = (input: KnowledgeInput) =>
  api.post<AdminKnowledgeDetail>('/admin/knowledge', input)
/** `GET /admin/knowledge/:id` */
export const knowledge = (id: ID, signal?: AbortSignal) =>
  api.get<AdminKnowledgeDetail>(`/admin/knowledge/${id}`, undefined, o(signal))
/** `PATCH /admin/knowledge/:id` */
export const updateKnowledge = (id: ID, input: Partial<KnowledgeInput>) =>
  api.patch<AdminKnowledgeDetail>(`/admin/knowledge/${id}`, input)
/** `DELETE /admin/knowledge/:id` */
export const deleteKnowledge = (id: ID) => api.delete(`/admin/knowledge/${id}`)
/** `POST /admin/knowledge/sort` */
export const sortKnowledge = (ids: ID[]) => api.post<true>('/admin/knowledge/sort', { ids })

// ---------------------------------------------------------------- settings & system

/** `GET /admin/settings` → all groups. */
export const settings = (signal?: AbortSignal) => api.get<Settings>('/admin/settings', undefined, o(signal))
/** `PUT /admin/settings` — flat `{key: value}` map. */
export const saveSettings = (input: SettingsUpdate) => api.put<true>('/admin/settings', input)
/** `POST /admin/settings/mail/test` */
export const testMail = (email: string) => api.post<true>('/admin/settings/mail/test', { email })

/** `GET /admin/rulesets` → `{builtin, custom}` */
export const rulesets = (signal?: AbortSignal) =>
  api.get<AdminRulesets>('/admin/rulesets', undefined, o(signal))
/** `PUT /admin/rulesets` — custom overrides merged over builtin. */
export const saveRulesets = (custom: Partial<RulesetCatalog>) => api.put<true>('/admin/rulesets', { custom })

/** `GET /admin/system/status` */
export const systemStatus = (signal?: AbortSignal) =>
  api.get<SystemStatus>('/admin/system/status', undefined, o(signal))
/** `GET /admin/system/logs?level=&limit=` (in-memory ring buffer, last 1000). */
export const systemLogs = (query: { level?: string; limit?: number } = {}, signal?: AbortSignal) =>
  api.get<LogEntry[]>('/admin/system/logs', query, o(signal))

// ---- FE-4 ----
// Endpoints with the full response shapes used by the admin pages (orders … system).
import type {
  AdminKnowledgeQuery,
  AdminLogEntry,
  AdminLogQuery,
  AdminRulesetsFull,
  AdminSettingsResponse,
  AdminSystemStatus,
  AdminTicketQuery,
  RulesetsCustom,
  SettingsPatch,
} from './types'

/** `GET /admin/tickets` with the `email` filter. */
export const ticketList = (query: AdminTicketQuery = {}, signal?: AbortSignal) =>
  api.list<AdminTicket>('/admin/tickets', query, o(signal))

/** `GET /admin/notices` (unpaginated list). */
export const noticeList = (signal?: AbortSignal) =>
  api.list<AdminNotice>('/admin/notices', undefined, o(signal))

/** `GET /admin/knowledge` (unpaginated, no body). */
export const knowledgeAll = (query: AdminKnowledgeQuery = {}, signal?: AbortSignal) =>
  api.list<AdminKnowledge>('/admin/knowledge', query, o(signal))

/** `GET /admin/settings` → `{"<group>": {...}, groups: [...]}` */
export const settingsWithMeta = (signal?: AbortSignal) =>
  api.get<AdminSettingsResponse>('/admin/settings', undefined, o(signal))
/** `PUT /admin/settings` — only the changed keys; 400 `errors` is keyed by setting key. */
export const patchSettings = (values: SettingsPatch) => api.put<true>('/admin/settings', values)

/** `GET /admin/rulesets` → `{builtin, custom, effective}` */
export const rulesetsFull = (signal?: AbortSignal) =>
  api.get<AdminRulesetsFull>('/admin/rulesets', undefined, o(signal))
/** `PUT /admin/rulesets {custom}` — overrides merged by id; 400 with `errors` when invalid. */
export const saveRulesetsCustom = (custom: RulesetsCustom) => api.put<true>('/admin/rulesets', { custom })

/** `GET /admin/system/status` (full shape). */
export const systemStatusFull = (signal?: AbortSignal) =>
  api.get<AdminSystemStatus>('/admin/system/status', undefined, o(signal))
/** `GET /admin/system/logs?level=&limit=` */
export const systemLogEntries = (query: AdminLogQuery = {}, signal?: AbortSignal) =>
  api.get<AdminLogEntry[]>('/admin/system/logs', query, o(signal))
