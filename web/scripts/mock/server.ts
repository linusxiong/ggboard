/**
 * Dev-only mock of the admin API used by the admin console pages (orders, coupons, payments,
 * tickets, notices, knowledge, settings, mail templates (`mail-templates.ts`), rule sets, system) so they can
 * be clicked through without the Go backend. In-memory state, fake data on example.com, no persistence.
 *
 *   node scripts/mock/server.ts            # http://localhost:8787 (MOCK_PORT to change)
 *   GGBOARD_API=http://localhost:8787 pnpm dev
 *
 * Sign in with any email/password (the user is an admin). Never part of the production bundle.
 */
import { readFileSync } from 'node:fs'
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http'
import path from 'node:path'
import { handleMailTemplates } from './mail-templates.ts'

const PORT = Number(process.env.MOCK_PORT ?? 8787)
const root = path.resolve(import.meta.dirname, '..', '..', '..')
const fixtures = path.resolve(import.meta.dirname, '..', 'fixtures')
const readJson = (file: string): unknown => JSON.parse(readFileSync(file, 'utf8'))

type Json = Record<string, unknown>
const now = Date.now()
const iso = (ms: number) => new Date(ms).toISOString().replace(/\.\d{3}Z$/, 'Z')
const DAY = 86_400_000
let seq = 1000
const nextId = () => ++seq
const pick = <T>(list: readonly T[], i: number): T => list[i % list.length]
/** unknown → string for ids/text fields of request bodies. */
const str = (v: unknown): string => (typeof v === 'string' ? v : typeof v === 'number' ? String(v) : '')

// ---------------------------------------------------------------- data

const user = { ...(readJson(path.join(fixtures, 'user.json')) as Json), is_admin: true }
const config = readJson(path.join(fixtures, 'guest-config.json')) as Json

const plans = [
  {
    id: 1,
    group_id: 1,
    name: 'Basic',
    prices: { monthly: 1500, quarterly: 4000, yearly: 15000 },
    user_count: 120,
  },
  {
    id: 2,
    group_id: 1,
    name: 'Pro',
    prices: { monthly: 3000, yearly: 30000, onetime: 9900 },
    user_count: 48,
  },
  { id: 3, group_id: 2, name: 'Traffic pack', prices: { reset_traffic: 800 }, user_count: 0 },
].map((p) => ({
  content: '',
  tags: [],
  transfer_enable: 100,
  speed_limit: null,
  device_limit: null,
  conn_limit: null,
  reset_traffic_method: null,
  show: true,
  sell: true,
  renew: true,
  sort: p.id,
  created_at: iso(now - 90 * DAY),
  updated_at: iso(now - 10 * DAY),
  ...p,
}))

const gateways = [
  {
    name: 'alipay_f2f',
    label: '支付宝当面付',
    fields: [
      {
        key: 'app_id',
        label: 'APPID',
        description: 'Open Platform app id (当面付 signed).',
        type: 'text',
        required: true,
      },
      {
        key: 'private_key',
        label: '应用私钥',
        description: 'RSA2 ≥ 2048, PKCS#1/PKCS#8, PEM or bare base64.',
        type: 'textarea',
        required: true,
      },
      {
        key: 'alipay_public_key',
        label: '支付宝公钥',
        description: 'The Alipay public key, not the app public key.',
        type: 'textarea',
        required: true,
      },
      {
        key: 'sandbox',
        label: '沙箱',
        description: '1 = sandbox gateway.',
        type: 'select',
        options: ['0', '1'],
        default: '0',
        required: false,
      },
    ],
  },
  {
    name: 'btcpay',
    label: 'BTCPay Server',
    fields: [
      {
        key: 'server_url',
        label: 'Server URL',
        description: 'e.g. https://btcpay.example.com',
        type: 'text',
        required: true,
      },
      {
        key: 'store_id',
        label: 'Store ID',
        description: 'Store → Settings → General.',
        type: 'text',
        required: true,
      },
      {
        key: 'api_key',
        label: 'API key',
        description: 'cancreateinvoice + canviewinvoices.',
        type: 'password',
        required: true,
      },
      {
        key: 'webhook_secret',
        label: 'Webhook secret',
        description: 'Store → Settings → Webhooks.',
        type: 'password',
        required: true,
      },
    ],
  },
  {
    name: 'coinbase',
    label: 'Coinbase Business',
    fields: [
      {
        key: 'api_key_name',
        label: 'CDP API key name',
        description: 'organizations/{org_id}/apiKeys/{key_id}',
        type: 'text',
        required: true,
      },
      {
        key: 'api_key_secret',
        label: 'CDP API key secret',
        description: 'ECDSA P-256 PEM.',
        type: 'textarea',
        required: true,
      },
      { key: 'webhook_secret', label: 'Webhook secret', description: '', type: 'password', required: true },
      {
        key: 'exchange_rate',
        label: 'Exchange rate',
        description: 'USDC per site-currency unit.',
        type: 'number',
        default: '1',
        required: false,
      },
    ],
  },
  {
    name: 'epay',
    label: '易支付 (EPay)',
    fields: [
      {
        key: 'url',
        label: '网关地址',
        description: 'Base URL, no /submit.php, no trailing /.',
        type: 'text',
        required: true,
      },
      { key: 'pid', label: '商户 ID', description: '', type: 'text', required: true },
      { key: 'key', label: '商户密钥', description: 'MD5 key.', type: 'password', required: true },
      {
        key: 'type',
        label: '支付方式',
        description: 'alipay / wxpay / qqpay or a provider value.',
        type: 'select',
        options: ['alipay', 'wxpay', 'qqpay'],
        required: false,
      },
    ],
  },
  {
    name: 'epusdt',
    label: 'Epusdt / BEpusdt',
    fields: [
      {
        key: 'api_url',
        label: 'API 地址',
        description: 'e.g. https://usdt.example.com',
        type: 'text',
        required: true,
      },
      { key: 'api_token', label: 'API Token', description: '', type: 'password', required: true },
      {
        key: 'currency_type',
        label: '交易类型',
        description: 'Sent as trade_type.',
        type: 'text',
        default: 'usdt.trc20',
        required: false,
      },
    ],
  },
  {
    name: 'stripe',
    label: 'Stripe Checkout',
    fields: [
      {
        key: 'secret_key',
        label: 'Secret key',
        description: 'sk_live_… / sk_test_… / rk_…',
        type: 'password',
        required: true,
      },
      {
        key: 'webhook_secret',
        label: 'Webhook signing secret',
        description: 'whsec_…',
        type: 'password',
        required: true,
      },
      {
        key: 'currency',
        label: 'Currency',
        description: 'Defaults to the site currency.',
        type: 'text',
        required: false,
      },
      {
        key: 'exchange_rate',
        label: 'Exchange rate',
        description: 'Needed when currency ≠ site currency.',
        type: 'number',
        required: false,
      },
      {
        key: 'payment_methods',
        label: 'Payment methods',
        description: '',
        type: 'select',
        options: ['auto', 'card', 'alipay', 'wechat_pay'],
        default: 'auto',
        required: false,
      },
    ],
  },
]

