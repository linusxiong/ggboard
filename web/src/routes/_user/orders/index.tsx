import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { Receipt } from 'lucide-react'
import { useMemo } from 'react'
import { DataTable } from '@/components/data-table'
import { EmptyState } from '@/components/EmptyState'
import { PageShell } from '@/components/PageShell'
import { SegmentedControl } from '@/components/SegmentedControl'
import { orderColumns } from '@/features/orders/columns'
import { isOrderStatus, ORDER_FILTERS, ORDER_STATUS_KEY } from '@/features/orders/status'
import { loadNamespaces, useT } from '@/i18n'
import { ordersQuery } from '@/lib/api/queries'
import type { OrderStatus } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { errorMessage } from '@/lib/errors'

const PAGE_SIZE = 15

interface OrdersSearch {
  page?: number
  status?: OrderStatus
}

export const Route = createFileRoute('/_user/orders/')({
  validateSearch: (search: Record<string, unknown>): OrdersSearch => {
    const page = Number(search.page)
    const status = Number(search.status)
    return {
      ...(Number.isInteger(page) && page > 1 ? { page } : {}),
      ...(search.status !== undefined && search.status !== '' && isOrderStatus(status) ? { status } : {}),
    }
  },
  // Loaders stay in the entry chunk (only components are split): import the query factories lazily.
  loader: async ({ context }) => {
    const q = await import('@/lib/api/queries')
    return Promise.all([
      loadNamespaces(['orders', 'common']),
      context.queryClient.query({ ...q.guestConfigQuery(), staleTime: 'static' }).catch(() => null),
    ])
  },
  component: OrdersPage,
})

function OrdersPage() {
  const t = useT(['orders', 'common'])
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const page = search.page ?? 1
  const query = useQuery(
    ordersQuery({
      page,
      page_size: PAGE_SIZE,
      ...(search.status !== undefined ? { status: search.status } : {}),
    }),
  )
  const columns = useMemo(() => orderColumns(t), [t])
  const tab = search.status === undefined ? 'all' : String(search.status)

  return (
    <PageShell
      title={t('list.title')}
      description={t('list.subtitle')}
      actions={
        <Link to="/shop" className={buttonClass({ variant: 'secondary', size: 'sm' })}>
          {t('list.shop')}
        </Link>
      }
    >
      <div className="flex flex-col gap-4">
        <SegmentedControl
          aria-label={t('list.filter')}
          value={tab}
          onChange={(value) =>
            void navigate({ search: value === 'all' ? {} : { status: Number(value) as OrderStatus } })
          }
          items={ORDER_FILTERS.map((status) => ({
            id: status === undefined ? 'all' : String(status),
            label: status === undefined ? t('list.all') : t(`status.${ORDER_STATUS_KEY[status]}`),
          }))}
        />
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
          getRowId={(o) => o.trade_no}
          rowActions={(o) => (
            <Link
              to="/orders/$tradeNo"
              params={{ tradeNo: o.trade_no }}
              className={buttonClass({ size: 'sm', variant: o.status === 0 ? 'primary' : 'tertiary' })}
              aria-label={
                o.status === 0
                  ? t('list.payNamed', { no: o.trade_no })
                  : t('list.viewNamed', { no: o.trade_no })
              }
            >
              {o.status === 0 ? t('list.pay') : t('list.view')}
            </Link>
          )}
          emptyState={
            query.isError ? (
              <EmptyState compact title={t('common:states.error')} description={errorMessage(query.error)} />
            ) : (
              <EmptyState
                compact
                icon={<Receipt className="size-5" strokeWidth={1.75} />}
                title={search.status === undefined ? t('list.empty') : t('list.emptyFiltered')}
                action={
                  search.status === undefined ? (
                    <Link to="/shop" className={buttonClass({ size: 'sm' })}>
                      {t('list.shop')}
                    </Link>
                  ) : undefined
                }
              />
            )
          }
        />
      </div>
    </PageShell>
  )
}
