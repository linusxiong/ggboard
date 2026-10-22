/**
 * Admin area (pathless layout) mounted under /admin/*. Guard: `is_admin || is_staff`.
 * Admin-only pages add `beforeLoad: requireAdmin`; staff pages (tickets/notices/knowledge) don't.
 * Adds `queryClient` to the context of every child route.
 */
import { createFileRoute } from '@tanstack/react-router'
import { AppShell } from '@/components/layout/AppShell'
import { ADMIN_NAV } from '@/components/layout/nav'
import { loadNamespaces } from '@/i18n'
import { requireStaff, withQueryClient } from '@/lib/guards'

export const Route = createFileRoute('/_admin')({
  beforeLoad: (args) => withQueryClient(args, requireStaff),
  loader: () => loadNamespaces(['nav', 'common']),
  component: AdminLayout,
})

function AdminLayout() {
  return <AppShell area="admin" items={ADMIN_NAV} />
}
