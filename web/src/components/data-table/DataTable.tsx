/**
 * DataTable — TanStack Table v9 (headless state) rendered with HeroUI v3 Table.
 *
 * Server-driven: pass the current page of rows plus `total`, `page`, `pageSize`, `sort` and the
 * change handlers (usually wired to route search params + React Query). Below `md` it switches
 * to a card list. See web/README.md → "List pages".
 */
import { Pagination, Table } from '@heroui/react'
import type { SortDescriptor } from '@heroui/react'
import { type RowData, useTable } from '@tanstack/react-table'
import { type ReactNode, useMemo, useSyncExternalStore } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { useLocale, useT } from '@/i18n'
import { formatNumber } from '@/lib/format'
import { cx } from '@/lib/cn'
import { type DataTableColumn, type DataTableColumnMeta, dataTableFeatures } from './features'
import { pageWindow, parseSort, toSortParam } from './sort'

export interface DataTableProps<TData extends RowData> {
  /** Accessible name of the table (required). */
  'aria-label': string
  columns: readonly DataTableColumn<TData>[]
  /** Current page of rows (`undefined` while the first load is pending). */
  data: readonly TData[] | undefined
  /** Total rows on the server (`Page.total`). Enables pagination. */
  total?: number
  /** 1-based page number. */
  page?: number
  pageSize?: number
  onPageChange?: (page: number) => void
  /** API sort param (`field` / `-field`). */
  sort?: string
  onSortChange?: (sort: string | undefined) => void
  /** First load (no data yet) → skeleton rows. */
  isLoading?: boolean
  /** Background refetch (data shown is stale) → dimmed rows. */
  isFetching?: boolean
  getRowId?: (row: TData, index: number) => string
  /** Trailing actions cell per row (buttons, dropdown). */
  rowActions?: (row: TData) => ReactNode
  /** Custom empty state (defaults to a generic "暂无数据"). */
  emptyState?: ReactNode
  /** Custom mobile card; defaults to a label/value list built from the columns. */
  renderCard?: (row: TData) => ReactNode
  className?: string
}

const EMPTY: readonly never[] = []
const MD = '(min-width: 768px)'
const subscribeMd = (cb: () => void) => {
  const mql = window.matchMedia?.(MD)
  mql?.addEventListener('change', cb)
  return () => mql?.removeEventListener('change', cb)
}
const isDesktop = () => window.matchMedia?.(MD).matches ?? true

const alignClass = (meta: DataTableColumnMeta | undefined) =>
  meta?.align === 'end' ? 'text-end' : meta?.align === 'center' ? 'text-center' : undefined

