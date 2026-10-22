import { createFileRoute } from '@tanstack/react-router'
import { PaymentsPage } from '@/features/admin/payments/PaymentsPage'
import { loadNamespaces } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

export const Route = createFileRoute('/_admin/admin/payments')({
  beforeLoad: requireAdmin,
  loader: () => loadNamespaces(['admin-payments', 'admin-settings']),
  component: PaymentsPage,
})
