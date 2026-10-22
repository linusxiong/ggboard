/**
 * Dev-only mock of the FE-S endpoints (subscription profiles + node operations, age keys, access log, token
 * rotation, Telegram binding), plugged into `user-portal.ts` (user side) and `admin-a.ts` (user drawer).
 * In-memory, fake data on example.com / documentation IP ranges. Never part of the bundle.
 *
 *   pnpm exec vite --config scripts/mock/vite.config.ts --port 5181     # user portal + FE-S
 *   MOCK_FES_LEGACY=1 …                                                  # server without profiles (404s)
 *
 * Telegram: after "bind", the mock binds the account ~6 s later (as if Start was pressed in Telegram).
 * Rule profiles: a group named `bad…` or an invalid regex in `nodes` is rejected with field errors.
 */
import type { IncomingMessage, ServerResponse } from 'node:http'

type Json = Record<string, unknown>

interface NodeOps {
  include: string[]
  exclude: string[]
  types: string[]
  regions: string[]
  rename: Array<{ match: string; replace: string }>
  prefix: string
  suffix: string
  emoji: string
  sort: string
  dedupe: boolean
  limit: number
}
interface StoredProfile {
  id: number
  slug: string
  name: string
  is_default: boolean
  updated_at: string
  data: Json
}

const emptyNodes = (): NodeOps => ({
  include: [],
  exclude: [],
  types: [],
  regions: [],
  rename: [],
  prefix: '',
  suffix: '',
  emoji: 'keep',
  sort: 'default',
  dedupe: false,
  limit: 0,
})

const iso = (ms = Date.now()) => new Date(ms).toISOString().replace(/\.\d{3}Z$/, 'Z')
const pick = (alphabet: string, n: number) =>
  Array.from({ length: n }, () => alphabet[Math.floor(Math.random() * alphabet.length)]).join('')
const BECH32 = 'qpzry9x8gf2tvdw0s3jn54khce6mua7l'
/** unknown → string for request fields (objects become ""). */
const str = (v: unknown): string => (typeof v === 'string' ? v : typeof v === 'number' ? String(v) : '')

function send(res: ServerResponse, status: number, body: unknown) {
  res.statusCode = status
  res.setHeader('Content-Type', 'application/json; charset=utf-8')
  res.setHeader('Cache-Control', 'no-store')
  res.end(JSON.stringify(body))
}
const ok = (res: ServerResponse, data: unknown) => send(res, 200, { data })
const fail = (res: ServerResponse, status: number, code: string, message: string, errors?: Json) =>
  send(res, status, { code, message, ...(errors ? { errors } : {}) })

async function readBody(req: IncomingMessage): Promise<Json> {
  const chunks: Buffer[] = []
  for await (const chunk of req) chunks.push(chunk as Buffer)
  const text = Buffer.concat(chunks).toString('utf8')
  if (!text) return {}
  try {
    return JSON.parse(text) as Json
  } catch {
    return {}
  }
}

const legacy = () => process.env.MOCK_FES_LEGACY === '1'
const TEMPLATE_IDS = ['none', 'minimal', 'standard', 'full']

// ---------------------------------------------------------------- state

let nextId = 2
let profiles: StoredProfile[] | null = null
let age: { enabled: boolean; public_key: string; private_key: string; created_at: string } | null = null
/** `GET|POST|PATCH|DELETE /user/subscription/age` view (null key fields without a key). */
const ageView = (owner = true) => ({
  enabled: age?.enabled ?? false,
  public_key: age?.public_key ?? null,
  private_key: owner ? (age?.private_key ?? null) : null,
  created_at: age?.created_at ?? null,
  formats: ['clash-meta'],
  optional: true,
  strict: process.env.MOCK_AGE_STRICT === '1',
})
const telegram = {
  bound: false,
  username: null as string | null,
  pendingSince: 0,
  bot: 'ggbond_demo_bot',
}

