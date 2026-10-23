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

/** Control characters (C0 incl. TAB/LF/CR, DEL, C1), any whitespace, and backslashes. */
const UNSAFE_CHAR = /[\p{Cc}\s\\]/u
const CONTROL_CHAR = /\p{Cc}/u
/** Two leading slashes/backslashes make a path protocol-relative: `//evil.example` is another site. */
const PROTOCOL_RELATIVE = /^[/\\]{2}/

function currentOrigin(): string {
  const origin = typeof location === 'undefined' ? '' : location.origin
  return origin.startsWith('http') ? origin : 'http://localhost'
}

/**
 * Post-login target → a same-origin path (`/orders?page=2`), else `fallback` (no open redirects).
 *
 * Checking raw prefixes is not enough: the URL parser drops TAB/LF/CR anywhere, reads `\` as `/` and resolves
 * dot segments, so `/\t/evil.example` becomes `https://evil.example/` and `/..//evil.example` the path
 * `//evil.example`. Control characters, whitespace and backslashes are refused outright; the target is then
 * resolved the way the browser will, and must stay on this origin with a path that is not protocol-relative,
 * even once percent-decoded. Returns the resolved path + search + hash, so callers navigate to exactly what was
 * checked. (TanStack Router normalizes these too; this guard does not rely on it.)
 */
export function safeRedirect(target: unknown, fallback = '/dashboard'): string {
  if (
    typeof target !== 'string' ||
    !target.startsWith('/') ||
    PROTOCOL_RELATIVE.test(target) ||
    UNSAFE_CHAR.test(target)
  )
    return fallback
  const origin = currentOrigin()
  let url: URL
  let path: string
  try {
    url = new URL(target, origin)
    path = decodeURIComponent(url.pathname)
  } catch {
    return fallback
  }
  if (
    url.origin !== origin ||
    !url.pathname.startsWith('/') ||
    PROTOCOL_RELATIVE.test(url.pathname) ||
    PROTOCOL_RELATIVE.test(path) ||
    CONTROL_CHAR.test(path)
  )
    return fallback
  return `${url.pathname}${url.search}${url.hash}`
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
export async function withQueryClient<A extends GuardArgs>(
  args: A,
  guard: (args: A) => void | Promise<void>,
) {
  requireAuth(args)
  const { queryClient } = await import('@/lib/queryClient')
  await guard({ ...args, context: { ...args.context, queryClient } })
  return { queryClient }
}

/** For login/register/forgot: signed-in users go straight to their home (or `?redirect=`). */
export function redirectIfAuthenticated({
  search,
}: {
  search: { redirect?: string | undefined; token?: string | undefined }
}) {
  const { token, user } = useAuthStore.getState()
  if (token && !search.token) throw redirect({ to: safeRedirect(search.redirect, homeFor(user)) })
}
