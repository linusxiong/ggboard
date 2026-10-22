import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { byteUnit, LineChart } from '@/components/charts'
import { useT } from '@/i18n'
import { adminSeriesQuery } from '@/lib/api/queries'
import type { StatsMetric, StatsRange } from '@/lib/api/types'
import { DASHBOARD_REFRESH } from '../shared/queries'
import { Segmented } from '../shared/Segmented'
import type { Formatters } from '../shared/useFormatters'

const METRICS: readonly StatsMetric[] = ['income', 'orders', 'registers', 'traffic']
const RANGES: readonly StatsRange[] = ['7d', '30d', '90d']

/** Value/tick formatters per metric (money in cents, traffic in bytes, counts). */
function metricFormat(metric: StatsMetric, fmt: Formatters) {
  switch (metric) {
    case 'income':
      return { value: fmt.money, tick: fmt.moneyCompact }
    case 'traffic':
      return { value: fmt.bytes, tick: fmt.bytesCompact }
    default:
      return { value: fmt.count, tick: fmt.countCompact }
  }
}

export function TrendCard({ fmt }: { fmt: Formatters }) {
  const t = useT('admin-stats')
  const [metric, setMetric] = useState<StatsMetric>('income')
  const [range, setRange] = useState<StatsRange>('30d')
  const series = useQuery({ ...adminSeriesQuery(metric, range), refetchInterval: DASHBOARD_REFRESH })
  const format = metricFormat(metric, fmt)
  const points = series.data ?? []
  const metricLabel = t(`metrics.${metric}`)

  return (
    <section className="surface-card flex flex-col gap-4 p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-title3 text-label">{t('sections.trend')}</h2>
        <div className="flex flex-wrap items-center gap-2">
          <Segmented
            aria-label={t('metricLabel')}
            value={metric}
            onChange={setMetric}
            options={METRICS.map((m) => ({ value: m, label: t(`metrics.${m}`) }))}
          />
          <Segmented
            aria-label={t('rangeLabel')}
            value={range}
            onChange={setRange}
            options={RANGES.map((r) => ({ value: r, label: t(`ranges.${r}`) }))}
          />
        </div>
      </div>
      {series.isError && !series.data ? (
        <p role="alert" className="text-subhead text-label-red">
          {t('loadError')}
        </p>
      ) : (
        <LineChart
          aria-label={t('chartLabel', { metric: metricLabel, range: t(`ranges.${range}`) })}
          labels={points.map((p) => p.date)}
          series={[{ id: metric, label: metricLabel, values: points.map((p) => p.value) }]}
          formatValue={format.value}
          formatTick={format.tick}
          tickUnit={metric === 'traffic' ? byteUnit(Math.max(0, ...points.map((p) => p.value))) : 1}
          formatLabel={fmt.shortDate}
          labelHeader={t('date')}
          isFetching={series.isFetching && !series.isPending}
          emptyLabel={series.isPending ? t('loading') : t('noSeries')}
          height={260}
        />
      )}
    </section>
  )
}
