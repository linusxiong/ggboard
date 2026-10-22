/**
 * Dev-only in-memory mock of the user-portal API (FE-1): guest config, passport (password, 2FA step,
 * passkey), dashboard data, subscription + rule profile + preview, profile/security. Lets the pages be
 * clicked through without the Go backend:
 *
 *   pnpm exec vite --config scripts/mock/vite.config.ts      # http://localhost:5173
 *
 * Demo: any email + password `password`. An email starting with `mfa` (or any account after enabling 2FA)
 * gets the second step; TOTP `123456` or a backup code works. Wrong passwords lock out after 5 tries
 * (429 + Retry-After). Rule profiles with a group named `bad` are rejected with field errors. Unknown
 * `/api/v1/*` paths answer 404 (never 401, so other areas don't log you out). Never part of the bundle.
 */
import { readFileSync } from 'node:fs'
import type { IncomingMessage, ServerResponse } from 'node:http'
import path from 'node:path'
import type { Plugin } from 'vite'

const here = import.meta.dirname
const fixtures = path.resolve(here, '../fixtures')
const catalogFile = path.resolve(here, '../../../internal/subscription/rulesets/catalog.json')
const read = (file: string) => JSON.parse(readFileSync(file, 'utf8')) as Record<string, unknown>

const b64url = (s: string) => Buffer.from(s).toString('base64url')
/** Unsigned demo JWT (exp 2100-01-01) — never accepted by the real backend. */
const token = (n: number) =>
  `eyJhbGciOiJub25lIn0.${b64url(JSON.stringify({ sub: 1, n, exp: 4102444800 }))}.mock`
let session = token(1)

interface Group {
  name: string
  type: string
  include: string
  url?: string
  interval?: number
}
interface Profile {
  template: string
  source: string
  categories: Record<string, string>
  groups: Group[]
  final: string
  geoip_cn_direct: boolean
  mirror: string
  dns: string
}

const state = {
  password: 'password',
  failures: 0,
  mfaFailures: 0,
  oldPasswordFailures: 0,
  totp: { enabled: false, enabledAt: null as string | null, pending: false },
  backupCodes: [] as string[],
  passkeys: [
    {
      id: 1,
      name: 'iCloud 钥匙串',
      created_at: '2026-09-01T08:00:00Z',
      last_used_at: '2026-10-20T12:30:00Z',
      backup_eligible: true,
      transports: ['internal', 'hybrid'],
    },
  ] as Array<Record<string, unknown>>,
  user: { ...read(path.join(fixtures, 'user.json')), is_admin: false, avatar_url: '' } as Record<
    string,
    unknown
  >,
  subToken: 'a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6',
  profile: null as Profile | null,
}

function catalog() {
  return read(catalogFile) as unknown as {
    templates: Array<{
      id: string
      categories: Record<string, string>
      groups: Group[]
      final: string
      geoip_cn_direct: boolean
    }>
  }
}

function defaultProfile(): Profile {
  const std = catalog().templates.find((t) => t.id === 'standard')!
  return {
    template: 'standard',
    source: 'blackmatrix7',
    categories: { ...std.categories },
    groups: structuredClone(std.groups),
    final: std.final,
    geoip_cn_direct: std.geoip_cn_direct,
    mirror: '',
    dns: 'default',
  }
}

function send(res: ServerResponse, status: number, body: unknown, headers: Record<string, string> = {}) {
  res.statusCode = status
  res.setHeader('Content-Type', 'application/json; charset=utf-8')
  res.setHeader('Cache-Control', 'no-store')
  for (const [k, v] of Object.entries(headers)) res.setHeader(k, v)
  res.end(JSON.stringify(body))
}
const ok = (res: ServerResponse, data: unknown) => send(res, 200, { data })
const fail = (
  res: ServerResponse,
  status: number,
  code: string,
  message: string,
  errors?: Record<string, string[]>,
  headers?: Record<string, string>,
) => send(res, status, { code, message, ...(errors ? { errors } : {}) }, headers)

