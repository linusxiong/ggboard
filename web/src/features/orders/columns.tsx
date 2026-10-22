import { Link } from '@tanstack/react-router'
import { createDataTableColumns } from '@/components/data-table'
import { DateTime } from '@/components/Formatters'
import type { TFunction } from '@/i18n'
import type { Order } from '@/lib/api/types'
import { SiteMoney } from '@/features/shop/SiteMoney'
import { OrderStatusChip } from './OrderStatusChip'
import { ORDER_TYPE_KEY } from './status'

const col = createDataTableColumns<Order>()

/** Columns for `/orders` (built per translator; memoise with `useMemo`). */
export function orderColumns(t: TFunction) {
  return col.columns([
    col.accessor('trade_no', {
      header: t('list.tradeNo'),
      enableSorting: false,
      meta: { isPrimary: true, isRowHeader: true },
      cell: (c) => (
        <Link
          to="/orders/$tradeNo"
          params={{ tradeNo: c.getValue() }}
          className="font-mono text-footnote text-label-blue hover:underline"
        >
          {c.getValue()}
        </Link>
      ),
    }),
    col.accessor((o) => o.plan.name, {
      id: 'plan',
      header: t('list.plan'),
      enableSorting: false,
      cell: (c) => (
        <span className="flex flex-col">
          <span className="text-label">{c.getValue()}</span>
          <span className="text-footnote text-label-secondary">
            {t(`common:periods.${c.row.original.period}`)} ·{' '}
            {t(`type.${ORDER_TYPE_KEY[c.row.original.type]}`)}
          </span>
        </span>
      ),
    }),
    col.accessor('total_amount', {
      header: t('list.amount'),
      enableSorting: false,
      meta: { align: 'end' },
      cell: (c) => <SiteMoney cents={c.getValue() + c.row.original.handling_amount} />,
    }),
    col.accessor('status', {
      header: t('list.status'),
      enableSorting: false,
      cell: (c) => <OrderStatusChip status={c.getValue()} />,
    }),
    col.accessor('created_at', {
      header: t('list.createdAt'),
      enableSorting: false,
      cell: (c) => <DateTime value={c.getValue()} />,
    }),
  ])
}
