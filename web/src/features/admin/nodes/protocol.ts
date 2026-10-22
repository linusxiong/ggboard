/**
 * `nodes.protocol_settings` model for the node form (docs/DATA_MODEL.md, docs/NODE_PROTOCOLS.md §2/§8).
 *
 * The form edits the JSON object directly (paths like `tls_settings.private_key`), so server
 * validation errors keyed `protocol_settings.<path>` land on the matching input. Every edit goes
 * through `updateSetting()`, which applies the dependent resets (`reconcile`) — e.g. a new
 * `network` gets fresh `network_settings`, leaving reality drops the reality keys. `finalizeSettings()`
 * normalises the object before it is sent, and `validateSettings()` mirrors the backend rules.
 */
import type { NetworkSettings, NodeType, TransportNetwork } from '@/lib/api/types'

export type Settings = Record<string, unknown>

// ---------------------------------------------------------------- option lists

export const NETWORKS: Partial<Record<NodeType, readonly TransportNetwork[]>> = {
  vmess: ['tcp', 'ws', 'grpc', 'http', 'h2', 'httpupgrade', 'xhttp'],
  vless: ['tcp', 'ws', 'grpc', 'http', 'h2', 'httpupgrade', 'xhttp'],
  trojan: ['tcp', 'ws', 'grpc'],
}

export const SS_CIPHERS = [
  'aes-128-gcm',
  'aes-256-gcm',
  'chacha20-ietf-poly1305',
  '2022-blake3-aes-128-gcm',
  '2022-blake3-aes-256-gcm',
  '2022-blake3-chacha20-poly1305',
] as const
export const isSs2022 = (cipher: unknown) => typeof cipher === 'string' && cipher.startsWith('2022-')
/** SS2022 key length in bytes (16 for aes-128, 32 otherwise). */
export const ss2022KeyBytes = (cipher: unknown) => (cipher === '2022-blake3-aes-128-gcm' ? 16 : 32)

export const SSR_CIPHERS = [
  'none',
  'rc4-md5',
  'aes-128-cfb',
  'aes-192-cfb',
  'aes-256-cfb',
  'aes-128-ctr',
  'aes-192-ctr',
  'aes-256-ctr',
  'camellia-128-cfb',
  'camellia-192-cfb',
  'camellia-256-cfb',
  'bf-cfb',
  'salsa20',
  'chacha20',
  'chacha20-ietf',
] as const
export const SSR_PROTOCOLS = [
  'origin',
  'auth_sha1_v4',
  'auth_aes128_md5',
  'auth_aes128_sha1',
  'auth_chain_a',
  'auth_chain_b',
] as const
export const SSR_OBFS = ['plain', 'http_simple', 'http_post', 'random_head', 'tls1.2_ticket_auth'] as const
export const FINGERPRINTS = [
  'chrome',
  'firefox',
  'safari',
  'ios',
  'android',
  'edge',
  '360',
  'qq',
  'random',
  'randomized',
] as const
export const XHTTP_MODES = ['auto', 'packet-up', 'stream-up', 'stream-one'] as const
export const TUIC_CONGESTION = ['bbr', 'cubic', 'new_reno'] as const
export const TUIC_UDP_RELAY = ['native', 'quic'] as const
export const MIERU_MULTIPLEXING = ['MULTIPLEXING_LOW', 'MULTIPLEXING_MIDDLE', 'MULTIPLEXING_HIGH'] as const

/** Upstream anytls padding scheme (used by the backend when `padding_scheme` is empty). */
export const ANYTLS_DEFAULT_PADDING = [
  'stop=8',
  '0=30-30',
  '1=100-400',
  '2=400-500,c,500-1000,c,500-1000,c,500-1000,c,500-1000',
  '3=9-9,500-1000',
  '4=500-1000',
  '5=500-1000',
  '6=500-1000',
  '7=500-1000',
]

const REALITY_KEYS = [
  'public_key',
  'private_key',
  'short_id',
  'dest',
  'server_port',
  'fingerprint',
  'server_names',
] as const

// ---------------------------------------------------------------- path helpers

export function getPath(obj: unknown, path: string): unknown {
  let node: unknown = obj
  for (const part of path.split('.')) {
    if (node === null || typeof node !== 'object' || Array.isArray(node)) return undefined
    node = (node as Settings)[part]
  }
  return node
}

const isPlainObject = (v: unknown): v is Settings => v !== null && typeof v === 'object' && !Array.isArray(v)

/** Immutable set; `undefined` deletes the key. Intermediate objects are created as needed. */
export function setPath<T extends Settings>(obj: T, path: string, value: unknown): T {
  const [head, ...rest] = path.split('.')
  if (head === undefined) return obj
  const copy: Settings = { ...obj }
  if (rest.length === 0) {
    if (value === undefined) delete copy[head]
    else copy[head] = value
    return copy as T
  }
  const current = copy[head]
  const child = isPlainObject(current) ? current : {}
  copy[head] = setPath(child, rest.join('.'), value)
  return copy as T
}

