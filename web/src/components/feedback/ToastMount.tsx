import { lazy, Suspense, useSyncExternalStore } from 'react'
import { createPortal } from 'react-dom'
import { LIVE_REGION_ID } from '@/lib/announce'
import { useIdle } from '@/lib/motion'
import { toastStore } from '@/lib/toast'

const ToastHost = lazy(() => import('./ToastHost'))

/**
 * The live region `announce()` writes to (`lib/announce.ts`). Rendered with the app, long before the first
 * message, and as a child of `<body>` marked `data-react-aria-top-layer`: React Aria makes everything else
 * inert while a modal is open, and a message raised from inside a dialog still has to be read.
 */
export function LiveRegion() {
  return createPortal(
    <div id={LIVE_REGION_ID} aria-live="polite" className="sr-only" data-react-aria-top-layer="" />,
    document.body,
  )
}

/** Mounts the toast region the first time a toast is requested (or after idle, so it is ready). */
export function ToastMount() {
  const requested = useSyncExternalStore(toastStore.subscribe, toastStore.isRequested, () => false)
  const idle = useIdle(5000)
  return (
    <>
      <LiveRegion />
      {requested || idle ? (
        <Suspense fallback={null}>
          <ToastHost />
        </Suspense>
      ) : null}
    </>
  )
}
