import { describe, expect, it } from 'vitest'
import type { AdminKnowledge } from '@/lib/api/types'
import { filterKnowledge } from './knowledge'
import { applyOrder, moveId } from './reorder'

describe('reorder helpers', () => {
  it('swaps with the neighbour and keeps the full list', () => {
    expect(moveId([1, 2, 3], 2, -1)).toEqual([2, 1, 3])
    expect(moveId([1, 2, 3], 3, 1)).toBeNull()
    expect(moveId([1, 2, 3], 9, 1)).toBeNull()
  })

  it('skips hidden items when a filter is active', () => {
    // visible: 1 and 4 → moving 4 up swaps it with 1, items 2/3 keep their slots
    expect(moveId([1, 2, 3, 4], 4, -1, new Set([1, 4]))).toEqual([4, 2, 3, 1])
  })

  it('applies an id order to items', () => {
    const items = [{ id: 1 }, { id: 2 }, { id: 3 }]
    expect(applyOrder(items, [3, 1]).map((i) => i.id)).toEqual([3, 1, 2])
  })
})

describe('knowledge filter', () => {
  const k = (id: number, language: string, category: string, title: string): AdminKnowledge => ({
    id,
    language,
    category,
    title,
    show: true,
    sort: id,
    created_at: '',
    updated_at: '',
  })
  const items = [
    k(1, 'zh-CN', '入门', 'Clash 教程'),
    k(2, 'en', 'Getting started', 'Clash guide'),
    k(3, 'en', 'FAQ', 'Billing'),
  ]
  it('filters by language, category and text', () => {
    expect(filterKnowledge(items, { language: 'en' }).map((i) => i.id)).toEqual([2, 3])
    expect(filterKnowledge(items, { language: 'en', category: 'FAQ' }).map((i) => i.id)).toEqual([3])
    expect(filterKnowledge(items, { q: 'clash' }).map((i) => i.id)).toEqual([1, 2])
  })
})
