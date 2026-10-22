import { createFileRoute } from '@tanstack/react-router'
import { AdminDashboard } from '@/features/admin/stats/Dashboard'
import { loadNamespaces } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

/** `/admin/dashboard` — overview tiles, trend chart and traffic rankings (`/admin` redirects here). */
export const Route = createFileRoute('/_admin/admin/dashboard')({
  beforeLoad: requireAdmin,
  loader: () => loadNamespaces(['admin-stats', 'common']),
  component: AdminDashboard,
})
