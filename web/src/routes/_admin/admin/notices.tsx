import { createFileRoute } from '@tanstack/react-router'
import { NoticesPage } from '@/features/admin/content/NoticesPage'
import { loadNamespaces } from '@/i18n'

/** Staff page (no `requireAdmin`). */
export const Route = createFileRoute('/_admin/admin/notices')({
  loader: () => loadNamespaces(['admin-support', 'admin-settings']),
  component: NoticesPage,
})