const uuid = () =>
  Array.from({ length: 32 }, () => '0123456789abcdef'[Math.floor(Math.random() * 16)]).join('')
const appUrl = str(config.app_url) || 'https://panel.example.com'
const notifyUrl = (p: Json) =>
  `${(str(p.notify_domain) || appUrl).replace(/\/+$/, '')}/api/v1/guest/payment/notify/${str(p.gateway)}/${str(p.uuid)}`

let payments: Json[] = [
  {
    id: 1,
    gateway: 'epay',
    name: '支付宝',
    icon: '',
    config: { url: 'https://pay.example.com', pid: '1001', key: 'demo', type: 'alipay' },
    handling_fee_fixed: 0,
    handling_fee_percent: 0.6,
    enabled: true,
    sort: 1,
  },
  {
    id: 2,
    gateway: 'stripe',
    name: 'Card (Stripe)',
    icon: '',
    config: { secret_key: 'sk_test_demo', webhook_secret: 'whsec_demo', payment_methods: 'auto' },
    handling_fee_fixed: 30,
    handling_fee_percent: 2.9,
    enabled: false,
    sort: 2,
  },
].map((p) => ({
  uuid: uuid(),
  notify_domain: '',
  created_at: iso(now - 30 * DAY),
  updated_at: iso(now - DAY),
  ...p,
}))

const emails = ['alice', 'bob', 'carol', 'dave', 'erin', 'frank', 'grace', 'heidi'].map(
  (n) => `${n}@example.com`,
)
const periods = ['monthly', 'quarterly', 'yearly', 'onetime', 'reset_traffic']
const orders: Json[] = Array.from({ length: 57 }, (_, i) => {
  const plan = pick(plans, i)
  const status = [3, 3, 0, 2, 3, 1, 4][i % 7]
  const created = now - i * 0.9 * DAY - (i % 5) * 3_600_000
  const total = 1500 + (i % 6) * 500
  return {
    id: i + 1,
    trade_no: `2026102${String(100000 + i * 7919).slice(-6)}${String(i).padStart(4, '0')}`,
    user_id: 10 + (i % emails.length),
    user: { id: 10 + (i % emails.length), email: pick(emails, i) },
    plan_id: plan.id,
    plan: { id: plan.id, name: plan.name },
    type: [1, 2, 3, 4][i % 4],
    period: pick(periods, i),
    status,
    commission_status: [0, 1, 2, 3][i % 4],
    commission_balance: i % 3 === 0 ? Math.round(total * 0.1) : 0,
    total_amount: total,
    handling_amount: i % 4 === 0 ? 50 : 0,
    discount_amount: i % 5 === 0 ? 300 : 0,
    surplus_amount: i % 7 === 6 ? 700 : 0,
    refund_amount: 0,
    balance_amount: i % 6 === 0 ? 200 : 0,
    surplus_order_ids: i % 7 === 6 ? [i - 2, i - 4].filter((x) => x > 0) : [],
    coupon_id: i % 5 === 0 ? 3 : null,
    payment_id: status === 0 ? null : pick(payments, i).id,
    payment: status === 0 ? null : { id: pick(payments, i).id, name: pick(payments, i).name },
    callback_no: status === 3 ? `CB${2026000000 + i}` : '',
    paid_at: status === 3 ? iso(created + 120_000) : null,
    created_at: iso(created),
    updated_at: iso(created + 120_000),
  }
})

