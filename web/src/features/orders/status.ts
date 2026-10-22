/** Order status / type metadata and amount helpers (docs/DATA_MODEL.md §orders, docs/PAYMENTS.md). */
import type { Order, OrderStatus, OrderType, PaymentMethod } from '@/lib/api/types'

export type OrderStatusKey = 'pending' | 'processing' | 'cancelled' | 'completed' | 'discounted'
export type ChipColor = 'default' | 'accent' | 'success' | 'warning' | 'danger'

export const ORDER_STATUS_KEY: Readonly<Record<OrderStatus, OrderStatusKey>> = {
  0: 'pending',
  1: 'processing',
  2: 'cancelled',
  3: 'completed',
  4: 'discounted',
}

export const ORDER_STATUS_COLOR: Readonly<Record<OrderStatus, ChipColor>> = {
  0: 'warning',
  1: 'accent',
  2: 'default',
  3: 'success',
  4: 'default',
}

export const ORDER_TYPE_KEY: Readonly<Record<OrderType, 'new' | 'renew' | 'upgrade' | 'reset_traffic'>> = {
  1: 'new',
  2: 'renew',
  3: 'upgrade',
  4: 'reset_traffic',
}

/** Filter tabs on `/orders` (`undefined` = all). */
export const ORDER_FILTERS: readonly (OrderStatus | undefined)[] = [undefined, 0, 1, 3, 2, 4]

export function isOrderStatus(value: unknown): value is OrderStatus {
  return value === 0 || value === 1 || value === 2 || value === 3 || value === 4
}

/** Settings default `order_timeout_minutes` (not exposed to users). */
export const ORDER_TIMEOUT_MINUTES = 30

/** Payment deadline (ms epoch): server `expired_at` when present, else `created_at + 30 min`. */
export function paymentDeadline(order: Pick<Order, 'created_at' | 'expired_at'>): number {
  const server = order.expired_at ? Date.parse(order.expired_at) : Number.NaN
  if (Number.isFinite(server)) return server
  return Date.parse(order.created_at) + ORDER_TIMEOUT_MINUTES * 60_000
}

/**
 * The plan price the order started from: `total + discount + surplus + balance − refund`
 * (pricing pipeline in docs/PAYMENTS.md; `handling_amount` is added on top at checkout).
 */
export function orderPrice(order: Order): number {
  return Math.max(
    0,
    order.total_amount +
      order.discount_amount +
      order.surplus_amount +
      order.balance_amount -
      order.refund_amount,
  )
}

/** Gateway fee estimate: `round(total × percent / 100) + fixed` (cents). */
export function handlingFee(
  method: Pick<PaymentMethod, 'handling_fee_fixed' | 'handling_fee_percent'>,
  total: number,
) {
  const percent = method.handling_fee_percent || 0
  const fixed = method.handling_fee_fixed || 0
  if (percent <= 0 && fixed <= 0) return 0
  return Math.round((total * percent) / 100) + fixed
}

/** Amount actually charged: the server value once checked out, else total + estimated fee. */
export function chargedAmount(order: Order, method?: PaymentMethod | null): number {
  if (order.handling_amount > 0 || !method) return order.total_amount + order.handling_amount
  return order.total_amount + handlingFee(method, order.total_amount)
}
