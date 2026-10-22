import { createRouter } from '@tanstack/react-router'
import { LoadingPage } from '@/components/LoadingPage'
import { NotFound } from '@/components/NotFound'
import { RouteError } from '@/components/RouteError'
import { setUnauthorizedHandler } from '@/lib/api/client'
import { routeTree } from './routeTree.gen'

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  // Let React Query decide freshness (authenticated areas); the router just triggers loaders.
  defaultPreloadStaleTime: 0,
  defaultPendingComponent: () => <LoadingPage />,
  defaultPendingMs: 400,
  defaultPendingMinMs: 200,
  defaultErrorComponent: RouteError,
  defaultNotFoundComponent: NotFound,
  scrollRestoration: true,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

// 401 on an authenticated request → session cleared in the API client → back to login.
setUnauthorizedHandler(() => {
  const { pathname, searchStr } = router.state.location
  if (pathname === '/login') return
  void router.navigate({ to: '/login', search: { redirect: `${pathname}${searchStr}` }, replace: true })
})
