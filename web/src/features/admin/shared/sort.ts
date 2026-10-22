/** Returns a copy of `ids` with `id` moved one step in `direction` (no-op at the edges). */
export function moveId<T>(ids: readonly T[], id: T, direction: -1 | 1): T[] {
  const next = [...ids]
  const from = next.indexOf(id)
  const to = from + direction
  if (from < 0 || to < 0 || to >= next.length) return next
  next.splice(from, 1)
  next.splice(to, 0, id)
  return next
}
