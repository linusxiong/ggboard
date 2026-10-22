import { lazy, Suspense, useSyncExternalStore } from 'react'
import { useIdle } from '@/lib/motion'
import { toastStore } from '@/lib/toast'

const ToastHost = lazy(() => import('./ToastHost'))

/** Mounts the toast region the first time a toast is requested (or after idle, so it is ready). */
export function ToastMount() {
  const requested = useSyncExternalStore(toastStore.subscribe, toastStore.isRequested, () => false)
  const idle = useIdle(5000)
  if (!requested && !idle) return null
  return (
    <Suspense fallback={null}>
      <ToastHost />
    </Suspense>
  )
}
