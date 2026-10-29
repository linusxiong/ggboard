import { describe, expect, it } from 'vitest'
import type { Node, NodeType } from '@/lib/api/types'
import { randomToken } from '../settings/settingsMeta'
import { integrationSnippets, sogaServerType, v2bxNodeType, xrayrNodeType } from './integration'
import {
  NO_PARENT,
  NODE_TOKEN_LENGTH,
  nodeToValues,
  portError,
  splitNodeErrors,
  tokenError,
  tokenOwner,
  valuesToNodeInput,
} from './nodeForm'
import { defaultSettings, finalizeSettings, type Settings, updateSetting, validateSettings } from './protocol'

type Edit = [path: string, value: unknown]

/** Applies form edits exactly like the UI (updateSetting = set + dependent resets), then finalizes. */
function build(type: NodeType, edits: Edit[], start: Settings = defaultSettings(type)): Settings {
  return finalizeSettings(
    type,
    edits.reduce((ps, [path, value]) => updateSetting(type, ps, path, value), start),
  )
}

const KEY16 = btoa('0123456789abcdef')

describe('protocol_settings produced by the node form', () => {
  it.each<[string, NodeType, Edit[], Settings]>([
    [
      'vmess ws + tls',
      'vmess',
      [
        ['network', 'ws'],
        ['network_settings.path', '/ws'],
        ['network_settings.headers.Host', 'v.example.com'],
        ['tls', 1],
        ['tls_settings.server_name', 'v.example.com'],
      ],
      {
        network: 'ws',
        network_settings: { path: '/ws', headers: { Host: 'v.example.com' } },
        tls: 1,
        tls_settings: { server_name: 'v.example.com', allow_insecure: false },
      },
    ],
    [
      'vless reality + vision',
      'vless',
      [
        ['tls', 2],
        ['flow', 'xtls-rprx-vision'],
        ['tls_settings.server_name', 'www.example.org'],
        ['tls_settings.dest', 'www.example.org:443'],
        ['tls_settings.private_key', 'PRIVATE_KEY_BASE64URL'],
        ['tls_settings.public_key', 'PUBLIC_KEY_BASE64URL'],
        ['tls_settings.short_id', '0123abcd'],
      ],
      {
        network: 'tcp',
        network_settings: { header: { type: 'none' } },
        tls: 2,
        flow: 'xtls-rprx-vision',
        encryption: 'none',
        tls_settings: {
          server_name: 'www.example.org',
          allow_insecure: false,
          fingerprint: 'chrome',
          dest: 'www.example.org:443',
          private_key: 'PRIVATE_KEY_BASE64URL',
          public_key: 'PUBLIC_KEY_BASE64URL',
          short_id: '0123abcd',
        },
      },
    ],
    [
      'vless leaving reality drops the reality keys',
      'vless',
      [
        ['tls', 2],
        ['tls_settings.server_name', 'www.example.org'],
        ['tls_settings.private_key', 'K'],
        ['tls_settings.short_id', 'ab'],
        ['tls', 1],
      ],
      {
        network: 'tcp',
        network_settings: { header: { type: 'none' } },
        tls: 1,
        flow: '',
        encryption: 'none',
        tls_settings: { server_name: 'www.example.org', allow_insecure: false },
      },
    ],
    [
      'vless xhttp + VLESS encryption',
      'vless',
      [
        ['network', 'xhttp'],
        ['network_settings.path', '/xh'],
        ['network_settings.host', 'vl.example.com'],
        ['decryption', 'mlkem768x25519plus.native.600s.PRIVATE_KEY_BASE64URL'],
      ],
      {
        network: 'xhttp',
        network_settings: { path: '/xh', host: 'vl.example.com', mode: 'auto', extra: {} },
        tls: 0,
        flow: '',
        encryption: 'none',
        decryption: 'mlkem768x25519plus.native.600s.PRIVATE_KEY_BASE64URL',
        tls_settings: { allow_insecure: false },
      },
    ],
    [
      'shadowsocks 2022 + http obfs',
      'shadowsocks',
      [
        ['cipher', '2022-blake3-aes-128-gcm'],
        ['server_key', KEY16],
        ['obfs', 'http'],
        ['obfs_settings.host', 'www.example.net'],
      ],
      {
        cipher: '2022-blake3-aes-128-gcm',
        server_key: KEY16,
        obfs: 'http',
        obfs_settings: { path: '/', host: 'www.example.net' },
      },
    ],
    [
      'shadowsocks back to AEAD drops server_key',
      'shadowsocks',
      [
        ['cipher', '2022-blake3-aes-256-gcm'],
        ['server_key', 'KEY'],
        ['cipher', 'aes-128-gcm'],
      ],
      { cipher: 'aes-128-gcm', obfs: 'plain' },
    ],
    [
      'shadowsocksr single port',
      'shadowsocksr',
      [
        ['protocol', 'auth_chain_a'],
        ['password', 'node-pass'],
        ['obfs', 'http_simple'],
        ['obfs_param', 'example.com'],
        ['single_port_type', 'obfs'],
      ],
      {
        cipher: 'chacha20-ietf',
        password: 'node-pass',
        protocol: 'auth_chain_a',
        obfs: 'http_simple',
        obfs_param: 'example.com',
        single_port_type: 'obfs',
      },
    ],
    [
      'trojan grpc',
      'trojan',
      [
        ['network', 'grpc'],
        ['network_settings.serviceName', 'trojan-grpc'],
        ['server_name', 't.example.com'],
      ],
      {
        network: 'grpc',
        network_settings: { serviceName: 'trojan-grpc' },
        server_name: 't.example.com',
        allow_insecure: false,
      },
    ],
    [
      'hysteria 2 salamander',
      'hysteria',
      [
        ['obfs', 'salamander'],
        ['obfs_password', 'obfs-pass-123'],
        ['up_mbps', 100.7],
        ['server_name', 'hy.example.com'],
      ],
      {
        version: 2,
        up_mbps: 100,
        down_mbps: 0,
        ignore_client_bandwidth: false,
        obfs: 'salamander',
        obfs_password: 'obfs-pass-123',
        server_name: 'hy.example.com',
        allow_insecure: false,
        alpn: 'h3',
        hop_interval: 30,
      },
    ],
    [
      'hysteria 1 without obfs',
      'hysteria',
      [
        ['version', 1],
        ['obfs_password', 'unused'],
      ],
      {
        version: 1,
        up_mbps: 0,
        down_mbps: 0,
        obfs: 'plain',
        allow_insecure: false,
        alpn: 'h3',
        hop_interval: 30,
      },
    ],
    [
      'anytls reality + padding',
      'anytls',
      [
        ['tls', 2],
        ['server_name', 'a.example.com'],
        ['tls_settings.server_names', ['www.example.org']],
        ['tls_settings.private_key', 'PRIV'],
        ['tls_settings.public_key', 'PUB'],
        ['padding_scheme', ['stop=8', '0=30-30']],
      ],
      {
        tls: 2,
        server_name: 'a.example.com',
        allow_insecure: false,
        padding_scheme: ['stop=8', '0=30-30'],
        tls_settings: {
          server_names: ['www.example.org'],
          private_key: 'PRIV',
          public_key: 'PUB',
          short_id: '',
        },
      },
    ],
    [
      'mieru',
      'mieru',
      [
        ['transport', 'UDP'],
        ['multiplexing', 'MULTIPLEXING_HIGH'],
      ],
      { transport: 'UDP', multiplexing: 'MULTIPLEXING_HIGH' },
    ],
    [
      'tuic',
      'tuic',
      [
        ['congestion_control', 'cubic'],
        ['zero_rtt_handshake', true],
        ['server_name', 'tu.example.com'],
        ['alpn', ' h3 '],
      ],
      {
        congestion_control: 'cubic',
        zero_rtt_handshake: true,
        udp_relay_mode: 'native',
        server_name: 'tu.example.com',
        allow_insecure: false,
        alpn: 'h3',
      },
    ],
  ])('%s', (_name, type, edits, expected) => {
    expect(build(type, edits)).toEqual(expected)
  })

  it('keeps keys added in the raw JSON editor', () => {
    expect(finalizeSettings('vmess', { ...defaultSettings('vmess'), custom: { a: 1 } })).toMatchObject({
      custom: { a: 1 },
    })
  })
})

