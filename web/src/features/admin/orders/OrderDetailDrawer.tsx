import { Button, Drawer, Skeleton } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { DateTime, Money } from '@/components/Formatters'
import { SelectInput } from '@/features/admin/settings/inputs/Controlled'
import { useT } from '@/i18n'
import { adminOrderQuery } from '@/lib/api/queries'
import type { AdminOrder, CommissionStatus, ID } from '@/lib/api/types'
import { useOrderActions } from './actions'
import { CommissionStatusChip, OrderStatusChip, OrderTypeLabel } from './chips'
import { COMMISSION_STATUSES } from './search'
import { useCurrency } from './useCurrency'
import { DrawerClose } from '@/components/feedback/DrawerClose'

interface OrderDetailDrawerProps {
  /** Order to show (`null` closes the drawer). */
  orderId: ID | null
  /** Row data shown instantly while the detail request runs. */
  preview?: AdminOrder
  onClose: () => void
}

function Row({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 px-4 py-2.5 text-subhead">
      <dt className="shrink-0 text-label-secondary">{label}</dt>
      <dd className="min-w-0 text-end text-label break-all">{children}</dd>
    </div>
  )
}

/** Right-side sheet with every amount, the user, plan, payment, callback number and timestamps. */
export function OrderDetailDrawer({ orderId, preview, onClose }: OrderDetailDrawerProps) {
  const t = useT(['admin-orders', 'common'])
  const isOpen = orderId !== null
  const q = useQuery({
    ...adminOrderQuery(orderId ?? 0),
    enabled: isOpen,
    placeholderData: preview && preview.id === orderId ? preview : undefined,
  })
  const order = q.data
  const { currency, symbol } = useCurrency()
  const actions = useOrderActions()
  const money = (cents: number | undefined) =>
    cents === undefined ? '—' : <Money cents={cents} currency={currency} symbol={symbol} />

  return (
    <Drawer.Backdrop isOpen={isOpen} onOpenChange={(open) => !open && onClose()}>
      <Drawer.Content placement="right">
        <Drawer.Dialog className="sm:w-[28rem]">
          <DrawerClose />
          <Drawer.Header>
            <Drawer.Heading>{t('detail.title')}</Drawer.Heading>
            {order ? (
              <p className="flex items-center gap-1 font-mono text-footnote text-label-secondary">
                {order.trade_no}
                <CopyButton value={order.trade_no} variant="ghost" />
              </p>
            ) : null}
          </Drawer.Header>
          <Drawer.Body className="flex flex-col gap-5 pt-4">
            {!order ? (
              <div className="flex flex-col gap-3" aria-busy="true">
                <Skeleton className="h-24 w-full rounded-card" />
                <Skeleton className="h-48 w-full rounded-card" />
              </div>
            ) : (
              <>
                <div className="flex flex-wrap items-center gap-2">
                  <OrderStatusChip status={order.status} />
                  <CommissionStatusChip status={order.commission_status} />
                </div>

                <section aria-labelledby="order-amounts">
                  <h3
                    id="order-amounts"
                    className="mb-2 text-footnote font-semibold text-label-secondary uppercase"
                  >
                    {t('detail.amounts')}
                  </h3>
                  <dl className="inset-group">
                    <Row label={t('fields.total_amount')}>{money(order.total_amount)}</Row>
                    <Row label={t('fields.handling_amount')}>{money(order.handling_amount)}</Row>
                    <Row label={t('fields.discount_amount')}>{money(order.discount_amount)}</Row>
                    <Row label={t('fields.surplus_amount')}>{money(order.surplus_amount)}</Row>
                    <Row label={t('fields.balance_amount')}>{money(order.balance_amount)}</Row>
                    <Row label={t('fields.refund_amount')}>{money(order.refund_amount)}</Row>
                    <Row label={t('fields.commission_balance')}>{money(order.commission_balance)}</Row>
                  </dl>
                </section>

                <section aria-labelledby="order-info">
                  <h3
                    id="order-info"
                    className="mb-2 text-footnote font-semibold text-label-secondary uppercase"
                  >
                    {t('detail.info')}
                  </h3>
                  <dl className="inset-group">
                    <Row label={t('fields.user')}>
                      {order.user?.email ?? '—'}{' '}
                      <span className="text-label-secondary">#{order.user_id}</span>
                    </Row>
                    <Row label={t('fields.plan')}>
                      {order.plan?.name ?? `#${order.plan_id}`} · {t(`common:periods.${order.period}`)}
                    </Row>
                    <Row label={t('fields.type')}>
                      <OrderTypeLabel type={order.type} />
                    </Row>
                    <Row label={t('fields.payment')}>{order.payment?.name ?? t('detail.noPayment')}</Row>
                    <Row label={t('fields.callback_no')}>
                      {order.callback_no ? (
                        <span className="inline-flex items-center gap-1 font-mono">
                          {order.callback_no}
                          <CopyButton value={order.callback_no} variant="ghost" />
                        </span>
                      ) : (
                        '—'
                      )}
                    </Row>
                    {order.surplus_order_ids?.length ? (
                      <Row label={t('fields.surplus_order_ids')}>
                        <span className="font-mono">
                          {order.surplus_order_ids.map((id) => `#${id}`).join(', ')}
                        </span>
                      </Row>
                    ) : null}
                    {order.coupon_id ? <Row label={t('fields.coupon')}>#{order.coupon_id}</Row> : null}
                    <Row label={t('fields.created_at')}>
                      <DateTime value={order.created_at} />
                    </Row>
                    <Row label={t('fields.paid_at')}>
                      <DateTime value={order.paid_at} />
                    </Row>
                    <Row label={t('fields.updated_at')}>
                      <DateTime value={order.updated_at} />
                    </Row>
                  </dl>
                </section>

                <SelectInput
                  label={t('detail.commissionStatus')}
                  name="commission_status"
                  value={String(order.commission_status)}
                  onChange={(v) => {
                    if (v !== null && Number(v) !== order.commission_status)
                      void actions.setCommission(order, Number(v) as CommissionStatus)
                  }}
                  options={COMMISSION_STATUSES.filter((s) => s !== 2).map((s) => ({
                    value: String(s),
                    label: t(`commission.${s}`),
                  }))}
                  description={t('detail.commissionHint')}
                />
              </>
            )}
          </Drawer.Body>
          <Drawer.Footer>
            {order?.status === 0 ? (
              <>
                <Button variant="danger-soft" onPress={() => void actions.cancel(order)}>
                  {t('actions.cancel')}
                </Button>
                <Button onPress={() => void actions.markPaid(order)}>{t('actions.markPaid')}</Button>
              </>
            ) : (
              <Button slot="close" variant="secondary">
                {t('common:actions.close')}
              </Button>
            )}
          </Drawer.Footer>
        </Drawer.Dialog>
      </Drawer.Content>
    </Drawer.Backdrop>
  )
}