const CLIENTS = [
  ['clash-verge/v2.2.3', 'clash-meta'],
  ['mihomo/1.19.10', 'clash-meta'],
  ['Shadowrocket/2.2.65 CFNetwork/1568 Darwin/24.4.0', 'shadowrocket'],
  ['sing-box 1.12.4 (SFI)', 'singbox'],
  ['Stash/3.1.1 Clash/1.9.0', 'stash'],
] as const
const accessLog = Array.from({ length: 9 }, (_, i) => {
  const [client, format] = CLIENTS[i % CLIENTS.length]
  const host = [45, 45, 17, 88, 45, 130, 17, 201, 9][i]
  return {
    ip: `${i % 3 === 2 ? '198.51.100' : '203.0.113'}.${host}`,
    client,
    format,
    at: iso(Date.now() - i * 5.5 * 3_600_000 - 7 * 60_000),
    ip_bucket: `${i % 3 === 2 ? '198.51.100' : '203.0.113'}.${host}`,
    token_tag: i < 6 ? '3f9a1c07' : 'b81e44d2',
  }
})
const masked = (ip: string) => ip.replace(/\.\d+$/, '.*')
const telegramOn = () => process.env.MOCK_TELEGRAM_OFF !== '1'

function ensureProfiles(defaultData: () => Json): StoredProfile[] {
  profiles ??= [
    {
      id: 1,
      slug: 'default',
      name: '默认',
      is_default: true,
      updated_at: iso(),
      data: { ...defaultData(), nodes: emptyNodes() },
    },
  ]
  return profiles
}

const summary = ({ data: _data, ...p }: StoredProfile) => p

