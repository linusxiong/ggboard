/**
 * Settings metadata: `GET /admin/settings` returns `{"<group>": {"<key>": value}, groups?: [...]}`
 * where `groups` is `settings.Groups()` (internal/settings/defs.go: key, type, default, secret,
 * options, min, max). When `groups` is absent (older backend), the metadata is inferred from the
 * values so the page still renders every key with a sensible input.
 */
import type { AdminSettingsResponse, SettingDef, SettingGroupMeta } from '@/lib/api/types'

/** Display order for groups the backend didn't order (or that are inferred). */
export const GROUP_ORDER = [
  'site',
  'register',
  'subscribe',
  'order',
  'invite',
  'server',
  'email',
  'frontend',
  'app',
  'security',
  'telegram',
] as const

const KNOWN_SECRETS = new Set([
  'captcha_secret',
  'email_password',
  'server_token',
  'telegram_bot_token',
  'telegram_webhook_secret',
])
const KNOWN_OPTIONS: Record<string, string[]> = {
  default_locale: ['zh-CN', 'en'],
  captcha_provider: ['none', 'turnstile', 'recaptcha', 'hcaptcha'],
  email_driver: ['log', 'smtp'],
  email_encryption: ['ssl', 'tls', 'none'],
}

export function inferType(value: unknown): SettingDef['type'] {
  if (typeof value === 'boolean') return 'bool'
  if (typeof value === 'number') return 'int'
  if (Array.isArray(value)) return 'strings'
  if (value !== null && typeof value === 'object') return 'json'
  return 'string'
}

/** Group values (`{key: value}`) of the response, skipping the `groups` metadata entry. */
function valueGroups(resp: AdminSettingsResponse): Array<[string, Record<string, unknown>]> {
  return Object.entries(resp).filter(
    (e): e is [string, Record<string, unknown>] =>
      e[0] !== 'groups' && e[1] !== null && typeof e[1] === 'object' && !Array.isArray(e[1]),
  )
}

/** Flat `{key: value}` map of every setting in the response (defaults filled from metadata). */
export function flattenValues(
  resp: AdminSettingsResponse,
  groups: readonly SettingGroupMeta[],
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const g of groups) for (const d of g.keys) out[d.key] = d.default
  for (const [, values] of valueGroups(resp)) Object.assign(out, values)
  return out
}

const rank = (name: string) => {
  const i = (GROUP_ORDER as readonly string[]).indexOf(name)
  return i === -1 ? GROUP_ORDER.length : i
}

/** Normalised, ordered group metadata (visible keys only). */
export function buildGroups(resp: AdminSettingsResponse): SettingGroupMeta[] {
  const values = new Map(valueGroups(resp))
  if (Array.isArray(resp.groups) && resp.groups.length > 0) {
    return resp.groups
      .map((g) => ({
        name: g.name,
        keys: (g.keys ?? [])
          .filter((d) => !d.hidden)
          .map((d) => ({
            ...d,
            type: d.type === 'int64' ? 'int' : d.type,
            secret: d.secret ?? KNOWN_SECRETS.has(d.key),
            options: d.options?.length ? d.options : KNOWN_OPTIONS[d.key],
          })),
      }))
      .filter((g) => g.keys.length > 0 || values.has(g.name))
  }
  return [...values.entries()]
    .sort(([a], [b]) => rank(a) - rank(b))
    .map(([name, group]) => ({
      name,
      keys: Object.entries(group).map(([key, value]) => ({
        key,
        group: name,
        type: inferType(value),
        secret: KNOWN_SECRETS.has(key),
        options: KNOWN_OPTIONS[key],
      })),
    }))
}

/** Structural equality for JSON-like values (order-sensitive for arrays). */
export function sameValue(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (typeof a !== typeof b || a === null || b === null) return false
  if (Array.isArray(a))
    return Array.isArray(b) && a.length === b.length && a.every((v, i) => sameValue(v, b[i]))
  if (typeof a === 'object') {
    const ka = Object.keys(a)
    const kb = Object.keys(b as object)
    return (
      ka.length === kb.length &&
      ka.every((k) => sameValue((a as Record<string, unknown>)[k], (b as Record<string, unknown>)[k]))
    )
  }
  return false
}

/** JSON settings are edited as text; a non-string draft is the parsed value itself. */
const jsonText = (v: unknown) => (typeof v === 'string' ? v : JSON.stringify(v ?? null))

/** Draft value of a JSON setting is its text; anything else is the typed value. */
export type Edits = Record<string, unknown>

export interface DirtyResult {
  /** Keys whose draft differs from the saved value → body of `PUT /admin/settings`. */
  patch: Record<string, unknown>
  /** Client-side problems (JSON parse errors, missing numbers, out of range). */
  errors: Record<string, string>
}

export interface ValidationMessages {
  json: string
  number: string
  min: (min: number) => string
  max: (max: number) => string
}

/** Builds the PUT body for one group from the edits, validating typed values. */
export function dirtyPatch(
  group: SettingGroupMeta,
  initial: Record<string, unknown>,
  edits: Edits,
  msg: ValidationMessages,
): DirtyResult {
  const patch: Record<string, unknown> = {}
  const errors: Record<string, string> = {}
  for (const def of group.keys) {
    if (!(def.key in edits)) continue
    let value = edits[def.key]
    if (def.type === 'json') {
      try {
        value = JSON.parse(jsonText(value))
      } catch {
        errors[def.key] = msg.json
        continue
      }
    }
    if (def.type === 'int') {
      if (typeof value !== 'number' || !Number.isFinite(value)) {
        errors[def.key] = msg.number
        continue
      }
      if (def.min !== undefined && value < def.min) {
        errors[def.key] = msg.min(def.min)
        continue
      }
      if (def.max !== undefined && value > def.max) {
        errors[def.key] = msg.max(def.max)
        continue
      }
    }
    if (!sameValue(value, initial[def.key])) patch[def.key] = value
  }
  return { patch, errors }
}

/** Whether a group has unsaved edits (cheap check used for tab badges). */
export function isGroupDirty(
  group: SettingGroupMeta,
  initial: Record<string, unknown>,
  edits: Edits,
): boolean {
  return group.keys.some((d) => {
    if (!(d.key in edits)) return false
    if (d.type === 'json') {
      try {
        return !sameValue(JSON.parse(jsonText(edits[d.key])), initial[d.key])
      } catch {
        return true
      }
    }
    return !sameValue(edits[d.key], initial[d.key])
  })
}

/** Random alphanumeric token (crypto), used to regenerate `server_token`. */
export function randomToken(length = 32): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'
  const out: string[] = []
  const buf = new Uint8Array(length * 2)
  while (out.length < length) {
    crypto.getRandomValues(buf)
    for (const b of buf) {
      // Rejection sampling keeps the distribution uniform (248 = 4 × 62).
      if (b < 248 && out.length < length) out.push(alphabet[b % 62] as string)
    }
  }
  return out.join('')
}
