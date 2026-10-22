/**
 * Dev-only mock of the commerce & support API (FE-2 pages: shop, orders/payment, invite, tickets,
 * knowledge, notices) for clicking through the UI without the Go backend. In-memory state, reset on
 * restart. Never part of the build.
 *
 *   node scripts/mock/commerce.ts                 # http://localhost:8787 (PORT=… to change)
 *   GGBOARD_API=http://localhost:8787 pnpm dev    # then sign in with any email / password
 *
 * Payment simulation: a `qrcode` checkout is "paid" 8 s after it starts; `html` after 12 s;
 * `url` redirects to `/api/v1/mock/pay/<trade_no>`, which marks the order paid and sends you back.
 * Coupons: `SAVE10` (10 % off) and `MINUS5` (5.00 off); anything else is rejected.
 */
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http'

const PORT = Number(process.env.PORT ?? 8787)
const now = () => new Date().toISOString()
const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()
const daysAgo = (d: number) => new Date(Date.now() - d * 86_400_000).toISOString()

// ------------------------------------------------------------------------------------- state

const TOKEN = 'mock-token'
const config = {
  app_name: 'GG Bond',
  app_description: 'Mock backend',
  app_url: process.env.APP_URL ?? 'http://localhost:5173',
  logo: '',
  tos_url: '',
  currency: 'CNY',
  currency_symbol: '¥',
  register_enabled: true,
  email_verify: false,
  email_whitelist_suffix: [] as string[],
  invite_force: false,
  captcha: { provider: 'none', site_key: '' },
  downloads: {
    windows: { url: '', version: '' },
    macos: { url: '', version: '' },
    android: { url: '', version: '' },
    ios: { url: '', version: '' },
  },
  locales: ['zh-CN', 'en'],
}

const user = {
  id: 1,
  email: 'demo@example.com',
  uuid: '00000000-0000-4000-8000-000000000000',
  plan_id: 2 as number | null,
  plan: { id: 2, name: 'Pro' } as { id: number; name: string } | null,
  group_id: 1,
  transfer_enable: 536870912000,
  u: 21474836480,
  d: 107374182400,
  expired_at: '2027-01-01T00:00:00Z',
  next_reset_at: '2026-11-01T00:00:00Z',
  speed_limit: null,
  device_limit: 5,
  balance: 1200,
  commission_balance: 4250,
  commission_rate: null,
  discount: null,
  remind_expire: true,
  remind_traffic: true,
  is_admin: false,
  is_staff: false,
  banned: false,
  avatar_url: '',
  created_at: '2026-01-01T00:00:00Z',
  last_login_at: now(),
}

const plan = (id: number, name: string, extra: Record<string, unknown>) => ({
  id,
  group_id: 1,
  name,
  content: '',
  tags: [] as string[],
  transfer_enable: 100,
  speed_limit: null,
  device_limit: 3,
  capacity_limit: null as number | null,
  prices: {} as Record<string, number>,
  reset_traffic_method: 1,
  show: true,
  sell: true,
  renew: true,
  sort: id,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...extra,
})

const plans = [
  plan(1, 'Lite', {
    content: '适合轻度使用：\n- 网页浏览、社交与邮件\n- 高清视频偶尔观看',
    tags: ['入门'],
    prices: { monthly: 1500, quarterly: 4000, yearly: 15000 },
  }),
  plan(2, 'Pro', {
    content: '**高清流媒体**、AI 服务与游戏加速。',
    tags: ['推荐', '流媒体'],
    transfer_enable: 500,
    device_limit: 5,
    prices: { monthly: 3000, quarterly: 8500, half_yearly: 16000, yearly: 30000, reset_traffic: 1000 },
  }),
  plan(3, 'Ultra', {
    content: '不限设备，全部节点与专属线路。',
    tags: ['全部节点'],
    transfer_enable: 2000,
    device_limit: null,
    speed_limit: 1000,
    prices: { monthly: 8800, yearly: 88000, two_yearly: 160000 },
  }),
  plan(4, 'Holiday', {
    content: '限量节日套餐。',
    tags: ['限量'],
    capacity_limit: 0,
    prices: { onetime: 9900 },
  }),
]