// ---------------------------------------------------------------- defaults

export function defaultNetworkSettings(
  network: TransportNetwork,
  type?: NodeType,
): NetworkSettings | undefined {
  switch (network) {
    case 'tcp':
      return type === 'trojan' ? undefined : { header: { type: 'none' } }
    case 'ws':
      return { path: '/', headers: { Host: '' } }
    case 'grpc':
      return { serviceName: '' }
    case 'http':
    case 'h2':
      return { path: '/', host: [] }
    case 'httpupgrade':
      return { path: '/', host: '' }
    case 'xhttp':
      return { path: '/', host: '', mode: 'auto', extra: {} }
  }
}

export function defaultSettings(type: NodeType): Settings {
  switch (type) {
    case 'vmess':
      return {
        network: 'tcp',
        network_settings: defaultNetworkSettings('tcp'),
        tls: 0,
        tls_settings: { server_name: '', allow_insecure: false },
      }
    case 'vless':
      return {
        network: 'tcp',
        network_settings: defaultNetworkSettings('tcp'),
        tls: 0,
        flow: '',
        encryption: 'none',
        decryption: '',
        tls_settings: { server_name: '', allow_insecure: false },
      }
    case 'shadowsocks':
      return { cipher: 'aes-128-gcm', obfs: 'plain' }
    case 'shadowsocksr':
      return {
        cipher: 'chacha20-ietf',
        password: '',
        protocol: 'origin',
        protocol_param: '',
        obfs: 'plain',
        obfs_param: '',
        single_port_type: 'protocol',
      }
    case 'trojan':
      return { network: 'tcp', server_name: '', allow_insecure: false }
    case 'hysteria':
      return {
        version: 2,
        up_mbps: 0,
        down_mbps: 0,
        ignore_client_bandwidth: false,
        obfs: 'plain',
        obfs_password: '',
        server_name: '',
        allow_insecure: false,
        alpn: 'h3',
        hop_interval: 30,
      }
    case 'anytls':
      return { tls: 1, server_name: '', allow_insecure: false, padding_scheme: [] }
    case 'mieru':
      return { transport: 'TCP', multiplexing: 'MULTIPLEXING_LOW' }
    case 'tuic':
      return {
        congestion_control: 'bbr',
        zero_rtt_handshake: false,
        udp_relay_mode: 'native',
        server_name: '',
        allow_insecure: false,
        alpn: 'h3',
      }
  }
}

// ---------------------------------------------------------------- edits

function stripReality(ts: unknown): Settings {
  const copy: Settings = isPlainObject(ts) ? { ...ts } : {}
  for (const key of REALITY_KEYS) delete copy[key]
  return copy
}

/** Applies the dependent resets after `prev → next` (pure). */
export function reconcile(type: NodeType, prev: Settings, next: Settings): Settings {
  let out = next
  if (NETWORKS[type] && next.network !== prev.network && typeof next.network === 'string') {
    out = setPath(out, 'network_settings', defaultNetworkSettings(next.network as TransportNetwork, type))
  }
  if ((type === 'vless' || type === 'anytls') && next.tls !== prev.tls) {
    if (next.tls === 2) {
      const ts = isPlainObject(out.tls_settings) ? out.tls_settings : {}
      out = setPath(out, 'tls_settings', {
        ...ts,
        ...(type === 'vless' ? { fingerprint: ts.fingerprint ?? 'chrome' } : {}),
        public_key: ts.public_key ?? '',
        private_key: ts.private_key ?? '',
        short_id: ts.short_id ?? '',
        dest: ts.dest ?? '',
      })
    } else {
      out = setPath(out, 'tls_settings', stripReality(out.tls_settings))
    }
  }
  if (type === 'shadowsocks') {
    if (next.cipher !== prev.cipher && !isSs2022(next.cipher)) out = setPath(out, 'server_key', undefined)
    if (next.obfs !== prev.obfs) {
      out = setPath(
        out,
        'obfs_settings',
        next.obfs === 'plain'
          ? undefined
          : { path: '/', host: '', ...(isPlainObject(out.obfs_settings) ? out.obfs_settings : {}) },
      )
    }
  }
  if (type === 'hysteria' && next.version !== prev.version) {
    if (next.version === 1) out = setPath(out, 'ignore_client_bandwidth', undefined)
    else if (out.ignore_client_bandwidth === undefined) out = setPath(out, 'ignore_client_bandwidth', false)
  }
  return out
}

