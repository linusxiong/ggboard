/* eslint-disable @typescript-eslint/no-base-to-string, @typescript-eslint/restrict-template-expressions -- dev mock over untyped JSON rows */
/**
 * Dev-only mock of the admin API used by FE-3 (dashboard, users, plans, groups, nodes, routes),
 * plus the guest/passport/me endpoints the shell needs. In-memory, resets on restart.
 * Never part of the build.
 *
 *   node scripts/mock/admin-a.ts                       # → http://localhost:8091
 *   GGBOARD_API=http://localhost:8091 pnpm dev         # vite proxies /api to the mock
 *
 * Demo login: any e-mail / password (an admin account). Data is fake (example.com).
 */
import { readFileSync } from 'node:fs'
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http'
import path from 'node:path'

const PORT = Number(process.env.MOCK_PORT ?? 8091)
const fixtures = path.resolve(import.meta.dirname, '..', 'fixtures')
const fixture = (name: string) =>
  JSON.parse(readFileSync(path.join(fixtures, name), 'utf8')) as Record<string, unknown>
const TOKEN = `eyJhbGciOiJub25lIn0.${Buffer.from(JSON.stringify({ sub: 1, exp: 4102444800 })).toString('base64url')}.mock`

type Json = Record<string, unknown>
const GB = 1024 ** 3
const DAY = 86_400_000

// ---------------------------------------------------------------- deterministic fake data
let seed = 42
const rand = () => {
  seed = (seed * 1664525 + 1013904223) % 4294967296
  return seed / 4294967296
}
const int = (min: number, max: number) => Math.floor(min + rand() * (max - min + 1))
const uuid = () => 'xxxxxxxx-xxxx-4xxx-8xxx-xxxxxxxxxxxx'.replace(/x/g, () => int(0, 15).toString(16))
const hex = (n: number) => Array.from({ length: n }, () => int(0, 15).toString(16)).join('')
const iso = (t: number) => new Date(t).toISOString().replace(/\.\d{3}Z$/, 'Z')
const day = (t: number) => iso(t).slice(0, 10)
const now = () => Date.now()

let nextId = 1000
const id = () => ++nextId

const groups: Json[] = [
  { id: 1, name: '基础组', created_at: iso(now() - 300 * DAY), updated_at: iso(now() - 30 * DAY) },
  { id: 2, name: '高级组', created_at: iso(now() - 200 * DAY), updated_at: iso(now() - 20 * DAY) },
  { id: 3, name: '空组', created_at: iso(now() - 10 * DAY), updated_at: iso(now() - 10 * DAY) },
]

const plans: Json[] = [
  {
    id: 1,
    group_id: 1,
    name: 'Lite',
    content: '## Lite\n\n- 100 GB / 月\n- 3 台设备',
    tags: ['入门'],
    transfer_enable: 100,
    speed_limit: null,
    device_limit: 3,
    capacity_limit: null,
    prices: { monthly: 1500, quarterly: 4200, yearly: 15000 },
    reset_traffic_method: null,
    show: true,
    sell: true,
    renew: true,
    sort: 1,
  },
  {
    id: 2,
    group_id: 2,
    name: 'Pro',
    content: '## Pro\n\n**500 GB** 高速流量',
    tags: ['推荐', '热门'],
    transfer_enable: 500,
    speed_limit: 300,
    device_limit: 5,
    capacity_limit: 200,
    prices: { monthly: 3500, half_yearly: 19000, yearly: 36000, reset_traffic: 1000 },
    reset_traffic_method: 1,
    show: true,
    sell: true,
    renew: true,
    sort: 2,
  },
  {
    id: 3,
    group_id: 2,
    name: 'Legacy',
    content: '',
    tags: [],
    transfer_enable: 50,
    speed_limit: null,
    device_limit: null,
    capacity_limit: null,
    prices: { onetime: 9900 },
    reset_traffic_method: 2,
    show: false,
    sell: false,
    renew: true,
    sort: 3,
  },
].map((p) => ({ ...p, created_at: iso(now() - 100 * DAY), updated_at: iso(now() - 5 * DAY) }))

