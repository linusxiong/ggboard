/**
 * Order page body (`/orders/$tradeNo`, also the payment return target): status hero with the
 * payment countdown / success check, payment method picker + checkout (url / qrcode / html / paid),
 * amounts, timeline and cancel. Polls the status every 3 s while pending (useOrderPolling).
 */
import { Button } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { CircleSlash, Clock, Loader, PauseCircle, Wallet } from 'lucide-react'
import { lazy, type ReactNode, Suspense, useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { EmptyState } from '@/components/EmptyState'
import { DateTime } from '@/components/Formatters'
import { FormError } from '@/components/FormError'
import { confirm } from '@/components/feedback/confirm'
import { useT } from '@/i18n'
import { userApi } from '@/lib/api'
import { paymentMethodsQuery, qk } from '@/lib/api/queries'
import { type CheckoutResult, type ID, ORDER_STATUS, type Order } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { SiteMoney } from '@/features/shop/SiteMoney'
import { AmountBreakdown } from './AmountBreakdown'
import { CheckoutResultView } from './CheckoutResultView'
import { OrderTimeline } from './OrderTimeline'
import { OrderPaymentMethod, PaymentMethodBadge, PaymentPicker } from './PaymentPicker'
import { redirectToGateway } from './redirect'
import { chargedAmount, handlingFee, ORDER_TYPE_KEY, paymentDeadline } from './status'
import { formatCountdown, useCountdown } from './useCountdown'
import { type OrderPolling, useOrderPolling } from './useOrderPolling'

const SuccessCheck = lazy(() => import('./SuccessCheck'))

export function OrderDetail({ order }: { order: Order }) {
  const t = useT(['orders', 'common'])
  const pending = order.status === ORDER_STATUS.pending
  const polling = useOrderPolling(order.trade_no, pending)

  return (
    <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,360px)]">
      <div className="flex min-w-0 flex-col gap-5">
        <StatusHero order={order} polling={polling} />
        {pending ? <PaymentSection order={order} polling={polling} /> : null}
      </div>

      <aside className="flex min-w-0 flex-col gap-5" aria-label={t('detail.side')}>
        <Card title={t('detail.summary')}>
          <dl className="flex flex-col gap-2.5 text-subhead">
            <Info label={t('detail.tradeNo')}>
              <span className="flex items-center justify-end gap-1">
                <span className="truncate font-mono text-footnote">{order.trade_no}</span>
                <CopyButton value={order.trade_no} variant="ghost" />
              </span>
            </Info>
            <Info label={t('detail.plan')}>{order.plan.name}</Info>
            <Info label={t('detail.period')}>{t(`common:periods.${order.period}`)}</Info>
            <Info label={t('detail.type')}>{t(`type.${ORDER_TYPE_KEY[order.type]}`)}</Info>
            <Info label={t('detail.createdAt')}>
              <DateTime value={order.created_at} />
            </Info>
            {order.paid_at ? (
              <Info label={t('detail.paidAt')}>
                <DateTime value={order.paid_at} />
              </Info>
            ) : null}
            {order.payment ? (
              <Info label={t('detail.payment')}>
                <OrderPaymentMethod payment={order.payment} />
              </Info>
            ) : null}
          </dl>
        </Card>
        <Card title={t('detail.amounts')}>
          <AmountBreakdown order={order} />
        </Card>
        <Card title={t('detail.progress')}>
          <OrderTimeline order={order} />
        </Card>
        {pending ? <CancelOrder tradeNo={order.trade_no} /> : null}
      </aside>
    </div>
  )
}

// ------------------------------------------------------------------------------- hero

function StatusHero({ order, polling }: { order: Order; polling: OrderPolling }) {
  const t = useT(['orders', 'common'])
  const { status } = order

  if (status === ORDER_STATUS.completed) {
    return (
      <Hero
        icon={
          <Suspense fallback={<span className="block size-[72px] rounded-full bg-system-green" />}>
            <SuccessCheck label={t('hero.completed')} />
          </Suspense>
        }
        title={t('hero.completed')}
        body={t('hero.completedBody')}
        actions={
          <>
            <Link to="/subscription" className={buttonClass()}>
              {t('hero.toSubscription')}
            </Link>
            <Link to="/dashboard" className={buttonClass({ variant: 'secondary' })}>
              {t('hero.toDashboard')}
            </Link>
          </>
        }
        live
      />
    )
  }
  if (status === ORDER_STATUS.processing) {
    return (
      <Hero
        icon={<IconBadge tone="blue" icon={<Loader className="size-8 motion-safe:animate-spin" />} />}
        title={t('hero.processing')}
        body={t('hero.processingBody')}
        live
      />
    )
  }
  if (status === ORDER_STATUS.cancelled) {
    return (
      <Hero
        icon={<IconBadge tone="gray" icon={<CircleSlash className="size-8" />} />}
        title={t('hero.cancelled')}
        body={t('hero.cancelledBody')}
        actions={
          <Link to="/shop" className={buttonClass({ variant: 'secondary' })}>
            {t('hero.buyAgain')}
          </Link>
        }
      />
    )
  }
  if (status === ORDER_STATUS.discounted) {
    return (
      <Hero
        icon={<IconBadge tone="gray" icon={<Wallet className="size-8" />} />}
        title={t('hero.discounted')}
        body={t('hero.discountedBody')}
      />
    )
  }
  return <PendingHero order={order} polling={polling} />
}

