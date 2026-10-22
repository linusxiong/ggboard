/**
 * TypeScript shapes for every object in docs/API.md, docs/SUBSCRIPTION.md and docs/DATA_MODEL.md.
 *
 * Conventions (AGENTS.md §4): money = integer cents, traffic = bytes (except `Plan.transfer_enable`,
 * which is GB as entered by the admin), time = RFC3339 strings, nullable fields are `null`.
 */

export type ID = number
/** RFC3339 timestamp, e.g. `2027-01-01T00:00:00Z`. */
export type ISODateTime = string
/** Calendar date `YYYY-MM-DD`. */
export type ISODate = string
/** Integer amount in the smallest currency unit (cents). */
export type Cents = number
/** Traffic in bytes. */
export type Bytes = number
export type Locale = 'zh-CN' | 'en'
/** Map of locale → text, e.g. `{"zh-CN": "广告拦截", "en": "Ad blocking"}`. */
export type LocalizedText = Partial<Record<Locale, string>> & Record<string, string>

// ---------------------------------------------------------------- envelope & errors

/** `{"data": T}` success envelope. */
export interface Envelope<T> {
  data: T
}
/** `{"data": [...], "total": N}` list envelope. */
export interface ListEnvelope<T> {
  data: T[]
  total: number
}
/** Normalised list result returned by `api.list()`. */
export interface Page<T> {
  items: T[]
  total: number
}
export type ErrorCode =
  | 'validation_failed'
  | 'unauthorized'
  | 'forbidden'
  | 'not_found'
  | 'conflict'
  | 'rate_limited'
  | 'payment_failed'
  | 'internal'
  // client-side only
  | 'network'
  | 'timeout'
  | 'aborted'
  | 'bad_response'
/** `{"code": "validation_failed", "message": "...", "errors": {"field": ["msg"]}}` */
export interface ErrorBody {
  code: ErrorCode | (string & {})
  message: string
  errors?: Record<string, string[]>
}

/** Common list query: `page` (1-based), `page_size` (default 20, max 200), `sort` (`field` / `-field`). */
export interface ListQuery {
  page?: number
  page_size?: number
  sort?: string
}
export type QueryValue = string | number | boolean | null | undefined
export type QueryParams = Record<string, QueryValue | readonly (string | number)[]>

// ---------------------------------------------------------------- users

export interface PlanRef {
  id: ID
  name: string
}

/** `User` (self view). */
export interface User {
  id: ID
  email: string
  uuid: string
  plan_id: ID | null
  plan: PlanRef | null
  group_id: ID | null
  transfer_enable: Bytes
  u: Bytes
  d: Bytes
  expired_at: ISODateTime | null
  next_reset_at: ISODateTime | null
  /** Mbps, `null` = unlimited. */
  speed_limit: number | null
  device_limit: number | null
  balance: Cents
  commission_balance: Cents
  /** Percent override, `null` = system default. */
  commission_rate: number | null
  /** Percent discount, `null` = none. */
  discount: number | null
  remind_expire: boolean
  remind_traffic: boolean
  is_admin: boolean
  is_staff: boolean
  banned: boolean
  avatar_url: string
  created_at: ISODateTime
  last_login_at: ISODateTime | null
}

/** 0 system default, 1 per period, 2 one-time. */
export type CommissionType = 0 | 1 | 2

/** `User` (admin view): self view + admin-only fields; the password hash is never returned. */
export interface AdminUser extends User {
  remarks: string
  invite_user_id: ID | null
  last_login_ip: string
  telegram_id: number | null
  /** Subscription token (admin can search by it). */
  token?: string
  commission_type?: CommissionType
}
export interface AdminUserDetail extends AdminUser {
  stats: { orders_total: Cents; commission_total: Cents }
}

export interface AuthResult {
  token: string
  user: User
}

// ---------------------------------------------------------------- plans

export type PlanPeriod =
  | 'monthly'
  | 'quarterly'
  | 'half_yearly'
  | 'yearly'
  | 'two_yearly'
  | 'three_yearly'
  | 'onetime'
  | 'reset_traffic'
export const PLAN_PERIODS: readonly PlanPeriod[] = [
  'monthly',
  'quarterly',
  'half_yearly',
  'yearly',
  'two_yearly',
  'three_yearly',
  'onetime',
  'reset_traffic',
]
/** Absent key = period not for sale. */
export type PlanPrices = Partial<Record<PlanPeriod, Cents>>
/** 0 monthly on the 1st, 1 monthly on purchase day, 2 never, 3 yearly on Jan 1, 4 yearly on purchase day. */
export type ResetTrafficMethod = 0 | 1 | 2 | 3 | 4