const users: Json[] = Array.from({ length: 57 }, (_, i) => {
  const n = i + 1
  const planId = n % 7 === 0 ? null : (n % 3) + 1
  const plan = plans.find((p) => p.id === planId)
  const transfer = plan ? (plan.transfer_enable as number) * GB : 0
  const expired = n % 9 === 0 ? null : now() + (n % 5 === 0 ? -int(1, 30) : int(1, 300)) * DAY
  return {
    id: n,
    email: n === 1 ? 'admin@example.com' : `user${n}@example.com`,
    uuid: uuid(),
    token: hex(32),
    plan_id: planId,
    plan: plan ? { id: plan.id, name: plan.name } : null,
    group_id: plan ? plan.group_id : null,
    transfer_enable: transfer,
    u: Math.floor(transfer * rand() * 0.2),
    d: Math.floor(transfer * rand() * 0.7),
    expired_at: expired === null ? null : iso(expired),
    next_reset_at: iso(now() + int(1, 30) * DAY),
    speed_limit: n % 4 === 0 ? 100 : null,
    device_limit: n % 2 === 0 ? 3 : null,
    balance: int(0, 20) * 100,
    commission_balance: int(0, 5) * 100,
    commission_type: 0,
    commission_rate: null,
    discount: null,
    remind_expire: true,
    remind_traffic: n % 3 !== 0,
    is_admin: n === 1,
    is_staff: n === 2,
    banned: n % 11 === 0,
    avatar_url: '',
    created_at: iso(now() - int(1, 400) * DAY),
    last_login_at: n % 6 === 0 ? null : iso(now() - int(0, 30) * DAY),
    remarks: n % 8 === 0 ? '老用户，手动续费' : '',
    invite_user_id: n > 5 && n % 5 === 0 ? 3 : null,
    invite_user_email: n > 5 && n % 5 === 0 ? 'user3@example.com' : null,
    last_login_ip: `203.0.113.${n}`,
    telegram_id: null,
  }
})

const routes: Json[] = [
  {
    id: 1,
    remarks: '屏蔽广告',
    match: ['(.*\\.)?doubleclick\\.net', 'adservice\\.example\\.com'],
    action: 'block',
    action_value: '',
  },
  {
    id: 2,
    remarks: 'Netflix DNS',
    match: ['netflix.com', 'nflxvideo.net'],
    action: 'dns',
    action_value: '1.1.1.1',
  },
].map((r) => ({ ...r, created_at: iso(now() - 50 * DAY), updated_at: iso(now() - 2 * DAY) }))

const status = (i: number) => {
  const state = i % 5 === 4 ? 'offline' : i % 4 === 3 ? 'idle' : 'online'
  return {
    state,
    online: state === 'online' ? int(1, 60) : 0,
    last_check_at: state === 'offline' ? iso(now() - 3 * DAY) : iso(now() - int(5, 50) * 1000),
    last_push_at: state === 'online' ? iso(now() - int(5, 50) * 1000) : iso(now() - 2 * DAY),
    load:
      state === 'offline'
        ? null
        : {
            cpu: int(3, 95),
            mem: { total: 2 * GB, used: int(1, 19) * 0.1 * GB },
            swap: { total: 0, used: 0 },
            disk: { total: 40 * GB, used: int(5, 38) * GB },
            uptime: int(1, 90) * 86400,
          },
    version: state === 'offline' ? '' : 'V2bX 0.4.1',
  }
}

