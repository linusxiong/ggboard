/**
 * Sub-Store-style node operations (`RuleProfile.nodes`): defaults, validation and an in-browser preview of
 * what the server will do to the user's node names. Pure functions (unit tested in nodeOps.test.ts).
 *
 * Mirrors docs/SUBSCRIPTION.md → "Node operations" (backend node operations / region detection):
 *   filters (types, regions, include, exclude) → dedupe (protocol + host + port) → rename (in order) → emoji →
 *   prefix/suffix → name normalisation → sort → limit → duplicate names get " 2", " 3", …
 *
 * The server matches with Go RE2. The browser preview uses JavaScript RegExp, so `compilePattern` accepts the
 * RE2 spellings people paste (`(?i)` flag prefix, `(?P<name>…)`, `\A`, `\z`) and rejects what RE2 cannot do
 * (look-arounds, back-references) so the server never sees a pattern it would refuse. Replacements follow
 * Go's `regexp.Expand` rules (`$1`, `${1}`, `${name}`, `$$`; `$1x` is the group named `1x`).
 */
import type { NodeOperations, NodeRenameRule, NodeType, UserNode } from '@/lib/api/types'

/** Protocol display names (product names, not translated). */
export const TYPE_LABELS: Record<string, string> = {
  vmess: 'VMess',
  vless: 'VLESS',
  shadowsocks: 'Shadowsocks',
  shadowsocksr: 'ShadowsocksR',
  trojan: 'Trojan',
  hysteria: 'Hysteria',
  anytls: 'AnyTLS',
  mieru: 'Mieru',
  tuic: 'TUIC',
}

export const EMOJI_MODES = ['keep', 'add', 'strip'] as const
export const SORT_MODES = ['default', 'name', 'region'] as const

/** Server limits (docs/SUBSCRIPTION.md → Node operations → Validation). */
export const LIMITS = {
  patterns: 32,
  patternBytes: 256,
  renameRules: 64,
  replaceBytes: 256,
  affixChars: 32,
  maxNodes: 1000,
} as const

// ---------------------------------------------------------------- regions

type RegionRow = readonly [code: string, keywords: readonly string[], tokens: readonly string[]]

/**
 * Region table in priority order: ISO code, name keywords (case-insensitive substrings), tokens (upper case,
 * whole word). Relay names start with the domestic entry city (`上海-香港 IPLC` is HK), hence CN last.
 */