const paymentMethods = [
  { id: 1, name: '支付宝', icon: '💙', handling_fee_fixed: 0, handling_fee_percent: 0 },
  { id: 2, name: 'USDT', icon: '', handling_fee_fixed: 100, handling_fee_percent: 1.5 },
  { id: 3, name: 'Stripe', icon: '', handling_fee_fixed: 0, handling_fee_percent: 3 },
]
const checkoutType: Record<number, 'qrcode' | 'url' | 'html'> = { 1: 'qrcode', 2: 'html', 3: 'url' }

type Order = {
  id: number
  trade_no: string
  user_id: number
  plan_id: number
  plan: { id: number; name: string }
  type: 1 | 2 | 3 | 4
  period: string
  status: 0 | 1 | 2 | 3 | 4
  commission_status: number
  total_amount: number
  handling_amount: number
  discount_amount: number
  surplus_amount: number
  refund_amount: number
  balance_amount: number
  payment_id: number | null
  payment: { id: number; name: string } | null
  callback_no: string | null
  paid_at: string | null
  created_at: string
  updated_at: string
  payAt?: number
}

let seq = 100
const tradeNo = () => `2026102212${String(++seq).padStart(4, '0')}${Math.floor(Math.random() * 9000 + 1000)}`
const orders: Order[] = [
  {
    id: 3,
    trade_no: '20261001093000000001',
    user_id: 1,
    plan_id: 2,
    plan: { id: 2, name: 'Pro' },
    type: 1,
    period: 'quarterly',
    status: 3,
    commission_status: 2,
    total_amount: 8500,
    handling_amount: 0,
    discount_amount: 0,
    surplus_amount: 0,
    refund_amount: 0,
    balance_amount: 0,
    payment_id: 1,
    payment: { id: 1, name: '支付宝' },
    callback_no: 'ALI123',
    paid_at: daysAgo(21),
    created_at: daysAgo(21),
    updated_at: daysAgo(21),
  },
  {
    id: 2,
    trade_no: '20260901093000000002',
    user_id: 1,
    plan_id: 1,
    plan: { id: 1, name: 'Lite' },
    type: 1,
    period: 'monthly',
    status: 2,
    commission_status: 0,
    total_amount: 1500,
    handling_amount: 0,
    discount_amount: 0,
    surplus_amount: 0,
    refund_amount: 0,
    balance_amount: 0,
    payment_id: null,
    payment: null,
    callback_no: null,
    paid_at: null,
    created_at: daysAgo(50),
    updated_at: daysAgo(50),
  },
]

const inviteCodes = [{ code: 'Ab3dEf7hJk9mNp2qRs4tUv6wXy8zA1bC', pv: 12, created_at: daysAgo(30) }]
const commissionLogs = Array.from({ length: 13 }, (_, i) => ({
  trade_no: `2026090${(i % 9) + 1}1200000${String(i).padStart(4, '0')}`,
  order_amount: 3000 + i * 500,
  get_amount: Math.round((3000 + i * 500) * 0.1),
  created_at: daysAgo(i * 3 + 1),
}))

