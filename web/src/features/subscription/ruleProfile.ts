/**
 * Pure rule-profile editing helpers for the subscription rule builder (docs/SUBSCRIPTION.md → Rule profiles,
 * docs/RULESETS.md → Schema). Every function returns a new `RuleProfile`; nothing mutates its input.
 */
import type {
  LocalizedText,
  PolicyGroup,
  PolicyGroupType,
  RuleDns,
  RuleProfile,
  RulesetCatalog,
  RulesetCategory,
  RulesetTemplate,
} from '@/lib/api/types'

export const DIRECT = 'DIRECT'
export const REJECT = 'REJECT'
export const PROXY_GROUP = 'Proxy'
export const SPECIAL_TARGETS = [DIRECT, REJECT] as const
export const GROUP_TYPES: readonly PolicyGroupType[] = ['select', 'url-test', 'fallback', 'load-balance']
export const DNS_MODES: readonly RuleDns[] = ['default', 'fakeip', 'none']
export const DEFAULT_TEST_URL = 'http://www.gstatic.com/generate_204'
export const DEFAULT_INTERVAL = 300
/** `RuleProfile.mirror` value meaning "the site's configured mirror". */
export const SITE_MIRROR = 'default'

/** Picks the text for `locale`, then zh-CN / en / any, then `fallback`. */
export function localized(text: LocalizedText | undefined, locale: string, fallback = ''): string {
  if (!text) return fallback
  return text[locale] ?? text['zh-CN'] ?? text.en ?? Object.values(text)[0] ?? fallback
}

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

export const needsProbe = (type: PolicyGroupType) => type !== 'select'

/** Drops `url`/`interval` from `select` groups and fills them for the probing types. */
export function normalizeGroup(group: PolicyGroup): PolicyGroup {
  const { url, interval, ...rest } = group
  if (!needsProbe(group.type)) return rest
  return { ...rest, url: url || DEFAULT_TEST_URL, interval: interval ?? DEFAULT_INTERVAL }
}

/** Canonical form: enabled categories only (string targets), normalised groups. */
export function normalizeProfile(profile: RuleProfile): RuleProfile {
  const categories: Record<string, string> = {}
  for (const [id, target] of Object.entries(profile.categories ?? {})) {
    if (typeof target === 'string' && target) categories[id] = target
  }
  return {
    template: profile.template,
    source: profile.source,
    categories,
    groups: (profile.groups ?? []).map(normalizeGroup),
    final: profile.final,
    geoip_cn_direct: Boolean(profile.geoip_cn_direct),
    mirror: profile.mirror ?? '',
    dns: profile.dns ?? 'default',
  }
}

/** Order-insensitive equality for categories, order-sensitive for groups. */
export function sameProfile(a: RuleProfile, b: RuleProfile): boolean {
  const key = (p: RuleProfile) => {
    const n = normalizeProfile(p)
    const cats = Object.keys(n.categories)
      .sort()
      .map((k) => [k, n.categories[k]])
    return JSON.stringify({ ...n, categories: cats })
  }
  return key(a) === key(b)
}

/** Applies a template's categories, groups, final policy and GEOIP switch; keeps source, mirror and DNS. */
export function applyTemplate(template: RulesetTemplate, base?: Partial<RuleProfile>): RuleProfile {
  return normalizeProfile({
    template: template.id,
    source: base?.source ?? '',
    categories: clone(template.categories ?? {}),
    groups: clone(template.groups ?? []),
    final: template.final || PROXY_GROUP,
    geoip_cn_direct: template.geoip_cn_direct,
    mirror: base?.mirror ?? '',
    dns: base?.dns ?? 'default',
  })
}

export function findTemplate(catalog: RulesetCatalog, id: string): RulesetTemplate | undefined {
  return catalog.templates.find((t) => t.id === id)
}

export const groupNames = (profile: RuleProfile) => profile.groups.map((g) => g.name)

/** Targets a category may use: DIRECT, REJECT and every policy group. */
export function targetOptions(profile: RuleProfile): string[] {
  return [...SPECIAL_TARGETS, ...groupNames(profile).filter((n) => n && !isSpecial(n))]
}

/** Policies for unmatched traffic: every group plus DIRECT. */
export function finalOptions(profile: RuleProfile): string[] {
  return [...groupNames(profile).filter((n) => n && !isSpecial(n)), DIRECT]
}

const isSpecial = (name: string) => (SPECIAL_TARGETS as readonly string[]).includes(name.toUpperCase())