let coupons: Json[] = Array.from({ length: 25 }, (_, i) => ({
  id: i + 1,
  code: `GG${(i * 7349 + 1000).toString(36).toUpperCase().padStart(6, 'X')}`,
  name: i % 2 ? `Spring sale ${i}` : `Welcome ${i}`,
  type: i % 3 === 0 ? 2 : 1,
  value: i % 3 === 0 ? 10 + (i % 5) * 5 : 500 + i * 100,
  show: i % 4 === 0,
  limit_use: i % 2 ? 100 : null,
  limit_use_with_user: i % 3 ? 1 : null,
  limit_plan_ids: i % 4 === 1 ? [1, 2] : null,
  limit_periods: i % 5 === 2 ? ['monthly', 'yearly'] : null,
  started_at: iso(now - (i % 3) * 20 * DAY),
  ended_at: iso(now + (i % 4 === 3 ? -5 : 25) * DAY),
  used_count: (i * 3) % 40,
  created_at: iso(now - i * DAY),
  updated_at: iso(now - i * DAY),
}))

const tickets: Json[] = Array.from({ length: 14 }, (_, i) => {
  const created = now - i * 0.6 * DAY
  const uid = 10 + (i % emails.length)
  const messages = [
    {
      id: i * 10 + 1,
      user_id: uid,
      is_me: false,
      message: `Hi, node ${i + 1} can't connect since this morning.\nClient: Clash Verge, macOS.`,
      created_at: iso(created),
    },
    ...(i % 3 !== 0
      ? [
          {
            id: i * 10 + 2,
            user_id: 1,
            is_me: true,
            message: 'Thanks — could you update the subscription and try again?',
            created_at: iso(created + 3_600_000),
          },
        ]
      : []),
    ...(i % 3 === 1
      ? [
          {
            id: i * 10 + 3,
            user_id: uid,
            is_me: false,
            message: 'Still failing after the update.',
            created_at: iso(created + 7_200_000),
          },
        ]
      : []),
  ]
  const last = messages[messages.length - 1]
  return {
    id: i + 1,
    user_id: uid,
    user: { id: uid, email: pick(emails, i) },
    subject: pick(
      ['Node offline', 'Refund request', 'How to import on iOS?', 'Payment not credited', 'Speed is slow'],
      i,
    ),
    level: i % 3,
    status: i % 5 === 4 ? 1 : 0,
    reply_status: last.is_me ? 1 : 0,
    last_reply_user_id: last.user_id,
    messages,
    created_at: iso(created),
    updated_at: last.created_at,
  }
})

let notices: Json[] = [
  {
    title: '国庆期间节点扩容',
    content: '## 扩容\n新增 **香港 / 日本** 节点，欢迎使用。',
    tags: ['节点'],
    img_url: '',
  },
  { title: '支付方式更新', content: '现已支持 Stripe 信用卡支付。', tags: ['支付'], img_url: '' },
  { title: 'Maintenance window', content: 'Sunday 02:00–03:00 UTC.', tags: ['maintenance'], img_url: '' },
].map((n, i) => ({
  id: i + 1,
  show: i !== 2,
  sort: i + 1,
  created_at: iso(now - i * 5 * DAY),
  updated_at: iso(now - i * 5 * DAY),
  ...n,
}))

let knowledge: Json[] = [
  {
    language: 'zh-CN',
    category: '入门',
    title: 'Clash Verge 使用教程',
    body: '# Clash Verge\n订阅地址：{{subscribeUrl}}\n\n<!--access start-->\n付费内容\n<!--access end-->',
  },
  {
    language: 'zh-CN',
    category: '入门',
    title: 'Shadowrocket 导入订阅',
    body: '在 {{siteName}} 复制订阅地址。',
  },
  { language: 'zh-CN', category: '常见问题', title: '无法连接怎么办？', body: '1. 更新订阅\n2. 切换节点' },
  {
    language: 'en',
    category: 'Getting started',
    title: 'Clash Verge guide',
    body: 'Subscription: {{subscribeUrl}}',
  },
  { language: 'en', category: 'FAQ', title: 'Billing questions', body: 'Contact support.' },
].map((k, i) => ({
  id: i + 1,
  show: i !== 4,
  sort: i + 1,
  created_at: iso(now - i * DAY),
  updated_at: iso(now - i * DAY),
  ...k,
}))

