/** Order amounts: plan price, discount, upgrade credit, balance used, refund, handling fee, total. */
import type { ReactNode } from 'react'
import { HelpTip } from '@/components/HelpTip'
import { useT } from '@/i18n'
import type { Order } from '@/lib/api/types'
import { SiteMoney } from '@/features/shop/SiteMoney'
import { orderPrice } from './status'

export function AmountBreakdown({ order, fee }: { order: Order; fee?: number }) {
  const t = useT('orders')
  const handling = order.handling_amount > 0 ? order.handling_amount : (fee ?? 0)
  const estimatedFee = order.handling_amount <= 0 && handling > 0
  return (
    <dl className="flex flex-col gap-2.5 text-subhead">
      <Row label={t('amounts.price')}>
        <SiteMoney cents={orderPrice(order)} />
      </Row>
      {order.discount_amount > 0 ? (
        <Row label={t('amounts.discount')} negative>
          <SiteMoney cents={order.discount_amount} />
        </Row>
      ) : null}
      {order.surplus_amount > 0 ? (
        <Row label={t('amounts.surplus')} negative>
          <SiteMoney cents={order.surplus_amount} />
        </Row>
      ) : null}
      {order.balance_amount > 0 ? (
        <Row label={t('amounts.balance')} negative>
          <SiteMoney cents={order.balance_amount} />
        </Row>
      ) : null}
      {order.refund_amount > 0 ? (
        <Row label={t('amounts.refund')} hint={t('amounts.refundHint')}>
          <SiteMoney cents={order.refund_amount} />
        </Row>
      ) : null}
      {handling > 0 ? (
        <Row label={estimatedFee ? t('amounts.feeEstimated') : t('amounts.fee')} positive>
          <SiteMoney cents={handling} />
        </Row>
      ) : null}
      <div className="mt-1 flex items-baseline justify-between gap-4 border-t-[0.5px] border-hairline pt-3">
        <dt className="text-headline text-label">{t('amounts.total')}</dt>
        <dd className="text-title2 text-label">
          <SiteMoney cents={order.total_amount + handling} />
        </dd>
      </div>
    </dl>
  )
}

function Row({
  label,
  hint,
  negative,
  positive,
  children,
}: {
  label: string
  /** Explanation behind an ⓘ after the label. */
  hint?: string
  negative?: boolean
  positive?: boolean
  children: ReactNode
}) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <dt className="flex items-center gap-0.5 text-label-secondary">
        {label}
        {hint ? <HelpTip subject={label}>{hint}</HelpTip> : null}
      </dt>
      <dd className="tabular text-label">
        {negative ? '−' : positive ? '+' : null}
        {children}
      </dd>
    </div>
  )
}