export interface Plan {
  id: ID
  group_id: ID
  name: string
  /** Markdown description. */
  content: string
  tags: string[]
  /** **GB** as entered (converted to bytes on purchase). */
  transfer_enable: number
  speed_limit: number | null
  device_limit: number | null
  /** Hidden from `/guest/plans` ("no capacity leak"). */
  capacity_limit?: number | null
  prices: PlanPrices
  /** `null` = follow setting `reset_traffic_method`. */
  reset_traffic_method: ResetTrafficMethod | null
  show: boolean
  sell: boolean
  renew: boolean
  sort: number
  created_at: ISODateTime
  updated_at: ISODateTime
}
export interface AdminPlan extends Plan {
  user_count: number
}
export type PlanInput = Partial<Omit<Plan, 'id' | 'created_at' | 'updated_at'>>

// ---------------------------------------------------------------- nodes

export type NodeType =
  'vmess' | 'vless' | 'shadowsocks' | 'shadowsocksr' | 'trojan' | 'hysteria' | 'anytls' | 'mieru' | 'tuic'
export const NODE_TYPES: readonly NodeType[] = [
  'vmess',
  'vless',
  'shadowsocks',
  'shadowsocksr',
  'trojan',
  'hysteria',
  'anytls',
  'mieru',
  'tuic',
]

export type TransportNetwork = 'tcp' | 'ws' | 'grpc' | 'http' | 'h2' | 'httpupgrade' | 'xhttp'
/** xray-style `network_settings` (ws / grpc / http / httpupgrade / xhttp / tcp). */
export interface NetworkSettings {
  path?: string
  host?: string | string[]
  headers?: Record<string, string>
  serviceName?: string
  mode?: string
  extra?: Record<string, unknown>
  header?: { type: 'none' | 'http'; [key: string]: unknown }
  [key: string]: unknown
}
export interface TlsSettings {
  server_name?: string
  allow_insecure?: boolean
  public_key?: string
  private_key?: string
  short_id?: string
  dest?: string
  server_port?: string
  fingerprint?: string
  server_names?: string[]
}
export interface VmessSettings {
  network?: TransportNetwork
  network_settings?: NetworkSettings
  tls?: 0 | 1
  tls_settings?: TlsSettings
}
export interface VlessSettings {
  network?: TransportNetwork
  network_settings?: NetworkSettings
  /** 0 none, 1 tls, 2 reality */
  tls?: 0 | 1 | 2
  flow?: '' | 'xtls-rprx-vision'
  encryption?: string
  tls_settings?: TlsSettings
}
export type ShadowsocksCipher =
  | 'aes-128-gcm'
  | 'aes-256-gcm'
  | 'chacha20-ietf-poly1305'
  | '2022-blake3-aes-128-gcm'
  | '2022-blake3-aes-256-gcm'
  | '2022-blake3-chacha20-poly1305'
export interface ShadowsocksSettings {
  cipher?: ShadowsocksCipher
  server_key?: string
  obfs?: 'plain' | 'http' | 'tls'
  obfs_settings?: { path?: string; host?: string }
  plugin?: string
  plugin_opts?: string
}
export interface ShadowsocksRSettings {
  cipher?: string
  protocol?: string
  protocol_param?: string
  obfs?: string
  obfs_param?: string
  single_port_type?: 'protocol' | 'obfs'
}
export interface TrojanSettings {
  network?: 'tcp' | 'ws' | 'grpc'
  network_settings?: NetworkSettings
  server_name?: string
  allow_insecure?: boolean
}
export interface HysteriaSettings {
  version?: 1 | 2
  up_mbps?: number
  down_mbps?: number
  obfs?: 'plain' | 'salamander'
  obfs_password?: string
  server_name?: string
  allow_insecure?: boolean
  alpn?: string
  hop_interval?: number
}
export interface AnyTlsSettings {
  tls?: 1 | 2
  server_name?: string
  allow_insecure?: boolean
  padding_scheme?: string[]
  tls_settings?: TlsSettings
}
export interface MieruSettings {
  transport?: 'TCP' | 'UDP'
  multiplexing?: 'MULTIPLEXING_LOW' | 'MULTIPLEXING_MIDDLE' | 'MULTIPLEXING_HIGH'
}
export interface TuicSettings {
  congestion_control?: string
  zero_rtt_handshake?: boolean
  udp_relay_mode?: string
  server_name?: string
  allow_insecure?: boolean
  alpn?: string
}
/** `protocol_settings` keyed by `Node.type`. */
export interface ProtocolSettingsMap {
  vmess: VmessSettings
  vless: VlessSettings
  shadowsocks: ShadowsocksSettings
  shadowsocksr: ShadowsocksRSettings
  trojan: TrojanSettings
  hysteria: HysteriaSettings
  anytls: AnyTlsSettings
  mieru: MieruSettings
  tuic: TuicSettings
}
export type ProtocolSettings = ProtocolSettingsMap[NodeType]

