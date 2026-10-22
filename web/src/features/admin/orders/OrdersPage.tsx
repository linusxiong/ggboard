import { Button } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { DateTime, Money } from '@/components/Formatters'
import { PageShell } from '@/components/PageShell'
import { useT } from '@/i18n'
import { adminOrdersQuery, adminPaymentsQuery, adminPlansQuery } from '@/lib/api/queries'
import type { AdminOrder, ID } from '@/lib/api/types'
import { AssignOrderDialog } from './AssignOrderDialog'
import { CommissionStatusChip, OrderStatusChip, OrderTypeLabel } from './chips'
import { OrderDetailDrawer } from './OrderDetailDrawer'
import { type OrderFilterPatch, OrderFilters } from './OrderFilters'
import { OrderRowActions } from './OrderRowActions'
import { orderSearchToQuery } from './query'
import type { OrderSearch } from './search'
import { useCurrency } from './useCurrency'

const col = createDataTableColumns<AdminOrder>()

interface OrdersPageProps {
  search: OrderSearch
  onSearchChange: (patch: Partial<OrderSearch>) => void
}

export function OrdersPage({ search, onSearchChange }: OrdersPageProps) {
  const t = useT(['admin-orders', 'common'])
  const { currency, symbol } = useCurrency()
  const orders = useQuery(adminOrdersQuery(orderSearchToQuery(search)))
  const plans = useQuery(adminPlansQuery())
  const payments = useQuery(adminPaymentsQuery())
  const [detail, setDetail] = useState<{ id: ID; row?: AdminOrder } | null>(null)
  const [assignOpen, setAssignOpen] = useState(false)

  const onFilter = useCallback(
    (patch: OrderFilterPatch) => onSearchChange({ ...patch, page: 1 }),
    [onSearchChange],
  )

  const columns = useMemo(
    () =>
      col.columns([
        col.accessor('trade_no', {
          header: t('fields.trade_no'),
          meta: { isPrimary: true },
          cell: (c) => (
            <button
              type="button"
              className="font-mono text-subhead text-label-blue underline-offset-2 hover:underline focus-visible:underline"
              onClick={() => setDetail({ id: c.row.original.id, row: c.row.original })}
            >
              {c.getValue()}
            </button>
          ),
        }),
        col.accessor((o) => o.user?.email ?? `#${o.user_id}`, {
          id: 'user',
          header: t('fields.user'),
          enableSorting: false,
          cell: (c) => (
            <span className="block max-w-44 truncate" title={c.getValue()}>
              {c.getValue()}
            </span>
          ),
        }),
        col.accessor((o) => o.plan?.name ?? `#${o.plan_id}`, {
          id: 'plan_id',
          header: t('fields.plan'),
          cell: (c) => (
            <span className="whitespace-nowrap">
              {c.getValue()}
              <span className="block text-footnote text-label-secondary">
                {t(`common:periods.${c.row.original.period}`)} · <OrderTypeLabel type={c.row.original.type} />
              </span>
            </span>
          ),
        }),
        col.accessor('total_amount', {
          header: t('fields.total_amount'),
          meta: { align: 'end', className: 'whitespace-nowrap' },
          cell: (c) => <Money cents={c.getValue()} currency={currency} symbol={symbol} />,
        }),
        col.accessor('status', {
          header: t('fields.status'),
          meta: { className: 'whitespace-nowrap' },
          cell: (c) => <OrderStatusChip status={c.getValue()} />,
        }),
        col.accessor('commission_status', {
          header: t('fields.commission_status'),
          meta: { hideOnMobile: true },
          cell: (c) =>
            (c.row.original.commission_balance ?? 0) > 0 ? (
              <span className="inline-flex flex-col items-start gap-0.5">
                <CommissionStatusChip status={c.getValue()} />
                <Money
                  cents={c.row.original.commission_balance ?? 0}
                  currency={currency}
                  symbol={symbol}
                  className="text-footnote text-label-secondary"
                />
              </span>
            ) : (
              <span className="text-label-secondary">—</span>
            ),
        }),
        col.accessor((o) => o.payment?.name ?? '', {
          id: 'payment_id',
          header: t('fields.payment'),
          meta: { hideOnMobile: true },
          cell: (c) => c.getValue() || <span className="text-label-secondary">—</span>,
        }),
        col.accessor('created_at', {
          header: t('fields.created_at'),
          meta: { className: 'whitespace-nowrap' },
          cell: (c) => <DateTime value={c.getValue()} />,
        }),
      ]),
    [t, currency, symbol],
  )

  return (
    <PageShell
      title={t('title')}
      description={t('description')}
      actions={
        <Button onPress={() => setAssignOpen(true)}>
          <Plus className="size-4" aria-hidden />
          {t('assign.open')}
        </Button>
      }
    >
      <OrderFilters
        value={search}
        onChange={onFilter}
        plans={plans.data ?? []}
        payments={payments.data ?? []}
      />
      <DataTable
        aria-label={t('title')}
        columns={columns}
        data={orders.data?.items}
        total={orders.data?.total}
        isLoading={orders.isPending}
        isFetching={orders.isFetching}
        page={search.page}
        pageSize={search.page_size}
        sort={search.sort ?? '-created_at'}
        onPageChange={(page) => onSearchChange({ page })}
        onSortChange={(sort) => onSearchChange({ sort, page: 1 })}
        getRowId={(o) => String(o.id)}
        rowActions={(o) => <OrderRowActions order={o} onDetails={() => setDetail({ id: o.id, row: o })} />}
      />
      <OrderDetailDrawer orderId={detail?.id ?? null} preview={detail?.row} onClose={() => setDetail(null)} />
      <AssignOrderDialog
        isOpen={assignOpen}
        onOpenChange={setAssignOpen}
        plans={plans.data ?? []}
        currency={currency}
        symbol={symbol}
      />
    </PageShell>
  )
}
