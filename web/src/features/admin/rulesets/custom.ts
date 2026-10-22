/**
 * Validation and editing helpers for the rule-set custom overrides (`rulesets_custom`,
 * docs/RULESETS.md "Schema"): `{sources?, categories?, templates?, mirrors?}` merged over the
 * built-in catalog by id (source key, category/template/mirror `id`).
 */
import type { RulesetCategoryFull, RulesetMirror, RulesetsCustom } from '@/lib/api/types'

export type IssueCode =
  | 'json'
  | 'root'
  | 'unknownKey'
  | 'expectObject'
  | 'expectArray'
  | 'expectString'
  | 'expectNumber'
  | 'missingId'
  | 'badId'
  | 'duplicateId'
  | 'missingField'
  | 'badGroupType'
  | 'badPrefix'

export interface Issue {
  /** JSON-path-like location, e.g. `categories[2].rules`. */
  path: string
  code: IssueCode
  /** Extra detail (offending key, field name, parser message). */
  detail?: string
}

export interface ParseResult {
  value: RulesetsCustom | null
  issues: Issue[]
}

const TOP_KEYS = ['sources', 'categories', 'templates', 'mirrors'] as const
const GROUP_TYPES = ['select', 'url-test', 'fallback', 'load-balance']
const ID_RE = /^[a-z0-9][a-z0-9_-]*$/

const isObject = (v: unknown): v is Record<string, unknown> =>
  v !== null && typeof v === 'object' && !Array.isArray(v)

function checkLocalized(v: unknown, path: string, issues: Issue[]) {
  if (v === undefined) return
  if (!isObject(v)) issues.push({ path, code: 'expectObject' })
  else
    for (const [k, s] of Object.entries(v))
      if (typeof s !== 'string') issues.push({ path: `${path}.${k}`, code: 'expectString' })
}

function checkIdList(
  list: unknown,
  key: string,
  issues: Issue[],
  each: (item: Record<string, unknown>, path: string) => void,
) {
  if (list === undefined) return
  if (!Array.isArray(list)) {
    issues.push({ path: key, code: 'expectArray' })
    return
  }
  const seen = new Set<string>()
  list.forEach((item, i) => {
    const path = `${key}[${i}]`
    if (!isObject(item)) {
      issues.push({ path, code: 'expectObject' })
      return
    }
    const id = item.id
    if (typeof id !== 'string' || id === '') issues.push({ path: `${path}.id`, code: 'missingId' })
    else if (key !== 'mirrors' && !ID_RE.test(id))
      issues.push({ path: `${path}.id`, code: 'badId', detail: id })
    else if (seen.has(id)) issues.push({ path: `${path}.id`, code: 'duplicateId', detail: id })
    if (typeof id === 'string') seen.add(id)
    each(item, path)
  })
}

