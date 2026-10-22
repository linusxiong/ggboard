import { createColumnHelper, rowPaginationFeature, rowSortingFeature, tableFeatures } from '@tanstack/react-table'
import type { ColumnDef, RowData } from '@tanstack/react-table'
import type { ReactNode } from 'react'

/** Extra column options understood by `<DataTable>`. */
export interface DataTableColumnMeta {
  /** Plain-text label for the mobile card layout (falls back to a string `header`). */
  label?: string
  /** Horizontal alignment (numbers → `end`). */
  align?: 'start' | 'center' | 'end'
  /** Class names applied to header + cells. */
  className?: string
  /** Hide in the mobile card layout. */
  hideOnMobile?: boolean
  /** Use this column as the row header (screen readers). Defaults to the first column. */
  isRowHeader?: boolean
  /** Rendered as the card title on mobile. */
  isPrimary?: boolean
}

/**
 * Feature set shared by every DataTable: server-side sorting + pagination (manual mode),
 * typed column meta. Build columns with `createDataTableColumns<Row>()`.
 */
export const dataTableFeatures = tableFeatures({
  rowSortingFeature,
  rowPaginationFeature,
  columnMeta: {} as DataTableColumnMeta,
})

export type DataTableFeatures = typeof dataTableFeatures

// eslint-disable-next-line @typescript-eslint/no-explicit-any -- column value types vary per column
export type DataTableColumn<TData extends RowData> = ColumnDef<DataTableFeatures, TData, any>

/**
 * Typed column helper:
 *   const col = createDataTableColumns<Order>()
 *   const columns = col.columns([
 *     col.accessor('trade_no', { header: t('tradeNo'), meta: { isPrimary: true } }),
 *     col.accessor('total_amount', { header: t('amount'), cell: (c) => <Money cents={c.getValue()} />, meta: { align: 'end' } }),
 *   ])
 * Keep `columns` stable (module scope or `useMemo`).
 */
export function createDataTableColumns<TData extends RowData>() {
  return createColumnHelper<DataTableFeatures, TData>()
}

export type RowActions<TData> = (row: TData) => ReactNode