export interface NodeLoad {
  cpu: number
  mem: number
  swap: number
  disk: number
  uptime: number
}
export interface NodeStatus {
  online: number
  last_check_at: ISODateTime | null
  last_push_at: ISODateTime | null
  load: NodeLoad | null
  version?: string
}
/** `Node` (admin). Narrow `protocol_settings` with `Node<'vless'>` etc. */
export interface Node<T extends NodeType = NodeType> {
  id: ID
  type: T
  name: string
  group_ids: ID[]
  route_ids: ID[]
  parent_id: ID | null
  host: string
  /** Client port; may be a range `10000-20000` (hysteria port hopping). */
  port: string
  server_port: number
  tags: string[]
  /** Traffic multiplier. */
  rate: number
  show: boolean
  sort: number
  protocol_settings: ProtocolSettingsMap[T]
  status: NodeStatus
  created_at: ISODateTime
  updated_at: ISODateTime
}
export type NodeInput = Partial<Omit<Node, 'id' | 'status' | 'created_at' | 'updated_at'>>

/** `/user/nodes` item. `host`/`port` only when `show_info_to_user` is enabled. */
export interface UserNode {
  id: ID
  name: string
  type: NodeType
  tags: string[]
  rate: number
  host?: string
  port?: string
  is_online: boolean
  sort: number
}

export interface ServerGroup {
  id: ID
  name: string
  user_count: number
  plan_count: number
  node_count: number
}

export type NodeRouteAction = 'block' | 'dns'
export interface NodeRoute {
  id: ID
  remarks: string
  match: string[]
  action: NodeRouteAction
  action_value: string
  created_at: ISODateTime
  updated_at: ISODateTime
}
export type NodeRouteInput = Pick<NodeRoute, 'remarks' | 'match' | 'action' | 'action_value'>

export interface NodeTrafficPoint {
  date: ISODate
  u: Bytes
  d: Bytes
}
export interface NodeOnlineUser {
  user_id: ID
  email: string
  ips: string[]
}

// ---------------------------------------------------------------- orders & payments

/** 1 new, 2 renew, 3 upgrade, 4 reset_traffic */
export type OrderType = 1 | 2 | 3 | 4
/** 0 pending, 1 processing, 2 cancelled, 3 completed, 4 discounted */
export type OrderStatus = 0 | 1 | 2 | 3 | 4
/** 0 pending, 1 processing, 2 valid, 3 invalid */
export type CommissionStatus = 0 | 1 | 2 | 3
export const ORDER_STATUS = { pending: 0, processing: 1, cancelled: 2, completed: 3, discounted: 4 } as const
export const ORDER_TYPE = { new: 1, renew: 2, upgrade: 3, reset_traffic: 4 } as const

export interface Order {
  id: ID
  trade_no: string
  user_id: ID
  plan_id: ID
  plan: PlanRef & Partial<Plan>
  type: OrderType
  period: PlanPeriod
  status: OrderStatus
  commission_status: CommissionStatus
  total_amount: Cents
  handling_amount: Cents
  discount_amount: Cents
  surplus_amount: Cents
  refund_amount: Cents
  balance_amount: Cents
  payment_id: ID | null
  payment: { id: ID; name: string } | null
  callback_no: string | null
  paid_at: ISODateTime | null
  created_at: ISODateTime
  updated_at: ISODateTime
}
export interface AdminOrder extends Order {
  user: { email: string }
  commission_balance?: Cents
}

export type CheckoutResultType = 'url' | 'qrcode' | 'paid' | 'html'
/** `url` → redirect, `qrcode` → render QR of `data`, `paid` → done, `html` → gateway form. */
export interface CheckoutResult {
  type: CheckoutResultType
  data: string
}

/** `/user/payment-methods` item. */
export interface PaymentMethod {
  id: ID
  name: string
  icon: string
  handling_fee_fixed: Cents
  handling_fee_percent: number
}
/** `Payment` (admin; `config` included, secret). */
export interface Payment {
  id: ID
  uuid: string
  gateway: string
  name: string
  icon: string
  config: Record<string, unknown>
  notify_domain: string
  handling_fee_fixed: Cents
  handling_fee_percent: number
  enabled: boolean
  sort: number
  /** Present in create/update responses. */
  notify_url?: string
  created_at: ISODateTime
  updated_at: ISODateTime
}
export interface PaymentInput {
  gateway: string
  name: string
  icon?: string
  config: Record<string, unknown>
  notify_domain?: string
  handling_fee_fixed?: Cents
  handling_fee_percent?: number
  enabled?: boolean
}
export type GatewayFieldType = 'text' | 'password' | 'select' | 'number' | 'textarea'
export interface GatewayField {
  key: string
  label: string
  description: string
  type: GatewayFieldType
  options?: string[]
  required?: boolean
}
export interface PaymentGateway {
  name: string
  label: string
  fields: GatewayField[]
}

// ---------------------------------------------------------------- coupons

/** 1 fixed amount (cents), 2 percent */
export type CouponType = 1 | 2
export interface Coupon {
  id: ID
  code: string
  name: string
  type: CouponType
  value: number
  show: boolean
  limit_use: number | null
  limit_use_with_user: number | null
  limit_plan_ids: ID[] | null
  limit_periods: PlanPeriod[] | null
  started_at: ISODateTime
  ended_at: ISODateTime
  created_at: ISODateTime
  updated_at: ISODateTime
}
export interface AdminCoupon extends Coupon {
  used_count: number
}
/** `/user/coupons/check` result. */
export type CouponCheck = Pick<Coupon, 'id' | 'code' | 'name' | 'type' | 'value'>
export interface CouponCreateInput {
  name: string
  code?: string
  type: CouponType
  value: number
  /** Batch-generate N codes. */
  count?: number
  limit_use?: number | null
  limit_use_with_user?: number | null
  limit_plan_ids?: ID[] | null
  limit_periods?: PlanPeriod[] | null
  started_at: ISODateTime
  ended_at: ISODateTime
  show?: boolean
}