type Ticket = {
  id: number
  user_id: number
  subject: string
  level: 0 | 1 | 2
  status: 0 | 1
  reply_status: 0 | 1
  last_reply_user_id: number | null
  created_at: string
  updated_at: string
  messages: { id: number; user_id: number; is_me: boolean; message: string; created_at: string }[]
}
let msgSeq = 10
const tickets: Ticket[] = [
  {
    id: 7,
    user_id: 1,
    subject: '香港节点晚高峰速度慢',
    level: 1,
    status: 0,
    reply_status: 1,
    last_reply_user_id: 99,
    created_at: minutesAgo(180),
    updated_at: minutesAgo(30),
    messages: [
      {
        id: 1,
        user_id: 1,
        is_me: true,
        message: '你好，最近每天晚上 9 点后香港 02 节点只有 2 Mbps。\n客户端：Clash Verge\n地区：上海电信',
        created_at: minutesAgo(180),
      },
      {
        id: 2,
        user_id: 99,
        is_me: false,
        message: '您好，已为线路扩容，请更新订阅后重试。如仍有问题请回复本工单。',
        created_at: minutesAgo(30),
      },
    ],
  },
  {
    id: 5,
    user_id: 1,
    subject: '如何在 iOS 上导入订阅？',
    level: 0,
    status: 1,
    reply_status: 1,
    last_reply_user_id: 99,
    created_at: daysAgo(20),
    updated_at: daysAgo(19),
    messages: [
      { id: 3, user_id: 1, is_me: true, message: '如题。', created_at: daysAgo(20) },
      {
        id: 4,
        user_id: 99,
        is_me: false,
        message: '请参考「使用文档」中的 Shadowrocket 教程。',
        created_at: daysAgo(19),
      },
    ],
  },
]

const notices = Array.from({ length: 12 }, (_, i) => ({
  id: 100 - i,
  title: i === 0 ? '十月维护公告' : `服务通知 #${12 - i}`,
  content:
    i === 0
      ? '## 维护时间\n\n10 月 25 日 02:00 – 04:00（北京时间）\n\n维护期间部分节点可能**短暂中断**，请提前做好准备。\n\n- 升级香港、日本线路\n- 新增新加坡 03 节点\n- 优化订阅生成速度\n\n感谢你的耐心与支持！如有问题请[提交工单](/tickets)。'
      : `这是第 ${12 - i} 条通知的内容。`,
  img_url: i === 0 ? 'https://picsum.photos/seed/ggbond/1200/500' : '',
  tags: i === 0 ? ['维护', '重要'] : ['通知'],
  created_at: daysAgo(i * 4),
}))

const knowledge = [
  {
    id: 1,
    language: 'zh-CN',
    category: '入门',
    title: '快速开始',
    body: '# 快速开始\n\n1. 在「购买套餐」中选择套餐并完成支付\n2. 打开「订阅」页面复制订阅链接\n3. 在客户端中导入\n\n[一键导入 Clash](clash://install-config?url=https%3A%2F%2Fpanel.example.com%2Fs%2Ftoken)\n\n[危险链接](javascript:alert(1))',
    updated_at: daysAgo(3),
  },
  {
    id: 2,
    language: 'zh-CN',
    category: '客户端',
    title: 'Shadowrocket（iOS）',
    body: '## 导入订阅\n\n复制以下链接后在 Shadowrocket 中添加：\n\n<div class="knowledge-no-access">你必须拥有有效的订阅才可以查看该区域的内容</div>\n\n更多问题请联系客服。',
    updated_at: daysAgo(10),
  },
  {
    id: 3,
    language: 'zh-CN',
    category: '客户端',
    title: 'Clash Verge（Windows / macOS）',
    body: '下载 Clash Verge 后，在「订阅」中粘贴链接即可。',
    updated_at: daysAgo(12),
  },
  {
    id: 4,
    language: 'en',
    category: 'Getting started',
    title: 'Quick start',
    body: '# Quick start\n\nBuy a plan, copy your subscription link and import it into your client.',
    updated_at: daysAgo(3),
  },
]

// ------------------------------------------------------------------------------------- helpers

function send(res: ServerResponse, status: number, body: unknown) {
  res.statusCode = status
  res.setHeader('Content-Type', 'application/json; charset=utf-8')
  res.setHeader('Cache-Control', 'no-store')
  res.end(JSON.stringify(body))
}
const str = (value: unknown): string =>
  typeof value === 'string' ? value : typeof value === 'number' ? String(value) : ''
