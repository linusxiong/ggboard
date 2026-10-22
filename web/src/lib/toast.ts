/**
 * App-wide toasts (Apple-style capsules). One API for every status:
 *
 *   import { toast } from '@/lib/toast'
 *   toast.success(t('saved'))
 *   toast.error(errorMessage(err))            // alias: toast.danger
 *   toast.info('…'); toast.warning('…'); toast('neutral text')
 *   toast.success(t('saved'), { description: t('savedHint'), timeout: 0 })   // 0 = stays until dismissed
 *   toast.promise(save(), { loading: '…', success: 'Saved', error: (e) => errorMessage(e) })
 *   toast.dismiss(id)
 *
 * This module is a dependency-free store (it ships in the first-load bundle); the renderer
 * (`components/feedback/ToastHost.tsx`) is lazy-loaded by `<ToastMount/>` on first use or after idle.
 * At most three toasts are visible; repeating the same message refreshes it instead of stacking.
 */
import type { ReactNode } from 'react'

export type ToastTone = 'neutral' | 'success' | 'error' | 'warning' | 'info' | 'loading'

export interface ToastOptions {
  /** Second line under the message. */
  description?: ReactNode
  /** Milliseconds before auto-dismiss; `0` keeps the toast until it is dismissed. */
  timeout?: number
  /** Stable id: a later toast with the same id replaces this one (used by `promise`). */
  id?: string | number
}

export interface ToastRecord {
  id: string
  tone: ToastTone
  message: ReactNode
  description?: ReactNode
  /** 0 = persistent. */
  timeout: number
  /** Changes whenever the toast is (re)shown, so the renderer restarts its timer. */
  version: number
}

/** Visible at once (older ones are dropped). */
export const MAX_TOASTS = 3
const DEFAULT_TIMEOUT: Record<ToastTone, number> = {
  neutral: 3200,
  success: 3200,
  info: 3600,
  warning: 4000,
  error: 4000,
  loading: 0,
}

let items: readonly ToastRecord[] = []
let seq = 0
let requested = false
const listeners = new Set<() => void>()
const emit = () => listeners.forEach((cb) => cb())

/** @internal store used by ToastHost / ToastMount (and tests). */
export const toastStore = {
  subscribe: (cb: () => void) => {
    listeners.add(cb)
    return () => {
      listeners.delete(cb)
    }
  },
  getSnapshot: (): readonly ToastRecord[] => items,
  /** True once any toast was requested (mounts the lazy renderer early). */
  isRequested: () => requested,
  dismiss: (id: string) => {
    if (!items.some((t) => t.id === id)) return
    items = items.filter((t) => t.id !== id)
    emit()
  },
  /** Removes every toast (tests, sign-out). */
  clear: () => {
    if (items.length === 0) return
    items = []
    emit()
  },
}

function push(tone: ToastTone, message: ReactNode, options: ToastOptions = {}): string {
  requested = true
  const record: ToastRecord = {
    id: options.id !== undefined ? String(options.id) : `toast-${++seq}`,
    tone,
    message,
    description: options.description,
    timeout: options.timeout ?? DEFAULT_TIMEOUT[tone],
    version: ++seq,
  }
  const same = items.findIndex(
    (t) =>
      t.id === record.id ||
      (options.id === undefined && typeof message === 'string' && t.tone === tone && t.message === message),
  )
  if (same >= 0) {
    const id = items[same]!.id
    items = items.map((t, i) => (i === same ? { ...record, id } : t))
    emit()
    return id
  }
  items = [...items, record].slice(-MAX_TOASTS)
  emit()
  return record.id
}

type Show = (message: ReactNode, options?: ToastOptions) => string

const error: Show = (message, options) => push('error', message, options)

export const toast = Object.assign(((message, options) => push('neutral', message, options)) as Show, {
  success: ((message, options) => push('success', message, options)) as Show,
  error,
  /** Alias of `error` (HeroUI naming, kept for existing call sites). */
  danger: error,
  info: ((message, options) => push('info', message, options)) as Show,
  warning: ((message, options) => push('warning', message, options)) as Show,
  dismiss: (id: string) => toastStore.dismiss(id),
  promise<T>(
    promise: Promise<T>,
    messages: {
      loading: ReactNode
      success: ReactNode | ((value: T) => ReactNode)
      error: ReactNode | ((error: unknown) => ReactNode)
    },
  ): Promise<T> {
    const id = push('loading', messages.loading)
    promise.then(
      (value) => {
        const m = messages.success
        push('success', typeof m === 'function' ? m(value) : m, { id })
      },
      (e: unknown) => {
        const m = messages.error
        push('error', typeof m === 'function' ? m(e) : m, { id })
      },
    )
    return promise
  },
})

export type Toaster = typeof toast

/** Hook form for components (stable reference). */
export function useToast(): Toaster {
  return toast
}