// ---------------------------------------------------------------- settings (mirrors internal/settings/defs.go)

type Def = {
  key: string
  type: string
  default: unknown
  secret?: boolean
  options?: string[]
  min?: number
  max?: number
}
const s = (key: string, def = '', options?: string[]): Def => ({
  key,
  type: 'string',
  default: def,
  ...(options ? { options } : {}),
})
const b = (key: string, def: boolean): Def => ({ key, type: 'bool', default: def })
const n = (key: string, def: number, min: number, max = 2_147_483_647): Def => ({
  key,
  type: 'int',
  default: def,
  min,
  max,
})
const ss = (key: string, def: string[] = []): Def => ({ key, type: 'strings', default: def })
const secret = (key: string): Def => ({ key, type: 'string', default: '', secret: true })
const defs: Record<string, Def[]> = {
  site: [
    s('app_name', 'GG Bond'),
    s('app_description'),
    s('app_url', appUrl),
    s('logo'),
    s('tos_url'),
    s('currency', 'CNY'),
    s('currency_symbol', '¥'),
    s('default_locale', 'zh-CN', ['zh-CN', 'en']),
    b('show_info_to_user', true),
  ],
  register: [
    b('stop_register', false),
    b('email_verify', false),
    b('email_whitelist_enable', false),
    ss('email_whitelist_suffix', ['gmail.com', 'outlook.com']),
    b('invite_force', false),
    b('register_limit_by_ip_enable', false),
    n('register_limit_count', 3, 1),
    n('register_limit_expire', 60, 1),
    b('password_limit_enable', true),
    n('password_limit_count', 5, 1),
    n('password_limit_expire', 60, 1),
    n('try_out_plan_id', 0, 0),
    n('try_out_hour', 0, 0),
    s('captcha_provider', 'none', ['none', 'turnstile', 'recaptcha', 'hcaptcha']),
    s('captcha_site_key'),
    secret('captcha_secret'),
  ],
  subscribe: [
    ss('subscribe_url'),
    b('plan_change_enable', true),
    n('reset_traffic_method', 0, 0, 4),
    b('surplus_enable', true),
    b('show_info_to_server_enable', true),
    b('show_protocol_to_server_enable', false),
    b('default_remind_expire', true),
    b('default_remind_traffic', true),
    s('ruleset_default_template', 'standard'),
    s('ruleset_default_source', 'blackmatrix7'),
    s('ruleset_mirror'),
    n('subscribe_update_interval', 24, 1, 720),
    b('allow_user_rule_profile', true),
  ],
  order: [n('order_timeout_minutes', 30, 1, 10080)],
  invite: [
    n('invite_commission', 10, 0, 100),
    n('invite_gen_limit', 5, 0),
    b('invite_never_expire', true),
    b('commission_first_time_enable', true),
    b('commission_auto_check_enable', true),
    n('commission_withdraw_limit', 100, 0),
    ss('commission_withdraw_method', ['USDT', 'Alipay']),
    b('withdraw_close_enable', false),
    b('commission_distribution_enable', false),
    n('commission_distribution_l1', 0, 0, 100),
    n('commission_distribution_l2', 0, 0, 100),
    n('commission_distribution_l3', 0, 0, 100),
  ],
  server: [
    secret('server_token'),
    n('server_pull_interval', 60, 10, 86400),
    n('server_push_interval', 60, 10, 86400),
    ss('server_white_list'),
    n('device_limit_mode', 0, 0, 1),
  ],
  email: [
    s('email_driver', 'log', ['log', 'smtp']),
    s('email_host'),
    n('email_port', 465, 1, 65535),
    s('email_encryption', 'ssl', ['ssl', 'tls', 'none']),
    s('email_username'),
    secret('email_password'),
    s('email_from_address'),
    s('email_from_name'),
    b('remind_mail_enable', true),
  ],
  frontend: [
    s('frontend_accent', '#007AFF'),
    s('frontend_background_url'),
    s('frontend_customer_service_url'),
    s('frontend_footer_html'),
  ],
  app: ['windows', 'macos', 'android', 'ios'].flatMap((p) => [s(`${p}_download_url`), s(`${p}_version`)]),
}
const settings: Record<string, unknown> = {}
for (const list of Object.values(defs)) for (const d of list) settings[d.key] = d.default
settings.server_token = 'Q2xhc2hWZXJnZURlbW9Ub2tlbjEyMzQ1'

function validateSetting(d: Def, v: unknown): string | null {
  if (d.type === 'string') {
    if (typeof v !== 'string') return 'must be a string'
    if (d.options && !d.options.includes(v)) return `must be one of: ${d.options.join(', ')}`
  }
  if (d.type === 'bool' && typeof v !== 'boolean') return 'must be a boolean'
  if (d.type === 'int') {
    if (typeof v !== 'number' || !Number.isInteger(v)) return 'must be an integer'
    if (d.min !== undefined && v < d.min) return `must be at least ${d.min}`
    if (d.max !== undefined && v > d.max) return `must be at most ${d.max}`
  }
  if (d.type === 'strings' && !(Array.isArray(v) && v.every((x) => typeof x === 'string')))
    return 'must be a list of strings'
  return null
}

