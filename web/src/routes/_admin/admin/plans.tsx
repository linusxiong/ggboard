import { createFileRoute } from '@tanstack/react-router'
import { PlansPage } from '@/features/admin/plans/PlansPage'
import { loadNamespaces } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

/** `/admin/plans` — plans (prices, limits, visibility, order). */
export const Route = createFileRoute('/_admin/admin/plans')({
  beforeLoad: requireAdmin,
  loader: () => loadNamespaces(['admin-plans', 'common']),
  component: PlansPage,
})
