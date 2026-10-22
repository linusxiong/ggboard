import { lazy, Suspense } from 'react'
import { LoadingPage } from './LoadingPage'

const NotFoundPage = lazy(() => import('./NotFoundPage'))

/** 404 (router `notFoundComponent`). The page body is lazy: it is rarely needed. */
export function NotFound() {
  return (
    <Suspense fallback={<LoadingPage />}>
      <NotFoundPage />
    </Suspense>
  )
}
