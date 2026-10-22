import { Link } from '@tanstack/react-router'
import { Megaphone } from 'lucide-react'
import { EmptyState } from '@/components/EmptyState'
import { DateTime } from '@/components/Formatters'
import { useT } from '@/i18n'
import type { Notice } from '@/lib/api/types'
import { excerpt } from './format'

export function NoticesCard({ notices }: { notices: readonly Notice[] }) {
  const t = useT(['dashboard', 'common'])
  return (
    <section className="surface-card flex flex-col gap-3 p-6" aria-labelledby="notices-title">
      <div className="flex items-baseline justify-between gap-2">
        <h2 id="notices-title" className="text-headline text-label">
          {t('notices.title')}
        </h2>
        <Link to="/notices" className="text-footnote text-label-blue hover:underline">
          {t('notices.all')}
        </Link>
      </div>
      {notices.length === 0 ? (
        <EmptyState compact icon={<Megaphone className="size-5" />} title={t('notices.empty')} />
      ) : (
        <ul className="flex flex-col">
          {notices.map((notice) => (
            <li key={notice.id} className="border-b-[0.5px] border-hairline py-3 last:border-b-0">
              <div className="flex items-baseline justify-between gap-3">
                <p className="truncate text-subhead font-semibold text-label">{notice.title}</p>
                <DateTime
                  value={notice.created_at}
                  format="short"
                  className="shrink-0 text-caption text-label-secondary"
                />
              </div>
              {notice.content ? (
                <p className="mt-0.5 line-clamp-2 text-footnote text-label-secondary">
                  {excerpt(notice.content)}
                </p>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
