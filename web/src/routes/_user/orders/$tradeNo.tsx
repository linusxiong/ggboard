import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/ComingSoon'
import { useT } from '@/i18n'

/**
 * Order detail — also the payment return target (`/guest/payment/return/:trade_no` → 302 here).
 * Placeholder: replace with the real page (see web/README.md).
 */
export const Route = createFileRoute('/_user/orders/$tradeNo')({
  component: OrderDetailPage,
})

function OrderDetailPage() {
  const t = useT('nav')
  const { tradeNo } = Route.useParams()
  return <ComingSoon title={t('user.orders')} description={tradeNo} />
}