// ---------------------------------------------------------------- tickets

/** 0 low, 1 medium, 2 high */
export type TicketLevel = 0 | 1 | 2
/** 0 open, 1 closed */
export type TicketStatus = 0 | 1
/** 0 awaiting staff, 1 replied */
export type TicketReplyStatus = 0 | 1
export interface TicketMessage {
  id: ID
  user_id: ID
  is_me: boolean
  message: string
  created_at: ISODateTime
}
export interface Ticket {
  id: ID
  user_id: ID
  subject: string
  level: TicketLevel
  status: TicketStatus
  reply_status: TicketReplyStatus
  created_at: ISODateTime
  updated_at: ISODateTime
  /** Only on detail endpoints. */
  messages?: TicketMessage[]
}
export interface AdminTicket extends Ticket {
  user: { email: string }
}

// ---------------------------------------------------------------- content

export interface Notice {
  id: ID
  title: string
  /** Markdown. */
  content: string
  img_url: string
  tags: string[]
  created_at: ISODateTime
}
export interface AdminNotice extends Notice {
  show: boolean
  sort: number
  updated_at: ISODateTime
}
export interface NoticeInput {
  title: string
  content: string
  img_url?: string
  tags?: string[]
  show?: boolean
}

export interface KnowledgeSummary {
  id: ID
  title: string
  updated_at: ISODateTime
}
/** `/user/knowledge` → `{"<category>": [KnowledgeSummary]}` */
export type KnowledgeIndex = Record<string, KnowledgeSummary[]>
export interface KnowledgeArticle {
  id: ID
  title: string
  /** Markdown with `{{siteName}}`/`{{subscribeUrl}}`… already substituted for users. */
  body: string
  category: string
  language: string
  updated_at: ISODateTime
}
/** Admin list item (no body). */
export interface AdminKnowledge {
  id: ID
  language: string
  category: string
  title: string
  show: boolean
  sort: number
  created_at: ISODateTime
  updated_at: ISODateTime
}
export interface AdminKnowledgeDetail extends AdminKnowledge {
  body: string
}
export interface KnowledgeInput {
  language: string
  category: string
  title: string
  body: string
  show?: boolean
}

// ---------------------------------------------------------------- user dashboard

export interface UserStats {
  pending_orders: number
  open_tickets: number
  notices_count: number
  invite_count: number
  commission_pending: Cents
}
export interface UserSubscription {
  subscribe_url: string
  token: string
  plan: (PlanRef & Partial<Plan>) | null
  transfer_enable: Bytes
  u: Bytes
  d: Bytes
  expired_at: ISODateTime | null
  next_reset_at: ISODateTime | null
  reset_day: number | null
  device_limit: number | null
  speed_limit: number | null
}
export interface SubscriptionReset {
  subscribe_url: string
  token: string
}
export interface TrafficLog {
  record_at: ISODate
  node_id: ID
  /** `null` for unknown/deleted nodes. */
  node_name: string | null
  rate: number
  u: Bytes
  d: Bytes
}
export interface InviteCode {
  code: string
  pv: number
  created_at: ISODateTime
}
export interface InviteInfo {
  codes: InviteCode[]
  stat: {
    register_count: number
    commission_pending: Cents
    commission_paid: Cents
    commission_rate: number
    invite_gen_limit: number
  }
}
export interface CommissionLog {
  trade_no: string
  order_amount: Cents
  get_amount: Cents
  created_at: ISODateTime
}

// ---------------------------------------------------------------- subscription formats & rule sets

/** Formats accepted by `flag=` (docs/SUBSCRIPTION.md). */
export type SubscribeFormat =
  | 'clash'
  | 'clash-meta'
  | 'stash'
  | 'singbox'
  | 'surge'
  | 'surfboard'
  | 'quantumultx'
  | 'shadowrocket'
  | 'loon'
  | 'v2ray'
  | 'ssr'

/** Query understood by `/client/subscribe` and `/user/subscription/preview`. */
export interface SubscribeQuery {
  flag?: SubscribeFormat | (string & {})
  types?: NodeType[]
  filter?: string
  profile?: string
  source?: string
  cats?: string[]
  final?: string
  mirror?: 0 | 1
  emoji?: 0 | 1
}

