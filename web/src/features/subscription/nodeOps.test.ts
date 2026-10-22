import { describe, expect, it } from 'vitest'
import type { UserNode } from '@/lib/api/types'
import {
  addFlag,
  applyNodeOps,
  compilePattern,
  countMatches,
  detectRegion,
  emptyNodeOps,
  isEmptyNodeOps,
  moveItem,
  normalizeNodeOps,
  regionCounts,
  sameNodeOps,
  expandTemplate,
  normalizeName,
  replaceAll,
  stripEmoji,
  validateNodeOps,
} from './nodeOps'

const node = (
  id: number,
  name: string,
  type: UserNode['type'] = 'vless',
  extra: Partial<UserNode> = {},
): UserNode => ({ id, name, type, tags: [], rate: 1, is_online: true, sort: id, ...extra })

// Shapes as `GET /user/nodes` returns them (docs/API.md).
const NODES: UserNode[] = [
  node(1, '香港 01', 'vless'),
  node(2, '🇭🇰 HK02 IPLC', 'trojan'),
  node(3, '日本 东京', 'hysteria'),
  node(4, '日本 大阪', 'vmess'),
  node(5, 'SG-01 新加坡', 'shadowsocks'),
  node(6, '美国 洛杉矶', 'vless'),
  node(7, '剩余流量：100 GB', 'shadowsocks'),
  node(8, 'Status page', 'anytls'),
]

describe('node operations: patterns', () => {
  it('accepts RE2 spellings and rejects what RE2 cannot do', () => {
    const ci = compilePattern('(?i)hk')
    expect(ci.ok && ci.re.test('🇭🇰 HK02')).toBe(true)
    const named = compilePattern('(?P<n>\\d+)')
    expect(named.ok && named.re.source).toBe('(?<n>\\d+)')
    expect(compilePattern('foo(?=bar)')).toEqual({ ok: false, issue: 'unsupported' })
    expect(compilePattern('(a)\\1')).toEqual({ ok: false, issue: 'unsupported' })
    expect(compilePattern('(?U)a+')).toEqual({ ok: false, issue: 'unsupported' })
    expect(compilePattern('([')).toEqual({ ok: false, issue: 'syntax' })
    expect(compilePattern('  ')).toEqual({ ok: false, issue: 'empty' })
  })

  it('expands replacements like Go regexp.Expand', () => {
    expect(expandTemplate('${2}-$1', ['x', 'a', 'b'])).toBe('b-a')
    expect(expandTemplate('$1x|${1}x|$$1', ['x', 'a'])).toBe('|ax|$1') // `$1x` is the group named 1x
    expect(expandTemplate('${city}', [], { city: 'Tokyo' })).toBe('Tokyo')
    const re = /(?<city>\S+) (\d+)/gu
    expect(replaceAll('Tokyo 01 Osaka 02', re, '${2}-$city')).toBe('01-Tokyo 02-Osaka')
  })

  it('validates lists, rename rules and the limit with server-style keys', () => {
    const ops = {
      ...emptyNodeOps(),
      include: ['香港', '(['],
      exclude: ['a(?!b)'],
      rename: [{ match: '', replace: 'x'.repeat(257) }],
      prefix: 'p'.repeat(33),
      limit: 1001,
    }
    expect(validateNodeOps(ops)).toEqual({
      'include.1': 'regex.syntax',
      'exclude.0': 'regex.unsupported',
      'rename.0.match': 'regex.empty',
      'rename.0.replace': 'replace.tooLong',
      prefix: 'affix.tooLong',
      limit: 'limit.invalid',
    })
    expect(
      validateNodeOps({ ...emptyNodeOps(), include: Array.from({ length: 33 }, (_, i) => `n${i}`) }),
    ).toEqual({
      exclude: 'regex.tooMany',
    })
    expect(validateNodeOps({ ...emptyNodeOps(), include: ['a'.repeat(257)] })).toEqual({
      'include.0': 'regex.tooLong',
    })
    expect(validateNodeOps(emptyNodeOps())).toEqual({})
  })
})

describe('node operations: regions & emoji', () => {
  it('detects regions like the server: flag, then keyword rows by priority, then whole-word tokens', () => {
    expect(detectRegion('香港 01')).toBe('HK')
    expect(detectRegion('🇬🇧 London')).toBe('GB')
    expect(detectRegion('🇧🇷 São Paulo')).toBe('BR')
    expect(detectRegion('SG-01 新加坡')).toBe('SG')
    expect(detectRegion('HK02 IPLC')).toBe('HK')
    expect(detectRegion('hk02')).toBeNull() // tokens are upper case
    expect(detectRegion('[UK] Node')).toBe('GB')
    expect(detectRegion('上海-香港 IPLC')).toBe('HK') // HK row before CN
    expect(detectRegion('日本 via 香港')).toBe('HK') // row priority, not position
    expect(detectRegion('印度尼西亚 01')).toBe('ID') // ID row before IN
    expect(detectRegion('内蒙古 中转')).toBe('CN') // never MN
    expect(detectRegion('CN2 GIA')).toBeNull() // carrier route
    expect(detectRegion('Premium in Asia')).toBeNull()
    expect(detectRegion('Status page')).toBeNull()
    expect(detectRegion('Node 7', ['JP'])).toBe('JP') // tags count
    expect(regionCounts(NODES).get('JP')).toBe(2)
  })

  it('normalises names like the server', () => {
    expect(normalizeName(' a,b=c \t d ')).toBe('a，b＝c d')
  })

  it('strips and adds flag emoji', () => {
    expect(stripEmoji('🇭🇰 HK02 ⚡️ IPLC')).toBe('HK02 IPLC')
    expect(addFlag('日本 东京', 'JP')).toBe('🇯🇵 日本 东京')
    expect(addFlag('🇭🇰 HK02', 'HK')).toBe('🇭🇰 HK02')
    expect(addFlag('Status page', null)).toBe('Status page')
  })
})