/** Target used when a category is switched on: its default if valid here, else the final policy. */
export function defaultTargetFor(profile: RuleProfile, category: RulesetCategory): string {
  const wanted = category.default_target
  if (targetOptions(profile).includes(wanted)) return wanted
  return profile.final || PROXY_GROUP
}

export function isEnabled(profile: RuleProfile, id: string): boolean {
  return typeof profile.categories[id] === 'string'
}

export function toggleCategory(
  profile: RuleProfile,
  category: RulesetCategory,
  enabled: boolean,
): RuleProfile {
  const categories = { ...profile.categories }
  if (enabled) categories[category.id] = defaultTargetFor(profile, category)
  else delete categories[category.id]
  return { ...profile, categories }
}

export function setCategoryTarget(profile: RuleProfile, id: string, target: string): RuleProfile {
  return { ...profile, categories: { ...profile.categories, [id]: target } }
}

/** Unique "Group 2", "Group 3", … name. */
export function nextGroupName(profile: RuleProfile, base = 'Group'): string {
  const taken = new Set(groupNames(profile).map((n) => n.toLowerCase()))
  for (let i = 1; ; i++) {
    const name = i === 1 ? base : `${base} ${i}`
    if (!taken.has(name.toLowerCase())) return name
  }
}

export function addGroup(profile: RuleProfile, name = nextGroupName(profile)): RuleProfile {
  // New groups go before the trailing probe group ("auto") when there is one, else at the end.
  const groups = [...profile.groups]
  const last = groups[groups.length - 1]
  const at = last && needsProbe(last.type) && groups.length > 1 ? groups.length - 1 : groups.length
  groups.splice(at, 0, { name, type: 'select', include: `${PROXY_GROUP}, *` })
  return { ...profile, groups }
}

export function updateGroup(profile: RuleProfile, index: number, patch: Partial<PolicyGroup>): RuleProfile {
  const current = profile.groups[index]
  if (!current) return profile
  const groups = [...profile.groups]
  groups[index] = normalizeGroup({ ...current, ...patch })
  return { ...profile, groups }
}

const splitTokens = (include: string) =>
  include
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean)

/** Replaces (or removes when `to` is null) a group token in an `include` list. */
function retoken(include: string, from: string, to: string | null): string {
  const tokens = splitTokens(include)
  if (!tokens.includes(from)) return include
  return tokens.flatMap((t) => (t === from ? (to === null ? [] : [to]) : [t])).join(', ')
}

/**
 * Renames group `index` and every reference to it (category targets, `final`, other groups' `include`).
 * Call it on commit (blur/Enter), never per keystroke. No-op when the name is empty, special or taken.
 */
export function renameGroup(profile: RuleProfile, index: number, rawName: string): RuleProfile {
  const group = profile.groups[index]
  const name = rawName.trim()
  if (!group || !name || name === group.name) return profile
  if (renameError(profile, index, name)) return profile
  const old = group.name
  const categories: Record<string, string | false> = {}
  for (const [id, target] of Object.entries(profile.categories))
    categories[id] = target === old ? name : target
  return {
    ...profile,
    categories,
    final: profile.final === old ? name : profile.final,
    groups: profile.groups.map((g, i) =>
      i === index ? { ...g, name } : { ...g, include: retoken(g.include, old, name) },
    ),
  }
}

export type GroupNameError = 'required' | 'reserved' | 'duplicate' | 'proxy'

export function renameError(
  profile: RuleProfile,
  index: number,
  rawName: string,
): GroupNameError | undefined {
  const name = rawName.trim()
  const group = profile.groups[index]
  if (!name) return 'required'
  if (group?.name === PROXY_GROUP && name !== PROXY_GROUP) return 'proxy'
  if (isSpecial(name)) return 'reserved'
  if (profile.groups.some((g, i) => i !== index && g.name.toLowerCase() === name.toLowerCase()))
    return 'duplicate'
  return undefined
}

/** Removes a group (never `Proxy`); references fall back to `final` (or `Proxy`). */
export function removeGroup(profile: RuleProfile, index: number): RuleProfile {
  const group = profile.groups[index]
  if (!group || group.name === PROXY_GROUP) return profile
  const old = group.name
  const final = profile.final === old ? PROXY_GROUP : profile.final
  const categories: Record<string, string | false> = {}
  for (const [id, target] of Object.entries(profile.categories))
    categories[id] = target === old ? final : target
  return {
    ...profile,
    categories,
    final,
    groups: profile.groups
      .filter((_, i) => i !== index)
      .map((g) => ({ ...g, include: retoken(g.include, old, null) || PROXY_GROUP })),
  }
}