/** One form edit: set `path` to `value`, then reconcile dependent keys. */
export function updateSetting(type: NodeType, settings: Settings, path: string, value: unknown): Settings {
  return reconcile(type, settings, setPath(settings, path, value))
}

// ---------------------------------------------------------------- finalize

/** Paths where an empty string is a meaningful value and must be kept. */
const KEEP_EMPTY = new Set(['flow', 'tls_settings.short_id'])
const INT_KEYS = new Set(['tls', 'version', 'up_mbps', 'down_mbps', 'hop_interval'])

function prune(value: unknown, path: string): unknown {
  if (Array.isArray(value)) {
    const items = value.filter((v) => !(typeof v === 'string' && v.trim() === ''))
    return items.length ? items : undefined
  }
  if (isPlainObject(value)) {
    const out: Settings = {}
    for (const [key, child] of Object.entries(value)) {
      const childPath = path ? `${path}.${key}` : key
      const pruned = prune(child, childPath)
      if (pruned !== undefined) out[key] = pruned
    }
    // keep `{}` only at the root (and for xhttp `extra`, which clients read verbatim)
    return Object.keys(out).length || path === '' || path.endsWith('.extra') ? out : undefined
  }
  if (typeof value === 'string') {
    const trimmed = value.trim()
    return trimmed === '' && !KEEP_EMPTY.has(path) ? undefined : trimmed
  }
  if (value === null || value === undefined) return undefined
  if (typeof value === 'number' && Number.isNaN(value)) return undefined
  if (typeof value === 'number' && INT_KEYS.has(path)) return Math.trunc(value)
  return value
}

/**
 * Normalises the edited object for the API: trims strings, drops empty optional values and
 * objects, truncates integer fields, and removes keys that do not apply to the current mode
 * (reality keys without reality, obfs settings for plain, server_key for non-2022 ciphers…).
 * Unknown keys (added in the raw JSON editor) are kept.
 */
export function finalizeSettings(type: NodeType, settings: Settings): Settings {
  let out = settings
  if ((type === 'vless' || type === 'anytls') && out.tls !== 2 && isPlainObject(out.tls_settings)) {
    out = setPath(out, 'tls_settings', stripReality(out.tls_settings))
  }
  if (type === 'shadowsocks') {
    if (!isSs2022(out.cipher)) out = setPath(out, 'server_key', undefined)
    if ((out.obfs ?? 'plain') === 'plain') out = setPath(out, 'obfs_settings', undefined)
  }
  if (type === 'hysteria') {
    if ((out.obfs ?? 'plain') === 'plain') out = setPath(out, 'obfs_password', undefined)
    if (out.version === 1) out = setPath(out, 'ignore_client_bandwidth', undefined)
  }
  if (type === 'vless' && out.flow === undefined) out = setPath(out, 'flow', '')
  return (prune(out, '') as Settings | undefined) ?? {}
}

// ---------------------------------------------------------------- validation

export type IssueKey =
  | 'required'
  | 'hex'
  | 'shortIdLength'
  | 'hostPort'
  | 'base64Key'
  | 'path'
  | 'integer'
  | 'object'
  | 'obfsPassword'

export interface Issue {
  key: IssueKey
  params?: Record<string, string | number>
}

const BASE64 = /^[A-Za-z0-9+/]+={0,2}$/
const BASE64URL = /^[A-Za-z0-9_-]+={0,2}$/

function base64Bytes(value: string): number | null {
  if (!BASE64.test(value) && !BASE64URL.test(value)) return null
  const clean = value.replace(/=+$/, '')
  return Math.floor((clean.length * 3) / 4)
}