export type PolicyGroupType = 'select' | 'url-test' | 'fallback' | 'load-balance'
export interface PolicyGroup {
  name: string
  type: PolicyGroupType
  /** Regex/glob on node names and/or other group names, comma separated; `*` = all nodes. */
  include: string
  url?: string
  interval?: number
}
export type RuleDns = 'default' | 'fakeip' | 'none'
export interface RuleProfile {
  /** Template id from `catalog.templates`; `none` = no rules. */
  template: string
  /** Preferred rule-set source id. */
  source: string
  /** Category id → target policy group; absent/false = disabled. */
  categories: Record<string, string | false>
  groups: PolicyGroup[]
  final: string
  geoip_cn_direct: boolean
  /** `""` direct GitHub raw, `"default"` = settings.ruleset_mirror, or a custom prefix. */
  mirror: string
  dns: RuleDns
}
export type RuleBehavior = 'classical' | 'domain' | 'ipcidr'
export interface RulesetRuleRef {
  name?: string
  behavior?: RuleBehavior
  geosite?: string
  geoip?: string
}
export interface RulesetSource {
  name: string
  homepage?: string
  /** Format key (clash, surge, singbox_geosite, …) → URL template with `{name}`. */
  urls: Record<string, string>
}
export interface RulesetCategory {
  id: string
  name?: LocalizedText
  default_target: string
  order: number
  rules: Record<string, RulesetRuleRef>
}
export interface RulesetTemplate {
  id: string
  name: LocalizedText
  categories: Record<string, string>
  groups: PolicyGroup[]
  final: string
  geoip_cn_direct: boolean
}
export interface RulesetCatalog {
  sources: Record<string, RulesetSource>
  categories: RulesetCategory[]
  templates: RulesetTemplate[]
}
export interface AdminRulesets {
  builtin: RulesetCatalog
  custom: Partial<RulesetCatalog>
}

// ---------------------------------------------------------------- guest config

export type CaptchaProvider = 'none' | 'turnstile' | 'recaptcha' | 'hcaptcha'
export interface DownloadInfo {
  url: string
  version: string
}
export interface GuestConfig {
  app_name: string
  app_description: string
  app_url: string
  logo: string
  tos_url: string
  currency: string
  currency_symbol: string
  register_enabled: boolean
  email_verify: boolean
  email_whitelist_suffix: string[]
  invite_force: boolean
  captcha: { provider: CaptchaProvider; site_key: string }
  downloads: { windows: DownloadInfo; macos: DownloadInfo; android: DownloadInfo; ios: DownloadInfo }
  locales: Locale[]
}

// ---------------------------------------------------------------- passport

export interface LoginInput {
  email: string
  password: string
  captcha?: string
}
export interface RegisterInput {
  email: string
  /** min 8 */
  password: string
  invite_code?: string
  email_code?: string
  captcha?: string
}
export interface ForgotInput {
  email: string
  email_code: string
  password: string
}
export interface SendCodeInput {
  email: string
  captcha?: string
}

// ---------------------------------------------------------------- admin stats & system

export interface AdminOverview {
  today: { income: Cents; paid_orders: number; registers: number; commission: Cents }
  month: { income: Cents; paid_orders: number; registers: number }
  totals: {
    users: number
    active_users: number
    online_users: number
    pending_tickets: number
    pending_orders: number
    pending_commission: number
  }
  nodes: { total: number; online: number }
}
export type StatsMetric = 'income' | 'orders' | 'registers' | 'traffic'
export type StatsRange = '7d' | '30d' | '90d'
export interface SeriesPoint {
  date: ISODate
  value: number
}
export interface NodeRanking {
  node_id: ID
  name: string
  u: Bytes
  d: Bytes
  total: Bytes
}
export interface UserRanking {
  user_id: ID
  email: string
  u: Bytes
  d: Bytes
  total: Bytes
}
export interface SchedulerJob {
  name: string
  spec: string
  last_run: ISODateTime | null
  last_error: string | null
}
export interface SystemStatus {
  version: string
  go_version: string
  /** Seconds. */
  uptime: number
  db_driver: 'sqlite' | 'postgres' | 'mysql' | (string & {})
  started_at: ISODateTime
  scheduler: SchedulerJob[]
  mail_queue: number
  cache: Record<string, unknown>
}
export type LogLevel = 'debug' | 'info' | 'warn' | 'error'
export interface LogEntry {
  time: ISODateTime
  level: LogLevel | (string & {})
  msg: string
  attrs: Record<string, unknown>
}

// ---------------------------------------------------------------- admin inputs & filters

export type ExpiredFilter = 'active' | 'expired' | 'never'
export interface AdminUserFilters extends ListQuery {
  email?: string
  plan_id?: ID
  group_id?: ID
  banned?: boolean
  is_admin?: boolean
  is_staff?: boolean
  invite_user_id?: ID
  expired?: ExpiredFilter
  id?: ID
  uuid?: string
  token?: string
  remarks?: string
}
export interface AdminUserCreateInput {
  email: string
  password: string
  plan_id?: ID | null
  group_id?: ID | null
  /** GB */
  transfer_enable?: number
  expired_at?: ISODateTime | null
  is_admin?: boolean
  is_staff?: boolean
  speed_limit?: number | null
  device_limit?: number | null
  balance?: Cents
  remarks?: string
}
/** Any editable field incl. `password`, `banned`, `u`, `d`, `invite_user_id`. */
export type AdminUserUpdateInput = Partial<
  Omit<AdminUser, 'id' | 'plan' | 'avatar_url' | 'created_at' | 'last_login_at' | 'last_login_ip'>
