/** Per-node drawers: traffic chart (7d/30d) and online users with their IPs. */
import { Button } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { BarChart, byteUnit } from '@/components/charts'
import { SegmentedControl } from '@/components/SegmentedControl'
import { useT } from '@/i18n'
import type { Node, StatsRange } from '@/lib/api/types'
import { FormDrawer } from '../shared/FormDrawer'
import { adminNodeOnlineQuery, adminNodeStatsQuery } from '../shared/queries'
import { useFormatters } from '../shared/useFormatters'

interface DrawerProps {
  node: Node
  onClose: () => void
}

export function NodeStatsDrawer({ node, onClose }: DrawerProps) {
  const t = useT(['admin-nodes', 'admin-stats'])
  const fmt = useFormatters()
  const [range, setRange] = useState<Extract<StatsRange, '7d' | '30d'>>('7d')
  const stats = useQuery(adminNodeStatsQuery(node.id, range))
  const points = stats.data ?? []
  const up = points.reduce((s, p) => s + p.u, 0)
  const down = points.reduce((s, p) => s + p.d, 0)
  return (
    <FormDrawer
      isOpen
      onOpenChange={(open) => !open && onClose()}
      size="xl"
      title={t('nodes.stats.title', { name: node.name })}
      subtitle={node.parent_id ? t('nodes.stats.childHint') : undefined}
    >
      <div className="flex flex-col gap-5">
        <SegmentedControl
          aria-label={t('admin-stats:rangeLabel')}
          value={range}
          onChange={setRange}
          items={(['7d', '30d'] as const).map((r) => ({ id: r, label: t(`admin-stats:ranges.${r}`) }))}
        />
        <dl className="grid grid-cols-3 gap-3">
          {[
            [t('nodes.stats.upload'), up],
            [t('nodes.stats.download'), down],
            [t('nodes.stats.total'), up + down],
          ].map(([label, value]) => (
            <div key={String(label)} className="rounded-[14px] bg-fill-quaternary p-3">
              <dt className="text-footnote text-label-secondary">{label}</dt>
              <dd className="text-headline text-label">{fmt.bytes(Number(value))}</dd>
            </div>
          ))}
        </dl>
        <BarChart
          aria-label={t('nodes.stats.chart', { name: node.name, range: t(`admin-stats:ranges.${range}`) })}
          labels={points.map((p) => p.date)}
          series={[
            { id: 'u', label: t('nodes.stats.upload'), values: points.map((p) => p.u) },
            { id: 'd', label: t('nodes.stats.download'), values: points.map((p) => p.d) },
          ]}
          stacked
          tickUnit={byteUnit(Math.max(0, ...points.map((p) => p.u + p.d)))}
          formatValue={fmt.bytes}
          formatTick={fmt.bytesCompact}
          formatLabel={fmt.shortDate}
          labelHeader={t('admin-stats:date')}
          isFetching={stats.isFetching && !stats.isPending}
          emptyLabel={stats.isPending ? t('admin-stats:loading') : t('admin-stats:noSeries')}
          height={260}
        />
      </div>
    </FormDrawer>
  )
}

export function NodeOnlineDrawer({ node, onClose }: DrawerProps) {
  const t = useT(['admin-nodes', 'common'])
  const online = useQuery(adminNodeOnlineQuery(node.id))
  const rows = online.data ?? []
  return (
    <FormDrawer
      isOpen
      onOpenChange={(open) => !open && onClose()}
      size="lg"
      title={t('nodes.online.title', { name: node.name })}
      subtitle={t('nodes.online.count', { count: rows.length })}
    >
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-2">
          <p className="text-footnote text-label-secondary">{t('nodes.online.hint')}</p>
          <Button
            size="sm"
            variant="secondary"
            isPending={online.isFetching}
            onPress={() => void online.refetch()}
          >
            <RefreshCw className="size-4" aria-hidden="true" />
            {t('common:actions.refresh')}
          </Button>
        </div>
        {online.isPending ? (
          <p className="text-subhead text-label-secondary">{t('common:states.loading')}</p>
        ) : rows.length === 0 ? (
          <p className="rounded-[14px] bg-fill-quaternary p-6 text-center text-subhead text-label-secondary">
            {t('nodes.online.empty')}
          </p>
        ) : (
          <table className="w-full text-start text-subhead">
            <caption className="sr-only">{t('nodes.online.title', { name: node.name })}</caption>
            <thead>
              <tr className="text-footnote text-label-secondary">
                <th scope="col" className="py-2 text-start font-normal">
                  {t('nodes.online.user')}
                </th>
                <th scope="col" className="py-2 text-start font-normal">
                  {t('nodes.online.ips')}
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.user_id} className="hairline-t align-top">
                  <th scope="row" className="py-2 pe-3 text-start font-normal">
                    <span className="block break-all text-label">{r.email}</span>
                    <span className="text-footnote text-label-secondary">#{r.user_id}</span>
                  </th>
                  <td className="py-2">
                    <ul className="flex flex-col gap-0.5 font-mono text-footnote text-label">
                      {r.ips.map((ip) => (
                        <li key={ip} className="break-all">
                          {ip}
                        </li>
                      ))}
                    </ul>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </FormDrawer>
  )
}