describe('protocol settings validation', () => {
  it.each<[string, NodeType, Settings, Record<string, string>]>([
    [
      'vless reality needs keys and SNI',
      'vless',
      { tls: 2, tls_settings: {} },
      {
        'tls_settings.server_name': 'required',
        'tls_settings.private_key': 'required',
        'tls_settings.public_key': 'required',
      },
    ],
    [
      'short id must be even-length hex',
      'vless',
      { tls: 2, tls_settings: { server_name: 's', private_key: 'k', public_key: 'p', short_id: 'abc' } },
      {
        'tls_settings.short_id': 'shortIdLength',
      },
    ],
    [
      'short id hex only',
      'vless',
      { tls: 2, tls_settings: { server_name: 's', private_key: 'k', public_key: 'p', short_id: 'zz' } },
      {
        'tls_settings.short_id': 'hex',
      },
    ],
    [
      'ss2022 key length',
      'shadowsocks',
      { cipher: '2022-blake3-aes-256-gcm', server_key: KEY16 },
      { server_key: 'base64Key' },
    ],
    ['ss2022 key ok', 'shadowsocks', { cipher: '2022-blake3-aes-128-gcm', server_key: KEY16 }, {}],
    [
      'hysteria obfs password',
      'hysteria',
      { obfs: 'salamander', obfs_password: '' },
      { obfs_password: 'obfsPassword' },
    ],
    [
      'ws path must start with /',
      'vmess',
      { network: 'ws', network_settings: { path: 'ws' } },
      { 'network_settings.path': 'path' },
    ],
    [
      'anytls reality needs a server name',
      'anytls',
      { tls: 2, tls_settings: { private_key: 'k', public_key: 'p' } },
      { server_name: 'required' },
    ],
  ])('%s', (_name, type, settings, expected) => {
    const issues = validateSettings(type, settings)
    expect(Object.fromEntries(Object.entries(issues).map(([k, v]) => [k, v.key]))).toEqual(expected)
  })
})