> & { password?: string }
export interface AdminUserGenerateInput {
  email_prefix?: string
  email_suffix: string
  /** 1..500 */
  count: number
  plan_id?: ID | null
  expired_at?: ISODateTime | null
  password?: string
}
export interface AdminUserGenerateResult {
  count: number
  csv: string
}
export interface AdminUserMailInput extends Omit<AdminUserFilters, keyof ListQuery> {
  subject: string
  content: string
}
export interface AdminOrderFilters extends ListQuery {
  trade_no?: string
  user_id?: ID
  email?: string
  status?: OrderStatus
  commission_status?: CommissionStatus
  plan_id?: ID
  payment_id?: ID
  date_from?: ISODate
  date_to?: ISODate
}
export interface AdminOrderCreateInput {
  email?: string
  user_id?: ID
  plan_id: ID
  period: PlanPeriod
  total_amount?: Cents
}
export interface AdminTicketFilters extends ListQuery {
  status?: TicketStatus
  reply_status?: TicketReplyStatus
  user_id?: ID
}
export interface SortInput {
  ids: ID[]
}

// ---------------------------------------------------------------- settings (group → key)

export interface SiteSettings {
  app_name: string
  app_description: string
  app_url: string
  logo: string
  tos_url: string
  currency: string
  currency_symbol: string
  default_locale: Locale
  show_info_to_user: boolean
}
export interface RegisterSettings {
  stop_register: boolean
  email_verify: boolean
  email_whitelist_enable: boolean
  email_whitelist_suffix: string[]
  invite_force: boolean
  register_limit_by_ip_enable: boolean
  register_limit_count: number
  /** minutes */
  register_limit_expire: number
  password_limit_enable: boolean
  password_limit_count: number
  password_limit_expire: number
  try_out_plan_id: number
  try_out_hour: number
  captcha_provider: CaptchaProvider
  captcha_site_key: string
  /** secret */
  captcha_secret: string
}
export interface SubscribeSettings {
  subscribe_url: string[]
  plan_change_enable: boolean
  reset_traffic_method: ResetTrafficMethod
  surplus_enable: boolean
  show_info_to_server_enable: boolean
  show_protocol_to_server_enable: boolean
  default_remind_expire: boolean
  default_remind_traffic: boolean
  ruleset_default_template: string
  ruleset_default_source: string
  ruleset_mirror: string
  /** hours */
  subscribe_update_interval: number
  allow_user_rule_profile: boolean
}
export interface OrderSettings {
  order_timeout_minutes: number
}
export interface InviteSettings {
  /** percent */
  invite_commission: number
  invite_gen_limit: number
  invite_never_expire: boolean
  commission_first_time_enable: boolean
  commission_auto_check_enable: boolean
  /** currency units */
  commission_withdraw_limit: number
  commission_withdraw_method: string[]
  withdraw_close_enable: boolean
  commission_distribution_enable: boolean
  commission_distribution_l1: number
  commission_distribution_l2: number
  commission_distribution_l3: number
}
export interface ServerSettings {
  /** secret */
  server_token: string
  server_pull_interval: number
  server_push_interval: number
  server_white_list: string[]
  /** 0 strict, 1 relaxed */
  device_limit_mode: 0 | 1
}
export interface EmailSettings {
  email_driver: 'log' | 'smtp'
  email_host: string
  email_port: number
  email_encryption: 'ssl' | 'tls' | 'none'
  email_username: string
  /** secret */
  email_password: string
  email_from_address: string
  email_from_name: string
  email_template: string
  remind_mail_enable: boolean
}
export interface FrontendSettings {
  frontend_accent: string
  frontend_background_url: string
  frontend_customer_service_url: string
  frontend_footer_html: string
}
export interface AppSettings {
  windows_download_url: string
  windows_version: string
  macos_download_url: string
  macos_version: string
  android_download_url: string
  android_version: string
  ios_download_url: string
  ios_version: string
}
/** `GET /admin/settings` → all groups. */
export interface Settings {
  site: SiteSettings
  register: RegisterSettings
  subscribe: SubscribeSettings
  order: OrderSettings
  invite: InviteSettings
  server: ServerSettings
  email: EmailSettings
  frontend: FrontendSettings
  app: AppSettings
}
export type SettingsGroup = keyof Settings
/** `PUT /admin/settings` takes a flat `{key: value}` map. */
export type SettingsUpdate = Partial<
  SiteSettings &
    RegisterSettings &
    SubscribeSettings &
    OrderSettings &
    InviteSettings &
    ServerSettings &
    EmailSettings &
    FrontendSettings &
    AppSettings
>

// ---- FE-1 ---- user portal: MFA / passkeys / security, guest flags, catalog extras (declaration merging)
import type {
  AuthenticationResponseJSON,
  PublicKeyCredentialCreationOptionsJSON,
  PublicKeyCredentialRequestOptionsJSON,
  RegistrationResponseJSON,
} from '@simplewebauthn/browser'

