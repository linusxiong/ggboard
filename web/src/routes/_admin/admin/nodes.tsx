import { createFileRoute, type SearchSchemaInput } from '@tanstack/react-router'
import { NodesPage } from '@/features/admin/nodes/NodesPage'
import { type NodeSearch, parseNodeSearch } from '@/features/admin/nodes/search'
import { loadNamespaces } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

/** `/admin/nodes` — nodes (filters in the URL: `?type=&group=&status=&q=`). */
export const Route = createFileRoute('/_admin/admin/nodes')({
  validateSearch: (search: Partial<NodeSearch> & SearchSchemaInput) => parseNodeSearch(search),
  beforeLoad: requireAdmin,
  loader: () => loadNamespaces(['admin-nodes', 'admin-stats', 'common']),
  component: NodesPage,
})