const REGION_TABLE: readonly RegionRow[] = [
  [
    'HK',
    ['香港', 'Hong Kong', 'HongKong', '深港', '沪港', '京港', '广港', '杭港', '莞港'],
    ['HK', 'HKG', 'HKT', 'HKBN', 'HGC', 'WTT'],
  ],
  ['MO', ['澳门', '澳門', 'Macao', 'Macau'], ['MO', 'MAC', 'CTM']],
  [
    'TW',
    ['台湾', '臺灣', '台灣', '台北', '臺北', '新北', '台中', '高雄', '彰化', 'Taiwan', 'Taipei'],
    ['TW', 'TWN', 'TPE', 'CHT', 'HINET'],
  ],
  [
    'JP',
    ['日本', '东京', '東京', '大阪', '埼玉', '名古屋', 'Japan', 'Tokyo', 'Osaka'],
    ['JP', 'JPN', 'TYO', 'NRT', 'HND', 'KIX', 'OSA'],
  ],
  [
    'KR',
    ['韩国', '韓國', '南韩', '南韓', '首尔', '首爾', '春川', 'Korea', 'Seoul'],
    ['KR', 'KOR', 'SEL', 'ICN'],
  ],
  ['SG', ['新加坡', '狮城', '獅城', 'Singapore'], ['SG', 'SGP', 'SIN']],
  [
    'US',
    [
      '美国',
      '美國',
      '美西',
      '美东',
      '美東',
      '洛杉矶',
      '洛杉磯',
      '圣何塞',
      '聖何塞',
      '西雅图',
      '西雅圖',
      '芝加哥',
      '纽约',
      '紐約',
      '硅谷',
      '矽谷',
      '达拉斯',
      '達拉斯',
      '凤凰城',
      '鳳凰城',
      '波特兰',
      '弗里蒙特',
      'United States',
      'Los Angeles',
      'San Jose',
      'Seattle',
      'Chicago',
      'New York',
      'Dallas',
      'Silicon Valley',
    ],
    ['US', 'USA', 'LAX', 'SJC', 'SEA', 'ORD', 'JFK', 'DFW', 'NYC'],
  ],
  [
    'GB',
    ['英国', '英國', '伦敦', '倫敦', 'United Kingdom', 'Britain', 'England', 'London'],
    ['GB', 'GBR', 'UK', 'LHR'],
  ],
  ['DE', ['德国', '德國', '法兰克福', '法蘭克福', '柏林', 'Germany', 'Frankfurt', 'Berlin'], ['DE', 'DEU']],
  ['FR', ['法国', '法國', '巴黎', 'France', 'Paris'], ['FR', 'FRA', 'CDG']],
  ['NL', ['荷兰', '荷蘭', '阿姆斯特丹', 'Netherlands', 'Amsterdam'], ['NL', 'NLD', 'AMS']],
  ['RU', ['俄罗斯', '俄羅斯', '莫斯科', '伯力', '圣彼得堡', 'Russia', 'Moscow'], ['RU', 'RUS']],
  [
    'CA',
    [
      '加拿大',
      '多伦多',
      '多倫多',
      '温哥华',
      '溫哥華',
      '蒙特利尔',
      'Canada',
      'Toronto',
      'Vancouver',
      'Montreal',
    ],
    ['CA', 'CAN', 'YVR', 'YYZ'],
  ],
  [
    'AU',
    ['澳大利亚', '澳大利亞', '澳洲', '悉尼', '墨尔本', '墨爾本', 'Australia', 'Sydney', 'Melbourne'],
    ['AU', 'AUS', 'SYD'],
  ],
  [
    'ID',
    ['印尼', '印度尼西亚', '印度尼西亞', '雅加达', '雅加達', 'Indonesia', 'Jakarta'],
    ['ID', 'IDN', 'CGK'],
  ],
  ['IN', ['印度', '孟买', '孟買', 'India', 'Mumbai'], ['IN', 'IND', 'BOM']],
  ['TH', ['泰国', '泰國', '曼谷', 'Thailand', 'Bangkok'], ['TH', 'THA', 'BKK']],
  ['VN', ['越南', '胡志明', '河内', '河內', 'Vietnam', 'Hanoi'], ['VN', 'VNM', 'SGN']],
  ['MY', ['马来西亚', '馬來西亞', '吉隆坡', 'Malaysia', 'Kuala Lumpur'], ['MY', 'MYS', 'KUL']],
  ['PH', ['菲律宾', '菲律賓', '马尼拉', '馬尼拉', 'Philippines', 'Manila'], ['PH', 'PHL', 'MNL']],
  ['TR', ['土耳其', '伊斯坦布尔', 'Turkey', 'Türkiye', 'Istanbul'], ['TR', 'TUR', 'IST']],
  ['AE', ['阿联酋', '阿聯酋', '迪拜', '杜拜', 'United Arab Emirates', 'Dubai'], ['AE', 'ARE', 'UAE', 'DXB']],
  ['IL', ['以色列', 'Israel'], ['IL', 'ISR']],
  ['BR', ['巴西', '圣保罗', '聖保羅', 'Brazil', 'Sao Paulo', 'São Paulo'], ['BR', 'BRA', 'GRU']],
  ['AR', ['阿根廷', 'Argentina'], ['AR', 'ARG']],
  ['MX', ['墨西哥', 'Mexico'], ['MX', 'MEX']],
  ['CL', ['智利', 'Chile'], ['CL', 'CHL']],
  ['ZA', ['南非', '约翰内斯堡', 'South Africa', 'Johannesburg'], ['ZA', 'ZAF', 'JNB']],
  ['EG', ['埃及', 'Egypt'], ['EG', 'EGY']],
  ['NG', ['尼日利亚', '尼日利亞', 'Nigeria'], ['NG', 'NGA']],
  ['IT', ['意大利', '義大利', '米兰', '米蘭', 'Italy', 'Milan'], ['IT', 'ITA', 'MXP']],
  ['ES', ['西班牙', '马德里', '馬德里', 'Spain', 'Madrid'], ['ES', 'ESP', 'MAD']],
  ['PT', ['葡萄牙', '里斯本', 'Portugal', 'Lisbon'], ['PT', 'PRT']],
  ['CH', ['瑞士', '苏黎世', '蘇黎世', 'Switzerland', 'Zurich'], ['CH', 'CHE', 'ZRH']],
  ['SE', ['瑞典', '斯德哥尔摩', 'Sweden', 'Stockholm'], ['SE', 'SWE']],
  ['NO', ['挪威', 'Norway', 'Oslo'], ['NO', 'NOR']],
  ['FI', ['芬兰', '芬蘭', '赫尔辛基', 'Finland', 'Helsinki'], ['FI', 'FIN', 'HEL']],
  ['DK', ['丹麦', '丹麥', 'Denmark', 'Copenhagen'], ['DK', 'DNK']],
  ['PL', ['波兰', '波蘭', '华沙', '華沙', 'Poland', 'Warsaw'], ['PL', 'POL', 'WAW']],
  ['AT', ['奥地利', '奧地利', '维也纳', '維也納', 'Austria', 'Vienna'], ['AT', 'AUT', 'VIE']],
  ['BE', ['比利时', '比利時', 'Belgium', 'Brussels'], ['BE', 'BEL']],
  ['IE', ['爱尔兰', '愛爾蘭', '都柏林', 'Ireland', 'Dublin'], ['IE', 'IRL', 'DUB']],
  ['UA', ['乌克兰', '烏克蘭', 'Ukraine', 'Kyiv'], ['UA', 'UKR']],
  ['KZ', ['哈萨克斯坦', '哈薩克', 'Kazakhstan'], ['KZ', 'KAZ']],
  ['MN', ['蒙古', 'Mongolia'], ['MN', 'MNG']],
  ['NZ', ['新西兰', '新西蘭', '紐西蘭', '奥克兰', 'New Zealand', 'Auckland'], ['NZ', 'NZL', 'AKL']],
  [
    'CN',
    [
      '中国',
      '中國',
      '回国',
      '回國',
      '内蒙古',
      '內蒙古',
      '北京',
      '上海',
      '广州',
      '廣州',
      '深圳',
      '杭州',
      '江苏',
      '江蘇',
      'China',
    ],
    ['CN', 'CHN'],
  ],
]