function PendingHero({ order, polling }: { order: Order; polling: OrderPolling }) {
  const t = useT('orders')
  const left = useCountdown(paymentDeadline(order))
  return (
    <Hero
      icon={<IconBadge tone="orange" icon={<Clock className="size-8" />} />}
      title={t('hero.pending')}
      body={
        left > 0 ? (
          <>
            {t('hero.payWithin')}{' '}
            <span className="font-semibold tabular text-label" aria-live="off">
              {formatCountdown(left)}
            </span>
          </>
        ) : (
          t('hero.expiring')
        )
      }
      actions={
        polling.timedOut ? (
          <div className="flex flex-wrap items-center justify-center gap-2">
            <span className="flex items-center gap-1.5 text-footnote text-label-secondary">
              <PauseCircle className="size-4" aria-hidden="true" />
              {t('hero.pollingPaused')}
            </span>
            <Button size="sm" variant="secondary" onPress={polling.restart}>
              {t('hero.refresh')}
            </Button>
          </div>
        ) : null
      }
    />
  )
}

function Hero({
  icon,
  title,
  body,
  actions,
  live,
}: {
  icon: ReactNode
  title: string
  body: ReactNode
  actions?: ReactNode
  live?: boolean
}) {
  return (
    <section
      className="surface-card flex flex-col items-center gap-3 px-6 py-8 text-center"
      aria-live={live ? 'polite' : undefined}
    >
      {icon}
      <h2 className="text-title2 text-label">{title}</h2>
      <p className="max-w-md text-subhead text-label-secondary">{body}</p>
      {actions ? <div className="mt-2 flex flex-wrap justify-center gap-2">{actions}</div> : null}
    </section>
  )
}

const TONES = {
  orange: 'bg-system-orange',
  blue: 'bg-system-blue',
  gray: 'bg-system-gray',
} as const

function IconBadge({ icon, tone }: { icon: ReactNode; tone: keyof typeof TONES }) {
  return (
    <span
      className={`flex size-[72px] items-center justify-center rounded-full text-white ${TONES[tone]}`}
      aria-hidden="true"
    >
      {icon}
    </span>
  )
}

// ------------------------------------------------------------------------------- payment

