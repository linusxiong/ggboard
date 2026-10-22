import { Hammer } from 'lucide-react'
import { useT } from '@/i18n'
import { EmptyState } from './EmptyState'
import { PageShell } from './PageShell'

/**
 * Placeholder body for routes that feature agents have not built yet.
 * Replace `<ComingSoon … />` with the real page (keep the route file and its guards).
 */
export function ComingSoon({ title, description }: { title: string; description?: string }) {
  const t = useT('common')
  return (
    <PageShell title={title} description={description}>
      <section className="surface-card">
        <EmptyState
          icon={<Hammer className="size-7" strokeWidth={1.75} />}
          title={t('states.comingSoon')}
          description={t('states.comingSoonHint')}
        />
      </section>
    </PageShell>
  )
}