/** Every known region code, in table (= default `sort: region`) order. */
export const REGIONS: readonly string[] = REGION_TABLE.map(([code]) => code)
/** The contract's core list (HK TW JP SG US KR DE UK), always offered as chips. */
export const CORE_REGIONS: readonly string[] = ['HK', 'TW', 'JP', 'SG', 'US', 'KR', 'DE', 'GB']

/** `UK` is accepted for `GB` (contract wording); the UI stores ISO codes. */
export const canonicalRegion = (code: string) => (code.toUpperCase() === 'UK' ? 'GB' : code.toUpperCase())

/** `HK` → 🇭🇰 (any two-letter code). */
export function regionFlag(code: string): string {
  const c = canonicalRegion(code)
  if (!/^[A-Z]{2}$/.test(c)) return ''
  return String.fromCodePoint(...[...c].map((ch) => 0x1f1e6 + ch.charCodeAt(0) - 65))
}

/** Two regional-indicator symbols = one flag emoji. */
const FLAG_RE = /[\u{1F1E6}-\u{1F1FF}]{2}/u
const flagCode = (flag: string) =>
  [...flag].map((c) => String.fromCharCode((c.codePointAt(0) ?? 0) - 0x1f1e6 + 65)).join('')

/** Whole word: not preceded or followed by an ASCII letter (`HK01`, `[US]`); `CN` never matches `CN2`. */
const tokenRe = (token: string) =>
  new RegExp(`(?<![A-Za-z])${token}(?![A-Za-z])${token === 'CN' ? '(?!2)' : ''}`, 'u')
const TOKENS = REGION_TABLE.map(([code, , tokens]) => [code, tokens.map(tokenRe)] as const)
const KEYWORDS = REGION_TABLE.map(([code, words]) => [code, words.map((w) => w.toLowerCase())] as const)

/**
 * Region of a node: the first flag emoji in the name (any flag), else the first table row with a keyword in the
 * name or a tag (case-insensitive substring), else the first row with a whole-word token in the name or a tag.
 */
