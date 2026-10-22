import { createFileRoute } from '@tanstack/react-router'
import { SystemPage } from '@/features/admin/system/SystemPage'
import { loadNamespaces } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

export const Route = createFileRoute('/_admin/admin/system')({
  beforeLoad: requireAdmin,
  loader: () => loadNamespaces(['admin-system', 'admin-settings']),
  component: SystemPage,
})
