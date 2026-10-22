/**
 * Toasts backed by HeroUI's `toast` queue, loaded lazily so the toast UI never weighs on the
 * initial bundle. `<ToastMount/>` (in the root route) mounts `Toast.Provider` on first use or idle.
 *
 *   import { toast } from '@/lib/toast'
 *   toast.success(t('saved'))
 *   toast.danger(errorMessage(err))
 *   toast.promise(save(), { loading: '…', success: 'Saved', error: (e) => errorMessage(e) })
 */
import type { ReactNode } from 'react'

type ToastModule = typeof import('@heroui/react/toast')
type Options = Parameters<ToastModule['toast']>[1]
type Variant = 'default' | 'success' | 'danger' | 'info' | 'warning'

let modulePromise: Promise<ToastModule> | null = null
let requested = false
const listeners = new Set<() => void>()

/** @internal used by ToastMount */
export const toastHost = {
  isRequested: () => requested,
  subscribe(cb: () => void) {
    listeners.add(cb)
    return () => listeners.delete(cb)
  },
}

function load(): Promise<ToastModule> {
  if (!requested) {
    requested = true
    listeners.forEach((cb) => cb())
  }
  modulePromise ??= import('@heroui/react/toast')
  return modulePromise
}

function show(variant: Variant, message: ReactNode, options?: Options) {
  void load().then(({ toast: heroToast }) => {
    if (variant === 'default') heroToast(message, options)
    else heroToast[variant](message, options)
  })
}

export const toast = Object.assign((message: ReactNode, options?: Options) => show('default', message, options), {
  success: (message: ReactNode, options?: Options) => show('success', message, options),
  danger: (message: ReactNode, options?: Options) => show('danger', message, options),
  /** Alias of `danger`. */
  error: (message: ReactNode, options?: Options) => show('danger', message, options),
  info: (message: ReactNode, options?: Options) => show('info', message, options),
  warning: (message: ReactNode, options?: Options) => show('warning', message, options),
  promise<T>(
    promise: Promise<T>,
    messages: { loading: ReactNode; success: ReactNode | ((value: T) => ReactNode); error: ReactNode | ((error: unknown) => ReactNode) },
  ): Promise<T> {
    void load().then(({ toast: heroToast }) => {
      heroToast.promise(promise, messages as Parameters<ToastModule['toast']['promise']>[1])
    })
    return promise
  },
})

export type Toaster = typeof toast

/** Hook form for components (stable reference). */
export function useToast(): Toaster {
  return toast
}
