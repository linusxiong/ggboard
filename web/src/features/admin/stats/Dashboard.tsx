/**
 * Admin dashboard (`/admin/dashboard`): overview tiles, trend chart, traffic rankings.
 * Every query refetches each minute while the tab is visible (React Query pauses interval
 * refetches in background tabs). Days are the server's calendar days: the overview's `today.date`
 * drives the ranking date, not the browser clock.
 */
import { Button } from '@heroui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { PageShell } from '@/components/PageShell'
import { useT } from '@/i18n'
import { adminOverviewQuery } from '@/lib/api/queries'
import { DASHBOARD_REFRESH } from '../shared/queries'
import { useFormatters } from '../shared/useFormatters'
import { OverviewTiles } from './OverviewTiles'
import { RankingCards } from './RankingCards'
import { TrendCard } from './TrendCard'

export function AdminDashboard() {
  const t = useT(['admin-stats', 'common'])
  const fmt = useFormatters()
  const queryClient = useQueryClient()
  const overview = useQuery({ ...adminOverviewQuery(), refetchInterval: DASHBOARD_REFRESH })
  const refreshing = overview.isFetching

  return (
    <PageShell
      title={t('title')}
      description={
        overview.dataUpdatedAt
          ? t('updatedAt', { time: fmt.dateTime(new Date(overview.dataUpdatedAt).toISOString()) })
          : t('loading')
      }
      help={t('help')}
      actions={
        <Button
          variant="secondary"
          size="sm"
          isPending={refreshing}
          onPress={() =>
            void queryClient.invalidateQueries({
              predicate: (q) =>
                q.queryKey[0] === 'admin' &&
                ['overview', 'series', 'ranking'].includes(String(q.queryKey[1])),
            })
          }
        >
          <RefreshCw className="size-4" aria-hidden="true" />
          {t('common:actions.refresh')}
        </Button>
      }
    >
      {overview.isError && !overview.data ? (
        <p role="alert" className="surface-card p-5 text-subhead text-label-red">
          {t('loadError')}
        </p>
      ) : null}
      <OverviewTiles data={overview.data} fmt={fmt} />
      <TrendCard fmt={fmt} />
      <RankingCards
        fmt={fmt}
        serverDay={overview.data?.today.date}
        zone={overview.data?.timezone}
        overviewLoading={overview.isPending}
      />
    </PageShell>
  )
}
