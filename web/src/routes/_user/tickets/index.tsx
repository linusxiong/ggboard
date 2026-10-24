import { Button } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { LifeBuoy, Plus } from 'lucide-react'
import { lazy, Suspense, useMemo, useState } from 'react'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { EmptyState } from '@/components/EmptyState'
import { DateTime } from '@/components/Formatters'
import { PageShell } from '@/components/PageShell'
import { SegmentedControl } from '@/components/SegmentedControl'
import { TicketLevelChip, TicketStatusChip } from '@/features/tickets/labels'
import { loadNamespaces, type TFunction, useT } from '@/i18n'
import { ticketsQuery } from '@/lib/api/queries'
import type { Ticket, TicketStatus } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { errorMessage } from '@/lib/errors'

const CreateTicketDialog = lazy(() => import('@/features/tickets/CreateTicketDialog'))

const PAGE_SIZE = 15
const FILTERS = ['all', '0', '1'] as const

interface TicketsSearch {
  page?: number
  status?: TicketStatus
  /** `?new=1` opens the "new ticket" dialog (deep link from other pages). */
  new?: 1
}

export const Route = createFileRoute('/_user/tickets/')({
  validateSearch: (search: Record<string, unknown>): TicketsSearch => {
    const page = Number(search.page)
    const status = Number(search.status)
    return {
      ...(Number.isInteger(page) && page > 1 ? { page } : {}),
      ...(search.status !== undefined && (status === 0 || status === 1) ? { status } : {}),
      ...(String(search.new) === '1' ? { new: 1 as const } : {}),
    }
  },
  loader: () => loadNamespaces(['tickets', 'common']),
  component: TicketsPage,
})

const col = createDataTableColumns<Ticket>()
function ticketColumns(t: TFunction) {
  return col.columns([
    col.accessor('subject', {
      header: t('list.subject'),
      enableSorting: false,
      meta: { isPrimary: true, isRowHeader: true },
      cell: (c) => (
        <Link
          to="/tickets/$id"
          params={{ id: String(c.row.original.id) }}
          className="font-semibold text-label hover:text-label-blue hover:underline"
        >
          {c.getValue()}
        </Link>
      ),
    }),
    col.accessor('level', {
      header: t('list.level'),
      enableSorting: false,
      cell: (c) => <TicketLevelChip level={c.getValue()} />,
    }),
    col.accessor('status', {
      header: t('list.status'),
      enableSorting: false,
      cell: (c) => <TicketStatusChip ticket={c.row.original} />,
    }),
    col.accessor('updated_at', {
      header: t('list.updatedAt'),
      enableSorting: false,
      cell: (c) => <DateTime value={c.getValue()} />,
    }),
  ])
}

function TicketsPage() {
  const t = useT(['tickets', 'common'])
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const page = search.page ?? 1
  const query = useQuery(
    ticketsQuery({
      page,
      page_size: PAGE_SIZE,
      ...(search.status !== undefined ? { status: search.status } : {}),
    }),
  )
  const columns = useMemo(() => ticketColumns(t), [t])
  const [creating, setCreating] = useState(false)
  const dialogOpen = creating || search.new === 1
  const tab = search.status === undefined ? 'all' : String(search.status)

  const setDialog = (open: boolean) => {
    setCreating(open)
    if (!open && search.new) void navigate({ search: ({ new: _new, ...rest }) => rest, replace: true })
  }

  const table = (
    <DataTable
      aria-label={t('list.title')}
      columns={columns}
      data={query.data?.items}
      total={query.data?.total}
      isLoading={query.isPending}
      isFetching={query.isFetching && !query.isPending}
      page={page}
      pageSize={PAGE_SIZE}
      onPageChange={(next) => void navigate({ search: (prev) => ({ ...prev, page: next }) })}
      getRowId={(row) => String(row.id)}
      rowActions={(row) => (
        <Link
          to="/tickets/$id"
          params={{ id: String(row.id) }}
          className={buttonClass({ size: 'sm', variant: 'tertiary' })}
          aria-label={t('list.openNamed', { subject: row.subject })}
        >
          {t('list.open')}
        </Link>
      )}
      emptyState={
        query.isError ? (
          <EmptyState compact title={t('common:states.error')} description={errorMessage(query.error)} />
        ) : (
          <EmptyState
            compact
            icon={<LifeBuoy className="size-5" strokeWidth={1.75} />}
            title={t('list.empty')}
            description={t('list.emptyHint')}
          />
        )
      }
    />
  )

  return (
    <PageShell
      title={t('list.title')}
      help={t('list.subtitle')}
      actions={
        <Button onPress={() => setCreating(true)}>
          <Plus className="size-4" aria-hidden="true" />
          {t('list.new')}
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        <SegmentedControl
          aria-label={t('list.filter')}
          value={tab}
          onChange={(value) =>
            void navigate({ search: value === 'all' ? {} : { status: Number(value) as TicketStatus } })
          }
          items={FILTERS.map((id) => ({ id, label: t(`filter.${id}`) }))}
        />
        {table}
      </div>

      {dialogOpen ? (
        <Suspense fallback={null}>
          <CreateTicketDialog isOpen onOpenChange={setDialog} />
        </Suspense>
      ) : null}
    </PageShell>
  )
}
