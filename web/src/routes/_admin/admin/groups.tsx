import { createFileRoute } from '@tanstack/react-router'
import { GroupsPage } from '@/features/admin/groups/GroupsPage'
import { loadNamespaces } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

/** `/admin/groups` — server groups. */
export const Route = createFileRoute('/_admin/admin/groups')({
  beforeLoad: requireAdmin,
  loader: () => loadNamespaces(['admin-nodes', 'common']),
  component: GroupsPage,
})