/** Parses and validates the overrides JSON. Empty text means "no overrides" (`{}`). */
export function parseCustom(text: string): ParseResult {
  const trimmed = text.trim()
  if (trimmed === '') return { value: {}, issues: [] }
  let data: unknown
  try {
    data = JSON.parse(trimmed)
  } catch (e) {
    return {
      value: null,
      issues: [{ path: '$', code: 'json', detail: e instanceof Error ? e.message : String(e) }],
    }
  }
  if (data === null) return { value: {}, issues: [] }
  if (!isObject(data)) return { value: null, issues: [{ path: '$', code: 'root' }] }

  const issues: Issue[] = []
  for (const key of Object.keys(data))
    if (!(TOP_KEYS as readonly string[]).includes(key))
      issues.push({ path: key, code: 'unknownKey', detail: key })

  if (data.sources !== undefined) {
    if (!isObject(data.sources)) issues.push({ path: 'sources', code: 'expectObject' })
    else
      for (const [id, src] of Object.entries(data.sources)) {
        const path = `sources.${id}`
        if (!ID_RE.test(id)) issues.push({ path, code: 'badId', detail: id })
        if (!isObject(src)) {
          issues.push({ path, code: 'expectObject' })
          continue
        }
        if (src.name !== undefined && typeof src.name !== 'string')
          issues.push({ path: `${path}.name`, code: 'expectString' })
        if (src.order !== undefined && typeof src.order !== 'number')
          issues.push({ path: `${path}.order`, code: 'expectNumber' })
        if (src.urls !== undefined) {
          if (!isObject(src.urls)) issues.push({ path: `${path}.urls`, code: 'expectObject' })
          else
            for (const [k, u] of Object.entries(src.urls))
              if (typeof u !== 'string') issues.push({ path: `${path}.urls.${k}`, code: 'expectString' })
        }
        checkLocalized(src.description, `${path}.description`, issues)
      }
  }

  checkIdList(data.categories, 'categories', issues, (c, path) => {
    checkLocalized(c.name, `${path}.name`, issues)
    if (typeof c.default_target !== 'string' || !c.default_target)
      issues.push({ path: `${path}.default_target`, code: 'missingField', detail: 'default_target' })
    if (c.order !== undefined && typeof c.order !== 'number')
      issues.push({ path: `${path}.order`, code: 'expectNumber' })
    if (!isObject(c.rules))
      issues.push({
        path: `${path}.rules`,
        code: c.rules === undefined ? 'missingField' : 'expectObject',
        detail: 'rules',
      })
    else
      for (const [src, ref] of Object.entries(c.rules)) {
        const refs = Array.isArray(ref) ? ref : [ref]
        refs.forEach((r, j) => {
          if (!isObject(r))
            issues.push({
              path: `${path}.rules.${src}${Array.isArray(ref) ? `[${j}]` : ''}`,
              code: 'expectObject',
            })
        })
      }
  })

  checkIdList(data.templates, 'templates', issues, (tpl, path) => {
    checkLocalized(tpl.name, `${path}.name`, issues)
    if (tpl.categories !== undefined && !isObject(tpl.categories))
      issues.push({ path: `${path}.categories`, code: 'expectObject' })
    if (tpl.groups !== undefined) {
      if (!Array.isArray(tpl.groups)) issues.push({ path: `${path}.groups`, code: 'expectArray' })
      else
        tpl.groups.forEach((g, j) => {
          const gp = `${path}.groups[${j}]`
          if (!isObject(g)) issues.push({ path: gp, code: 'expectObject' })
          else {
            if (typeof g.name !== 'string' || !g.name)
              issues.push({ path: `${gp}.name`, code: 'missingField', detail: 'name' })
            if (typeof g.type !== 'string' || !GROUP_TYPES.includes(g.type))
              issues.push({ path: `${gp}.type`, code: 'badGroupType', detail: String(g.type) })
          }
        })
    }
  })

  checkIdList(data.mirrors, 'mirrors', issues, (m, path) => {
    checkLocalized(m.name, `${path}.name`, issues)
    if (typeof m.prefix !== 'string')
      issues.push({ path: `${path}.prefix`, code: 'missingField', detail: 'prefix' })
    else if (m.prefix !== '' && !/^https?:\/\/\S+\/$/.test(m.prefix))
      issues.push({ path: `${path}.prefix`, code: 'badPrefix', detail: m.prefix })
  })

  return { value: issues.length ? null : data, issues }
}

/** Pretty JSON for the editor (`{}` → empty object text). */
export function formatCustom(custom: RulesetsCustom | null | undefined): string {
  return JSON.stringify(custom ?? {}, null, 2)
}

function upsert<T extends { id: string }>(list: readonly T[] | undefined, item: T): T[] {
  const out = [...(list ?? [])]
  const i = out.findIndex((x) => x.id === item.id)
  if (i === -1) out.push(item)
  else out[i] = item
  return out
}

/** Adds (or replaces by id) a mirror in the overrides. */
export function upsertMirror(custom: RulesetsCustom, mirror: RulesetMirror): RulesetsCustom {
  return { ...custom, mirrors: upsert(custom.mirrors, mirror) }
}

/** Adds (or replaces by id) a category in the overrides. */
export function upsertCategory(custom: RulesetsCustom, category: RulesetCategoryFull): RulesetsCustom {
  return { ...custom, categories: upsert(custom.categories, category) }
}
