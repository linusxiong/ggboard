import type { SortingState } from '@tanstack/react-table'

/** `-created_at` → `[{ id: 'created_at', desc: true }]` (API `sort` param, docs/ARCHITECTURE.md). */
export function parseSort(sort: string | undefined | null): SortingState {
  if (!sort) return []
  const desc = sort.startsWith('-')
  const id = desc ? sort.slice(1) : sort
  return id ? [{ id, desc }] : []
}

/** Inverse of `parseSort`. */
export function toSortParam(sorting: SortingState): string | undefined {
  const first = sorting[0]
  if (!first) return undefined
  return first.desc ? `-${first.id}` : first.id
}

/** Page numbers with ellipses: [1, '…', 4, 5, 6, '…', 20]. */
export function pageWindow(page: number, pageCount: number, siblings = 1): Array<number | 'ellipsis'> {
  if (pageCount <= 0) return []
  const total = siblings * 2 + 5
  if (pageCount <= total) return Array.from({ length: pageCount }, (_, i) => i + 1)
  const start = Math.max(2, page - siblings)
  const end = Math.min(pageCount - 1, page + siblings)
  const out: Array<number | 'ellipsis'> = [1]
  if (start > 2) out.push('ellipsis')
  for (let p = start; p <= end; p++) out.push(p)
  if (end < pageCount - 1) out.push('ellipsis')
  out.push(pageCount)
  return out
}
