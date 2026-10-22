import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/ComingSoon'
import { useT } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

/** Placeholder — replace the component body with the real page (see web/README.md). */
export const Route = createFileRoute('/_admin/admin/payments')({
  beforeLoad: requireAdmin,
  component: AdminPaymentsPage,
})

function AdminPaymentsPage() {
  const t = useT('nav')
  return <ComingSoon title={t('admin.payments')} />
}