describe('node form helpers', () => {
  it('validates client ports and ranges', () => {
    expect(portError('443')).toBeUndefined()
    expect(portError('20000-30000')).toBeUndefined()
    expect(portError('')).toBe('required')
    expect(portError('30000-20000')).toBe('port')
    expect(portError('70000')).toBe('port')
    expect(portError('443,8443')).toBe('port')
  })

  it('splits server errors keyed by JSON path', () => {
    expect(
      splitNodeErrors({
        name: ['required'],
        'group_ids.0': ['unknown group'],
        'protocol_settings.tls_settings.private_key': ['missing'],
        protocol_settings: ['invalid'],
      }),
    ).toEqual({
      protocol: { 'tls_settings.private_key': 'missing', '': 'invalid' },
      rest: { name: ['required'], group_ids: ['unknown group'] },
    })
  })

  it('builds the node payload', () => {
    const values = {
      ...nodeToValues(null, 'trojan'),
      name: ' T ',
      host: 't.example.com',
      port: '443',
      server_port: 443,
      group_ids: ['1', '2'],
      tags: 'a, b',
      parent_id: NO_PARENT,
    }
    expect(valuesToNodeInput(values)).toEqual({
      type: 'trojan',
      name: 'T',
      group_ids: [1, 2],
      route_ids: [],
      parent_id: null,
      host: 't.example.com',
      port: '443',
      server_port: 443,
      tags: ['a', 'b'],
      rate: 1,
      show: true,
      token: '',
      protocol_settings: { network: 'tcp', allow_insecure: false },
    })
  })

  it('sends the node token trimmed, and always empty for child nodes', () => {
    const base = { ...nodeToValues(null, 'vless'), name: 'N', host: 'n.example.com', port: '443' }
    const token = 'abcdefghijklmnop0123456789ABCDEF'
    expect(valuesToNodeInput({ ...base, token: ` ${token} ` })).toMatchObject({ parent_id: null, token })
    expect(valuesToNodeInput({ ...base, token, parent_id: '5' })).toMatchObject({ parent_id: 5, token: '' })
    expect(nodeToValues({ token } as Node).token).toBe(token)
    expect(nodeToValues(null).token).toBe('')
  })

  it('validates node tokens like the server (empty or 16–64 visible ASCII, no spaces)', () => {
    expect(tokenError('')).toBeUndefined()
    expect(tokenError('   ')).toBeUndefined()
    expect(tokenError('a'.repeat(16))).toBeUndefined()
    expect(tokenError('!#$%&()*+,-./:;<=>?@[]^_`{|}~0123')).toBeUndefined()
    expect(tokenError('a'.repeat(64))).toBeUndefined()
    expect(tokenError('a'.repeat(15))).toBe('token')
    expect(tokenError('a'.repeat(65))).toBe('token')
    expect(tokenError('abcdefgh ijklmnop')).toBe('token')
    expect(tokenError('密钥密钥密钥密钥密钥密钥密钥密钥')).toBe('token')
  })

  it('generates 32-character alphanumeric node tokens', () => {
    const token = randomToken(NODE_TOKEN_LENGTH)
    expect(token).toMatch(/^[A-Za-z0-9]{32}$/)
    expect(tokenError(token)).toBeUndefined()
    expect(randomToken(NODE_TOKEN_LENGTH)).not.toBe(token)
  })

  it('knows whose token a node authenticates with', () => {
    const nodes = [
      { id: 1, name: 'HK', token: 'parentTokenparentToken' },
      { id: 2, name: 'JP', token: '' },
    ] as Node[]
    expect(tokenOwner(NO_PARENT, '', nodes)).toBeNull()
    expect(tokenOwner(NO_PARENT, '  ', nodes)).toBeNull()
    expect(tokenOwner(NO_PARENT, 'x'.repeat(16), nodes)).toEqual({ kind: 'node' })
    // A child uses its parent's token, whatever its own form value says.
    expect(tokenOwner('1', '', nodes)).toEqual({ kind: 'parent', name: 'HK' })
    expect(tokenOwner('2', 'x'.repeat(16), nodes)).toBeNull()
    expect(tokenOwner('9', '', nodes)).toBeNull()
  })

  it('uses the <node_token> placeholder when the node has its own token', () => {
    const input = { type: 'vless', nodeId: 3, appUrl: 'https://panel.example.com' } as const
    const shared = integrationSnippets(input)
    const own = integrationSnippets({ ...input, nodeToken: true })
    for (const s of [shared, own]) expect(Object.values(s).every((code) => code !== null)).toBe(true)
    expect(`${shared.xrayr}${shared.v2bx}${shared.soga}`).not.toContain('<node_token>')
    expect(shared.xrayr).toContain('ApiKey: "<server_token>"')
    expect(own.xrayr).toContain('ApiKey: "<node_token>"')
    expect(own.v2bx).toContain('"ApiKey": "<node_token>"')
    expect(own.soga).toContain('webapi_key=<node_token>')
    expect(`${own.xrayr}${own.v2bx}${own.soga}`).not.toContain('<server_token>')
  })

  it('maps node types to backend types', () => {
    expect([
      xrayrNodeType('vless'),
      v2bxNodeType('hysteria', 2),
      v2bxNodeType('hysteria', 1),
      sogaServerType('shadowsocks'),
    ]).toEqual(['Vless', 'hysteria2', 'hysteria', 'ss'])
    expect([
      xrayrNodeType('tuic'),
      sogaServerType('tuic'),
      sogaServerType('hysteria', 1),
      v2bxNodeType('mieru'),
    ]).toEqual([null, null, null, null])
    const s = integrationSnippets({ type: 'shadowsocks', nodeId: 7, appUrl: 'https://panel.example.com' })
    expect(s.soga).toContain('webapi_url=https://panel.example.com/soga/')
    expect(s.soga).toContain('server_type=ss')
    expect(s.xrayr).toContain('NodeType: Shadowsocks')
    expect(s.v2bx).toContain('"NodeID": 7')
    expect(`${s.soga}${s.xrayr}${s.v2bx}`).not.toMatch(/[0-9a-f]{32}/) // never a real token
  })
})
