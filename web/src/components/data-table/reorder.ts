/** Pure helpers for drag-to-reorder rows (no React Aria import: `DataTable` uses these too). */

/** Marks a `<DragHandle>`; DataTable lets pointer drags start only inside such an element. */
export const DRAG_HANDLE_ATTR = 'data-drag-handle'

/**
 * New order after dropping `moved` (row ids, kept in their current relative order) before or after
 * `target`. Returns a copy of `ids` unchanged when the drop is a no-op (target is one of the moved
 * rows, unknown ids, nothing moved). Ids in `moved` that are not in `ids` are ignored.
 */
export function reorderIds<K>(
  ids: readonly K[],
  moved: Iterable<K>,
  target: K,
  position: 'before' | 'after',
): K[] {
  const movedSet = new Set(moved)
  if (movedSet.has(target)) return [...ids]
  const block = ids.filter((id) => movedSet.has(id))
  const rest = ids.filter((id) => !movedSet.has(id))
  const at = rest.indexOf(target)
  if (block.length === 0 || at < 0) return [...ids]
  rest.splice(position === 'before' ? at : at + 1, 0, ...block)
  return rest
}

/** Whether two id lists have the same order. */
export function sameOrder<K>(a: readonly K[], b: readonly K[]): boolean {
  return a.length === b.length && a.every((id, i) => id === b[i])
}
