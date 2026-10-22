import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { MessagesSquare } from 'lucide-react'
import { useCallback } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { PageShell } from '@/components/PageShell'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import { cx } from '@/lib/cn'
import { ticketSearchToQuery } from './query'
import type { TicketSearch } from './search'
import { TicketList } from './TicketList'
import { TicketThread } from './TicketThread'

interface TicketsPageProps {
  search: TicketSearch
  onSearchChange: (patch: Partial<TicketSearch>) => void
}

/** Master–detail support inbox. On phones the list and the thread are separate screens. */
export function TicketsPage({ search, onSearchChange }: TicketsPageProps) {
  const t = useT(['admin-support', 'common'])
  const query = ticketSearchToQuery(search)
  const list = useQuery({
    queryKey: qk.admin.tickets(query),
    queryFn: ({ signal }) => adminApi.ticketList(query, signal),
    placeholderData: keepPreviousData,
    refetchInterval: 60_000,
  })
  const awaitingQuery = { status: 0 as const, reply_status: 0 as const, page_size: 1 }
  const awaiting = useQuery({
    queryKey: qk.admin.tickets(awaitingQuery),
    queryFn: ({ signal }) => adminApi.ticketList(awaitingQuery, signal),
    refetchInterval: 60_000,
  })
  const onCommitQuery = useCallback(
    (q: string | undefined) => onSearchChange({ q, page: 1 }),
    [onSearchChange],
  )
  const selected = search.id

  return (
    <PageShell title={t('tickets.title')} description={t('tickets.description')}>
      <div className="grid min-h-[70dvh] gap-4 md:h-[calc(100dvh-14rem)] md:grid-cols-[minmax(17rem,22rem)_1fr]">
        <TicketList
          search={search}
          data={list.data}
          isLoading={list.isPending}
          isFetching={list.isFetching}
          awaitingCount={awaiting.data?.total}
          onSearchChange={onSearchChange}
          onCommitQuery={onCommitQuery}
          className={cx(selected !== undefined && 'hidden md:flex')}
        />
        {selected !== undefined ? (
          <TicketThread
            key={selected}
            id={selected}
            onBack={() => onSearchChange({ id: undefined })}
            className="min-h-[70dvh] md:min-h-0"
          />
        ) : (
          <div className="surface-card hidden items-center justify-center md:flex">
            <EmptyState
              icon={<MessagesSquare className="size-7" strokeWidth={1.75} />}
              title={t('tickets.pickTitle')}
              description={t('tickets.pickHint')}
            />
          </div>
        )}
      </div>
    </PageShell>
  )
}
