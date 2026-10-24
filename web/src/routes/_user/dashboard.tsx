import { useQuery } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { LifeBuoy, Receipt, Wallet } from 'lucide-react'
import { Money } from '@/components/Formatters'
import { PageShell } from '@/components/PageShell'
import { StatCard } from '@/components/StatCard'
import { NodesList } from '@/features/dashboard/NodesList'
import { NoticesCard } from '@/features/dashboard/NoticesCard'
import { PlanCard } from '@/features/dashboard/PlanCard'
import { QueryStatus } from '@/features/dashboard/QueryStatus'
import { QuickActions } from '@/features/dashboard/QuickActions'
import { TrafficChart } from '@/features/dashboard/TrafficChart'
import { lastDays } from '@/features/dashboard/traffic'
import { loadNamespaces, useLocale, useT } from '@/i18n'
import {
  guestConfigQuery,
  noticesQuery,
  subscriptionQuery,
  trafficLogsQuery,
  userNodesQuery,
  userStatsQuery,
} from '@/lib/api/queries'
import { formatBytes } from '@/lib/format'
import { useAuthStore } from '@/stores/auth'

const NOTICES = { page: 1, page_size: 3 }
/** Newest first; 200 rows cover 7 days for up to ~28 nodes. */
const TRAFFIC = { page: 1, page_size: 200 }

export const Route = createFileRoute('/_user/dashboard')({
  // Loaders stay in the entry bundle (only components are auto-split): keep the query factories lazy.
  loader: async ({ context: { queryClient } }) => {
    const [q] = await Promise.all([import('@/lib/api/queries'), loadNamespaces(['dashboard'])])
    const prime = (p: Promise<unknown>) => p.catch(() => undefined)
    await Promise.all([
      prime(queryClient.query({ ...q.subscriptionQuery(), staleTime: 'static' })),
      prime(queryClient.query({ ...q.userStatsQuery(), staleTime: 'static' })),
      prime(queryClient.query({ ...q.noticesQuery(NOTICES), staleTime: 'static' })),
      prime(queryClient.query({ ...q.userNodesQuery(), staleTime: 'static' })),
      prime(queryClient.query({ ...q.trafficLogsQuery(TRAFFIC), staleTime: 'static' })),
    ])
  },
  component: DashboardPage,
})

function greetingKey(hour: number) {
  if (hour >= 5 && hour < 11) return 'greeting.morning'
  if (hour >= 11 && hour < 13) return 'greeting.noon'
  if (hour >= 13 && hour < 18) return 'greeting.afternoon'
  return 'greeting.evening'
}

function DashboardPage() {
  const t = useT(['dashboard', 'common'])
  const locale = useLocale()
  const user = useAuthStore((s) => s.user)
  const sub = useQuery(subscriptionQuery())
  const stats = useQuery(userStatsQuery())
  const notices = useQuery(noticesQuery(NOTICES))
  const nodes = useQuery(userNodesQuery())
  const logs = useQuery(trafficLogsQuery(TRAFFIC))
  const config = useQuery(guestConfigQuery())
  const currency = config.data ? { currency: config.data.currency, symbol: config.data.currency_symbol } : {}

  const name = user?.email.split('@')[0] ?? ''
  const days = logs.data ? lastDays(logs.data.items, 7) : undefined
  const weekTotal = days?.reduce((sum, d) => sum + d.total, 0) ?? 0

  return (
    <PageShell title={t(greetingKey(new Date().getHours()), { name })} setDocumentTitle={false}>
      <title>{`${t('title')} · GG Bond`}</title>

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <QueryStatus query={sub} minHeight={292}>
            {(data) => <PlanCard sub={data} />}
          </QueryStatus>
        </div>
        <QuickActions subscribeUrl={sub.data?.subscribe_url} />
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Link to="/orders" className="block rounded-card">
          <StatCard
            label={t('stats.pendingOrders')}
            value={stats.data?.pending_orders ?? '—'}
            icon={<Receipt className="size-4" />}
            accent="bg-system-orange"
            className="h-full"
          />
        </Link>
        <Link to="/tickets" className="block rounded-card">
          <StatCard
            label={t('stats.openTickets')}
            value={stats.data?.open_tickets ?? '—'}
            icon={<LifeBuoy className="size-4" />}
            accent="bg-system-teal"
            className="h-full"
          />
        </Link>
        <StatCard
          label={t('stats.balance')}
          value={user ? <Money cents={user.balance} {...currency} /> : '—'}
          hint={
            user && user.commission_balance > 0 ? (
              <>
                {t('stats.commission')} <Money cents={user.commission_balance} {...currency} />
              </>
            ) : undefined
          }
          icon={<Wallet className="size-4" />}
          accent="bg-system-green"
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-5">
        <section
          className="surface-card flex flex-col gap-4 p-6 lg:col-span-3"
          aria-labelledby="traffic-title"
        >
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <h2 id="traffic-title" className="text-headline text-label">
              {t('traffic.title')}
            </h2>
            {days ? (
              <p className="text-footnote text-label-secondary">
                {t('traffic.weekTotal', { value: formatBytes(weekTotal, locale) })}
              </p>
            ) : null}
          </div>
          {days ? (
            <TrafficChart days={days} />
          ) : logs.isError ? (
            <p
              role="alert"
              className="flex h-[180px] items-center justify-center text-subhead text-label-secondary"
            >
              {t('traffic.error')}
            </p>
          ) : (
            <div className="h-[180px] shimmer rounded-[12px] bg-fill-quaternary" aria-busy="true" />
          )}
        </section>
        <div className="lg:col-span-2">
          <QueryStatus query={notices} minHeight={260}>
            {(data) => <NoticesCard notices={data.items} />}
          </QueryStatus>
        </div>
      </div>

      <QueryStatus query={nodes} minHeight={200}>
        {(data) => <NodesList nodes={data} />}
      </QueryStatus>
    </PageShell>
  )
}