export function moveGroup(profile: RuleProfile, index: number, delta: -1 | 1): RuleProfile {
  const to = index + delta
  if (to < 0 || to >= profile.groups.length) return profile
  const groups = [...profile.groups]
  const [moved] = groups.splice(index, 1)
  if (!moved) return profile
  groups.splice(to, 0, moved)
  return { ...profile, groups }
}

/** Client-side checks mirroring the server rules; keys follow the server's `errors` paths. */
export type ProfileIssue =
  | 'groupName.required'
  | 'groupName.reserved'
  | 'groupName.duplicate'
  | 'include.required'
  | 'interval.invalid'
  | 'url.invalid'
  | 'proxy.missing'
  | 'final.invalid'
  | 'target.invalid'

export function validateProfile(profile: RuleProfile): Record<string, ProfileIssue> {
  const issues: Record<string, ProfileIssue> = {}
  const seen = new Set<string>()
  profile.groups.forEach((g, i) => {
    const name = g.name.trim()
    if (!name) issues[`groups.${i}.name`] = 'groupName.required'
    else if (isSpecial(name)) issues[`groups.${i}.name`] = 'groupName.reserved'
    else if (seen.has(name.toLowerCase())) issues[`groups.${i}.name`] = 'groupName.duplicate'
    seen.add(name.toLowerCase())
    if (!g.include.trim()) issues[`groups.${i}.include`] = 'include.required'
    if (needsProbe(g.type)) {
      if (!g.interval || !Number.isInteger(g.interval) || g.interval < 10 || g.interval > 86_400)
        issues[`groups.${i}.interval`] = 'interval.invalid'
      if (!g.url || !/^https?:\/\/\S+$/i.test(g.url)) issues[`groups.${i}.url`] = 'url.invalid'
    }
  })
  if (!profile.groups.some((g) => g.name === PROXY_GROUP)) issues.groups = 'proxy.missing'
  if (!finalOptions(profile).includes(profile.final)) issues.final = 'final.invalid'
  const targets = new Set(targetOptions(profile))
  for (const [id, target] of Object.entries(profile.categories)) {
    if (typeof target === 'string' && !targets.has(target)) issues[`categories.${id}`] = 'target.invalid'
  }
  return issues
}

/** Normalises server error keys: `groups[2].name` → `groups.2.name`. */
export function errorKey(key: string): string {
  return key.replace(/\[(\d+)\]/g, '.$1').replace(/^\./, '')
}

export type CategoryBand = 'basics' | 'ai' | 'social' | 'streaming' | 'services' | 'games' | 'regions'

/** Display bands derived from `Category.order` (emission order: specific → broad). */
export function bandOf(order: number): CategoryBand {
  if (order < 30) return 'basics'
  if (order < 40) return 'ai'
  if (order < 50) return 'social'
  if (order < 60) return 'streaming'
  if (order < 70) return 'services'
  if (order < 80) return 'games'
  return 'regions'
}

export function groupCategories(categories: readonly RulesetCategory[]) {
  const bands = new Map<CategoryBand, RulesetCategory[]>()
  for (const category of [...categories].sort((a, b) => a.order - b.order)) {
    const band = bandOf(category.order)
    const list = bands.get(band)
    if (list) list.push(category)
    else bands.set(band, [category])
  }
  return [...bands.entries()].map(([band, items]) => ({ band, items }))
}

/** Source ids sorted by fallback priority. */
export function sortedSources(catalog: RulesetCatalog): string[] {
  return Object.entries(catalog.sources)
    .sort(([, a], [, b]) => (a.order ?? 999) - (b.order ?? 999))
    .map(([id]) => id)
}

/** Number of categories a source publishes. */
export function sourceCoverage(catalog: RulesetCatalog, source: string): number {
  return catalog.categories.filter((c) => source in (c.rules ?? {})).length
}

/** The source that will actually serve `category`: the preferred one, else the first by `order` that has it. */
export function effectiveSource(
  catalog: RulesetCatalog,
  category: RulesetCategory,
  preferred: string,
): string | null {
  const rules = category.rules ?? {}
  if (preferred in rules) return preferred
  return sortedSources(catalog).find((id) => id in rules) ?? null
}