export function DataTable<TData extends RowData>(props: DataTableProps<TData>) {
  const {
    columns,
    data,
    total,
    page = 1,
    pageSize = 20,
    onPageChange,
    sort,
    onSortChange,
    isLoading = false,
    isFetching = false,
    getRowId,
    rowActions,
    emptyState,
    renderCard,
    className,
  } = props
  const t = useT('common')
  const desktop = useSyncExternalStore(subscribeMd, isDesktop, () => true)
  const sorting = useMemo(() => parseSort(sort), [sort])

  const table = useTable({
    features: dataTableFeatures,
    columns: columns as DataTableColumn<TData>[],
    data: (data ?? EMPTY) as TData[],
    getRowId: getRowId ? (row, index) => getRowId(row, index) : undefined,
    manualSorting: true,
    manualPagination: true,
    rowCount: total,
    state: { sorting, pagination: { pageIndex: page - 1, pageSize } },
    onSortingChange: (updater) => {
      const next = typeof updater === 'function' ? updater(sorting) : updater
      onSortChange?.(toSortParam(next))
    },
    onPaginationChange: (updater) => {
      const current = { pageIndex: page - 1, pageSize }
      const next = typeof updater === 'function' ? updater(current) : updater
      if (next.pageIndex !== current.pageIndex) onPageChange?.(next.pageIndex + 1)
    },
  })

  const headers = table.getHeaderGroups()[0]?.headers ?? []
  const rows = table.getRowModel().rows
  const pageCount = total !== undefined ? Math.max(1, Math.ceil(total / pageSize)) : 1
  const showSkeleton = isLoading && !data
  const rowHeaderId = (headers.find((h) => h.column.columnDef.meta?.isRowHeader) ?? headers[0])?.id

  const sortDescriptor: SortDescriptor | undefined = sorting[0]
    ? { column: sorting[0].id, direction: sorting[0].desc ? 'descending' : 'ascending' }
    : undefined

  const empty = emptyState ?? <EmptyState compact title={t('states.empty')} />

  const pagination =
    total !== undefined && total > 0 ? (
      <TablePagination page={page} pageCount={pageCount} pageSize={pageSize} total={total} onPageChange={onPageChange} />
    ) : null

  // ------------------------------------------------------------- mobile cards
  if (!desktop) {
    return (
      <div className={cx('flex flex-col gap-3', className)}>
        {showSkeleton ? (
          Array.from({ length: 3 }, (_, i) => <div key={i} className="surface-card h-28 animate-pulse" />)
        ) : rows.length === 0 ? (
          <div className="surface-card">{empty}</div>
        ) : (
          <ul aria-label={props['aria-label']} className={cx('flex flex-col gap-3', isFetching && 'opacity-60')}>
            {rows.map((row) => (
              <li key={row.id} className="surface-card p-4">
                {renderCard ? (
                  renderCard(row.original)
                ) : (
                  <dl className="flex flex-col gap-2">
                    {row.getAllCells().map((cell) => {
                      const meta = cell.column.columnDef.meta
                      if (meta?.hideOnMobile) return null
                      const header = cell.column.columnDef.header
                      const label = meta?.label ?? (typeof header === 'string' ? header : cell.column.id)
                      return meta?.isPrimary ? (
                        <div key={cell.id} className="text-headline text-label">
                          <dt className="sr-only">{label}</dt>
                          <dd>
                            <table.FlexRender cell={cell} />
                          </dd>
                        </div>
                      ) : (
                        <div key={cell.id} className="flex items-start justify-between gap-4 text-subhead">
                          <dt className="shrink-0 text-label-secondary">{label}</dt>
                          <dd className="min-w-0 text-end text-label">
                            <table.FlexRender cell={cell} />
                          </dd>
                        </div>
                      )
                    })}
                  </dl>
                )}
                {rowActions ? <div className="mt-3 flex flex-wrap justify-end gap-2">{rowActions(row.original)}</div> : null}
              </li>
            ))}
          </ul>
        )}
        {pagination}
      </div>
    )
  }

  // ------------------------------------------------------------- desktop table
  return (
    <Table variant="secondary" className={cx('surface-card overflow-hidden', className)}>
      <Table.ScrollContainer>
        <Table.Content
          aria-label={props['aria-label']}
          aria-busy={isLoading || isFetching || undefined}
          sortDescriptor={sortDescriptor}
          onSortChange={(d) => onSortChange?.(toSortParam([{ id: String(d.column), desc: d.direction === 'descending' }]))}
          className={cx('min-w-full', isFetching && !showSkeleton && 'opacity-60 transition-opacity')}
        >
          <Table.Header>
            {[
              ...headers.map((header) => {
                const meta = header.column.columnDef.meta
                const canSort = Boolean(onSortChange) && header.column.getCanSort()
                return (
                  <Table.Column
                    key={header.id}
                    id={header.id}
                    allowsSorting={canSort}
                    isRowHeader={header.id === rowHeaderId}
                    className={cx(alignClass(meta), meta?.className)}
                  >
                    {({ sortDirection }) =>
                      canSort ? (
                        <Table.SortableColumnHeader sortDirection={sortDirection}>
                          <table.FlexRender header={header} />
                        </Table.SortableColumnHeader>
                      ) : (
                        <table.FlexRender header={header} />
                      )
                    }
                  </Table.Column>
                )
              }),
              ...(rowActions
                ? [
                    <Table.Column key="__actions" id="__actions" className="text-end">
                      <span className="sr-only">{t('table.actions')}</span>
                    </Table.Column>,
                  ]
                : []),
            ]}
          </Table.Header>
          <Table.Body renderEmptyState={showSkeleton ? undefined : () => empty}>
            {showSkeleton
              ? Array.from({ length: Math.min(pageSize, 8) }, (_, i) => (
                  <Table.Row key={`skeleton-${i}`} id={`skeleton-${i}`}>
                    {[
                      ...headers.map((header) => (
                        <Table.Cell key={header.id}>
                          <span className="block h-4 w-3/4 animate-pulse rounded bg-fill-tertiary" />
                        </Table.Cell>
                      )),
                      ...(rowActions ? [<Table.Cell key="__actions" />] : []),
                    ]}
                  </Table.Row>
                ))
              : rows.map((row) => (
                  <Table.Row key={row.id} id={row.id}>
                    {[
                      ...row.getAllCells().map((cell) => {
                        const meta = cell.column.columnDef.meta
                        return (
                          <Table.Cell key={cell.id} className={cx(alignClass(meta), meta?.className)}>
                            <table.FlexRender cell={cell} />
                          </Table.Cell>
                        )
                      }),
                      ...(rowActions
                        ? [
                            <Table.Cell key="__actions" className="text-end">
                              <div className="flex justify-end gap-1">{rowActions(row.original)}</div>
                            </Table.Cell>,
                          ]
                        : []),
                    ]}
                  </Table.Row>
                ))}
          </Table.Body>
        </Table.Content>
      </Table.ScrollContainer>
      {pagination ? <Table.Footer>{pagination}</Table.Footer> : null}
    </Table>
  )

}

