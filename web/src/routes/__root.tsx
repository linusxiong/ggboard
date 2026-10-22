/**
 * Root route — kept tiny (never code-split): Suspense for lazily loaded dictionaries and route
 * chunks, the lazy toast region and the 404. Theme + `<html lang>` are bootstrapped in
 * `main.tsx`. React Query + React Aria providers live in the authenticated shells (`AppShell`),
 * so the public pages (landing / auth) stay inside the first-load budget.
 */
import { createRootRoute, Outlet } from '@tanstack/react-router'
import { Suspense } from 'react'
import { ToastMount } from '@/components/feedback/ToastMount'
import { LoadingPage } from '@/components/LoadingPage'
import { NotFound } from '@/components/NotFound'

export const Route = createRootRoute({
  component: RootComponent,
  notFoundComponent: NotFound,
})

function RootComponent() {
  return (
    <>
      <Suspense fallback={<LoadingPage fullscreen />}>
        <Outlet />
      </Suspense>
      <ToastMount />
    </>
  )
}