const nodes: Json[] = [
  {
    type: 'vless',
    name: '香港 01',
    host: 'hk1.example.com',
    port: '443',
    server_port: 443,
    tags: ['HK', 'Reality'],
    protocol_settings: {
      network: 'tcp',
      tls: 2,
      flow: 'xtls-rprx-vision',
      tls_settings: {
        server_name: 'www.example.org',
        dest: 'www.example.org:443',
        public_key: 'PUBLIC_KEY_BASE64URL',
        private_key: 'PRIVATE_KEY_BASE64URL',
        short_id: '0123abcd',
        fingerprint: 'chrome',
      },
    },
  },
  {
    type: 'vmess',
    name: '日本 01',
    host: 'jp1.example.com',
    port: '443',
    server_port: 10086,
    tags: ['JP'],
    protocol_settings: {
      network: 'ws',
      network_settings: { path: '/ws', headers: { Host: 'jp1.example.com' } },
      tls: 1,
      tls_settings: { server_name: 'jp1.example.com', allow_insecure: false },
    },
  },
  {
    type: 'shadowsocks',
    name: '新加坡 SS',
    host: 'sg.example.com',
    port: '8388',
    server_port: 8388,
    tags: ['SG'],
    protocol_settings: { cipher: '2022-blake3-aes-128-gcm', obfs: 'plain' },
  },
  {
    type: 'trojan',
    name: '美国 Trojan',
    host: 'us.example.com',
    port: '443',
    server_port: 443,
    tags: [],
    protocol_settings: {
      network: 'grpc',
      network_settings: { serviceName: 'trojan-grpc' },
      server_name: 'us.example.com',
      allow_insecure: false,
    },
  },
  {
    type: 'hysteria',
    name: '台湾 HY2',
    host: 'tw.example.com',
    port: '20000-30000',
    server_port: 36712,
    tags: ['TW'],
    protocol_settings: {
      version: 2,
      up_mbps: 0,
      down_mbps: 0,
      obfs: 'salamander',
      obfs_password: 'obfs-pass-123',
      server_name: 'tw.example.com',
      alpn: 'h3',
      hop_interval: 30,
    },
  },
  {
    type: 'anytls',
    name: '德国 AnyTLS',
    host: 'de.example.com',
    port: '8443',
    server_port: 8443,
    tags: [],
    protocol_settings: { tls: 1, server_name: 'de.example.com', padding_scheme: [] },
  },
  {
    type: 'mieru',
    name: '韩国 Mieru',
    host: 'kr.example.com',
    port: '2999',
    server_port: 2999,
    tags: [],
    protocol_settings: { transport: 'TCP', multiplexing: 'MULTIPLEXING_LOW' },
  },
  {
    type: 'tuic',
    name: '英国 TUIC',
    host: 'uk.example.com',
    port: '8443',
    server_port: 8443,
    tags: [],
    protocol_settings: {
      congestion_control: 'bbr',
      zero_rtt_handshake: false,
      udp_relay_mode: 'native',
      server_name: 'uk.example.com',
      alpn: 'h3',
    },
  },
  {
    type: 'shadowsocksr',
    name: '俄罗斯 SSR',
    host: 'ru.example.com',
    port: '443',
    server_port: 443,
    tags: [],
    protocol_settings: {
      cipher: 'chacha20-ietf',
      protocol: 'auth_chain_a',
      obfs: 'plain',
      single_port_type: 'protocol',
    },
  },
  {
    type: 'vless',
    name: '香港 01 中转',
    host: 'relay.example.com',
    port: '8443',
    server_port: 443,
    tags: ['HK', '中转'],
    parent_id: 1,
    protocol_settings: { network: 'tcp', tls: 2 },
  },
].map((n, i) => ({
  id: i + 1,
  group_ids: i % 2 === 0 ? [1, 2] : [2],
  route_ids: i === 0 ? [1] : i === 1 ? [1, 2] : [],
  parent_id: null,
  rate: i === 4 ? 0.5 : i === 1 ? 1.5 : 1,
  show: i !== 6,
  sort: i + 1,
  created_at: iso(now() - (100 - i) * DAY),
  updated_at: iso(now() - i * DAY),
  ...n,
}))

// ---------------------------------------------------------------- helpers
function send(res: ServerResponse, status: number, body: unknown) {
  res.statusCode = status
  res.setHeader('Content-Type', 'application/json; charset=utf-8')
  res.setHeader('Cache-Control', 'no-store')
  res.end(JSON.stringify(body))
}
const ok = (res: ServerResponse, data: unknown) => send(res, 200, { data })
const list = (res: ServerResponse, data: unknown[], total: number) => send(res, 200, { data, total })
const fail = (
  res: ServerResponse,
  status: number,
  code: string,
  message: string,
  errors?: Record<string, string[]>,
) => send(res, status, { code, message, ...(errors ? { errors } : {}) })