function validate(data: Json): Json | null {
  const errors: Record<string, string[]> = {}
  const groups = Array.isArray(data.groups) ? (data.groups as Json[]) : []
  groups.forEach((g, i) => {
    if (str(g.name).toLowerCase().startsWith('bad'))
      errors[`groups[${i}].name`] = ['is not allowed (mock rule)']
  })
  const nodes = (data.nodes ?? {}) as Partial<NodeOps>
  const check = (key: string, source: unknown) => {
    try {
      new RegExp(String(source))
      if (/\(\?<?[=!]/.test(String(source))) throw new Error('lookaround')
    } catch {
      errors[key] = ['invalid regular expression (RE2)']
    }
  }
  nodes.include?.forEach((p, i) => check(`nodes.include[${i}]`, p))
  nodes.exclude?.forEach((p, i) => check(`nodes.exclude[${i}]`, p))
  nodes.rename?.forEach((r, i) => check(`nodes.rename[${i}].match`, r.match))
  if (typeof nodes.limit === 'number' && (nodes.limit < 0 || !Number.isInteger(nodes.limit)))
    errors['nodes.limit'] = ['must be a non-negative integer']
  return Object.keys(errors).length ? errors : null
}

function newAge() {
  return {
    enabled: false,
    public_key: `age1${pick(BECH32, 58)}`,
    private_key: `AGE-SECRET-KEY-1${pick(BECH32.toUpperCase(), 58)}`,
    created_at: iso(),
  }
}

// ---------------------------------------------------------------- user side

export interface FeSUserContext {
  /** The stored default profile of the host mock (seed for the default FE-S profile). */
  defaultProfile: () => Json
  /** Plain subscription URL (no profile). */
  subscribeUrl: () => string
  /** Replaces the token; returns the new URL + token. */
  rotateToken: () => { subscribe_url: string; token: string }
}

/** Extra fields of `GET /user/subscription`. */
export function subscriptionExtras(ctx: FeSUserContext): Json {
  if (legacy()) return {}
  const base = ctx.subscribeUrl()
  return {
    profiles: ensureProfiles(ctx.defaultProfile).map((p) => ({
      slug: p.slug,
      name: p.name,
      is_default: p.is_default,
      subscribe_url: p.is_default ? base : `${base}${base.includes('?') ? '&' : '?'}profile=${p.slug}`,
    })),
    formats: [
      { id: 'clash-meta', name: 'Clash Meta / mihomo', supports_age: true },
      { id: 'stash', name: 'Stash', supports_age: false },
      { id: 'singbox', name: 'sing-box', supports_age: false },
      { id: 'v2ray', name: 'v2rayN / base64', supports_age: false },
    ],
  }
}

/** Extra fields of `GET /guest/config`. */
export const guestExtras = (): Json =>
  legacy() ? {} : { subscribe_age_optional: true, subscribe_age_strict: false }

/** Handles `METHOD /path` (relative to `/api/v1`) when it is an FE-S user route; `false` otherwise. */
export async function handleFeSUser(
  route: string,
  req: IncomingMessage,
  res: ServerResponse,
  ctx: FeSUserContext,
): Promise<boolean> {
  if (legacy()) return false
  const list = ensureProfiles(ctx.defaultProfile)
  const byDefault = () => list.find((p) => p.is_default)!

  const put = async (target: StoredProfile) => {
    if (process.env.MOCK_RULES_LOCKED === '1')
      return fail(res, 403, 'forbidden', 'custom rule profiles are disabled')
    const input = await readBody(req)
    // View fields are accepted back and ignored.
    const {
      id: _id,
      slug: _slug,
      name: _name,
      is_default: _def,
      customized: _c,
      editable: _e,
      effective: _f,
      ...data
    } = input
    const errors = validate(data)
    if (errors) return fail(res, 400, 'validation_failed', 'invalid rule profile', errors)
    // `PUT` without `nodes` keeps the stored node operations.
    const nodes =
      data.nodes === undefined ? (target.data.nodes as Json) : { ...emptyNodes(), ...(data.nodes as Json) }
    target.data = { ...target.data, ...data, nodes }
    target.updated_at = iso()
    return ok(res, view(target))
  }
  const view = (p: StoredProfile) => ({
    ...p.data,
    id: p.id,
    slug: p.slug,
    name: p.name,
    is_default: p.is_default,
  })

  const slugMatch = /^(GET|PUT|PATCH|DELETE|POST) \/user\/rule-profiles\/([^/]+)(\/default)?$/.exec(route)
  if (slugMatch) {
    const [, method, rawSlug, isDefault] = slugMatch
    const slug = decodeURIComponent(rawSlug)
    const target = list.find((p) => p.slug === slug)
    if (!target) return (fail(res, 404, 'not_found', 'profile not found'), true)
    if (method === 'POST' && isDefault) {
      for (const p of list) p.is_default = p === target
      return (ok(res, summary(target)), true)
    }
    if (method === 'GET' && !isDefault) return (ok(res, view(target)), true)
    if (method === 'PUT' && !isDefault) return (await put(target), true)
    if (method === 'PATCH' && !isDefault) {
      const input = await readBody(req)
      const name = str(input.name).trim()
      if (input.name !== undefined && !name)
        return (fail(res, 400, 'validation_failed', 'invalid input', { name: ['is required'] }), true)
      if (name) target.name = name
      target.updated_at = iso()
      return (ok(res, summary(target)), true)
    }
    if (method === 'DELETE' && !isDefault) {
      if (target.is_default)
        return (fail(res, 409, 'conflict', 'the default profile cannot be deleted'), true)
      profiles = list.filter((p) => p !== target)
      return (ok(res, true), true)
    }
    return false
  }

  switch (route) {
    case 'GET /user/rule-profile':
      ok(res, view(byDefault()))
      return true
    case 'PUT /user/rule-profile':
      await put(byDefault())
      return true
    case 'GET /user/rule-profiles':
      ok(res, list.map(summary))
      return true
    case 'POST /user/rule-profiles': {
      const input = await readBody(req)
      const name = str(input.name).trim()
      const slug = str(input.slug).trim() || `p${nextId}`
      if (list.length >= 10) return (fail(res, 409, 'conflict', 'at most 10 profiles'), true)
      if (!name)
        return (fail(res, 400, 'validation_failed', 'invalid input', { name: ['is required'] }), true)
      if (!/^[a-z0-9][a-z0-9_-]{0,31}$/.test(slug) || TEMPLATE_IDS.includes(slug))
        return (fail(res, 400, 'validation_failed', 'invalid input', { slug: ['is invalid'] }), true)
      if (list.some((p) => p.slug === slug))
        return (fail(res, 400, 'validation_failed', 'invalid input', { slug: ['is already taken'] }), true)
      const source = input.copy_from ? list.find((p) => p.slug === input.copy_from) : undefined
      if (input.copy_from && !source)
        return (
          fail(res, 400, 'validation_failed', 'invalid input', { copy_from: ['unknown profile'] }),
          true
        )
      const created: StoredProfile = {
        id: nextId++,
        slug,
        name,
        is_default: false,
        updated_at: iso(),
        data: structuredClone(source?.data ?? { ...ctx.defaultProfile(), nodes: emptyNodes() }),
      }
      list.push(created)
      ok(res, summary(created))
      return true
    }
    case 'GET /user/subscription/age':
      ok(res, ageView())
      return true
    case 'POST /user/subscription/age': {
      const input = await readBody(req)
      if (age && !input.regenerate) return (fail(res, 409, 'conflict', 'key exists'), true)
      const enabled = typeof input.enabled === 'boolean' ? input.enabled : (age?.enabled ?? false)
      age = { ...newAge(), enabled }
      ok(res, ageView())
      return true
    }
    case 'PATCH /user/subscription/age': {
      if (!age) return (fail(res, 404, 'not_found', 'no age key'), true)
      const input = await readBody(req)
      age.enabled = Boolean(input.enabled)
      ok(res, ageView())
      return true
    }
    case 'DELETE /user/subscription/age':
      age = null
      ok(res, ageView())
      return true
    case 'GET /user/subscription/access-log':
      ok(
        res,
        accessLog.map(({ ip, client, format, at }) => ({ ip: masked(ip), client, format, at })),
      )
      return true
    case 'POST /user/subscription/rotate-token':
      ok(res, ctx.rotateToken())
      return true
    case 'GET /user/telegram':
      if (!telegram.bound && telegram.pendingSince && Date.now() - telegram.pendingSince > 6000) {
        telegram.bound = true
        telegram.username = 'demo_user'
        telegram.pendingSince = 0
      }
      ok(res, {
        enabled: telegramOn(),
        bound: telegram.bound,
        ...(telegram.bound ? { username: telegram.username } : {}),
        bot_username: telegram.bot,
        discuss_link: 'https://t.me/example_group',
      })
      return true
    case 'POST /user/telegram/bind-code': {
      if (!telegramOn()) return (fail(res, 403, 'forbidden', 'telegram is not configured'), true)
      if (process.env.MOCK_TELEGRAM_DOWN === '1')
        return (fail(res, 502, 'telegram_unavailable', 'telegram is unreachable'), true)
      if (telegram.bound) return (fail(res, 409, 'conflict', 'already bound'), true)
      const code = pick('ABCDEFGHJKLMNPQRSTUVWXYZ23456789', 8)
      telegram.pendingSince = Date.now()
      ok(res, { code, bot_username: telegram.bot, deep_link: `https://t.me/${telegram.bot}?start=${code}` })
      return true
    }
    case 'DELETE /user/telegram':
      telegram.bound = false
      telegram.username = null
      ok(res, true)
      return true
  }
  return false
}

// ---------------------------------------------------------------- admin side

/** `GET /admin/users/:id/subscription/access-log`, `POST /admin/users/:id/subscription/age/reset`. */
export function handleFeSAdmin(method: string, rest: readonly string[], res: ServerResponse): boolean {
  const path = rest.join('/')
  if (method === 'GET' && path === 'access-log') {
    ok(res, accessLog)
    return true
  }
  if (method === 'GET' && path === 'age') {
    ok(res, ageView(false))
    return true
  }
  if (method === 'POST' && path === 'age/reset') {
    if (!age) {
      fail(res, 404, 'not_found', 'the user has no age key')
      return true
    }
    age = { ...newAge(), enabled: age.enabled }
    ok(res, ageView(false))
    return true
  }
  return false
}