export interface GuestConfig {
  /** Passkey (WebAuthn) sign-in is offered on the login page. */
  passkey_enabled?: boolean
  /** `false` → users cannot customise their rule profile (everyone gets `ruleset_default_template`). */
  allow_user_rule_profile?: boolean
}

export interface SubscriptionReset {
  /** New session JWT: the reset bumps `password_version`, which ends the old session. */
  auth_token?: string
}

export interface RulesetMirror {
  id: string
  /** Optional for admin-defined mirrors; fall back to `id` when absent. */
  name?: LocalizedText
  /** Prepended to raw URLs; `""` = direct GitHub. */
  prefix: string
}
export interface RulesetCatalog {
  mirrors?: RulesetMirror[]
  meta?: { generated_at: ISODateTime; version: number }
}
export interface RulesetSource {
  description?: LocalizedText
  license?: string
  /** Fallback priority (lower first). */
  order?: number
}
export interface RulesetTemplate {
  description?: LocalizedText
}

export type MfaMethod = 'totp' | (string & {})
/** `POST /passport/login` second-factor branch. */
export interface MfaChallenge {
  mfa_required: true
  mfa_token: string
  methods: MfaMethod[]
  has_passkey: boolean
}
/** `POST /passport/login` → session or second-factor challenge. */
export type LoginResult = AuthResult | MfaChallenge

/** WebAuthn ceremony start: `options` goes to `startAuthentication` / `startRegistration` as `optionsJSON`. */
export interface PasskeyLoginBegin {
  challenge_id: string
  options: PublicKeyCredentialRequestOptionsJSON
}
export interface PasskeyRegisterBegin {
  challenge_id: string
  options: PublicKeyCredentialCreationOptionsJSON
}
export type PasskeyAssertion = AuthenticationResponseJSON
export type PasskeyAttestation = RegistrationResponseJSON

export interface Passkey {
  id: ID | string
  name: string
  created_at: ISODateTime
  last_used_at: ISODateTime | null
  /** Synced passkey (iCloud Keychain, Google Password Manager, 1Password …). */
  backup_eligible: boolean
  transports: string[]
}
/** `GET /user/security` */
export interface SecurityOverview {
  totp_enabled: boolean
  totp_enabled_at: ISODateTime | null
  backup_codes_remaining: number
  passkeys: Passkey[]
}
/** `POST /user/security/totp/setup` */
export interface TotpSetup {
  secret: string
  otpauth_url: string
}
/** `POST /user/security/totp/enable` and `/backup-codes` — shown once. */
export interface BackupCodes {
  backup_codes: string[]
}

// ---- FE-2 ----
// Fields the backend returns beyond the original contract (interface merging keeps this append-only).

export interface Ticket {
  /** Author of the latest message (`null` before the first reply). */
  last_reply_user_id?: ID | null
}

export interface Order {
  /**
   * Payment deadline when the server exposes it; otherwise the UI uses
   * `created_at + order_timeout_minutes` (default 30).
   */
  expired_at?: ISODateTime | null
}

// ---- FE-3 ----
// Admin console part A (dashboard, users, plans, groups, nodes, routes). Declaration merging only
// adds fields the backend contract (docs/API.md, docs/NODE_PROTOCOLS.md §5/§8) returns but the
// foundation types did not list yet.

/** Three-state node status (docs/NODE_PROTOCOLS.md §5.1). */
export type NodeStatusState = 'online' | 'idle' | 'offline'
/** soga/upstream `status` body stores `{total, used}` byte pairs; older shapes send a percentage. */
export type NodeLoadValue = number | { total: number; used: number }
export interface NodeStatus {
  /** online = push within W, idle = only checks within W, offline. Derived client-side when absent. */
  state?: NodeStatusState
}
// eslint-disable-next-line @typescript-eslint/no-unused-vars -- merged declaration must repeat the type parameter
export interface Node<T extends NodeType = NodeType> {
  /** Parent node name for child (relay) nodes, `null` otherwise. */
  parent?: string | null
}
export interface AdminUser {
  /** Inviter e-mail (admin view), `null` when the user was not invited. */
  invite_user_email?: string | null
}
export interface VlessSettings {
  /** Server-side VLESS encryption string, e.g. `mlkem768x25519plus.native.600s.<key>` (§8 delta). */
  decryption?: string
}
export interface HysteriaSettings {
  /** Hysteria 2 only (V2bX, §8 delta). */
  ignore_client_bandwidth?: boolean
}
export interface ShadowsocksRSettings {
  /** Node password (§8 delta); empty → derived from created_at. */
  password?: string
}
/** `/admin/stats/users/ranking` query. */
export interface UserRankingQuery {
  date?: ISODate
  limit?: number
}

// ---- FE-4 ----
// Shapes for admin orders/coupons/payments/support/content/settings/rulesets/system that the
// sketch above lacks (verified against internal/model, internal/settings, internal/scheduler,
// internal/mail, internal/cache and internal/logx). Interface declarations below merge with the
// ones above (only optional fields are added).

