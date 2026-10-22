/**
 * Authenticated user area (pathless layout): /dashboard, /shop, /orders, … — see nav.ts.
 * Guard: any signed-in user. Adds `queryClient` to the context of every child route.
 */
import { createFileRoute } from '@tanstack/react-router'
import { AppShell } from '@/components/layout/AppShell'
import { USER_NAV } from '@/components/layout/nav'
import { loadNamespaces } from '@/i18n'
import { requireAuth, withQueryClient } from '@/lib/guards'

export const Route = createFileRoute('/_user')({
  beforeLoad: (args) => withQueryClient(args, requireAuth),
  loader: () => loadNamespaces(['nav', 'common']),
  component: UserLayout,
})

function UserLayout() {
  return <AppShell area="user" items={USER_NAV} />
}