/** JSON body; every value coerced to a string-ish primitive for the mock's simple checks. */
async function body(req: IncomingMessage): Promise<Record<string, string | undefined>> {
  const chunks: Buffer[] = []
  for await (const chunk of req) chunks.push(chunk as Buffer)
  const text = Buffer.concat(chunks).toString('utf8')
  if (!text) return {}
  try {
    return JSON.parse(text) as Record<string, string | undefined>
  } catch {
    return {}
  }
}

const day = (offset: number) => {
  const d = new Date()
  d.setDate(d.getDate() - offset)
  return d.toISOString().slice(0, 10)
}

const NODES = [
  ['香港 01', 'vless', ['IPLC', '流媒体'], 1, true],
  ['香港 02', 'trojan', ['BGP'], 1, true],
  ['日本 东京', 'hysteria', ['Hysteria2'], 1, true],
  ['日本 大阪', 'vmess', [], 1, false],
  ['新加坡 01', 'shadowsocks', ['2022'], 1, true],
  ['美国 洛杉矶', 'vless', ['Reality', 'AI'], 1.5, true],
  ['美国 圣何塞', 'anytls', [], 1, true],
  ['台湾 01', 'tuic', [], 1, true],
  ['英国 伦敦', 'mieru', [], 2, true],
  ['德国 法兰克福', 'shadowsocksr', [], 0.5, false],
] as const

const backupCodes = () =>
  Array.from(
    { length: 10 },
    (_, i) =>
      `${(i * 7919 + 10007).toString(36).padStart(5, 'k').slice(-5)}-${(i * 104729 + 3001).toString(36).padStart(5, 'm').slice(-5)}`,
  )

function profileErrors(p: Profile): Record<string, string[]> | null {
  const errors: Record<string, string[]> = {}
  p.groups.forEach((g, i) => {
    if (g.name.toLowerCase().includes('bad')) errors[`groups[${i}].name`] = ['is not allowed (mock rule)']
  })
  return Object.keys(errors).length ? errors : null
}

function preview(client: string, override: string | null): string {
  const p = state.profile ?? defaultProfile()
  const template = override ?? p.template
  const enabled = Object.entries(p.categories)
  if (client === 'v2ray')
    return Buffer.from(
      NODES.map(([n]) => `vless://uuid@example.com:443#${encodeURIComponent(n)}`).join('\n'),
    ).toString('base64')
  return [
    `# ${client} config generated by the FE-1 mock (template: ${template}, source: ${p.source})`,
    'proxy-groups:',
    ...p.groups.map((g) => `  - { name: ${g.name}, type: ${g.type}, include: "${g.include}" }`),
    'rules:',
    ...enabled.map(([id, target]) => `  - RULE-SET,${id},${target}`),
    ...(p.geoip_cn_direct ? ['  - GEOIP,CN,DIRECT'] : []),
    `  - MATCH,${p.final}`,
  ].join('\n')
}