describe('node operations: preview pipeline', () => {
  it('keeps everything with empty operations', () => {
    const { kept, dropped } = applyNodeOps(NODES, emptyNodeOps())
    expect(kept.map((n) => n.name)).toEqual(NODES.map((n) => n.name))
    expect(dropped).toEqual([])
  })

  it('accepts type aliases and UK for GB', () => {
    const { kept } = applyNodeOps([node(1, '伦敦', 'hysteria'), node(2, '伦敦 B', 'vless')], {
      ...emptyNodeOps(),
      types: ['hy2'],
      regions: ['UK'],
    })
    expect(kept.map((n) => n.id)).toEqual([1])
  })

  it('filters by type, region, include and exclude with a reason per node', () => {
    const { kept, dropped } = applyNodeOps(NODES, {
      ...emptyNodeOps(),
      types: ['vless', 'trojan', 'hysteria', 'vmess', 'shadowsocks'],
      regions: ['HK', 'JP'],
      include: ['香港|HK|日本'],
      exclude: ['大阪'],
    })
    expect(kept.map((n) => n.id)).toEqual([1, 2, 3])
    expect(Object.fromEntries(dropped.map((n) => [n.id, n.reason]))).toEqual({
      4: 'exclude',
      5: 'region',
      6: 'region',
      7: 'region',
      8: 'type',
    })
  })

  it('renames in order, then emoji, prefix/suffix, sort and limit', () => {
    const ops = {
      ...emptyNodeOps(),
      exclude: ['剩余|Status'],
      rename: [
        { match: '^(\\S+) (\\S+)$', replace: '${2}·$1' },
        { match: '(?i)\\s*iplc', replace: '' },
        { match: '\\d+', replace: '' },
      ],
      emoji: 'add' as const,
      prefix: '[GG] ',
      suffix: '',
      sort: 'region' as const,
      limit: 3,
    }
    const { kept, dropped } = applyNodeOps(NODES, ops)
    // 香港 01 → "·香港" → "🇭🇰 ·香港"; HK02 IPLC (already flagged) → "🇭🇰 HK"; the region sort keeps HK first.
    expect(kept.map((n) => n.name)).toEqual(['[GG] 🇭🇰 ·香港', '[GG] 🇭🇰 HK', '[GG] 🇯🇵 东京·日本'])
    expect(dropped.filter((n) => n.reason === 'limit').map((n) => n.id)).toEqual([4, 5, 6])
    expect(kept[0]?.original).toBe('香港 01')
  })

  it('sorts by the order of `regions` when it is set', () => {
    const { kept } = applyNodeOps(NODES, { ...emptyNodeOps(), regions: ['JP', 'HK'], sort: 'region' })
    expect(kept.map((n) => n.id)).toEqual([3, 4, 1, 2])
  })

  it('dedupes by protocol + host + port before renaming; colliding names get a number', () => {
    const list = [
      node(1, 'HK 01', 'vless', { host: 'hk.example.com', port: '443' }),
      node(2, 'HK 02', 'vless', { host: 'HK.example.com', port: '443' }),
      node(3, 'HK 03', 'trojan', { host: 'hk.example.com', port: '443' }),
      node(4, 'JP 01'),
    ]
    const ops = { ...emptyNodeOps(), rename: [{ match: '\\s*\\d+$', replace: '' }] }
    expect(applyNodeOps(list, ops).kept.map((n) => n.name)).toEqual(['HK', 'HK 2', 'HK 3', 'JP'])
    const deduped = applyNodeOps(list, { ...ops, dedupe: true })
    expect(deduped.kept.map((n) => n.name)).toEqual(['HK', 'HK 2', 'JP'])
    expect(deduped.dropped).toEqual([expect.objectContaining({ id: 2, reason: 'duplicate' })])
  })

  it('sorts by name naturally', () => {
    const list = [node(1, 'HK 10'), node(2, 'HK 2'), node(3, '🇯🇵 A')]
    const sorted = applyNodeOps(list, { ...emptyNodeOps(), sort: 'name' }).kept.map((n) => n.name)
    expect(sorted).toEqual(['🇯🇵 A', 'HK 2', 'HK 10'])
  })

  it('counts live matches and ignores invalid patterns', () => {
    expect(countMatches(NODES, '日本')).toBe(2)
    expect(countMatches(NODES, '([')).toBeNull()
    expect(applyNodeOps(NODES, { ...emptyNodeOps(), include: ['(['] }).kept).toHaveLength(NODES.length)
  })
})

describe('node operations: normalisation', () => {
  it('fills defaults, drops unknown keys and compares structurally', () => {
    const partial = {
      include: ['a'],
      emoji: 'bogus',
      limit: Number.NaN,
      regions: ['uk', 'GB'],
      extra: 1,
    } as never
    expect(normalizeNodeOps(partial)).toEqual({ ...emptyNodeOps(), include: ['a'], regions: ['GB'] })
    expect(isEmptyNodeOps(undefined)).toBe(true)
    expect(sameNodeOps({ prefix: '' }, emptyNodeOps())).toBe(true)
    expect(sameNodeOps({ prefix: 'x' }, emptyNodeOps())).toBe(false)
    expect(moveItem(['a', 'b', 'c'], 2, -1)).toEqual(['a', 'c', 'b'])
    expect(moveItem(['a', 'b'], 0, -1)).toEqual(['a', 'b'])
  })
})