function PaymentSection({ order, polling }: { order: Order; polling: OrderPolling }) {
  const t = useT(['orders', 'common'])
  const queryClient = useQueryClient()
  const free = order.total_amount <= 0
  const methods = useQuery({ ...paymentMethodsQuery(), enabled: !free })
  const [picked, setPicked] = useState<ID | null>(null)
  const [result, setResult] = useState<CheckoutResult | null>(null)
  const [error, setError] = useState<string>()

  const list = methods.data ?? []
  const methodId =
    picked ?? (list.some((m) => m.id === order.payment_id) ? order.payment_id : (list[0]?.id ?? null))
  const method = list.find((m) => m.id === methodId) ?? null
  const fee = method ? handlingFee(method, order.total_amount) : 0
  const amount = chargedAmount(order, method)

  const refreshOrder = () =>
    queryClient.invalidateQueries({ queryKey: qk.user.order(order.trade_no), exact: true })

  const pay = useMutation({
    mutationFn: () =>
      userApi.checkout(order.trade_no, free || methodId === null ? {} : { payment_id: methodId }),
    onSuccess: async (res) => {
      setError(undefined)
      if (res.type === 'paid') {
        setResult(null)
        await refreshOrder()
        return
      }
      setResult(res)
      if (res.type === 'url' && !redirectToGateway(res.data)) setError(t('pay.badUrl'))
      // The server has now set payment_id / handling_amount: refresh the summary.
      void refreshOrder()
      polling.restart()
    },
    onError: (e) => setError(errorMessage(e)),
  })

  const check = useMutation({
    mutationFn: () => userApi.orderStatus(order.trade_no),
    onSuccess: async ({ status }) => {
      if (status === ORDER_STATUS.pending) {
        toast.info(t('pay.notYet'))
        polling.restart()
      } else {
        await refreshOrder()
      }
    },
    onError: (e) => toast.danger(errorMessage(e)),
  })

  if (free) {
    return (
      <Card title={t('pay.title')}>
        <div className="flex flex-col items-start gap-3">
          <p className="text-subhead text-label-secondary">{t('pay.freeBody')}</p>
          <FormError message={error} />
          <Button size="lg" isPending={pay.isPending} onPress={() => pay.mutate()}>
            {t('pay.activate')}
          </Button>
        </div>
      </Card>
    )
  }

  if (result && result.type !== 'url') {
    return (
      <Card
        title={t('pay.title')}
        action={
          <Button size="sm" variant="ghost" onPress={() => setResult(null)}>
            {t('pay.change')}
          </Button>
        }
      >
        {method ? <PaymentMethodBadge method={method} className="self-center" /> : null}
        <CheckoutResultView
          result={result}
          amount={amount}
          methodName={method?.name}
          onCheckNow={() => check.mutate()}
          isChecking={check.isPending}
        />
      </Card>
    )
  }

  return (
    <Card title={t('pay.title')}>
      <div className="flex flex-col gap-5">
        {methods.isPending ? (
          <div className="grid gap-2 sm:grid-cols-2" aria-busy="true">
            <div className="h-[66px] animate-pulse rounded-[14px] bg-fill-quaternary" />
            <div className="h-[66px] animate-pulse rounded-[14px] bg-fill-quaternary" />
          </div>
        ) : methods.isError ? (
          <FormError message={errorMessage(methods.error)} />
        ) : list.length === 0 ? (
          <EmptyState compact title={t('pay.noMethods')} description={t('pay.noMethodsHint')} />
        ) : (
          <PaymentPicker
            methods={list}
            value={methodId}
            onChange={setPicked}
            total={order.total_amount}
            isDisabled={pay.isPending}
          />
        )}
        {result?.type === 'url' ? (
          <CheckoutResultView result={result} amount={amount} onCheckNow={() => check.mutate()} />
        ) : null}
        <FormError message={error} />
        <div className="flex flex-wrap items-center justify-between gap-3 border-t-[0.5px] border-hairline pt-4">
          <div>
            <p className="text-footnote text-label-secondary">
              {fee > 0 ? t('pay.amountWithFee') : t('pay.amount')}
            </p>
            <SiteMoney cents={amount} className="text-title2 text-label" />
          </div>
          <Button
            size="lg"
            isPending={pay.isPending}
            isDisabled={methodId === null || list.length === 0}
            onPress={() => pay.mutate()}
          >
            {t('pay.submit')}
          </Button>
        </div>
      </div>
    </Card>
  )
}

function CancelOrder({ tradeNo }: { tradeNo: string }) {
  const t = useT(['orders', 'common'])
  const queryClient = useQueryClient()
  const cancel = useMutation({
    mutationFn: () => userApi.cancelOrder(tradeNo),
    onSuccess: async () => {
      toast.success(t('cancel.done'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: qk.user.order(tradeNo), exact: true }),
        queryClient.invalidateQueries({ queryKey: ['user', 'orders'] }),
        queryClient.invalidateQueries({ queryKey: qk.user.me() }),
        queryClient.invalidateQueries({ queryKey: qk.user.stats() }),
      ])
    },
    onError: (e) => toast.danger(errorMessage(e)),
  })
  return (
    <Button
      variant="danger-soft"
      fullWidth
      isPending={cancel.isPending}
      onPress={async () => {
        const ok = await confirm({
          title: t('cancel.title'),
          description: t('cancel.body'),
          confirmLabel: t('cancel.confirm'),
          cancelLabel: t('cancel.keep'),
          tone: 'danger',
        })
        if (ok) cancel.mutate()
      }}
    >
      {t('cancel.button')}
    </Button>
  )
}

// ------------------------------------------------------------------------------- bits

function Card({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="surface-card flex flex-col gap-4 p-5" aria-label={title}>
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-headline text-label">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  )
}

function Info({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <dt className="shrink-0 text-label-secondary">{label}</dt>
      <dd className="min-w-0 text-end text-label">{children}</dd>
    </div>
  )
}
