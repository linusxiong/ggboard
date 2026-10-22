/**
 * Route guards for `beforeLoad`. They throw TanStack Router redirects.
 *
 *   export const Route = createFileRoute('/_admin/admin/users')({ beforeLoad: requireAdmin, … })
 *
 * The `_user` / `_admin` layouts use `withQueryClient(...)`: they load React Query lazily and
 * add `queryClient` to the router context of every child route.
 */
import type { QueryClient } from '@tanstack/react-query'
import { redirect } from '@tanstack/react-router'
import { me as fetchMe } from '@/lib/api/user'
import type { User } from '@/lib/api/types'
import { isAdmin, isStaff, useAuthStore } from '@/stores/auth'

interface GuardArgs {
  location: { href: string }
  context?: { queryClient?: QueryClient }
}

/** Only same-origin absolute paths are accepted as post-login targets (no open redirects). */
export function safeRedirect(target: unknown, fallback = '/dashboard'): string {
  if (typeof target !== 'string' || !target.startsWith('/') || target.startsWith('//') || target.startsWith('/\\'))
    return fallback
  return target
}

export function homeFor(user: User | null | undefined): string {
  return isStaff(user) ? '/admin/dashboard' : '/dashboard'
}

/** Requires a session; redirects to `/login?redirect=<current>`. */
export function requireAuth({ location }: Pick<GuardArgs, 'location'>) {
  if (!useAuthStore.getState().token) {
    throw redirect({ to: '/login', search: { redirect: location.href } })
  }
}

/** Ensures `user` is loaded (token present but user missing, e.g. after a manual storage edit). */
async function ensureUser(context: GuardArgs['context']): Promise<User | null> {
  const { user } = useAuthStore.getState()
  if (user) return user
  try {
    const me = await fetchMe()
    useAuthStore.getState().setUser(me)
    context?.queryClient?.setQueryData(['user', 'me'], me)
    return me
  } catch {
    return null
  }
}

/** `is_admin || is_staff` (support & content pages). */
export async function requireStaff(args: GuardArgs) {
  requireAuth(args)
  const user = await ensureUser(args.context)
  if (!isStaff(user)) throw redirect({ to: '/dashboard' })
}

/** `is_admin` only. Staff are sent to the tickets page they can use. */
export async function requireAdmin(args: GuardArgs) {
  requireAuth(args)
  const user = await ensureUser(args.context)
  if (!isAdmin(user)) throw redirect({ to: isStaff(user) ? '/admin/tickets' : '/dashboard' })
}

/**
 * Layout `beforeLoad` helper: runs `guard`, then returns `{ queryClient }` so every child route
 * gets `context.queryClient` (typed) for its loaders.
 */
export async function withQueryClient<A extends GuardArgs>(args: A, guard: (args: A) => void | Promise<void>) {
  requireAuth(args)
  const { queryClient } = await import('@/lib/queryClient')
  await guard({ ...args, context: { ...args.context, queryClient } })
  return { queryClient }
}

/** For login/register/forgot: signed-in users go straight to their home (or `?redirect=`). */
export function redirectIfAuthenticated({ search }: { search: { redirect?: string | undefined; token?: string | undefined } }) {
  const { token, user } = useAuthStore.getState()
  if (token && !search.token) throw redirect({ to: safeRedirect(search.redirect, homeFor(user)) })
}
