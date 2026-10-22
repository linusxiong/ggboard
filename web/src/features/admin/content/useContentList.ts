import { type QueryKey, useMutation, useQueryClient } from '@tanstack/react-query'
import type { ID, Page } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { applyOrder } from './reorder'

interface Options {
  /** Query key of the (unpaginated) `Page<T>` list. */
  queryKey: QueryKey
  /** Prefix invalidated after every change. */
  invalidate: QueryKey
  setShow: (id: ID, show: boolean) => Promise<unknown>
  sort: (ids: ID[]) => Promise<unknown>
}

/** Optimistic show toggle + reorder for notices and knowledge (sort = full id list). */
export function useContentList<T extends { id: ID; show: boolean }>({
  queryKey,
  invalidate,
  setShow,
  sort,
}: Options) {
  const qc = useQueryClient()
  const snapshot = async () => {
    await qc.cancelQueries({ queryKey })
    return qc.getQueryData<Page<T>>(queryKey)
  }
  const rollback = (prev: Page<T> | undefined, e: unknown) => {
    if (prev) qc.setQueryData(queryKey, prev)
    toast.danger(errorMessage(e))
  }

  const toggle = useMutation({
    mutationFn: ({ id, show }: { id: ID; show: boolean }) => setShow(id, show),
    onMutate: async ({ id, show }) => {
      const prev = await snapshot()
      qc.setQueryData<Page<T>>(
        queryKey,
        (p) => p && { ...p, items: p.items.map((i) => (i.id === id ? { ...i, show } : i)) },
      )
      return { prev }
    },
    onError: (e, _v, ctx) => rollback(ctx?.prev, e),
    onSettled: () => qc.invalidateQueries({ queryKey: invalidate }),
  })

  const reorder = useMutation({
    mutationFn: (ids: ID[]) => sort(ids),
    onMutate: async (ids) => {
      const prev = await snapshot()
      qc.setQueryData<Page<T>>(queryKey, (p) => p && { ...p, items: applyOrder(p.items, ids) })
      return { prev }
    },
    onError: (e, _v, ctx) => rollback(ctx?.prev, e),
    onSettled: () => qc.invalidateQueries({ queryKey: invalidate }),
  })

  return { toggle, reorder }
}
