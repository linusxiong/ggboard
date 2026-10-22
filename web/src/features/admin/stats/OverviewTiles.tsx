import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { Sparkline } from '@/components/charts'
import { useT } from '@/i18n'
import { adminSeriesQuery } from '@/lib/api/queries'
import type { AdminOverview, StatsMetric } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { DASHBOARD_REFRESH } from '../shared/queries'
import type { Formatters } from '../shared/useFormatters'

interface TileProps {
  label: string
  value: ReactNode
  hint?: ReactNode
  trend?: ReactNode
  isLoading?: boolean
}

/** KPI tile: label, proportional-figure value, optional hint and 7-day sparkline. */
function Tile({ label, value, hint, trend, isLoading }: TileProps) {
  return (
    <section className="surface-card flex min-h-[7.5rem] flex-col justify-between gap-2 p-5">
      <h3 className="text-subhead text-label-secondary">{label}</h3>
      <div className="flex items-end justify-between gap-3">
        {isLoading ? (
          <span className="block h-7 w-24 animate-pulse rounded bg-fill-tertiary" aria-hidden="true" />
        ) : (
          <p className="text-title2 text-label">{value}</p>
        )}
        {trend}
      </div>
      {hint ? <p className="text-footnote text-label-secondary">{hint}</p> : null}
    </section>
  )
}

/** 7-day sparkline for a metric (own query; cached and shared with the trend card). */
function Trend({
  metric,
  label,
  format,
}: {
  metric: StatsMetric
  label: string
  format: (v: number) => string
}) {
  const t = useT('admin-stats')
  const { data } = useQuery({ ...adminSeriesQuery(metric, '7d'), refetchInterval: DASHBOARD_REFRESH })
  if (!data || data.length < 2) return <span className="h-7 w-24" aria-hidden="true" />
  const values = data.map((p) => p.value)
  return (
    <Sparkline
      values={values}
      aria-label={t('sparkline', {
        metric: label,
        first: format(values[0] ?? 0),
        last: format(values.at(-1) ?? 0),
      })}
    />
  )
}

function Group({ title, children, className }: { title: string; children: ReactNode; className?: string }) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-title3 text-label">{title}</h2>
      <div className={cx('grid gap-3 sm:grid-cols-2', className)}>{children}</div>
    </section>
  )
}

export function OverviewTiles({ data, fmt }: { data: AdminOverview | undefined; fmt: Formatters }) {
  const t = useT('admin-stats')
  const loading = !data
  return (
    <div className="flex flex-col gap-6">
      <Group title={t('sections.today')} className="lg:grid-cols-4">
        <Tile
          label={t('tiles.income')}
          value={fmt.money(data?.today.income ?? 0)}
          isLoading={loading}
          trend={<Trend metric="income" label={t('tiles.income')} format={fmt.money} />}
        />
        <Tile
          label={t('tiles.paidOrders')}
          value={fmt.count(data?.today.paid_orders ?? 0)}
          isLoading={loading}
          trend={<Trend metric="orders" label={t('tiles.paidOrders')} format={fmt.count} />}
        />
        <Tile
          label={t('tiles.registers')}
          value={fmt.count(data?.today.registers ?? 0)}
          isLoading={loading}
          trend={<Trend metric="registers" label={t('tiles.registers')} format={fmt.count} />}
        />
        <Tile
          label={t('tiles.commission')}
          value={fmt.money(data?.today.commission ?? 0)}
          isLoading={loading}
        />
      </Group>
      <Group title={t('sections.month')} className="lg:grid-cols-3">
        <Tile label={t('tiles.income')} value={fmt.money(data?.month.income ?? 0)} isLoading={loading} />
        <Tile
          label={t('tiles.paidOrders')}
          value={fmt.count(data?.month.paid_orders ?? 0)}
          isLoading={loading}
        />
        <Tile
          label={t('tiles.registers')}
          value={fmt.count(data?.month.registers ?? 0)}
          isLoading={loading}
        />
      </Group>
      <Group title={t('sections.operations')} className="lg:grid-cols-5">
        <Tile
          label={t('tiles.users')}
          value={fmt.count(data?.totals.users ?? 0)}
          isLoading={loading}
          hint={t('tiles.usersHint', {
            active: fmt.count(data?.totals.active_users ?? 0),
            online: fmt.count(data?.totals.online_users ?? 0),
          })}
        />
        <Tile
          label={t('tiles.nodes')}
          value={`${fmt.count(data?.nodes.online ?? 0)} / ${fmt.count(data?.nodes.total ?? 0)}`}
          isLoading={loading}
          hint={t('tiles.nodesHint', { total: fmt.count(data?.nodes.total ?? 0) })}
        />
        <Tile
          label={t('tiles.pendingTickets')}
          value={fmt.count(data?.totals.pending_tickets ?? 0)}
          isLoading={loading}
        />
        <Tile
          label={t('tiles.pendingOrders')}
          value={fmt.count(data?.totals.pending_orders ?? 0)}
          isLoading={loading}
        />
        <Tile
          label={t('tiles.pendingCommission')}
          value={fmt.money(data?.totals.pending_commission ?? 0)}
          isLoading={loading}
        />
      </Group>
    </div>
  )
}