// ---------------------------------------------------------------- rule sets

const builtin = readJson(path.join(root, 'internal/subscription/rulesets/catalog.json')) as Json
let custom: Json = {}
function effective(): Json {
  const merged = structuredClone(builtin) as Json & {
    sources: Json
    categories: Json[]
    templates: Json[]
    mirrors: Json[]
  }
  const c = custom as { sources?: Json; categories?: Json[]; templates?: Json[]; mirrors?: Json[] }
  Object.assign(merged.sources, c.sources ?? {})
  for (const key of ['categories', 'templates', 'mirrors'] as const) {
    for (const item of c[key] ?? []) {
      const i = merged[key].findIndex((x) => x.id === item.id)
      if (i === -1) merged[key].push(item)
      else merged[key][i] = item
    }
  }
  merged.categories.sort((a, z) => Number(a.order) - Number(z.order))
  return merged
}

// ---------------------------------------------------------------- system

const levels = ['INFO', 'INFO', 'INFO', 'DEBUG', 'WARN', 'ERROR']
const logs: Json[] = Array.from({ length: 300 }, (_, i) => {
  const level = pick(levels, i * 7)
  return {
    time: iso(now - (300 - i) * 37_000),
    level,
    msg:
      level === 'ERROR'
        ? 'mail send failed'
        : level === 'WARN'
          ? 'payment notify verification failed'
          : pick(['http request', 'traffic flushed', 'node push accepted', 'scheduler job done'], i),
    ...(i % 3
      ? {
          attrs: {
            status: 200,
            path: pick(['/api/v1/user/me', '/api/v1/server/UniProxy/push'], i),
            latency_ms: (i * 13) % 90,
            node_id: i % 9,
          },
        }
      : {}),
  }
})

function status(): Json {
  const jobs = [
    ['traffic_reset', '0 0 * * *'],
    ['expire_remind', '0 10 * * *'],
    ['traffic_remind', '0 10 * * *'],
    ['order_timeout', '*/1 * * * *'],
    ['commission_settle', '0 */1 * * *'],
    ['stats_daily', '5 0 * * *'],
    ['online_cleanup', '*/5 * * * *'],
    ['traffic_flush', '@every 10s'],
  ]
  return {
    version: '0.1.0-dev',
    go_version: 'go1.27.0',
    uptime: Math.round((Date.now() - now) / 1000) + 3 * 86_400 + 4_000,
    db_driver: 'sqlite',
    started_at: iso(now - 3 * DAY),
    scheduler: jobs.map(([name, spec], i) => ({
      name,
      spec,
      running: name === 'traffic_flush' && Date.now() % 3 === 0,
      last_run: i === 1 ? null : iso(Date.now() - (i + 1) * 61_000),
      last_duration: `${(i * 37) % 900}ms`,
      last_error: name === 'commission_settle' ? 'database is locked' : '',
      next_run: iso(Date.now() + (i + 1) * 45_000),
      runs: 1200 - i * 97,
      failures: name === 'commission_settle' ? 3 : 0,
    })),
    mail: { pending: 2, sent: 431, failed: 5, driver: String(settings.email_driver) },
    cache: { items: 1834, max_items: 100000, hits: 98765, misses: 4321, hit_ratio: 0.958 },
    online: { nodes: 7, users: 213 },
    goroutines: 57,
    memory: { alloc: 38_000_000, sys: 81_000_000 },
  }
}

// ---------------------------------------------------------------- helpers

function send(res: ServerResponse, statusCode: number, body: unknown) {
  res.statusCode = statusCode
  res.setHeader('Content-Type', 'application/json; charset=utf-8')
  res.setHeader('Cache-Control', 'no-store')
  res.end(JSON.stringify(body))
}
const ok = (res: ServerResponse, data: unknown, statusCode = 200) => send(res, statusCode, { data })
const list = (res: ServerResponse, data: unknown[], total = data.length) => send(res, 200, { data, total })
const fail = (
  res: ServerResponse,
  statusCode: number,
  code: string,
  message: string,
  errors?: Record<string, string[]>,
) => send(res, statusCode, { code, message, ...(errors ? { errors } : {}) })

async function body(req: IncomingMessage): Promise<Json> {
  const chunks: Buffer[] = []
  for await (const c of req) chunks.push(c as Buffer)
  const text = Buffer.concat(chunks).toString('utf8')
  return text ? (JSON.parse(text) as Json) : {}
}

function paginate<T>(items: T[], q: URLSearchParams): { page: T[]; total: number } {
  const page = Math.max(1, Number(q.get('page') ?? 1))
  const size = Math.min(200, Math.max(1, Number(q.get('page_size') ?? 20)))
  return { page: items.slice((page - 1) * size, page * size), total: items.length }
}