const ok = (res: ServerResponse, data: unknown, status = 200) => send(res, status, { data })
const list = (res: ServerResponse, items: unknown[], url: URL) => {
  const page = Math.max(1, Number(url.searchParams.get('page') ?? 1))
  const size = Math.min(200, Math.max(1, Number(url.searchParams.get('page_size') ?? 20)))
  send(res, 200, { data: items.slice((page - 1) * size, page * size), total: items.length })
}
const fail = (
  res: ServerResponse,
  status: number,
  code: string,
  message: string,
  errors?: Record<string, string[]>,
) => send(res, status, { code, message, ...(errors ? { errors } : {}) })

async function readBody(req: IncomingMessage): Promise<Record<string, unknown>> {
  const chunks: Buffer[] = []
  for await (const chunk of req) chunks.push(chunk as Buffer)
  if (chunks.length === 0) return {}
  try {
    return JSON.parse(Buffer.concat(chunks).toString('utf8')) as Record<string, unknown>
  } catch {
    return {}
  }
}

/** Settles simulated payments whose time has come. */
function tick() {
  for (const order of orders) {
    if (order.status === 0 && order.payAt && Date.now() >= order.payAt) markPaid(order)
    if (order.status === 0 && Date.now() - Date.parse(order.created_at) > 30 * 60_000) {
      order.status = 2
      order.updated_at = now()
    }
  }
}

function markPaid(order: Order) {
  order.status = 3
  order.paid_at = now()
  order.updated_at = now()
  order.callback_no = `MOCK${order.id}`
  const p = plans.find((x) => x.id === order.plan_id)
  if (p && order.period !== 'reset_traffic') {
    user.plan_id = p.id
    user.plan = { id: p.id, name: p.name }
  }
  if (order.period === 'reset_traffic') {
    user.u = 0
    user.d = 0
  }
}

const COUPONS: Record<string, { id: number; code: string; name: string; type: 1 | 2; value: number }> = {
  SAVE10: { id: 1, code: 'SAVE10', name: '新用户九折', type: 2, value: 10 },
  MINUS5: { id: 2, code: 'MINUS5', name: '立减 5 元', type: 1, value: 500 },
}

// ------------------------------------------------------------------------------------- routes

