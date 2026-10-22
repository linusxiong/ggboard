import { lazy, Suspense, useSyncExternalStore } from 'react'
import { useIdle } from '@/lib/motion'
import { toastHost } from '@/lib/toast'

const ToastHost = lazy(() => import('./ToastHost'))

/** Mounts the HeroUI toast region the first time a toast is requested (or after idle). */
export function ToastMount() {
  const requested = useSyncExternalStore(toastHost.subscribe, toastHost.isRequested, () => false)
  const idle = useIdle(5000)
  if (!requested && !idle) return null
  return (
    <Suspense fallback={null}>
      <ToastHost />
    </Suspense>
  )
}
