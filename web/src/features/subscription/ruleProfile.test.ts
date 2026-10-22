import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import type { RulesetCatalog } from '@/lib/api/types'
import {
  addGroup,
  applyTemplate,
  bandOf,
  effectiveSource,
  errorKey,
  findTemplate,
  moveGroup,
  normalizeProfile,
  removeGroup,
  renameError,
  renameGroup,
  sameProfile,
  sortedSources,
  sourceCoverage,
  toggleCategory,
  updateGroup,
  validateProfile,
} from './ruleProfile'

// The real embedded catalog the builder renders.
const catalog = JSON.parse(
  readFileSync(
    path.resolve(import.meta.dirname, '../../../../internal/subscription/rulesets/catalog.json'),
    'utf8',
  ),
) as RulesetCatalog
const standard = () => applyTemplate(findTemplate(catalog, 'standard')!, { source: 'blackmatrix7' })
const category = (id: string) => catalog.categories.find((c) => c.id === id)!

describe('rule profile helpers', () => {
  it('applies a template but keeps source, mirror and DNS', () => {
    const base = { ...standard(), mirror: 'https://ghfast.top/', dns: 'fakeip' as const }
    const minimal = applyTemplate(findTemplate(catalog, 'minimal')!, base)
    expect(minimal.template).toBe('minimal')
    expect(minimal.categories).toEqual({ lan: 'DIRECT', cn: 'DIRECT' })
    expect(minimal.source).toBe('blackmatrix7')
    expect(minimal.mirror).toBe('https://ghfast.top/')
    expect(minimal.dns).toBe('fakeip')
    expect(validateProfile(minimal)).toEqual({})
  })

  it('toggles categories with their default target, falling back to the final policy', () => {
    const p = standard()
    expect(toggleCategory(p, category('twitter'), true).categories.twitter).toBe('Proxy')
    // `steam` → Games, which the standard template lacks → final policy.
    expect(toggleCategory(p, category('steam'), true).categories.steam).toBe('Proxy')
    const off = toggleCategory(p, category('openai'), false)
    expect('openai' in off.categories).toBe(false)
    expect(sameProfile(toggleCategory(off, category('openai'), true), p)).toBe(true)
  })

  it('renames a group everywhere it is referenced', () => {
    const p = standard()
    const ai = p.groups.findIndex((g) => g.name === 'AI')
    const renamed = renameGroup(p, ai, 'LLM')
    expect(renamed.groups[ai]?.name).toBe('LLM')
    expect(renamed.categories.openai).toBe('LLM')
    expect(renameError(p, ai, 'Proxy')).toBe('duplicate')
    expect(renameError(p, ai, 'direct')).toBe('reserved')
    expect(renameError(p, 0, 'Main')).toBe('proxy')
    expect(renameGroup(p, ai, 'Streaming')).toBe(p)
  })

  it('removes a group, retargeting its categories and include tokens', () => {
    const p = renameGroup(standard(), 0, 'Proxy')
    const streaming = p.groups.findIndex((g) => g.name === 'Streaming')
    const out = removeGroup(p, streaming)
    expect(out.groups.some((g) => g.name === 'Streaming')).toBe(false)
    expect(out.categories.netflix).toBe('Proxy')
    expect(removeGroup(p, 0)).toBe(p) // Proxy stays
    const auto = p.groups.findIndex((g) => g.name === 'auto')
    expect(removeGroup(p, auto).groups[0]?.include).toBe('*')
  })

  it('adds, moves and edits groups; probe groups get url + interval', () => {
    let p = addGroup(standard(), 'Work')
    const at = p.groups.findIndex((g) => g.name === 'Work')
    expect(p.groups[p.groups.length - 1]?.name).toBe('auto') // inserted before the trailing probe group
    p = moveGroup(p, at, -1)
    expect(p.groups[at - 1]?.name).toBe('Work')
    p = updateGroup(p, at - 1, { type: 'url-test' })
    expect(p.groups[at - 1]).toMatchObject({ type: 'url-test', interval: 300 })
    expect(p.groups[at - 1]?.url).toMatch(/^http/)
    p = updateGroup(p, at - 1, { interval: 1 })
    expect(validateProfile(p)[`groups.${at - 1}.interval`]).toBe('interval.invalid')
    p = updateGroup(p, at - 1, { type: 'select' })
    expect(p.groups[at - 1]).not.toHaveProperty('url')
  })

  it('normalises, compares and maps server error keys', () => {
    const p = standard()
    expect(
      normalizeProfile({ ...p, categories: { ...p.categories, ads: false } }).categories,
    ).not.toHaveProperty('ads')
    expect(
      sameProfile(p, { ...p, categories: Object.fromEntries(Object.entries(p.categories).reverse()) }),
    ).toBe(true)
    expect(errorKey('groups[2].name')).toBe('groups.2.name')
  })

  it('knows bands, source order and fallbacks from the catalog', () => {
    expect(bandOf(10)).toBe('basics')
    expect(bandOf(90)).toBe('regions')
    expect(sortedSources(catalog)[0]).toBe('blackmatrix7')
    expect(effectiveSource(catalog, category('applications'), 'blackmatrix7')).toBe('loyalsoldier')
    expect(effectiveSource(catalog, category('openai'), 'metacubex')).toBe('metacubex')
  })

  it('reads the public catalog shape (categories list `sources`, no `rules`)', () => {
    // GET /guest|user/rulesets/catalog (docs/SUBSCRIPTION.md): no URL templates, `sources: [...]` per category.
    const pub: RulesetCatalog = {
      ...catalog,
      categories: catalog.categories.map(({ rules, ...c }) => ({ ...c, sources: Object.keys(rules ?? {}) })),
    }
    const full = catalog.categories.filter((c) => 'metacubex' in (c.rules ?? {})).length
    expect(full).toBeGreaterThan(0)
    expect(sourceCoverage(pub, 'metacubex')).toBe(full)
    expect(sourceCoverage(catalog, 'metacubex')).toBe(full)
    const apps = pub.categories.find((c) => c.id === 'applications')!
    expect(effectiveSource(pub, apps, 'blackmatrix7')).toBe('loyalsoldier')
  })
})
