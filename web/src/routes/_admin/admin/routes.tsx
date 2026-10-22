import { createFileRoute } from '@tanstack/react-router'
import { RoutesPage } from '@/features/admin/routes/RoutesPage'
import { loadNamespaces } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

/** `/admin/routes` — node routes (block / dns). */
export const Route = createFileRoute('/_admin/admin/routes')({
  beforeLoad: requireAdmin,
  loader: () => loadNamespaces(['admin-nodes', 'common']),
  component: RoutesPage,
})
