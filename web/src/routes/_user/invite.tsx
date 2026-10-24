import { Button } from '@heroui/react'
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { ArrowRightLeft, BadgePercent, Banknote, Hourglass, Users, Wallet } from 'lucide-react'
import { lazy, Suspense, useMemo, useState } from 'react'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { EmptyState } from '@/components/EmptyState'
import { DateTime } from '@/components/Formatters'
import { TitleWithHelp } from '@/components/HelpTip'
import { PageShell } from '@/components/PageShell'
import { StatCard } from '@/components/StatCard'
import { CodesList } from '@/features/invite/CodesList'
import { InviteLinkCard } from '@/features/invite/InviteLinkCard'
import { useFormatMoney } from '@/features/shop/site'
import { SiteMoney } from '@/features/shop/SiteMoney'
import { loadNamespaces, type TFunction, useT } from '@/i18n'
import * as userApi from '@/lib/api/user'
import { commissionsQuery, inviteQuery, qk } from '@/lib/api/queries'
import type { CommissionLog } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { useAuthStore } from '@/stores/auth'

const TransferDialog = lazy(() => import('@/features/invite/TransferDialog'))
const WithdrawDialog = lazy(() => import('@/features/invite/WithdrawDialog'))

const PAGE_SIZE = 10

export const Route = createFileRoute('/_user/invite')({
  validateSearch: (search: Record<string, unknown>): { page?: number } => {
    const page = Number(search.page)
    return Number.isInteger(page) && page > 1 ? { page } : {}
  },
  // Loaders stay in the entry chunk (only components are split): import the query factories lazily.
  loader: async ({ context }) => {
    const q = await import('@/lib/api/queries')
    return Promise.all([
      loadNamespaces(['invite', 'common']),
      context.queryClient.query({ ...q.inviteQuery(), staleTime: 'static' }),
      context.queryClient.query({ ...q.guestConfigQuery(), staleTime: 'static' }).catch(() => null),
    ])
  },
  component: InvitePage,
})

const col = createDataTableColumns<CommissionLog>()
function commissionColumns(t: TFunction) {
  return col.columns([
    col.accessor('trade_no', {
      header: t('history.tradeNo'),
      enableSorting: false,
      meta: { isPrimary: true, isRowHeader: true },
      cell: (c) => <span className="font-mono text-footnote">{c.getValue()}</span>,
    }),
    col.accessor('order_amount', {
      header: t('history.orderAmount'),
      enableSorting: false,
      meta: { align: 'end' },
      cell: (c) => <SiteMoney cents={c.getValue()} />,
    }),
    col.accessor('get_amount', {
      header: t('history.commission'),
      enableSorting: false,
      meta: { align: 'end' },
      cell: (c) => <SiteMoney cents={c.getValue()} className="font-semibold text-label-green" />,
    }),
    col.accessor('created_at', {
      header: t('history.date'),
      enableSorting: false,
      cell: (c) => <DateTime value={c.getValue()} />,
    }),
  ])
}

