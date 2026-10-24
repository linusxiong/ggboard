/**
 * Every `help={t('…')}` in the admin console must resolve to a string in each locale. Renaming a page
 * `description` to `help` once collided with an existing `help.*` object (admin-payments), and the
 * ⓘ would have shown the raw key.
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

type Dict = { [key: string]: string | Dict }
const root = path.resolve(import.meta.dirname)
const i18n = path.resolve(root, '../../i18n')

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const file = path.join(dir, name)
    if (statSync(file).isDirectory()) return sources(file)
    return file.endsWith('.tsx') && !file.includes('.test.') ? [file] : []
  })
}

function lookup(dict: Dict, key: string): unknown {
  return key.split('.').reduce<unknown>((o, k) => (o as Dict | undefined)?.[k], dict)
}

const dicts = new Map<string, Dict>()
function dict(locale: string, ns: string): Dict {
  const id = `${locale}/${ns}`
  if (!dicts.has(id))
    dicts.set(id, JSON.parse(readFileSync(path.join(i18n, locale, `${ns}.json`), 'utf8')) as Dict)
  return dicts.get(id)!
}

describe('admin help texts', () => {
  const uses = sources(root).flatMap((file) => {
    const src = readFileSync(file, 'utf8')
    const ns = /useT\(\s*\[?\s*'([^']+)'/.exec(src)?.[1]
    return [...src.matchAll(/help=\{[^}]*?\bt\('([^']+)'/g)].map((m) => {
      const raw = m[1] ?? ''
      const [prefix, key] = raw.includes(':') ? raw.split(':') : [ns, raw]
      return { file: path.relative(root, file), ns: prefix ?? '', key: key ?? '' }
    })
  })

  it('finds the page and section help keys', () => {
    expect(uses.length).toBeGreaterThan(10)
  })

  it.each(['zh-CN', 'en'])('resolves each one to a string (%s)', (locale) => {
    for (const { file, ns, key } of uses) {
      const value = lookup(dict(locale, ns), key)
      expect(typeof value, `${file}: ${ns}:${key}`).toBe('string')
      expect(value, `${file}: ${ns}:${key}`).not.toBe('')
    }
  })
})