interface TablePaginationProps {
  page: number
  pageCount: number
  pageSize: number
  total: number
  onPageChange: ((page: number) => void) | undefined
}

/** HeroUI Pagination with a locale-aware "1–20 of 135" summary. */
export function TablePagination({ page: current, pageCount: count, pageSize: size, total: rowsTotal, onPageChange: change }: TablePaginationProps) {
  const t = useT('common')
  const locale = useLocale()
  const from = (current - 1) * size + 1
  const to = Math.min(current * size, rowsTotal)
  return (
    <Pagination size="sm" aria-label={t('table.pagination')} className="w-full justify-between px-2 py-1">
      <Pagination.Summary>
        {t('table.range', {
          from: formatNumber(from, locale),
          to: formatNumber(to, locale),
          total: formatNumber(rowsTotal, locale),
        })}
      </Pagination.Summary>
      <Pagination.Content>
        <Pagination.Item>
          <Pagination.Previous
            isDisabled={current <= 1 || !change}
            onPress={() => change?.(current - 1)}
            aria-label={t('table.previous')}
          >
            <Pagination.PreviousIcon />
          </Pagination.Previous>
        </Pagination.Item>
        {pageWindow(current, count).map((p, i) =>
          p === 'ellipsis' ? (
            <Pagination.Item key={`e${i}`}>
              <Pagination.Ellipsis />
            </Pagination.Item>
          ) : (
            <Pagination.Item key={p}>
              <Pagination.Link
                isActive={p === current}
                aria-label={t('table.page', { page: p })}
                aria-current={p === current ? 'page' : undefined}
                onPress={() => change?.(p)}
              >
                {p}
              </Pagination.Link>
            </Pagination.Item>
          ),
        )}
        <Pagination.Item>
          <Pagination.Next
            isDisabled={current >= count || !change}
            onPress={() => change?.(current + 1)}
            aria-label={t('table.next')}
          >
            <Pagination.NextIcon />
          </Pagination.Next>
        </Pagination.Item>
      </Pagination.Content>
    </Pagination>
  )
}
