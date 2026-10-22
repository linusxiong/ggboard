/**
 * The app's QueryClient. Loaded lazily by the authenticated layouts (`_user`, `_admin`), which
 * put it into the router context (`context.queryClient` in every child loader) and provide it
 * to React (`QueryClientProvider` in `AppShell`). Public pages never pay for React Query.
 */
import { QueryClient } from '@tanstack/react-query'
import { isApiError } from '@/lib/api/client'

/** Do not retry client errors (4xx); retry network/5xx twice. */
function retry(failureCount: number, error: unknown) {
  if (isApiError(error) && error.status >= 400 && error.status < 500) return false
  return failureCount < 2
}

export function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { staleTime: 30_000, gcTime: 5 * 60_000, retry, refetchOnWindowFocus: true },
      mutations: { retry: false },
    },
  })
}

export const queryClient = createQueryClient()

/** For `beforeLoad`: `const { queryClient } = await loadQueryClient()`. */
export const loadQueryClient = () => import('@/lib/queryClient')
