import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/ComingSoon'
import { useT } from '@/i18n'

/** Placeholder — replace the component body with the real page (see web/README.md). */
export const Route = createFileRoute('/_admin/admin/knowledge')({
  component: AdminKnowledgePage,
})

function AdminKnowledgePage() {
  const t = useT('nav')
  return <ComingSoon title={t('admin.knowledge')} />
}
