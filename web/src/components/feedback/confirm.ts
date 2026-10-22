/**
 * Imperative confirmation:
 *
 *   if (await confirm({ title: t('delete.title'), tone: 'danger' })) remove.mutate(id)
 *
 * Requires `<ConfirmHost/>` (mounted by the `_user` and `_admin` layouts). The dialog code is
 * lazy-loaded on first use.
 */
import type { ReactNode } from 'react'
import { create } from 'zustand'

export interface ConfirmOptions {
  title: ReactNode
  description?: ReactNode
  confirmLabel?: ReactNode
  cancelLabel?: ReactNode
  tone?: 'primary' | 'danger'
}

interface ConfirmRequest extends ConfirmOptions {
  id: number
  resolve: (ok: boolean) => void
}

interface ConfirmState {
  current: ConfirmRequest | null
  queue: ConfirmRequest[]
  /** Resolves request `id` (no-op if it is no longer current) and shows the next one. */
  settle: (id: number, ok: boolean) => void
}

let seq = 0

export const useConfirmStore = create<ConfirmState>()((set, get) => ({
  current: null,
  queue: [],
  settle: (id, ok) => {
    const { current, queue } = get()
    if (!current || current.id !== id) return
    current.resolve(ok)
    const [next = null, ...rest] = queue
    set({ current: next, queue: rest })
  },
}))

export function confirm(options: ConfirmOptions): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    const request: ConfirmRequest = { ...options, id: ++seq, resolve }
    const { current, queue } = useConfirmStore.getState()
    if (current) useConfirmStore.setState({ queue: [...queue, request] })
    else useConfirmStore.setState({ current: request })
  })
}