async function handle(req: IncomingMessage, res: ServerResponse): Promise<boolean> {
  const url = new URL(req.url ?? '/', 'http://localhost')
  if (!url.pathname.startsWith('/api/v1/')) return false
  const route = `${req.method} ${url.pathname.slice('/api/v1'.length)}`
  const authed = req.headers.authorization === `Bearer ${session}`
  const user = state.user

  // ---------------------------------------------------------------- public
  switch (route) {
    case 'GET /guest/config':
      ok(res, {
        ...read(path.join(fixtures, 'guest-config.json')),
        passkey_enabled: true,
        allow_user_rule_profile: process.env.MOCK_RULES_LOCKED !== '1',
      })
      return true
    case 'GET /guest/plans':
      ok(res, JSON.parse(readFileSync(path.join(fixtures, 'guest-plans.json'), 'utf8')))
      return true
    case 'GET /guest/rulesets/catalog':
      ok(res, read(catalogFile))
      return true
    case 'POST /passport/login': {
      const input = await body(req)
      if (state.failures >= 5) {
        fail(res, 429, 'rate_limited', 'too many failed attempts', undefined, { 'Retry-After': '900' })
        return true
      }
      if (input.password !== state.password) {
        state.failures++
        fail(res, 401, 'unauthorized', 'invalid email or password')
        return true
      }
      state.failures = 0
      if (state.totp.enabled || String(input.email).startsWith('mfa')) {
        state.mfaFailures = 0
        ok(res, {
          mfa_required: true,
          mfa_token: 'mock-mfa-token',
          methods: ['totp'],
          has_passkey: state.passkeys.length > 0,
        })
        return true
      }
      ok(res, { token: session, user })
      return true
    }
    case 'POST /passport/login/mfa': {
      const input = await body(req)
      if (input.mfa_token !== 'mock-mfa-token') {
        fail(res, 401, 'mfa_expired', 'mfa token expired')
        return true
      }
      const code = String(input.code ?? '').toLowerCase()
      const backup = state.backupCodes.indexOf(code.replace(/^(\w{5})(\w{5})$/, '$1-$2'))
      if (code === '123456' || backup >= 0) {
        if (backup >= 0) state.backupCodes.splice(backup, 1)
        ok(res, { token: session, user })
        return true
      }
      state.mfaFailures++
      if (state.mfaFailures >= 5)
        fail(res, 429, 'rate_limited', 'too many attempts', undefined, { 'Retry-After': '300' })
      else fail(res, 401, 'mfa_invalid', 'invalid code')
      return true
    }
    case 'POST /passport/passkey/login/begin':
      ok(res, {
        challenge_id: 'mock-challenge',
        options: {
          challenge: b64url('mock-challenge-bytes'),
          timeout: 120000,
          rpId: 'localhost',
          userVerification: 'preferred',
        },
      })
      return true
    case 'POST /passport/passkey/login/finish':
      ok(res, { token: session, user })
      return true
    case 'POST /passport/login/token':
      ok(res, { token: session, user })
      return true
    case 'POST /passport/register':
      ok(res, { token: session, user })
      return true
    case 'POST /passport/forgot':
    case 'POST /passport/email/send-code':
      ok(res, true)
      return true
  }

  if (!authed) {
    fail(res, 401, 'unauthorized', 'mock backend: not signed in')
    return true
  }

  // ---------------------------------------------------------------- user
  const passkeyMatch = /^(PATCH|DELETE) \/user\/security\/passkeys\/(\d+)$/.exec(route)
  if (passkeyMatch) {
    const id = Number(passkeyMatch[2])
    const index = state.passkeys.findIndex((p) => p.id === id)
    if (index < 0) return (fail(res, 404, 'not_found', 'passkey not found'), true)
    const input = await body(req)
    if (passkeyMatch[1] === 'PATCH') {
      state.passkeys[index] = { ...state.passkeys[index], name: String(input.name ?? '').slice(0, 64) }
      ok(res, state.passkeys[index])
    } else if (input.password !== state.password) {
      fail(res, 400, 'validation_failed', 'invalid input', { password: ['is incorrect'] })
    } else {
      state.passkeys.splice(index, 1)
      ok(res, true)
    }
    return true
  }

  switch (route) {
    case 'GET /user/me':
      ok(res, user)
      return true
    case 'PATCH /user/me':
      Object.assign(user, await body(req))
      ok(res, user)
      return true
    case 'POST /user/logout':
      ok(res, true)
      return true
    case 'POST /user/password': {
      const input = await body(req)
      if (state.oldPasswordFailures >= 5) {
        fail(res, 429, 'rate_limited', 'too many failed attempts', undefined, { 'Retry-After': '600' })
        return true
      }
      if (input.old_password !== state.password) {
        state.oldPasswordFailures++
        fail(res, 400, 'validation_failed', 'invalid input', { old_password: ['is incorrect'] })
        return true
      }
      state.password = String(input.new_password)
      session = token(Date.now())
      ok(res, true)
      return true
    }
    case 'GET /user/stats':
      ok(res, {
        pending_orders: 1,
        open_tickets: 2,
        notices_count: 3,
        invite_count: 4,
        commission_pending: 1200,
      })
      return true
    case 'GET /user/subscription':
      ok(res, {
        subscribe_url: `http://localhost:5173/s/${state.subToken}`,
        token: state.subToken,
        plan: user.plan,
        transfer_enable: user.transfer_enable,
        u: user.u,
        d: user.d,
        expired_at: user.expired_at,
        next_reset_at: user.next_reset_at,
        reset_day: 1,
        device_limit: user.device_limit,
        speed_limit: user.speed_limit,
      })
      return true
    case 'POST /user/subscription/reset':
      state.subToken = Math.random().toString(16).slice(2).padEnd(32, '0')
      user.uuid = crypto.randomUUID()
      session = token(Date.now())
      ok(res, {
        subscribe_url: `http://localhost:5173/s/${state.subToken}`,
        token: state.subToken,
        auth_token: session,
      })
      return true
    case 'GET /user/subscription/preview':
      res.statusCode = 200
      res.setHeader('Content-Type', 'text/plain; charset=utf-8')
      res.end(preview(url.searchParams.get('client') ?? 'clash-meta', url.searchParams.get('profile')))
      return true
    case 'GET /user/traffic-logs': {
      const items = []
      for (let i = 0; i < 9; i++)
        for (const [n, [name]] of NODES.slice(0, 3).entries())
          if ((i + n) % 4 !== 3)
            items.push({
              record_at: day(i),
              node_id: n + 1,
              node_name: i === 5 && n === 2 ? null : name,
              rate: 1,
              u: (n + 1) * 157_286_400 * ((i % 3) + 1),
              d: (n + 2) * 1_073_741_824 * ((i % 4) + 1),
            })
      send(res, 200, { data: items, total: items.length })
      return true
    }
    case 'GET /user/nodes':
      ok(
        res,
        NODES.map(([name, type, tags, rate, online], i) => ({
          id: i + 1,
          name,
          type,
          tags,
          rate,
          host: `node${i + 1}.example.com`,
          port: type === 'hysteria' ? '20000-30000' : '443',
          is_online: online,
          sort: i,
        })),
      )
      return true
    case 'GET /user/notices':
      send(res, 200, {
        data: [
          {
            id: 3,
            title: '十一月套餐优惠',
            content: '**全场 8 折**，使用优惠码 `NOV` 即可。[查看详情](https://example.com)',
            img_url: '',
            tags: ['活动'],
            created_at: '2026-10-20T08:00:00Z',
          },
          {
            id: 2,
            title: '日本节点维护通知',
            content: '10 月 25 日 02:00–04:00 日本大阪节点维护，期间请切换至其他节点。',
            img_url: '',
            tags: [],
            created_at: '2026-10-18T08:00:00Z',
          },
          {
            id: 1,
            title: '新增通行密钥登录',
            content: '现在可以在个人中心 → 安全 中添加通行密钥，无需密码即可登录。',
            img_url: '',
            tags: [],
            created_at: '2026-10-10T08:00:00Z',
          },
        ],
        total: 3,
      })
      return true
    case 'GET /user/rulesets/catalog':
      ok(res, read(catalogFile))
      return true
    case 'GET /user/rule-profile':
      ok(res, state.profile ?? defaultProfile())
      return true
    case 'PUT /user/rule-profile': {
      if (process.env.MOCK_RULES_LOCKED === '1') {
        fail(res, 403, 'forbidden', 'custom rule profiles are disabled')
        return true
      }
      const input = (await body(req)) as unknown as Profile
      const errors = profileErrors(input)
      if (errors) {
        fail(res, 400, 'validation_failed', 'invalid rule profile', errors)
        return true
      }
      state.profile = input
      ok(res, input)
      return true
    }
    case 'GET /user/security':
      ok(res, {
        totp_enabled: state.totp.enabled,
        totp_enabled_at: state.totp.enabledAt,
        backup_codes_remaining: state.totp.enabled ? state.backupCodes.length : 0,
        passkeys: state.passkeys,
      })
      return true
    case 'POST /user/security/totp/setup':
      if (state.totp.enabled) return (fail(res, 409, 'conflict', 'already enabled'), true)
      state.totp.pending = true
      ok(res, {
        secret: 'JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP',
        otpauth_url: `otpauth://totp/GG%20Bond:${encodeURIComponent(String(user.email))}?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=GG%20Bond`,
      })
      return true
    case 'POST /user/security/totp/enable': {
      const input = await body(req)
      if (state.totp.enabled) return (fail(res, 409, 'conflict', 'already enabled'), true)
      if (!state.totp.pending || input.code !== '123456')
        return (fail(res, 400, 'validation_failed', 'invalid input', { code: ['is invalid'] }), true)
      state.totp = { enabled: true, enabledAt: new Date().toISOString(), pending: false }
      state.backupCodes = backupCodes()
      ok(res, { backup_codes: state.backupCodes })
      return true
    }
    case 'POST /user/security/totp/disable': {
      const input = await body(req)
      if (!state.totp.enabled) return (fail(res, 409, 'conflict', 'not enabled'), true)
      const errors: Record<string, string[]> = {}
      if (input.password !== state.password) errors.password = ['is incorrect']
      if (input.code !== '123456' && !state.backupCodes.includes(String(input.code)))
        errors.code = ['is invalid']
      if (Object.keys(errors).length)
        return (fail(res, 400, 'validation_failed', 'invalid input', errors), true)
      state.totp = { enabled: false, enabledAt: null, pending: false }
      state.backupCodes = []
      ok(res, true)
      return true
    }
    case 'POST /user/security/backup-codes': {
      const input = await body(req)
      if (!state.totp.enabled) return (fail(res, 409, 'conflict', 'totp is off'), true)
      if (input.code !== '123456')
        return (fail(res, 400, 'validation_failed', 'invalid input', { code: ['is invalid'] }), true)
      state.backupCodes = backupCodes().reverse()
      ok(res, { backup_codes: state.backupCodes })
      return true
    }
    case 'POST /user/security/passkeys/begin':
      ok(res, {
        challenge_id: 'mock-reg',
        options: {
          rp: { name: 'GG Bond', id: 'localhost' },
          user: { name: user.email, displayName: user.email, id: b64url('0123456789abcdef') },
          challenge: b64url(`reg-${Date.now()}`),
          pubKeyCredParams: [
            { type: 'public-key', alg: -7 },
            { type: 'public-key', alg: -257 },
          ],
          timeout: 120000,
          excludeCredentials: [],
          authenticatorSelection: {
            requireResidentKey: true,
            residentKey: 'required',
            userVerification: 'preferred',
          },
          attestation: 'none',
          extensions: { credProps: true },
        },
      })
      return true
    case 'POST /user/security/passkeys/finish': {
      const input = await body(req)
      const passkey = {
        id: Date.now(),
        name: String(input.name || 'Passkey'),
        created_at: new Date().toISOString(),
        last_used_at: null,
        backup_eligible: true,
        transports: ['internal'],
      }
      state.passkeys.push(passkey)
      ok(res, passkey)
      return true
    }
  }
  fail(res, 404, 'not_found', `mock backend: ${route} not mocked`)
  return true
}

export function userPortalMock(): Plugin {
  return {
    name: 'ggboard:user-portal-mock',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        handle(req, res).then(
          (handled) => {
            if (!handled) next()
          },
          (error: unknown) => next(error),
        )
      })
    },
  }
}