function InvitePage() {
  const t = useT(['invite', 'common'])
  const money = useFormatMoney()
  const queryClient = useQueryClient()
  const navigate = useNavigate({ from: Route.fullPath })
  const { page = 1 } = Route.useSearch()
  const { data: info } = useSuspenseQuery(inviteQuery())
  const history = useQuery(commissionsQuery({ page, page_size: PAGE_SIZE }))
  const columns = useMemo(() => commissionColumns(t), [t])
  const commission = useAuthStore((s) => s.user?.commission_balance ?? 0)
  const [dialog, setDialog] = useState<'transfer' | 'withdraw' | null>(null)

  const limit = info.stat.invite_gen_limit
  const canGenerate = limit <= 0 || info.codes.length < limit
  const generate = useMutation({
    mutationFn: () => userApi.createInviteCode(),
    onSuccess: async () => {
      toast.success(t('codes.generated'))
      await queryClient.invalidateQueries({ queryKey: qk.user.invite() })
    },
    onError: (e) => toast.danger(errorMessage(e)),
  })

  return (
    <PageShell title={t('title')} help={t('subtitle')}>
      <h2 className="sr-only">{t('stats.label')}</h2>
      <ul className="grid grid-cols-2 gap-3 lg:grid-cols-4" aria-label={t('stats.label')}>
        <li>
          <StatCard
            className="h-full"
            label={t('stats.registered')}
            value={info.stat.register_count}
            icon={<Users className="size-4" />}
            accent="bg-system-indigo"
          />
        </li>
        <li>
          <StatCard
            className="h-full"
            label={t('stats.pending')}
            value={info.stat.commission_pending}
            format={money}
            icon={<Hourglass className="size-4" />}
            accent="bg-system-orange"
            help={t('stats.pendingHint')}
          />
        </li>
        <li>
          <StatCard
            className="h-full"
            label={t('stats.paid')}
            value={info.stat.commission_paid}
            format={money}
            icon={<Banknote className="size-4" />}
            accent="bg-system-green"
          />
        </li>
        <li>
          <StatCard
            className="h-full"
            label={t('stats.rate')}
            value={`${info.stat.commission_rate}%`}
            icon={<BadgePercent className="size-4" />}
            accent="bg-system-blue"
          />
        </li>
      </ul>

      <div className="grid gap-5 lg:grid-cols-2">
        <InviteLinkCard
          codes={info.codes}
          canGenerate={canGenerate}
          onGenerate={() => generate.mutate()}
          isGenerating={generate.isPending}
        />
        <section className="surface-card flex flex-col gap-4 p-5" aria-labelledby="wallet-title">
          <div className="flex items-center gap-3">
            <span
              className="flex size-9 items-center justify-center rounded-[10px] bg-system-green text-white"
              aria-hidden="true"
            >
              <Wallet className="size-5" />
            </span>
            <TitleWithHelp help={t('wallet.subtitle')} subject={t('wallet.title')}>
              <h2 id="wallet-title" className="text-headline text-label">
                {t('wallet.title')}
              </h2>
            </TitleWithHelp>
          </div>
          <SiteMoney cents={commission} className="text-large-title text-label" />
          <div className="mt-auto flex flex-wrap gap-2">
            <Button variant="secondary" isDisabled={commission <= 0} onPress={() => setDialog('transfer')}>
              <ArrowRightLeft className="size-4" aria-hidden="true" />
              {t('wallet.transfer')}
            </Button>
            <Button variant="secondary" isDisabled={commission <= 0} onPress={() => setDialog('withdraw')}>
              <Banknote className="size-4" aria-hidden="true" />
              {t('wallet.withdraw')}
            </Button>
          </div>
          {commission <= 0 ? <p className="text-footnote text-label-secondary">{t('wallet.empty')}</p> : null}
        </section>
      </div>

      <CodesList
        codes={info.codes}
        limit={limit}
        onGenerate={() => generate.mutate()}
        isGenerating={generate.isPending}
      />

      <section className="flex flex-col gap-3" aria-labelledby="history-title">
        <h2 id="history-title" className="text-title3 text-label">
          {t('history.title')}
        </h2>
        <DataTable
          aria-label={t('history.title')}
          columns={columns}
          data={history.data?.items}
          total={history.data?.total}
          isLoading={history.isPending}
          isFetching={history.isFetching && !history.isPending}
          page={page}
          pageSize={PAGE_SIZE}
          onPageChange={(next) => void navigate({ search: { page: next }, resetScroll: false })}
          getRowId={(row, i) => `${row.trade_no}-${i}`}
          emptyState={
            history.isError ? (
              <EmptyState
                compact
                title={t('common:states.error')}
                description={errorMessage(history.error)}
              />
            ) : (
              <EmptyState compact title={t('history.empty')} description={t('history.emptyHint')} />
            )
          }
        />
      </section>

      <Suspense fallback={null}>
        {dialog === 'transfer' ? (
          <TransferDialog isOpen onOpenChange={(open) => !open && setDialog(null)} available={commission} />
        ) : null}
        {dialog === 'withdraw' ? (
          <WithdrawDialog isOpen onOpenChange={(open) => !open && setDialog(null)} available={commission} />
        ) : null}
      </Suspense>
    </PageShell>
  )
}