async function readBody(req: IncomingMessage): Promise<Json> {
  const chunks: Buffer[] = []
  for await (const chunk of req) chunks.push(chunk as Buffer)
  if (chunks.length === 0) return {}
  try {
    return JSON.parse(Buffer.concat(chunks).toString('utf8')) as Json
  } catch {
    return {}
  }
}

const find = (rows: Json[], rid: number) => rows.find((r) => r.id === rid)
const planWithCount = (p: Json) => ({ ...p, user_count: users.filter((u) => u.plan_id === p.id).length })
const groupWithCount = (g: Json) => ({
  ...g,
  user_count: users.filter((u) => u.group_id === g.id).length,
  plan_count: plans.filter((p) => p.group_id === g.id).length,
  node_count: nodes.filter((n) => (n.group_ids as number[]).includes(g.id as number)).length,
})
const nodeView = (n: Json) => {
  const parent = n.parent_id ? find(nodes, n.parent_id as number) : undefined
  const index = nodes.indexOf(n)
  return { ...n, parent: parent ? parent.name : null, status: status(parent ? nodes.indexOf(parent) : index) }
}

function filterUsers(q: URLSearchParams | Json): Json[] {
  const get = (k: string) =>
    q instanceof URLSearchParams ? q.get(k) : q[k] === undefined ? null : String(q[k])
  return users.filter((u) => {
    const email = get('email')
    if (email && !String(u.email).includes(email.toLowerCase())) return false
    for (const k of ['plan_id', 'group_id', 'invite_user_id', 'id'] as const) {
      const v = get(k)
      if (v && String(u[k]) !== v) return false
    }
    for (const k of ['banned', 'is_admin', 'is_staff'] as const) {
      const v = get(k)
      if (v && String(u[k]) !== v) return false
    }
    for (const k of ['uuid', 'token'] as const) {
      const v = get(k)
      if (v && u[k] !== v) return false
    }
    const remarks = get('remarks')
    if (remarks && !String(u.remarks).includes(remarks)) return false
    const expired = get('expired')
    if (expired === 'never' && u.expired_at !== null) return false
    if (expired === 'expired' && (u.expired_at === null || Date.parse(String(u.expired_at)) > now()))
      return false
    if (expired === 'active' && u.expired_at !== null && Date.parse(String(u.expired_at)) <= now())
      return false
    return true
  })
}

function validateNode(body: Json, selfId?: number): Record<string, string[]> {
  const errors: Record<string, string[]> = {}
  if (!body.name) errors.name = ['名称不能为空']
  if (!body.host) errors.host = ['地址不能为空']
  const sp = Number(body.server_port)
  if (!(sp > 0 && sp < 65536)) errors.server_port = ['服务端口必须在 1–65535 之间']
  if (body.parent_id) {
    const parent = find(nodes, Number(body.parent_id))
    if (!parent) errors.parent_id = ['父节点不存在']
    else if (parent.type !== body.type) errors.parent_id = ['子节点类型必须与父节点一致']
    else if (parent.parent_id) errors.parent_id = ['父节点不能是子节点']
    else if (parent.id === selfId) errors.parent_id = ['不能以自身为父节点']
  }
  const ps = (body.protocol_settings ?? {}) as Json
  const ts = (ps.tls_settings ?? {}) as Json
  if ((body.type === 'vless' || body.type === 'anytls') && ps.tls === 2 && !body.parent_id) {
    if (!ts.private_key)
      errors['protocol_settings.tls_settings.private_key'] = ['Reality 需要私钥（mock 校验）']
    if (!ts.public_key)
      errors['protocol_settings.tls_settings.public_key'] = ['Reality 需要公钥（mock 校验）']
  }
  if (body.type === 'shadowsocks' && !ps.cipher) errors['protocol_settings.cipher'] = ['请选择加密方式']
  return errors
}

