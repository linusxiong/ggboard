import { useSuspenseQuery } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'
import { PageShell } from '@/components/PageShell'
import { OrderDetail } from '@/features/orders/OrderDetail'
import { OrderStatusChip } from '@/features/orders/OrderStatusChip'
import { loadNamespaces, useT } from '@/i18n'
import { guestConfigQuery, orderQuery } from '@/lib/api/queries'
import { buttonClass } from '@/lib/button'

/** Order detail — also the payment return target (`/guest/payment/return/:trade_no` → 302 here). */
export const Route = createFileRoute('/_user/orders/$tradeNo')({
  loader: ({ context, params }) =>
    Promise.all([
      loadNamespaces(['orders', 'common']),
      context.queryClient.query(orderQuery(params.tradeNo)),
      context.queryClient.query({ ...guestConfigQuery(), staleTime: 'static' }).catch(() => null),
    ]),
  component: OrderDetailPage,
})

function OrderDetailPage() {
  const t = useT(['orders', 'common'])
  const { tradeNo } = Route.useParams()
  const { data: order } = useSuspenseQuery(orderQuery(tradeNo))
  return (
    <PageShell
      title={t('detail.title')}
      description={
        <span className="flex flex-wrap items-center gap-2">
          <span>{order.plan.name}</span>
          <OrderStatusChip status={order.status} />
        </span>
      }
      actions={
        <Link to="/orders" className={buttonClass({ variant: 'ghost', size: 'sm' })}>
          <ChevronLeft className="size-4" aria-hidden="true" />
          {t('detail.back')}
        </Link>
      }
    >
      <OrderDetail order={order} />
    </PageShell>
  )
}