/** Client-side checks keyed by path inside `protocol_settings` (no prefix). */
export function validateSettings(type: NodeType, s: Settings): Record<string, Issue> {
  const issues: Record<string, Issue> = {}
  const str = (path: string) => {
    const v = getPath(s, path)
    return typeof v === 'string' ? v.trim() : ''
  }
  const requireStr = (path: string) => {
    if (!str(path)) issues[path] = { key: 'required' }
  }
  const nonNegativeInt = (path: string) => {
    const v = getPath(s, path)
    if (v !== undefined && (typeof v !== 'number' || !Number.isInteger(v) || v < 0))
      issues[path] = { key: 'integer' }
  }
  const reality = () => {
    requireStr('tls_settings.private_key')
    requireStr('tls_settings.public_key')
    const sid = str('tls_settings.short_id')
    if (sid && !/^[0-9a-fA-F]+$/.test(sid)) issues['tls_settings.short_id'] = { key: 'hex' }
    else if (sid && (sid.length > 16 || sid.length % 2 !== 0))
      issues['tls_settings.short_id'] = { key: 'shortIdLength' }
    const dest = str('tls_settings.dest')
    if (dest && !/^(\[[0-9a-fA-F:.]+\]|[^\s:[\]]+)(:\d{1,5})?$/.test(dest))
      issues['tls_settings.dest'] = { key: 'hostPort' }
  }

  if (NETWORKS[type]) {
    const network = s.network as TransportNetwork | undefined
    const path = str('network_settings.path')
    if (path && !path.startsWith('/') && network !== 'grpc') issues['network_settings.path'] = { key: 'path' }
    const extra = getPath(s, 'network_settings.extra')
    if (network === 'xhttp' && extra !== undefined && !isPlainObject(extra))
      issues['network_settings.extra'] = { key: 'object' }
  }

  switch (type) {
    case 'vless':
      if (s.tls === 2) {
        requireStr('tls_settings.server_name')
        reality()
      }
      break
    case 'anytls':
      if (s.tls === 2) {
        const names = getPath(s, 'tls_settings.server_names')
        if (!str('server_name') && !(Array.isArray(names) && names.length > 0))
          issues['server_name'] = { key: 'required' }
        reality()
      }
      break
    case 'shadowsocks': {
      if (!str('cipher')) issues.cipher = { key: 'required' }
      const key = str('server_key')
      if (key && isSs2022(s.cipher)) {
        const bytes = ss2022KeyBytes(s.cipher)
        if (base64Bytes(key) !== bytes) issues.server_key = { key: 'base64Key', params: { bytes } }
      }
      break
    }
    case 'shadowsocksr':
      requireStr('cipher')
      requireStr('protocol')
      requireStr('obfs')
      break
    case 'hysteria':
      nonNegativeInt('up_mbps')
      nonNegativeInt('down_mbps')
      nonNegativeInt('hop_interval')
      if ((s.obfs ?? 'plain') !== 'plain' && !str('obfs_password'))
        issues.obfs_password = { key: 'obfsPassword' }
      break
    case 'mieru':
      requireStr('transport')
      break
    default:
      break
  }
  return issues
}

// ---------------------------------------------------------------- compatibility notes

export type CompatNote =
  | 'v2bxHttp'
  | 'sogaHttpUpgrade'
  | 'ssChacha2022'
  | 'sogaHysteria1'
  | 'anytlsRealitySogaOnly'
  | 'mieruUdp'
  | 'tuicV2bxOnly'
  | 'mieruSogaOnly'
  | 'visionNeedsTls'

/** Non-blocking warnings about backend support (NODE_PROTOCOLS §2, §3, §8). */
export function compatNotes(type: NodeType, s: Settings): CompatNote[] {
  const notes: CompatNote[] = []
  if ((type === 'vmess' || type === 'vless') && (s.network === 'http' || s.network === 'h2'))
    notes.push('v2bxHttp')
  if (s.network === 'httpupgrade') notes.push('sogaHttpUpgrade')
  if (type === 'shadowsocks' && s.cipher === '2022-blake3-chacha20-poly1305') notes.push('ssChacha2022')
  if (type === 'hysteria' && s.version === 1) notes.push('sogaHysteria1')
  if (type === 'anytls' && s.tls === 2) notes.push('anytlsRealitySogaOnly')
  if (type === 'mieru') notes.push(s.transport === 'UDP' ? 'mieruUdp' : 'mieruSogaOnly')
  if (type === 'tuic') notes.push('tuicV2bxOnly')
  if (type === 'vless' && s.flow === 'xtls-rprx-vision' && s.tls === 0) notes.push('visionNeedsTls')
  return notes
}

// ---------------------------------------------------------------- key helpers

const toBase64Url = (bytes: Uint8Array) =>
  btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '')

/**
 * X25519 key pair for REALITY (Xray `x25519` format: unpadded base64url). Uses WebCrypto;
 * resolves `null` where the browser lacks X25519.
 */
export async function generateRealityKeys(): Promise<{ privateKey: string; publicKey: string } | null> {
  try {
    const pair = await crypto.subtle.generateKey({ name: 'X25519' }, true, ['deriveBits'])
    const pub = new Uint8Array(await crypto.subtle.exportKey('raw', pair.publicKey))
    const pkcs8 = new Uint8Array(await crypto.subtle.exportKey('pkcs8', pair.privateKey))
    return { privateKey: toBase64Url(pkcs8.slice(-32)), publicKey: toBase64Url(pub) }
  } catch {
    return null
  }
}

export function randomHex(bytes: number): string {
  const buf = crypto.getRandomValues(new Uint8Array(bytes))
  return Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('')
}

/** Random SS2022 PSK (standard base64 of `bytes` random bytes). */
export function randomBase64Key(bytes: number): string {
  return btoa(String.fromCharCode(...crypto.getRandomValues(new Uint8Array(bytes))))
}