function series(metric: string, range: string) {
  const days = range === '7d' ? 7 : range === '90d' ? 90 : 30
  seed = metric.length * 97 + days
  return Array.from({ length: days }, (_, i) => {
    const t = now() - (days - 1 - i) * DAY
    const wave = 1 + Math.sin(i / 3) * 0.3
    const value =
      metric === 'income'
        ? Math.round(int(20000, 60000) * wave)
        : metric === 'orders'
          ? Math.round(int(8, 30) * wave)
          : metric === 'registers'
            ? Math.round(int(5, 40) * wave)
            : Math.round(int(200, 900) * GB * wave)
    return { date: day(t), value }
  })
}

// ---------------------------------------------------------------- router
async function handle(req: IncomingMessage, res: ServerResponse) {
  const url = new URL(req.url ?? '/', 'http://localhost')
  const p = url.pathname.replace(/^\/api\/v1/, '')
  const method = req.method ?? 'GET'
  const q = url.searchParams
  const seg = p.split('/').filter(Boolean)
  const rid = Number(seg[2])

  if (p === '/guest/config') return ok(res, fixture('guest-config.json'))
  if (p === '/passport/login' || p === '/passport/login/token' || p === '/passport/register')
    return ok(res, { token: TOKEN, user: { ...fixture('user.json'), email: 'admin@example.com' } })
  if (req.headers.authorization !== `Bearer ${TOKEN}`)
    return fail(res, 401, 'unauthorized', 'mock: not signed in')
  if (p === '/user/me') return ok(res, { ...fixture('user.json'), email: 'admin@example.com' })
  if (p === '/user/logout') return ok(res, true)

  // ---- stats
  if (p === '/admin/stats/overview')
    return ok(res, {
      today: { income: 45600, paid_orders: 18, registers: 23, commission: 2300 },
      month: { income: 1234500, paid_orders: 412, registers: 530 },
      totals: {
        users: users.length,
        active_users: 41,
        online_users: 17,
        pending_tickets: 3,
        pending_orders: 6,
        pending_commission: 8800,
      },
      nodes: {
        total: nodes.length,
        online: nodes.filter((n) => nodeView(n).status.state !== 'offline').length,
      },
    })
  if (p === '/admin/stats/series')
    return ok(res, series(q.get('metric') ?? 'income', q.get('range') ?? '30d'))
  if (p === '/admin/stats/nodes/ranking') {
    seed = Number((q.get('date') ?? '').replace(/-/g, '')) % 1000
    return ok(
      res,
      nodes
        .slice(0, 8)
        .map((n) => {
          const u = int(1, 40) * GB
          const d = int(20, 400) * GB
          return { node_id: n.id, name: n.name, u, d, total: u + d }
        })
        .sort((a, b) => b.total - a.total),
    )
  }
  if (p === '/admin/stats/users/ranking') {
    seed = Number((q.get('date') ?? '').replace(/-/g, '')) % 997
    const limit = Number(q.get('limit') ?? 10)
    return ok(
      res,
      users
        .slice(0, limit)
        .map((u) => {
          const up = int(1, 20) * GB
          const down = int(5, 120) * GB
          return { user_id: u.id, email: u.email, u: up, d: down, total: up + down }
        })
        .sort((a, b) => b.total - a.total),
    )
  }

  // ---- users
  if (p === '/admin/users' && method === 'GET') {
    let rows = filterUsers(q)
    const sort = q.get('sort')
    if (sort) {
      const desc = sort.startsWith('-')
      const key = desc ? sort.slice(1) : sort
      rows = [...rows].sort((a, b) => {
        const av = a[key] as number | string | null
        const bv = b[key] as number | string | null
        const cmp = av === bv ? 0 : av === null ? 1 : bv === null ? -1 : av > bv ? 1 : -1
        return desc ? -cmp : cmp
      })
    }
    const page = Number(q.get('page') ?? 1)
    const size = Number(q.get('page_size') ?? 20)
    return list(res, rows.slice((page - 1) * size, page * size), rows.length)
  }
  if (p === '/admin/users' && method === 'POST') {
    const body = await readBody(req)
    const errors: Record<string, string[]> = {}
    if (!body.email || !String(body.email).includes('@')) errors.email = ['邮箱格式不正确']
    else if (users.some((u) => u.email === body.email)) errors.email = ['该邮箱已被注册']
    if (!body.password || String(body.password).length < 8) errors.password = ['密码至少 8 位']
    if (Object.keys(errors).length) return fail(res, 400, 'validation_failed', '请检查填写的内容', errors)
    const plan = body.plan_id ? find(plans, Number(body.plan_id)) : undefined
    const user = {
      ...users[3],
      id: id(),
      email: body.email,
      uuid: uuid(),
      token: hex(32),
      plan_id: plan?.id ?? null,
      plan: plan ? { id: plan.id, name: plan.name } : null,
      group_id: body.group_id ?? plan?.group_id ?? null,
      transfer_enable: Number(body.transfer_enable ?? 0) * GB,
      u: 0,
      d: 0,
      expired_at: body.expired_at ?? null,
      is_admin: Boolean(body.is_admin),
      is_staff: Boolean(body.is_staff),
      banned: false,
      balance: Number(body.balance ?? 0),
      speed_limit: body.speed_limit ?? null,
      device_limit: body.device_limit ?? null,
      remarks: body.remarks ?? '',
      created_at: iso(now()),
      last_login_at: null,
      invite_user_id: null,
      invite_user_email: null,
    }
    users.unshift(user)
    return ok(res, user)
  }
  if (p === '/admin/users/generate') {
    const body = await readBody(req)
    const count = Number(body.count ?? 0)
    if (!body.email_suffix)
      return fail(res, 400, 'validation_failed', '请检查填写的内容', { email_suffix: ['请填写邮箱后缀'] })
    if (!(count >= 1 && count <= 500))
      return fail(res, 400, 'validation_failed', '请检查填写的内容', { count: ['数量需在 1–500 之间'] })
    const lines = ['email,password,subscribe_url']
    for (let i = 0; i < count; i++) {
      const email = `${String(body.email_prefix ?? '')}${hex(6)}@${String(body.email_suffix)}`
      lines.push(`${email},${String(body.password || hex(8))},https://panel.example.com/s/${hex(32)}`)
    }
    return ok(res, { count, csv: lines.join('\n') })
  }
  if (p === '/admin/users/export') {
    const body = await readBody(req)
    const rows = filterUsers(body)
    res.statusCode = 200
    res.setHeader('Content-Type', 'text/csv; charset=utf-8')
    res.end(
      [
        'id,email,plan,expired_at',
        ...rows.map(
          (u) =>
            `${String(u.id)},${String(u.email)},${(u.plan as Json | null)?.name ?? ''},${String(u.expired_at ?? '')}`,
        ),
      ].join('\n'),
    )
    return
  }
  if (p === '/admin/users/mail') {
    const body = await readBody(req)
    if (!body.subject || !body.content)
      return fail(res, 400, 'validation_failed', '请检查填写的内容', {
        ...(body.subject ? {} : { subject: ['请填写主题'] }),
        ...(body.content ? {} : { content: ['请填写内容'] }),
      })
    return ok(res, { queued: filterUsers(body).length })
  }
  if (seg[0] === 'admin' && seg[1] === 'users' && rid) {
    const user = find(users, rid)
    if (!user) return fail(res, 404, 'not_found', '用户不存在')
    const action = seg[3]
    if (method === 'GET' && !action)
      return ok(res, { ...user, stats: { orders_total: 128800, commission_total: 2300 } })
    if (method === 'PATCH') {
      const body = await readBody(req)
      if (body.email && users.some((u) => u.email === body.email && u.id !== rid))
        return fail(res, 400, 'validation_failed', '请检查填写的内容', { email: ['该邮箱已被注册'] })
      if (body.password !== undefined && String(body.password).length < 8)
        return fail(res, 400, 'validation_failed', '请检查填写的内容', { password: ['密码至少 8 位'] })
      const { password: _pw, ...patch } = body
      Object.assign(user, patch)
      if ('plan_id' in patch) {
        const plan = patch.plan_id ? find(plans, Number(patch.plan_id)) : undefined
        user.plan = plan ? { id: plan.id, name: plan.name } : null
      }
      return ok(res, user)
    }
    if (method === 'DELETE') {
      users.splice(users.indexOf(user), 1)
      return ok(res, true)
    }
    if (action === 'reset-secret') {
      user.uuid = uuid()
      user.token = hex(32)
      return ok(res, user)
    }
    if (action === 'login-token') return ok(res, { url: `/login?token=${hex(32)}` })
    if (action === 'traffic-reset') {
      user.u = 0
      user.d = 0
      return ok(res, user)
    }
  }

  // ---- plans
  if (p === '/admin/plans' && method === 'GET')
    return ok(res, [...plans].sort((a, b) => (a.sort as number) - (b.sort as number)).map(planWithCount))
  if (p === '/admin/plans' && method === 'POST') {
    const body = await readBody(req)
    if (!body.name) return fail(res, 400, 'validation_failed', '请检查填写的内容', { name: ['请填写名称'] })
    const plan = { ...body, id: id(), sort: plans.length + 1, created_at: iso(now()), updated_at: iso(now()) }
    plans.push(plan)
    return ok(res, plan)
  }
  if (p === '/admin/plans/sort') {
    const body = await readBody(req)
    ;(body.ids as number[]).forEach((pid, i) => {
      const plan = find(plans, pid)
      if (plan) plan.sort = i + 1
    })
    return ok(res, true)
  }
  if (seg[0] === 'admin' && seg[1] === 'plans' && rid) {
    const plan = find(plans, rid)
    if (!plan) return fail(res, 404, 'not_found', '套餐不存在')
    if (method === 'GET') return ok(res, planWithCount(plan))
    if (method === 'PATCH') {
      Object.assign(plan, await readBody(req), { updated_at: iso(now()) })
      return ok(res, plan)
    }
    if (method === 'DELETE') {
      const count = users.filter((u) => u.plan_id === rid).length
      if (count) return fail(res, 409, 'conflict', `仍有 ${count} 个用户使用该套餐，无法删除`)
      plans.splice(plans.indexOf(plan), 1)
      return ok(res, true)
    }
  }

  // ---- groups
  if (p === '/admin/groups' && method === 'GET') return ok(res, groups.map(groupWithCount))
  if (p === '/admin/groups' && method === 'POST') {
    const body = await readBody(req)
    if (!body.name) return fail(res, 400, 'validation_failed', '请检查填写的内容', { name: ['请填写名称'] })
    const group = { id: id(), name: body.name, created_at: iso(now()), updated_at: iso(now()) }
    groups.push(group)
    return ok(res, groupWithCount(group))
  }
  if (seg[0] === 'admin' && seg[1] === 'groups' && rid) {
    const group = find(groups, rid)
    if (!group) return fail(res, 404, 'not_found', '权限组不存在')
    if (method === 'PATCH') {
      const body = await readBody(req)
      if (!body.name) return fail(res, 400, 'validation_failed', '请检查填写的内容', { name: ['请填写名称'] })
      group.name = body.name
      return ok(res, groupWithCount(group))
    }
    if (method === 'DELETE') {
      const g = groupWithCount(group)
      if (g.user_count || g.plan_count || g.node_count)
        return fail(
          res,
          409,
          'conflict',
          `权限组仍被 ${g.user_count} 个用户、${g.plan_count} 个套餐、${g.node_count} 个节点使用`,
        )
      groups.splice(groups.indexOf(group), 1)
      return ok(res, true)
    }
  }

  // ---- nodes
  if (p === '/admin/nodes' && method === 'GET')
    return ok(res, [...nodes].sort((a, b) => (a.sort as number) - (b.sort as number)).map(nodeView))
  if (p === '/admin/nodes' && method === 'POST') {
    const body = await readBody(req)
    const errors = validateNode(body)
    if (Object.keys(errors).length) return fail(res, 400, 'validation_failed', '请检查填写的内容', errors)
    const node = {
      ...body,
      id: id(),
      sort: body.sort ?? nodes.length + 1,
      created_at: iso(now()),
      updated_at: iso(now()),
    }
    nodes.push(node)
    return ok(res, nodeView(node))
  }
  if (p === '/admin/nodes/sort') {
    const body = await readBody(req)
    ;(body.ids as number[]).forEach((nid, i) => {
      const node = find(nodes, nid)
      if (node) node.sort = i + 1
    })
    return ok(res, true)
  }
  if (seg[0] === 'admin' && seg[1] === 'nodes' && rid) {
    const node = find(nodes, rid)
    if (!node) return fail(res, 404, 'not_found', '节点不存在')
    const action = seg[3]
    if (method === 'GET' && !action) return ok(res, nodeView(node))
    if (method === 'PATCH') {
      const body = await readBody(req)
      const errors = validateNode({ ...node, ...body }, rid)
      if (Object.keys(errors).length) return fail(res, 400, 'validation_failed', '请检查填写的内容', errors)
      Object.assign(node, body, { updated_at: iso(now()) })
      return ok(res, nodeView(node))
    }
    if (method === 'DELETE') {
      nodes.splice(nodes.indexOf(node), 1)
      return ok(res, true)
    }
    if (action === 'copy') {
      const copy = {
        ...structuredClone(node),
        id: id(),
        name: `${String(node.name)} (副本)`,
        sort: nodes.length + 1,
      }
      nodes.push(copy)
      return ok(res, nodeView(copy))
    }
    if (action === 'stats') {
      const days = q.get('range') === '30d' ? 30 : 7
      seed = rid * 31 + days
      return ok(
        res,
        Array.from({ length: days }, (_, i) => ({
          date: day(now() - (days - 1 - i) * DAY),
          u: int(1, 30) * GB,
          d: int(10, 200) * GB,
        })),
      )
    }
    if (action === 'online') {
      const count = nodeView(node).status.online
      return ok(
        res,
        users
          .slice(0, Math.min(count, 12))
          .map((u, i) => ({
            user_id: u.id,
            email: u.email,
            ips: i % 3 === 0 ? ['203.0.113.7', '2001:db8::1'] : [`198.51.100.${i + 1}`],
          })),
      )
    }
  }

  // ---- routes
  if (p === '/admin/routes' && method === 'GET') return ok(res, routes)
  if ((p === '/admin/routes' && method === 'POST') || (seg[1] === 'routes' && rid && method === 'PATCH')) {
    const body = await readBody(req)
    const match = (body.match ?? []) as string[]
    const errors: Record<string, string[]> = {}
    if (!Array.isArray(match) || match.length === 0) errors.match = ['至少填写一条匹配规则']
    else if (match.length > 999) errors.match = ['最多 999 条']
    if (body.action === 'dns' && !body.action_value) errors.action_value = ['DNS 动作需要填写 DNS 服务器']
    if (Object.keys(errors).length) return fail(res, 400, 'validation_failed', '请检查填写的内容', errors)
    if (method === 'POST') {
      const route = { ...body, id: id(), created_at: iso(now()), updated_at: iso(now()) }
      routes.push(route)
      return ok(res, route)
    }
    const route = find(routes, rid)
    if (!route) return fail(res, 404, 'not_found', '路由不存在')
    Object.assign(route, body, { updated_at: iso(now()) })
    return ok(res, route)
  }
  if (seg[1] === 'routes' && rid && method === 'DELETE') {
    const route = find(routes, rid)
    if (!route) return fail(res, 404, 'not_found', '路由不存在')
    const using = nodes.filter((n) => (n.route_ids as number[]).includes(rid)).length
    if (using) return fail(res, 409, 'conflict', `仍有 ${using} 个节点引用该路由，无法删除`)
    routes.splice(routes.indexOf(route), 1)
    return ok(res, true)
  }

  return fail(res, 404, 'not_found', `mock: ${method} ${p} is not implemented`)
}

createServer((req, res) => {
  handle(req, res).catch((error: unknown) => fail(res, 500, 'internal', String(error)))
}).listen(PORT, () => {
  console.log(
    `[mock] admin API on http://localhost:${PORT} — run: GGBOARD_API=http://localhost:${PORT} pnpm dev`,
  )
})
