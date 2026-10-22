import type { ErrorComponentProps } from '@tanstack/react-router'
import { lazy, Suspense } from 'react'

const RouteErrorPage = lazy(() => import('./RouteErrorPage'))

/** Router `errorComponent` — lazy body, self-contained Suspense (works for any route level). */
export function RouteError(props: ErrorComponentProps) {
  return (
    <Suspense fallback={<div role="alert" className="min-h-[50vh]" />}>
      <RouteErrorPage {...props} />
    </Suspense>
  )
}