async function handle(req: IncomingMessage, res: ServerResponse) {
  tick()
  const url = new URL(req.url ?? '/', `http://localhost:${PORT}`)
  const path = url.pathname.replace(/^\/api\/v1/, '')
  const method = req.method ?? 'GET'
  const authed = req.headers.authorization === `Bearer ${TOKEN}`

  if (path === '/guest/config') return ok(res, config)
  if (path === '/guest/plans')
    return ok(
      res,
      plans.filter((p) => p.sell),
    )
  if (path === '/passport/login' || path === '/passport/register')
    return ok(res, { token: TOKEN, user: { ...user, last_login_at: now() } })
  if (path.startsWith('/mock/pay/')) {
    const order = orders.find((o) => o.trade_no === path.slice('/mock/pay/'.length))
    if (order && order.status === 0) markPaid(order)
    res.statusCode = 302
    res.setHeader('Location', `/orders/${order?.trade_no ?? ''}`)
    return res.end()
  }
  if (!path.startsWith('/user/')) return fail(res, 404, 'not_found', 'mock: no such route')
  if (!authed) return fail(res, 401, 'unauthorized', 'mock: sign in first')

  const body = method === 'GET' ? {} : await readBody(req)
  let m: RegExpMatchArray | null

  // account (enough for the shell and other pages)
  if (path === '/user/me') return ok(res, user)
  if (path === '/user/logout') return ok(res, true)
  if (path === '/user/stats')
    return ok(res, {
      pending_orders: orders.filter((o) => o.status === 0).length,
      open_tickets: tickets.filter((t) => t.status === 0).length,
      notices_count: notices.length,
      invite_count: 3,
      commission_pending: 1800,
    })
  if (path === '/user/subscription')
    return ok(res, {
      subscribe_url: 'https://panel.example.com/s/token',
      token: 'token',
      plan: user.plan,
      transfer_enable: user.transfer_enable,
      u: user.u,
      d: user.d,
      expired_at: user.expired_at,
      next_reset_at: user.next_reset_at,
      reset_day: 10,
      device_limit: user.device_limit,
      speed_limit: user.speed_limit,
    })

  // shop
  if (path === '/user/plans') return ok(res, plans)
  if ((m = path.match(/^\/user\/plans\/(\d+)$/))) {
    const p = plans.find((x) => x.id === Number(m?.[1]))
    return p ? ok(res, p) : fail(res, 404, 'not_found', 'plan not found')
  }
  if (path === '/user/payment-methods') return ok(res, paymentMethods)
  if (path === '/user/coupons/check' && method === 'POST') {
    const coupon = COUPONS[str(body.code).toUpperCase()]
    return coupon
      ? ok(res, coupon)
      : fail(res, 400, 'validation_failed', '优惠券无效', { code: ['优惠券不存在或已过期'] })
  }
  if (path === '/user/orders' && method === 'GET') {
    const status = url.searchParams.get('status')
    const rows = orders.filter((o) => status === null || o.status === Number(status))
    return list(res, rows, url)
  }
  if (path === '/user/orders' && method === 'POST') {
    const pending = orders.find((o) => o.status === 0)
    if (pending)
      return fail(res, 409, 'conflict', 'you have a pending order', { trade_no: [pending.trade_no] })
    const p = plans.find((x) => x.id === Number(body.plan_id))
    const period = str(body.period)
    const price = p?.prices[period]
    if (!p || price === undefined) return fail(res, 400, 'validation_failed', 'invalid plan or period')
    const coupon = body.coupon_code ? COUPONS[str(body.coupon_code).toUpperCase()] : undefined
    if (body.coupon_code && !coupon)
      return fail(res, 400, 'validation_failed', 'invalid coupon', { coupon_code: ['优惠券无效'] })
    const discount = coupon
      ? coupon.type === 2
        ? Math.round((price * coupon.value) / 100)
        : coupon.value
      : 0
    const balanceUsed = Math.min(user.balance, price - discount)
    user.balance -= balanceUsed
    const order: Order = {
      id: ++seq,
      trade_no: tradeNo(),
      user_id: 1,
      plan_id: p.id,
      plan: { id: p.id, name: p.name },
      type: period === 'reset_traffic' ? 4 : user.plan_id === p.id ? 2 : user.plan_id ? 3 : 1,
      period,
      status: 0,
      commission_status: 0,
      total_amount: price - discount - balanceUsed,
      handling_amount: 0,
      discount_amount: discount,
      surplus_amount: 0,
      refund_amount: 0,
      balance_amount: balanceUsed,
      payment_id: null,
      payment: null,
      callback_no: null,
      paid_at: null,
      created_at: now(),
      updated_at: now(),
    }
    orders.unshift(order)
    return ok(res, { trade_no: order.trade_no }, 201)
  }
  if ((m = path.match(/^\/user\/orders\/([^/]+)(?:\/(checkout|status|cancel))?$/))) {
    const order = orders.find((o) => o.trade_no === decodeURIComponent(m?.[1] ?? ''))
    if (!order) return fail(res, 404, 'not_found', 'order not found')
    const action = m[2]
    if (!action) {
      const { payAt: _payAt, ...view } = order
      return ok(res, view)
    }
    if (action === 'status') return ok(res, { status: order.status })
    if (action === 'cancel' && method === 'POST') {
      if (order.status !== 0)
        return fail(res, 400, 'validation_failed', 'only pending orders can be cancelled')
      order.status = 2
      order.updated_at = now()
      user.balance += order.balance_amount
      return ok(res, true)
    }
    if (action === 'checkout' && method === 'POST') {
      if (order.status !== 0) return fail(res, 400, 'validation_failed', 'order is not pending')
      if (order.total_amount === 0) {
        markPaid(order)
        return ok(res, { type: 'paid', data: '' })
      }
      const pm = paymentMethods.find((x) => x.id === Number(body.payment_id))
      if (!pm)
        return fail(res, 400, 'validation_failed', 'choose a payment method', { payment_id: ['required'] })
      order.payment_id = pm.id
      order.payment = { id: pm.id, name: pm.name }
      order.handling_amount =
        Math.round((order.total_amount * pm.handling_fee_percent) / 100) + pm.handling_fee_fixed
      const type = checkoutType[pm.id] ?? 'qrcode'
      if (type === 'qrcode') {
        order.payAt = Date.now() + 8_000
        return ok(res, { type, data: `https://qr.alipay.com/mock${order.trade_no}` })
      }
      if (type === 'html') {
        order.payAt = Date.now() + 12_000
        return ok(res, {
          type,
          data: `<!doctype html><html><body style="font-family:system-ui;padding:24px"><h1>Mock USDT gateway</h1><p>Send ${((order.total_amount + order.handling_amount) / 100).toFixed(2)} USDT.</p><p>This order is marked paid automatically in ~12 s.</p></body></html>`,
        })
      }
      // Relative: resolved against the SPA origin, so it goes through the Vite dev proxy.
      return ok(res, { type, data: `/api/v1/mock/pay/${order.trade_no}` })
    }
  }

  // invite
  if (path === '/user/invite')
    return ok(res, {
      codes: inviteCodes,
      stat: {
        register_count: 3,
        commission_pending: 1800,
        commission_paid: commissionLogs.reduce((s, l) => s + l.get_amount, 0),
        commission_rate: 10,
        invite_gen_limit: 3,
      },
    })
  if (path === '/user/invite/codes' && method === 'POST') {
    if (inviteCodes.length >= 3) return fail(res, 400, 'validation_failed', 'invite code limit reached')
    const code = Array.from(
      { length: 32 },
      () => 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'[Math.floor(Math.random() * 55)],
    ).join('')
    inviteCodes.unshift({ code, pv: 0, created_at: now() })
    return ok(res, { code }, 201)
  }
  if (path === '/user/invite/commissions') return list(res, commissionLogs, url)
  if (path === '/user/commission/transfer' && method === 'POST') {
    const amount = Number(body.amount)
    if (!(amount > 0))
      return fail(res, 400, 'validation_failed', 'invalid amount', { amount: ['must be > 0'] })
    if (amount > user.commission_balance)
      return fail(res, 400, 'validation_failed', 'too much', { amount: ['exceeds your commission balance'] })
    user.commission_balance -= amount
    user.balance += amount
    return ok(res, user)
  }
  if (path === '/user/commission/withdraw' && method === 'POST') {
    if (tickets.some((t) => t.status === 0))
      return fail(res, 409, 'conflict', 'you already have an open ticket')
    if (!['USDT', 'Alipay'].includes(str(body.method)))
      return fail(res, 400, 'validation_failed', 'invalid method', {
        method: ['is not a supported withdrawal method'],
      })
    const ticket = newTicket(
      `佣金提现申请（${str(body.method)}）`,
      2,
      `方式：${str(body.method)}\n账户：${str(body.account)}`,
    )
    return ok(res, { ticket_id: ticket.id }, 201)
  }

  // tickets
  if (path === '/user/tickets' && method === 'GET') {
    const status = url.searchParams.get('status')
    const rows = tickets
      .filter((t) => status === null || t.status === Number(status))
      .map(({ messages: _messages, ...t }) => t)
    return list(res, rows, url)
  }
  if (path === '/user/tickets' && method === 'POST') {
    if (tickets.some((t) => t.status === 0))
      return fail(
        res,
        409,
        'conflict',
        'you already have an open ticket; close it or wait for it to be resolved before opening another',
      )
    const subject = str(body.subject).trim()
    const message = str(body.message).trim()
    if (!subject) return fail(res, 400, 'validation_failed', 'invalid', { subject: ['is required'] })
    if (!message) return fail(res, 400, 'validation_failed', 'invalid', { message: ['is required'] })
    const level = Number(body.level ?? 1)
    return ok(res, newTicket(subject, level === 0 || level === 2 ? level : 1, message), 201)
  }
  if ((m = path.match(/^\/user\/tickets\/(\d+)(?:\/(reply|close))?$/))) {
    const ticket = tickets.find((t) => t.id === Number(m?.[1]))
    if (!ticket) return fail(res, 404, 'not_found', 'ticket not found')
    if (!m[2]) return ok(res, ticket)
    if (m[2] === 'close') {
      ticket.status = 1
      ticket.updated_at = now()
      return ok(res, true)
    }
    if (ticket.status === 1)
      return fail(res, 400, 'validation_failed', 'the ticket is closed and cannot be replied to')
    const message = str(body.message).trim()
    if (!message) return fail(res, 400, 'validation_failed', 'invalid', { message: ['is required'] })
    ticket.messages.push({ id: ++msgSeq, user_id: 1, is_me: true, message, created_at: now() })
    ticket.reply_status = 0
    ticket.last_reply_user_id = 1
    ticket.updated_at = now()
    setTimeout(() => {
      ticket.messages.push({
        id: ++msgSeq,
        user_id: 99,
        is_me: false,
        message: '收到，我们正在处理，请稍候。',
        created_at: now(),
      })
      ticket.reply_status = 1
      ticket.last_reply_user_id = 99
      ticket.updated_at = now()
    }, 5_000)
    return ok(res, ticket)
  }

  // content
  if (path === '/user/notices') return list(res, notices, url)
  if (path === '/user/knowledge') {
    const language = url.searchParams.get('language') ?? 'zh-CN'
    const keyword = (url.searchParams.get('keyword') ?? '').toLowerCase()
    const index: Record<string, { id: number; title: string; updated_at: string }[]> = {}
    for (const a of knowledge) {
      if (!a.language.startsWith(language.split('-')[0] ?? language)) continue
      if (keyword && !a.title.toLowerCase().includes(keyword) && !a.body.toLowerCase().includes(keyword))
        continue
      ;(index[a.category] ??= []).push({ id: a.id, title: a.title, updated_at: a.updated_at })
    }
    return ok(res, index)
  }
  if ((m = path.match(/^\/user\/knowledge\/(\d+)$/))) {
    const article = knowledge.find((a) => a.id === Number(m?.[1]))
    return article ? ok(res, article) : fail(res, 404, 'not_found', 'article not found')
  }

  return fail(res, 404, 'not_found', `mock: ${method} ${path} is not mocked`)
}

function newTicket(subject: string, level: 0 | 1 | 2, message: string): Ticket {
  const ticket: Ticket = {
    id: Math.max(0, ...tickets.map((t) => t.id)) + 1,
    user_id: 1,
    subject,
    level,
    status: 0,
    reply_status: 0,
    last_reply_user_id: 1,
    created_at: now(),
    updated_at: now(),
    messages: [{ id: ++msgSeq, user_id: 1, is_me: true, message, created_at: now() }],
  }
  tickets.unshift(ticket)
  return ticket
}

createServer((req, res) => {
  handle(req, res).catch((error: unknown) => fail(res, 500, 'internal', String(error)))
}).listen(PORT, () => {
  console.log(
    `[mock] commerce API on http://localhost:${PORT}/api/v1 — run: GGBOARD_API=http://localhost:${PORT} pnpm dev`,
  )
})