export function detectRegion(name: string, tags: readonly string[] = []): string | null {
  const flag = FLAG_RE.exec(name)
  if (flag) return flagCode(flag[0])
  const texts = [name, ...tags]
  const lower = texts.map((s) => s.toLowerCase())
  const innerMongolia = /内蒙古|內蒙古/.test(name)
  for (const [code, words] of KEYWORDS) {
    if (code === 'MN' && innerMongolia) continue
    if (words.some((w) => lower.some((s) => s.includes(w)))) return code
  }
  for (const [code, tokens] of TOKENS) {
    if (tokens.some((re) => texts.some((s) => re.test(s)))) return code
  }
  return null
}

// ---------------------------------------------------------------- defaults

export function emptyNodeOps(): NodeOperations {
  return {
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
  }
}

const strings = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter((v): v is string => typeof v === 'string') : []

/** Fills every field (servers may omit empty ones), drops unknown keys, stores regions as ISO codes. */
export function normalizeNodeOps(ops: Partial<NodeOperations> | null | undefined): NodeOperations {
  const base = emptyNodeOps()
  if (!ops) return base
  return {
    include: strings(ops.include),
    exclude: strings(ops.exclude),
    types: strings(ops.types),
    regions: [...new Set(strings(ops.regions).map(canonicalRegion))],
    rename: Array.isArray(ops.rename)
      ? ops.rename
          .filter((r): r is NodeRenameRule => typeof r === 'object' && r !== null)
          .map((r) => ({ match: String(r.match ?? ''), replace: String(r.replace ?? '') }))
      : [],
    prefix: typeof ops.prefix === 'string' ? ops.prefix : '',
    suffix: typeof ops.suffix === 'string' ? ops.suffix : '',
    emoji: (EMOJI_MODES as readonly string[]).includes(ops.emoji ?? '') ? ops.emoji! : base.emoji,
    sort: (SORT_MODES as readonly string[]).includes(ops.sort ?? '') ? ops.sort! : base.sort,
    dedupe: Boolean(ops.dedupe),
    limit: typeof ops.limit === 'number' && Number.isFinite(ops.limit) ? ops.limit : 0,
  }
}

export const sameNodeOps = (a: Partial<NodeOperations> | undefined, b: Partial<NodeOperations> | undefined) =>
  JSON.stringify(normalizeNodeOps(a)) === JSON.stringify(normalizeNodeOps(b))

export const isEmptyNodeOps = (ops: Partial<NodeOperations> | undefined) => sameNodeOps(ops, emptyNodeOps())

// ---------------------------------------------------------------- patterns

export type PatternIssue = 'empty' | 'syntax' | 'unsupported' | 'tooLong'
export type CompiledPattern = { ok: true; re: RegExp } | { ok: false; issue: PatternIssue }

