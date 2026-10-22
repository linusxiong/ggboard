import type { ID } from '@/lib/api/types'

/**
 * Moves `id` one step up (-1) or down (+1) in the full id list. When `visible` is given (a filtered
 * view), the item swaps with its nearest *visible* neighbour so the move is what the admin sees,
 * while every other id keeps its position — `/sort {ids}` always receives the whole list.
 * Returns `null` when the move is impossible (edge of the list or unknown id).
 */
export function moveId(ids: readonly ID[], id: ID, dir: -1 | 1, visible?: ReadonlySet<ID>): ID[] | null {
  const from = ids.indexOf(id)
  if (from === -1) return null
  let to = from + dir
  while (to >= 0 && to < ids.length && visible && !visible.has(ids[to] as ID)) to += dir
  if (to < 0 || to >= ids.length) return null
  const next = [...ids]
  next[from] = ids[to] as ID
  next[to] = id
  return next
}

/** Reorders `items` to follow `ids` (unknown ids are dropped, missing items appended). */
export function applyOrder<T extends { id: ID }>(items: readonly T[], ids: readonly ID[]): T[] {
  const byId = new Map(items.map((i) => [i.id, i]))
  const out: T[] = []
  for (const id of ids) {
    const item = byId.get(id)
    if (item) {
      out.push(item)
      byId.delete(id)
    }
  }
  return [...out, ...byId.values()]
}
