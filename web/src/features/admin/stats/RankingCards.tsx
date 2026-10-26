import { Label, ListBox, Select } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChartNoAxesColumn } from 'lucide-react'
import { useMemo, useState } from 'react'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { EmptyState } from '@/components/EmptyState'
import { TitleWithHelp } from '@/components/HelpTip'
import { useT } from '@/i18n'
import type { ISODate, NodeRanking, StatsTimeZone, UserRanking } from '@/lib/api/types'
import { adminNodeRankingQuery, adminUserRankingQuery } from '../shared/queries'
import type { Formatters } from '../shared/useFormatters'
import { offsetName } from './zone'

const nodeCol = createDataTableColumns<NodeRanking & { rank: number }>()
const userCol = createDataTableColumns<UserRanking & { rank: number }>()
const LIMITS = ['10', '20', '50'] as const

/** Share-of-total bar behind the total value (decorative; the number is the content). */
function ShareBar({ ratio }: { ratio: number }) {
  return (
    <span className="mt-1 block h-1 w-full overflow-hidden rounded-full bg-fill-tertiary" aria-hidden="true">
      <span
        className="block h-full rounded-full bg-system-blue"
        style={{ width: `${Math.round(ratio * 100)}%` }}
      />
    </span>
  )
}

interface RankingCardsProps {
  fmt: Formatters
  /** The server's statistics day (overview `today.date`): the default and latest rankable day. */
  serverDay: ISODate | undefined
  /** The zone statistics days are counted in (overview `timezone`). */
  zone: StatsTimeZone | undefined
  /** The overview has not answered yet (`serverDay` may still arrive). */
  overviewLoading: boolean
}

/**
 * Node and user traffic of one statistics day. Days are server calendar days, which can be a day
 * ahead of or behind the browser's, so the day comes from the overview, never from the browser clock.
 */
