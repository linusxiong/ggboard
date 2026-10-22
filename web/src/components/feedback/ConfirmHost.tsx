import { lazy, Suspense } from 'react'
import { useConfirmStore } from './confirm'

const ConfirmDialog = lazy(() => import('./ConfirmDialog'))

/** Renders pending `confirm()` requests. Mount once per layout. */
export function ConfirmHost() {
  const current = useConfirmStore((s) => s.current)
  const settle = useConfirmStore((s) => s.settle)
  if (!current) return null
  return (
    <Suspense fallback={null}>
      <ConfirmDialog
        key={current.id}
        isOpen
        onOpenChange={(open) => {
          if (!open) settle(current.id, false)
        }}
        title={current.title}
        description={current.description}
        confirmLabel={current.confirmLabel}
        cancelLabel={current.cancelLabel}
        tone={current.tone ?? 'primary'}
        onConfirm={() => settle(current.id, true)}
      />
    </Suspense>
  )
}