/** RE2 has no look-around and no back-references. */
const RE2_UNSUPPORTED = /\(\?<?[=!]|\\[1-9]|\\k</
const bytes = (s: string) => new TextEncoder().encode(s).length

/** RE2 pattern → JavaScript RegExp (`g` for replacement, `u` for emoji / CJK ranges). */
export function compilePattern(source: string, global = false): CompiledPattern {
  if (!source.trim()) return { ok: false, issue: 'empty' }
  if (bytes(source) > LIMITS.patternBytes) return { ok: false, issue: 'tooLong' }
  if (RE2_UNSUPPORTED.test(source)) return { ok: false, issue: 'unsupported' }
  let body = source
  let flags = 'u'
  const lead = /^\(\?([imsU]+)\)/.exec(body)
  if (lead?.[1]) {
    if (lead[1].includes('U')) return { ok: false, issue: 'unsupported' }
    body = body.slice(lead[0].length)
    for (const f of lead[1]) if (!flags.includes(f)) flags += f
  }
  body = body
    .replace(/\(\?P</g, '(?<')
    .replace(/\\A/g, '^')
    .replace(/\\z/g, '$')
  try {
    return { ok: true, re: new RegExp(body, global ? `${flags}g` : flags) }
  } catch {
    return { ok: false, issue: 'syntax' }
  }
}

/**
 * Go `regexp.Expand` for one match: `$$` → `$`, `${name}` / `$name` (longest run of letters, digits and `_`)
 * → that group (numeric = index); unknown groups expand to "".
 */
export function expandTemplate(
  template: string,
  groups: readonly (string | undefined)[],
  named: Readonly<Record<string, string | undefined>> = {},
): string {
  const lookup = (name: string) => (/^\d+$/.test(name) ? groups[Number(name)] : named[name]) ?? ''
  return template.replace(/\$(?:\$|\{(\w+)\}|(\w+))/g, (whole, braced?: string, bare?: string) =>
    whole === '$$' ? '$' : lookup(braced ?? bare ?? ''),
  )
}

/** Replaces every match of `re` (global) in `name` using a Go replacement template. */
export function replaceAll(name: string, re: RegExp, template: string): string {
  return name.replace(re, (...args: unknown[]) => {
    const last = args[args.length - 1]
    const hasNamed = typeof last === 'object' && last !== null
    const named = (hasNamed ? last : {}) as Record<string, string | undefined>
    const groups = args.slice(0, hasNamed ? -3 : -2) as (string | undefined)[]
    return expandTemplate(template, groups, named)
  })
}

// ---------------------------------------------------------------- names & emoji

const EMOJI_RE = /[\u{1F1E6}-\u{1F1FF}]|\p{Extended_Pictographic}|\u{FE0F}|\u{200D}|\u{20E3}/gu
// eslint-disable-next-line no-control-regex -- strips control characters like the server does
const CONTROL_RE = /[\u0000-\u001f\u007f-\u009f\u2028\u2029]/g

/** Server name normalisation: `,` → `，`, `=` → `＝`, control characters dropped, whitespace collapsed. */
export function normalizeName(name: string): string {
  return name.replace(CONTROL_RE, ' ').replace(/,/g, '，').replace(/=/g, '＝').replace(/\s+/g, ' ').trim()
}

export function stripEmoji(name: string): string {
  return name.replace(EMOJI_RE, '').replace(/\s+/g, ' ').trim()
}

export function addFlag(name: string, region: string | null): string {
  if (!region || FLAG_RE.test(name)) return name
  const flag = regionFlag(region)
  return flag ? `${flag} ${name}` : name
}

/** Region → number of the user's nodes in it. */
export function regionCounts(nodes: readonly UserNode[]): Map<string, number> {
  const counts = new Map<string, number>()
  for (const node of nodes) {
    const region = detectRegion(normalizeName(node.name), node.tags)
    if (region) counts.set(region, (counts.get(region) ?? 0) + 1)
  }
  return counts
}

// ---------------------------------------------------------------- validation

export type NodeOpsIssue =
  `regex.${PatternIssue}` | 'regex.tooMany' | 'replace.tooLong' | 'affix.tooLong' | 'limit.invalid'

/** Client-side checks; keys follow the server's error paths below `nodes.` (`include.0`, `rename.1.match`, `limit`). */
export function validateNodeOps(ops: NodeOperations): Record<string, NodeOpsIssue> {
  const issues: Record<string, NodeOpsIssue> = {}
  const check = (key: string, source: string) => {
    const result = compilePattern(source)
    if (!result.ok) issues[key] = `regex.${result.issue}`
  }
  ops.include.forEach((p, i) => check(`include.${i}`, p))
  ops.exclude.forEach((p, i) => check(`exclude.${i}`, p))
  if (ops.include.length + ops.exclude.length > LIMITS.patterns) issues.exclude = 'regex.tooMany'
  ops.rename.forEach((r, i) => {
    check(`rename.${i}.match`, r.match)
    if (bytes(r.replace) > LIMITS.replaceBytes) issues[`rename.${i}.replace`] = 'replace.tooLong'
  })
  if ([...ops.prefix].length > LIMITS.affixChars) issues.prefix = 'affix.tooLong'
  if ([...ops.suffix].length > LIMITS.affixChars) issues.suffix = 'affix.tooLong'
  if (!Number.isInteger(ops.limit) || ops.limit < 0 || ops.limit > LIMITS.maxNodes)
    issues.limit = 'limit.invalid'
  return issues
}

// ---------------------------------------------------------------- preview

export type DropReason = 'type' | 'region' | 'include' | 'exclude' | 'duplicate' | 'limit'

export interface PreviewNode {
  id: UserNode['id']
  type: NodeType
  original: string
  /** Final name (renamed / prefixed / emoji applied). */
  name: string
  region: string | null
}
export interface DroppedNode extends PreviewNode {
  reason: DropReason
}
export interface NodeOpsPreview {
  kept: PreviewNode[]
  dropped: DroppedNode[]
}

/** `?types=` aliases accepted in `types` (`/user/nodes` reports hysteria v1 and v2 as `hysteria`). */
const TYPE_ALIASES: Record<string, readonly string[]> = {
  ss: ['shadowsocks'],
  ssr: ['shadowsocksr'],
  hy: ['hysteria'],
  hy1: ['hysteria'],
  hysteria1: ['hysteria'],
  hy2: ['hysteria'],
  hysteria2: ['hysteria'],
}

/** `/user/nodes` → what the subscription will contain for these operations (best effort, see file header). */
export function applyNodeOps(nodes: readonly UserNode[], raw: NodeOperations): NodeOpsPreview {
  const ops = normalizeNodeOps(raw)
  const compiled = (list: string[]) => list.map((p) => compilePattern(p)).flatMap((c) => (c.ok ? [c.re] : []))
  const include = compiled(ops.include)
  const exclude = compiled(ops.exclude)
  const rename = ops.rename.flatMap((r) => {
    const c = compilePattern(r.match, true)
    return c.ok ? [{ re: c.re, replace: r.replace }] : []
  })
  const types = new Set(ops.types.flatMap((t) => TYPE_ALIASES[t] ?? [t]))
  const regions = new Set(ops.regions)

  const dropped: DroppedNode[] = []
  let kept: PreviewNode[] = []
  const seen = new Set<string>()
  for (const node of nodes) {
    const original = normalizeName(node.name)
    const region = detectRegion(original, node.tags)
    const item: PreviewNode = { id: node.id, type: node.type, original, name: original, region }
    const reason: DropReason | null =
      types.size > 0 && !types.has(node.type)
        ? 'type'
        : regions.size > 0 && (region === null || !regions.has(region))
          ? 'region'
          : include.length > 0 && !include.some((re) => re.test(original))
            ? 'include'
            : exclude.some((re) => re.test(original))
              ? 'exclude'
              : null
    if (reason) {
      dropped.push({ ...item, reason })
      continue
    }
    // Dedupe by protocol + host + port (visible only when the site shows node addresses to users).
    if (ops.dedupe && node.host) {
      const key = `${node.type}|${node.host.toLowerCase()}|${node.port ?? ''}`
      if (seen.has(key)) {
        dropped.push({ ...item, reason: 'duplicate' })
        continue
      }
      seen.add(key)
    }
    kept.push(item)
  }

  kept = kept.map((item) => {
    let name = item.original
    for (const rule of rename) name = replaceAll(name, rule.re, rule.replace)
    if (ops.emoji === 'strip') name = stripEmoji(name)
    else if (ops.emoji === 'add') name = addFlag(name, item.region)
    return { ...item, name: normalizeName(`${ops.prefix}${name}${ops.suffix}`) }
  })

  if (ops.sort === 'name') {
    const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })
    kept = [...kept].sort((a, b) => collator.compare(a.name, b.name))
  } else if (ops.sort === 'region') {
    const order = ops.regions.length > 0 ? ops.regions : REGIONS
    const rank = (r: string | null) => {
      const i = r === null ? -1 : order.indexOf(r)
      return i < 0 ? order.length : i
    }
    kept = [...kept].sort((a, b) => rank(a.region) - rank(b.region))
  }

  if (ops.limit > 0 && kept.length > ops.limit) {
    for (const item of kept.slice(ops.limit)) dropped.push({ ...item, reason: 'limit' })
    kept = kept.slice(0, ops.limit)
  }

  // Colliding names get " 2", " 3", … (the render's unique-name pass).
  const used = new Set<string>()
  kept = kept.map((item) => {
    let name = item.name
    for (let n = 2; used.has(name); n++) name = `${item.name} ${n}`
    used.add(name)
    return name === item.name ? item : { ...item, name }
  })
  return { kept, dropped }
}

/** Number of node names a single pattern matches (live hint while typing). */
export function countMatches(nodes: readonly UserNode[], source: string): number | null {
  const c = compilePattern(source)
  if (!c.ok) return null
  return nodes.filter((n) => c.re.test(normalizeName(n.name))).length
}

/** Moves item `index` by `delta` (rename rules are ordered). */
export function moveItem<T>(list: readonly T[], index: number, delta: -1 | 1): T[] {
  const to = index + delta
  if (to < 0 || to >= list.length) return [...list]
  const next = [...list]
  const [item] = next.splice(index, 1)
  if (item !== undefined) next.splice(to, 0, item)
  return next
}