export function RankingCards({ fmt, serverDay, zone, overviewLoading }: RankingCardsProps) {
  const t = useT('admin-stats')
  // null = the server's today, following it across midnight; otherwise the past day the admin picked
  // (kept across refreshes).
  const [picked, setPicked] = useState<ISODate | null>(null)
  const date = picked !== null && (serverDay === undefined || picked <= serverDay) ? picked : serverDay
  const [limit, setLimit] = useState<(typeof LIMITS)[number]>('10')
  // Wait for the server's day so each request names the day the date field shows; if the overview
  // fails, rank without a date (the server then uses its own today).
  const enabled = date !== undefined || !overviewLoading
  const nodes = useQuery({ ...adminNodeRankingQuery(date), enabled })
  const users = useQuery({ ...adminUserRankingQuery(date, Number(limit)), enabled })
  const zoneName = zone
    ? zone.name.startsWith('UTC')
      ? zone.name
      : t('zoneWithOffset', { name: zone.name, offset: offsetName(zone.offset) })
    : null

  const nodeRows = useMemo(() => (nodes.data ?? []).map((r, i) => ({ ...r, rank: i + 1 })), [nodes.data])
  const userRows = useMemo(() => (users.data ?? []).map((r, i) => ({ ...r, rank: i + 1 })), [users.data])
  const nodeMax = Math.max(1, ...nodeRows.map((r) => r.total))
  const userMax = Math.max(1, ...userRows.map((r) => r.total))

  const nodeColumns = useMemo(
    () =>
      nodeCol.columns([
        nodeCol.accessor('rank', {
          header: t('columns.rank'),
          enableSorting: false,
          meta: { hideOnMobile: true, className: 'w-12 whitespace-nowrap' },
        }),
        nodeCol.accessor('name', {
          header: t('columns.node'),
          enableSorting: false,
          meta: { isPrimary: true, isRowHeader: true },
        }),
        nodeCol.accessor('u', {
          header: t('columns.upload'),
          enableSorting: false,
          meta: { align: 'end' },
          cell: (c) => <span className="tabular whitespace-nowrap">{fmt.bytes(c.getValue())}</span>,
        }),
        nodeCol.accessor('d', {
          header: t('columns.download'),
          enableSorting: false,
          meta: { align: 'end' },
          cell: (c) => <span className="tabular whitespace-nowrap">{fmt.bytes(c.getValue())}</span>,
        }),
        nodeCol.accessor('total', {
          header: t('columns.total'),
          enableSorting: false,
          meta: { align: 'end', className: 'min-w-28' },
          cell: (c) => (
            <span className="block">
              <span className="tabular whitespace-nowrap font-semibold">{fmt.bytes(c.getValue())}</span>
              <ShareBar ratio={c.getValue() / nodeMax} />
            </span>
          ),
        }),
      ]),
    [t, fmt, nodeMax],
  )
  const userColumns = useMemo(
    () =>
      userCol.columns([
        userCol.accessor('rank', {
          header: t('columns.rank'),
          enableSorting: false,
          meta: { hideOnMobile: true, className: 'w-12 whitespace-nowrap' },
        }),
        userCol.accessor('email', {
          header: t('columns.user'),
          enableSorting: false,
          meta: { isPrimary: true, isRowHeader: true },
          cell: (c) => (
            <Link
              to="/admin/users"
              search={{ email: c.getValue() }}
              title={c.getValue()}
              className="block max-w-48 truncate text-label-blue underline-offset-2 hover:underline"
            >
              {c.getValue()}
            </Link>
          ),
        }),
        userCol.accessor('u', {
          header: t('columns.upload'),
          enableSorting: false,
          meta: { align: 'end' },
          cell: (c) => <span className="tabular whitespace-nowrap">{fmt.bytes(c.getValue())}</span>,
        }),
        userCol.accessor('d', {
          header: t('columns.download'),
          enableSorting: false,
          meta: { align: 'end' },
          cell: (c) => <span className="tabular whitespace-nowrap">{fmt.bytes(c.getValue())}</span>,
        }),
        userCol.accessor('total', {
          header: t('columns.total'),
          enableSorting: false,
          meta: { align: 'end', className: 'min-w-28' },
          cell: (c) => (
            <span className="block">
              <span className="tabular whitespace-nowrap font-semibold">{fmt.bytes(c.getValue())}</span>
              <ShareBar ratio={c.getValue() / userMax} />
            </span>
          ),
        }),
      ]),
    [t, fmt, userMax],
  )

  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <TitleWithHelp
          help={zoneName ? t('dayHelp', { zone: zoneName }) : t('dayHelpPending')}
          subject={t('sections.ranking')}
        >
          <h2 className="text-title3 text-label">{t('sections.ranking')}</h2>
        </TitleWithHelp>
        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1.5 text-[0.8125rem] leading-[1.125rem] font-semibold text-label-secondary">
            {t('rankingDate')}
            <input
              type="date"
              value={date ?? ''}
              max={serverDay}
              onChange={(e) => {
                const v = e.target.value
                setPicked(v && v !== serverDay ? v : null)
              }}
              className="h-[var(--field-h)] rounded-input bg-fill-tertiary px-3 text-[length:var(--field-fs)] font-normal text-label transition-colors outline-none hover:bg-fill-secondary focus-visible:ring-2 focus-visible:ring-focus"
            />
          </label>
          <Select
            aria-label={t('limit')}
            value={limit}
            onChange={(k) => k !== null && setLimit(String(k) as (typeof LIMITS)[number])}
            className="w-28"
          >
            <Label>{t('limit')}</Label>
            <Select.Trigger>
              <Select.Value />
              <Select.Indicator />
            </Select.Trigger>
            <Select.Popover>
              <ListBox>
                {LIMITS.map((l) => (
                  <ListBox.Item key={l} id={l} textValue={l}>
                    {l}
                    <ListBox.ItemIndicator />
                  </ListBox.Item>
                ))}
              </ListBox>
            </Select.Popover>
          </Select>
        </div>
      </div>
      <div className="grid gap-4 2xl:grid-cols-2">
        <div className="flex flex-col gap-2">
          <h3 className="text-headline text-label">{t('nodeRanking')}</h3>
          <DataTable
            aria-label={t('nodeRanking')}
            columns={nodeColumns}
            data={nodes.data ? nodeRows : undefined}
            isLoading={nodes.isPending}
            isFetching={nodes.isFetching && !nodes.isPending}
            getRowId={(r) => String(r.node_id)}
            emptyState={<EmptyState compact icon={<ChartNoAxesColumn />} title={t('empty')} />}
          />
        </div>
        <div className="flex flex-col gap-2">
          <h3 className="text-headline text-label">{t('userRanking')}</h3>
          <DataTable
            aria-label={t('userRanking')}
            columns={userColumns}
            data={users.data ? userRows : undefined}
            isLoading={users.isPending}
            isFetching={users.isFetching && !users.isPending}
            getRowId={(r) => String(r.user_id)}
            emptyState={<EmptyState compact icon={<ChartNoAxesColumn />} title={t('empty')} />}
          />
        </div>
      </div>
    </section>
  )
}