export interface Order {
  /** Earlier orders consumed by an upgrade (status 4). */
  surplus_order_ids?: ID[] | null
  coupon_id?: ID | null
  invite_user_id?: ID | null
  /** Commission amount (cents) owed to the inviter. */
  commission_balance?: Cents
}

export interface GatewayField {
  /** Prefilled value for new payments. */
  default?: string
}

export interface Ticket {
  last_reply_user_id?: ID | null
}

/** `GET /admin/tickets` filters (backend also accepts `email`). */
export interface AdminTicketQuery extends AdminTicketFilters {
  email?: string
}

/** `GET /admin/knowledge` filters (the admin list is unpaginated). */
export interface AdminKnowledgeQuery extends ListQuery {
  language?: string
  category?: string
}

/** Value types of `internal/settings` definitions. */
export type SettingType = 'string' | 'int' | 'int64' | 'bool' | 'strings' | 'json'
/** One key of `settings.Groups()` (admin-UI metadata). */
export interface SettingDef {
  key: string
  group?: string
  type: SettingType | (string & {})
  default?: unknown
  /** Returned to admins only; masked in logs. */
  secret?: boolean
  hidden?: boolean
  /** Allowed values (enum) for string keys. */
  options?: string[]
  /** Inclusive bounds for ints. */
  min?: number
  max?: number
  description?: string
}
export interface SettingGroupMeta {
  name: string
  keys: SettingDef[]
}
/** `GET /admin/settings` → `{"<group>": {"<key>": value}, groups?: [...]}`. */
export type AdminSettingsResponse = Record<string, unknown> & { groups?: SettingGroupMeta[] }
/** `PUT /admin/settings` body (flat `{key: value}`; `null` resets a key to its default). */
export type SettingsPatch = Record<string, unknown>

/** RuleRef as published in `catalog.json` (docs/RULESETS.md). */
export interface RulesetRuleRefFull extends RulesetRuleRef {
  split?: boolean
  formats?: string[]
}
export interface RulesetUrlMeta {
  field: 'name' | 'geosite' | 'geoip' | (string & {})
  behavior?: RuleBehavior | (string & {})
  format?: string
}
export interface RulesetSourceFull extends RulesetSource {
  license?: string
  order?: number
  description?: LocalizedText
  url_meta?: Record<string, RulesetUrlMeta>
  aliases?: Record<string, string>
}
export interface RulesetCategoryFull {
  id: string
  name?: LocalizedText
  default_target: string
  order: number
  rules: Record<string, RulesetRuleRefFull | RulesetRuleRefFull[]>
}
export interface RulesetTemplateFull extends RulesetTemplate {
  description?: LocalizedText
}
export interface RulesetCatalogFull {
  sources: Record<string, RulesetSourceFull>
  categories: RulesetCategoryFull[]
  templates: RulesetTemplateFull[]
  mirrors?: RulesetMirror[]
  meta?: { generated_at?: string; version?: number }
}
/** Admin overrides (`rulesets_custom`), merged over the built-in catalog by id. */
export interface RulesetsCustom {
  sources?: Record<string, Partial<RulesetSourceFull>>
  categories?: RulesetCategoryFull[]
  templates?: RulesetTemplateFull[]
  mirrors?: RulesetMirror[]
}
/** `GET /admin/rulesets` */
export interface AdminRulesetsFull {
  builtin: RulesetCatalogFull
  custom: RulesetsCustom | null
  effective?: RulesetCatalogFull
}

/** `internal/scheduler.JobStatus`. */
export interface SchedulerJobStatus {
  name: string
  spec: string
  running?: boolean
  last_run: ISODateTime | null
  last_duration?: string
  /** `""` when the last run succeeded. */
  last_error: string | null
  next_run?: ISODateTime | null
  runs?: number
  failures?: number
}
/** `internal/mail.QueueStats`. */
export interface MailQueueStats {
  pending: number
  sent: number
  failed: number
  driver: string
}
/** `internal/cache.Stats`. */
export interface CacheStats {
  items: number
  max_items: number
  hits: number
  misses: number
  hit_ratio: number
}
/** `GET /admin/system/status` (full shape from the backend contract notes). */
export interface AdminSystemStatus {
  version: string
  go_version: string
  /** Seconds (number) or a Go duration string. */
  uptime: number | string
  db_driver: string
  started_at: ISODateTime
  scheduler: SchedulerJobStatus[]
  mail?: Partial<MailQueueStats>
  /** Older sketch: pending mail count. */
  mail_queue?: number
  cache?: Partial<CacheStats> & Record<string, unknown>
  online?: { nodes: number; users: number }
  goroutines?: number
  /** Bytes. */
  memory?: { alloc: number; sys: number }
}
/** `internal/logx.Entry` (`attrs` is omitted when empty). */
export interface AdminLogEntry {
  time: ISODateTime
  level: string
  msg: string
  attrs?: Record<string, unknown> | null
}
export interface AdminLogQuery {
  level?: string
  limit?: number
}
