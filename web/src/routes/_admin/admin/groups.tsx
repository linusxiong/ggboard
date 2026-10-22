import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/ComingSoon'
import { useT } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

/** Placeholder — replace the component body with the real page (see web/README.md). */
export const Route = createFileRoute('/_admin/admin/groups')({
  beforeLoad: requireAdmin,
  component: AdminGroupsPage,
})

function AdminGroupsPage() {
  const t = useT('nav')
  return <ComingSoon title={t('admin.groups')} />
}
