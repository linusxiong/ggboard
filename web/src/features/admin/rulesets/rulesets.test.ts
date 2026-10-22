import { describe, expect, it } from 'vitest'
import { formatCustom, parseCustom, upsertCategory, upsertMirror } from './custom'

const codes = (text: string) => parseCustom(text).issues.map((i) => `${i.path}:${i.code}`)

describe('rule-set overrides validation', () => {
  it('accepts empty text, null and a valid document', () => {
    expect(parseCustom('')).toEqual({ value: {}, issues: [] })
    expect(parseCustom('null')).toEqual({ value: {}, issues: [] })
    const doc = {
      sources: { mysrc: { name: 'me/rules', order: 70, urls: { clash: 'https://example.com/{name}.yaml' } } },
      categories: [
        {
          id: 'my-ai',
          name: { 'zh-CN': '我的 AI', en: 'My AI' },
          default_target: 'AI',
          order: 33,
          rules: { mysrc: { name: 'AI' } },
        },
      ],
      templates: [
        {
          id: 'mine',
          name: { en: 'Mine' },
          categories: { 'my-ai': 'AI' },
          groups: [{ name: 'Proxy', type: 'select', include: '*' }],
          final: 'Proxy',
          geoip_cn_direct: true,
        },
      ],
      mirrors: [{ id: 'my-mirror', name: { en: 'Mine' }, prefix: 'https://mirror.example.com/' }],
    }
    const result = parseCustom(JSON.stringify(doc))
    expect(result.issues).toEqual([])
    expect(result.value).toEqual(doc)
  })

  it('reports JSON syntax errors and a non-object root', () => {
    expect(codes('{')).toEqual(['$:json'])
    expect(codes('[1]')).toEqual(['$:root'])
  })

  it('reports unknown keys, wrong shapes, ids and required fields', () => {
    expect(
      codes(
        JSON.stringify({
          extra: 1,
          sources: [],
          categories: [
            { id: 'A b', rules: {} },
            { id: 'x', default_target: 'DIRECT', rules: { s: 'nope' } },
            { id: 'x', default_target: 'P', rules: {} },
          ],
          templates: [{ id: 'standard', groups: [{ name: 'Proxy', type: 'round-robin' }] }],
          mirrors: [{ id: 'm', prefix: 'ftp://x' }, { id: 'n' }],
        }),
      ),
    ).toEqual([
      'extra:unknownKey',
      'sources:expectObject',
      'categories[0].id:badId',
      'categories[0].default_target:missingField',
      'categories[1].rules.s:expectObject',
      'categories[2].id:duplicateId',
      'templates[0].groups[0].type:badGroupType',
      'mirrors[0].prefix:badPrefix',
      'mirrors[1].prefix:missingField',
    ])
    expect(parseCustom('{"categories":{}}').value).toBeNull()
  })

  it('upserts mirrors and categories by id (form helpers)', () => {
    const base = { mirrors: [{ id: 'a', prefix: 'https://a.example.com/' }] }
    const added = upsertMirror(base, { id: 'b', prefix: 'https://b.example.com/' })
    expect(added.mirrors?.map((m) => m.id)).toEqual(['a', 'b'])
    const replaced = upsertMirror(added, { id: 'a', prefix: 'https://a2.example.com/' })
    expect(replaced.mirrors?.[0]?.prefix).toBe('https://a2.example.com/')
    const withCat = upsertCategory(
      {},
      { id: 'c', default_target: 'DIRECT', order: 1, rules: { metacubex: { geosite: 'c' } } },
    )
    expect(withCat.categories).toHaveLength(1)
    expect(parseCustom(formatCustom(withCat)).issues).toEqual([])
  })
})