function sortBy<T extends Json>(items: T[], sort: string | null): T[] {
  if (!sort) return items
  const desc = sort.startsWith('-')
  const key = desc ? sort.slice(1) : sort
  return [...items].sort((a, z) => {
    const x = a[key] as string | number
    const y = z[key] as string | number
    return (x < y ? -1 : x > y ? 1 : 0) * (desc ? -1 : 1)
  })
}

function reorder(items: Json[], ids: unknown): string | null {
  if (!Array.isArray(ids)) return 'ids must be a list'
  if (new Set(ids).size !== ids.length) return 'duplicate ids'
  for (const id of ids) if (!items.some((x) => x.id === id)) return `unknown id ${String(id)}`
  ids.forEach((id, i) => {
    const item = items.find((x) => x.id === id)
    if (item) item.sort = i + 1
  })
  items.sort((a, z) => Number(a.sort) - Number(z.sort))
  return null
}

const TOKEN = 'mock-admin-token'

// ---------------------------------------------------------------- router

async function handle(req: IncomingMessage, res: ServerResponse) {
  const url = new URL(req.url ?? '/', 'http://localhost')
  const p = url.pathname.replace(/^\/api\/v1/, '')
  const m = req.method ?? 'GET'
  const q = url.searchParams
  const seg = p.split('/').filter(Boolean)
  const id = Number(seg[2])

  if (p === '/guest/config') return ok(res, config)
  if (p === '/guest/plans') return ok(res, plans)
  if (p === '/passport/login') return ok(res, { token: TOKEN, user })
  if (p === '/user/logout') return ok(res, true)
  if (req.headers.authorization !== `Bearer ${TOKEN}`)
    return fail(res, 401, 'unauthorized', 'mock: sign in first')
  if (p === '/user/me') return ok(res, user)
  if (seg[0] !== 'admin') return fail(res, 404, 'not_found', `mock: ${m} ${p} not implemented`)

  switch (seg[1]) {
    case 'plans':
      return ok(res, plans)
    case 'orders': {
      if (m === 'GET' && !seg[2]) {
        let rows = orders.filter((o) => {
          const u = o.user as { email: string }
          if (q.get('trade_no') && !String(o.trade_no).includes(q.get('trade_no')!)) return false
          if (q.get('email') && !u.email.includes(q.get('email')!)) return false
          if (q.get('user_id') && o.user_id !== Number(q.get('user_id'))) return false
          for (const k of ['status', 'commission_status', 'plan_id', 'payment_id'])
            if (q.get(k) !== null && o[k] !== Number(q.get(k))) return false
          // RFC 3339 bounds (date_to exclusive); a bare YYYY-MM-DD is a (UTC) day, date_to inclusive.
          const at = Date.parse(String(o.created_at))
          const from = q.get('date_from')
          const to = q.get('date_to')
          if (from && at < Date.parse(from.length === 10 ? `${from}T00:00:00Z` : from)) return false
          if (to && at >= (to.length === 10 ? Date.parse(`${to}T00:00:00Z`) + DAY : Date.parse(to)))
            return false
          return true
        })
        rows = sortBy(rows, q.get('sort') ?? '-created_at')
        const { page, total } = paginate(rows, q)
        return list(res, page, total)
      }
      if (m === 'POST' && !seg[2]) {
        const b = await body(req)
        const target = b.email ? emails.indexOf(str(b.email)) : Number(b.user_id) - 10
        if (target < 0 || target >= emails.length)
          return fail(res, 400, 'validation_failed', 'invalid', { email: ['user not found'] })
        const plan = plans.find((x) => x.id === Number(b.plan_id))
        if (!plan) return fail(res, 400, 'validation_failed', 'invalid', { plan_id: ['unknown plan'] })
        const price = (plan.prices as Partial<Record<string, number>>)[str(b.period)]
        if (price === undefined)
          return fail(res, 400, 'validation_failed', 'invalid', { period: ['not for sale'] })
        const order = {
          ...orders[0],
          id: nextId(),
          trade_no: `2026${Date.now()}`,
          user_id: 10 + target,
          user: { id: 10 + target, email: emails[target] },
          plan_id: plan.id,
          plan: { id: plan.id, name: plan.name },
          period: b.period,
          type: 1,
          status: 0,
          commission_status: 0,
          commission_balance: 0,
          total_amount: typeof b.total_amount === 'number' ? b.total_amount : price,
          handling_amount: 0,
          discount_amount: 0,
          surplus_amount: 0,
          balance_amount: 0,
          refund_amount: 0,
          surplus_order_ids: [],
          payment_id: null,
          payment: null,
          callback_no: '',
          paid_at: null,
          created_at: iso(Date.now()),
          updated_at: iso(Date.now()),
        }
        orders.unshift(order)
        return ok(res, order, 201)
      }
      const order = orders.find((o) => o.id === id)
      if (!order) return fail(res, 404, 'not_found', 'order not found')
      if (m === 'GET') return ok(res, order)
      if (m === 'PATCH') {
        Object.assign(order, {
          commission_status: (await body(req)).commission_status,
          updated_at: iso(Date.now()),
        })
        return ok(res, order)
      }
      if (seg[3] === 'paid' || seg[3] === 'cancel') {
        if (order.status !== 0) return fail(res, 409, 'conflict', 'order is not pending')
        Object.assign(
          order,
          seg[3] === 'paid' ? { status: 3, paid_at: iso(Date.now()), callback_no: 'manual' } : { status: 2 },
          { updated_at: iso(Date.now()) },
        )
        return ok(res, order)
      }
      break
    }
    case 'coupons': {
      if (m === 'GET') {
        const { page, total } = paginate(sortBy(coupons, q.get('sort') ?? '-created_at'), q)
        return list(res, page, total)
      }
      if (m === 'POST') {
        const b = await body(req)
        if (!b.name) return fail(res, 400, 'validation_failed', 'invalid', { name: ['required'] })
        const { count: rawCount, ...fields } = b
        const count = Number(rawCount ?? 1)
        const made = Array.from({ length: count }, () => {
          const code =
            count === 1 && b.code ? str(b.code) : Math.random().toString(36).slice(2, 10).toUpperCase()
          return {
            ...fields,
            id: nextId(),
            code,
            used_count: 0,
            created_at: iso(Date.now()),
            updated_at: iso(Date.now()),
          }
        })
        coupons = [...made, ...coupons]
        return ok(res, made, 201)
      }
      const coupon = coupons.find((c) => c.id === id)
      if (!coupon) return fail(res, 404, 'not_found', 'coupon not found')
      if (m === 'PATCH')
        return ok(res, Object.assign(coupon, await body(req), { updated_at: iso(Date.now()) }))
      if (m === 'DELETE') {
        coupons = coupons.filter((c) => c !== coupon)
        return ok(res, true)
      }
      break
    }
    case 'payments': {
      if (seg[2] === 'gateways') return ok(res, gateways)
      if (seg[2] === 'sort') {
        const err = reorder(payments, (await body(req)).ids)
        return err ? fail(res, 400, 'validation_failed', err) : ok(res, true)
      }
      if (m === 'GET')
        return ok(
          res,
          payments.map((x) => ({ ...x, notify_url: notifyUrl(x) })),
        )
      if (m === 'POST') {
        const b = await body(req)
        const gw = gateways.find((g) => g.name === b.gateway)
        if (!gw) return fail(res, 400, 'validation_failed', 'invalid', { gateway: ['unknown gateway'] })
        const cfg = (b.config ?? {}) as Json
        const errors: Record<string, string[]> = {}
        for (const f of gw.fields) if (f.required && !cfg[f.key]) errors[`config.${f.key}`] = ['required']
        if (Object.keys(errors).length) return fail(res, 400, 'validation_failed', 'invalid config', errors)
        const pay = {
          ...b,
          id: nextId(),
          uuid: uuid(),
          sort: payments.length + 1,
          created_at: iso(Date.now()),
          updated_at: iso(Date.now()),
        }
        payments.push(pay)
        return ok(res, { ...pay, notify_url: notifyUrl(pay) }, 201)
      }
      const pay = payments.find((x) => x.id === id)
      if (!pay) return fail(res, 404, 'not_found', 'payment not found')
      if (m === 'PATCH') {
        Object.assign(pay, await body(req), { updated_at: iso(Date.now()) })
        return ok(res, { ...pay, notify_url: notifyUrl(pay) })
      }
      if (m === 'DELETE') {
        payments = payments.filter((x) => x !== pay)
        return ok(res, true)
      }
      break
    }
    case 'tickets': {
      if (m === 'GET' && !seg[2]) {
        let rows = tickets.filter((t) => {
          if (q.get('status') !== null && t.status !== Number(q.get('status'))) return false
          if (q.get('reply_status') !== null && t.reply_status !== Number(q.get('reply_status'))) return false
          if (q.get('user_id') && t.user_id !== Number(q.get('user_id'))) return false
          if (q.get('email') && !(t.user as { email: string }).email.includes(q.get('email')!)) return false
          return true
        })
        rows = sortBy(rows, q.get('sort') ?? '-updated_at')
        const { page, total } = paginate(rows, q)
        return list(
          res,
          page.map(({ messages: _m, ...t }) => t),
          total,
        )
      }
      const ticket = tickets.find((t) => t.id === id)
      if (!ticket) return fail(res, 404, 'not_found', 'ticket not found')
      if (m === 'GET') return ok(res, ticket)
      if (seg[3] === 'reply') {
        const msg = {
          id: nextId(),
          user_id: 1,
          is_me: true,
          message: str((await body(req)).message),
          created_at: iso(Date.now()),
        }
        ;(ticket.messages as Json[]).push(msg)
        Object.assign(ticket, {
          status: 0,
          reply_status: 1,
          last_reply_user_id: 1,
          updated_at: msg.created_at,
        })
        return ok(res, ticket)
      }
      if (seg[3] === 'close') {
        ticket.status = 1
        return ok(res, true)
      }
      break
    }
    case 'notices':
    case 'knowledge': {
      const isNotice = seg[1] === 'notices'
      const items = isNotice ? notices : knowledge
      if (seg[2] === 'sort') {
        const err = reorder(items, (await body(req)).ids)
        return err ? fail(res, 400, 'validation_failed', err) : ok(res, true)
      }
      if (m === 'GET' && !seg[2]) return list(res, isNotice ? items : items.map(({ body: _b, ...k }) => k))
      if (m === 'POST') {
        const b = await body(req)
        const required = isNotice ? ['title', 'content'] : ['language', 'category', 'title', 'body']
        const errors = Object.fromEntries(required.filter((k) => !b[k]).map((k) => [k, ['required']]))
        if (Object.keys(errors).length) return fail(res, 400, 'validation_failed', 'invalid', errors)
        const item = {
          show: true,
          tags: [],
          img_url: '',
          ...b,
          id: nextId(),
          sort: items.length + 1,
          created_at: iso(Date.now()),
          updated_at: iso(Date.now()),
        }
        if (isNotice) notices = [...notices, item]
        else knowledge = [...knowledge, item]
        return ok(res, item, 201)
      }
      const item = items.find((x) => x.id === id)
      if (!item) return fail(res, 404, 'not_found', 'not found')
      if (m === 'GET') return ok(res, item)
      if (m === 'PATCH') return ok(res, Object.assign(item, await body(req), { updated_at: iso(Date.now()) }))
      if (m === 'DELETE') {
        if (isNotice) notices = notices.filter((x) => x !== item)
        else knowledge = knowledge.filter((x) => x !== item)
        return ok(res, true)
      }
      break
    }
    case 'settings': {
      if (seg[2] === 'mail') {
        const b = await body(req)
        return String(b.email).includes('@')
          ? ok(res, true)
          : fail(res, 400, 'validation_failed', 'invalid', { email: ['invalid email'] })
      }
      if (m === 'GET') {
        const grouped: Json = {}
        for (const [g, list] of Object.entries(defs))
          grouped[g] = Object.fromEntries(list.map((d) => [d.key, settings[d.key]]))
        return ok(res, {
          ...grouped,
          groups: Object.entries(defs).map(([name, keys]) => ({
            name,
            keys: keys.map((d) => ({ ...d, group: name })),
          })),
        })
      }
      if (m === 'PUT') {
        const b = await body(req)
        const all = Object.values(defs).flat()
        const errors: Record<string, string[]> = {}
        for (const [k, v] of Object.entries(b)) {
          const d = all.find((x) => x.key === k)
          const problem = d ? (v === null ? null : validateSetting(d, v)) : 'unknown setting'
          if (problem) errors[k] = [problem]
        }
        if (Object.keys(errors).length) return fail(res, 400, 'validation_failed', 'invalid settings', errors)
        for (const [k, v] of Object.entries(b))
          settings[k] = v === null ? all.find((x) => x.key === k)?.default : v
        return ok(res, true)
      }
      break
    }
    case 'mail-templates': {
      const b = m === 'PUT' || m === 'POST' ? await body(req) : {}
      if (handleMailTemplates(m, seg.slice(2), q, b, res)) return
      break
    }
    case 'rulesets': {
      if (m === 'GET') return ok(res, { builtin, custom, effective: effective() })
      if (m === 'PUT') {
        const b = await body(req)
        const c = (b.custom ?? {}) as Json
        const errors: Record<string, string[]> = {}
        for (const k of Object.keys(c))
          if (!['sources', 'categories', 'templates', 'mirrors'].includes(k)) errors[k] = ['unknown key']
        if (Object.keys(errors).length)
          return fail(res, 400, 'validation_failed', 'invalid overrides', errors)
        custom = c
        return ok(res, true)
      }
      break
    }
    case 'system': {
      if (seg[2] === 'status') return ok(res, status())
      if (seg[2] === 'logs') {
        const order = ['DEBUG', 'INFO', 'WARN', 'ERROR']
        const min = q.get('level') ? order.indexOf(q.get('level')!.toUpperCase()) : 0
        const rows = logs.filter((l) => order.indexOf(String(l.level)) >= min)
        return ok(res, rows.slice(-Number(q.get('limit') ?? 200)))
      }
      break
    }
  }
  return fail(res, 404, 'not_found', `mock: ${m} ${p} not implemented`)
}

createServer((req, res) => {
  handle(req, res).catch((e: unknown) =>
    fail(res, 500, 'internal', e instanceof Error ? e.message : String(e)),
  )
}).listen(PORT, () => {
  console.log(
    `[mock] admin API on http://localhost:${PORT}/api/v1 — run: GGBOARD_API=http://localhost:${PORT} pnpm dev`,
  )
})
