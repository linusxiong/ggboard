/**
 * The app's QueryClient. Only ever imported dynamically (`withQueryClient` in the `_user` /
 * `_admin` layouts' `beforeLoad`), which puts it into the router context — `context.queryClient`
 * in every child loader, `Route.useRouteContext()` in the layout → `QueryClientProvider` in
 * `AppShell`. Public pages never pay for React Query.
 *
 * Session boundary: the cache is bound to the auth store (`bindSessionCache`), so every token change drops what
 * the previous account cached — no call site has to remember it.
 */
import { type Query, QueryClient } from '@tanstack/react-query'
import { isApiError } from '@/lib/api/client'
import { useAuthStore } from '@/stores/auth'

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

/**
 * `qk.guest.*` (`['guest', …]`) holds `/guest/*` data — site config, public plans, rule-set catalog — which is the
 * same for every visitor (the server never reads the session there). It survives a session change so the shell
 * does not flash; every other key is treated as private to the account that fetched it.
 */
export function isPublicQuery(query: Pick<Query, 'queryKey'>): boolean {
  return query.queryKey[0] === 'guest'
}
const isPrivateQuery = (query: Query) => !isPublicQuery(query)

/**
 * Drops everything cached for the account that just left. Runs on every token change: sign-out (logout, a 401
 * from the API, password change, an expired token, another tab signing out) and sign-in (login, register, MFA,
 * passkey, admin "log in as", another tab signing in).
 *
 * - In-flight private queries are cancelled (and their requests aborted), so a late answer for the old account
 *   cannot land in the cache.
 * - Mutations are cleared (their results hold secrets such as a reset subscription token or a one-time login link).
 * - Private queries nobody observes are removed.
 * - Observed ones (views still on screen) depend on the direction. Signing in (another tab switched accounts):
 *   reset to empty and refetched with the new token. Signing out: the shell stays mounted until the router reaches
 *   `/login`, and blanking it would only make it refetch without a token (a pointless 401), so they are removed the
 *   moment their last observer unmounts (see `bindSessionCache`); the next sign-in clears anything left regardless.
 */
export function resetSessionCache(client: QueryClient, { signedIn }: { signedIn: boolean }) {
  void client.cancelQueries({ predicate: isPrivateQuery })
  client.getMutationCache().clear()
  client.removeQueries({ predicate: (query) => isPrivateQuery(query) && query.getObserversCount() === 0 })
  if (signedIn) void client.resetQueries({ predicate: isPrivateQuery })
}

/**
 * Ties `client` to the session: `resetSessionCache` on every token change (also the cross-tab `storage` sync,
 * which writes the store directly), and while signed out no private query outlives its last observer.
 * Returns the unbind function.
 */
export function bindSessionCache(client: QueryClient): () => void {
  const offAuth = useAuthStore.subscribe((state, prev) => {
    if (state.token !== prev.token) resetSessionCache(client, { signedIn: state.token !== null })
  })
  const cache = client.getQueryCache()
  const offCache = cache.subscribe((event) => {
    if (
      event.type === 'observerRemoved' &&
      !useAuthStore.getState().token &&
      !isPublicQuery(event.query) &&
      event.query.getObserversCount() === 0
    )
      cache.remove(event.query)
  })
  return () => {
    offAuth()
    offCache()
  }
}

export const queryClient = createQueryClient()
bindSessionCache(queryClient)
