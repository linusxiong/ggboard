import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { Megaphone } from 'lucide-react'
import { TablePagination } from '@/components/data-table/DataTable'
import { EmptyState } from '@/components/EmptyState'
import { PageShell } from '@/components/PageShell'
import { NoticeCard } from '@/features/content/NoticeCard'
import { loadNamespaces, useT } from '@/i18n'
import { noticesQuery } from '@/lib/api/queries'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'

const PAGE_SIZE = 10

export const Route = createFileRoute('/_user/notices')({
  validateSearch: (search: Record<string, unknown>): { page?: number } => {
    const page = Number(search.page)
    return Number.isInteger(page) && page > 1 ? { page } : {}
  },
  loader: () => loadNamespaces(['content', 'common']),
  component: NoticesPage,
})

function NoticesPage() {
  const t = useT(['content', 'common'])
  const { page = 1 } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const query = useQuery(noticesQuery({ page, page_size: PAGE_SIZE }))
  const total = query.data?.total ?? 0

  return (
    <PageShell title={t('notices.title')}>
      {query.isPending ? (
        <div className="flex flex-col gap-4" aria-busy="true">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="surface-card h-44 shimmer" />
          ))}
        </div>
      ) : query.isError ? (
        <section className="surface-card">
          <EmptyState title={t('common:states.error')} description={errorMessage(query.error)} />
        </section>
      ) : query.data.items.length === 0 ? (
        <section className="surface-card">
          <EmptyState icon={<Megaphone className="size-7" strokeWidth={1.75} />} title={t('notices.empty')} />
        </section>
      ) : (
        <>
          <ul
            className={cx('flex w-full max-w-3xl flex-col gap-5', query.isPlaceholderData && 'opacity-60')}
            aria-label={t('notices.title')}
          >
            {query.data.items.map((notice) => (
              <li key={notice.id}>
                <NoticeCard notice={notice} />
              </li>
            ))}
          </ul>
          {total > PAGE_SIZE ? (
            <div className="surface-card w-full max-w-3xl">
              <TablePagination
                page={page}
                pageCount={Math.ceil(total / PAGE_SIZE)}
                pageSize={PAGE_SIZE}
                total={total}
                onPageChange={(next) => void navigate({ search: { page: next } })}
              />
            </div>
          ) : null}
        </>
      )}
    </PageShell>
  )
}
