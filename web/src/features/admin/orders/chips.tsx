import { Chip } from '@heroui/react'
import { useT } from '@/i18n'
import type { CommissionStatus, OrderStatus, OrderType } from '@/lib/api/types'

type ChipColor = 'default' | 'accent' | 'success' | 'warning' | 'danger'

const ORDER_COLOR: Record<OrderStatus, ChipColor> = {
  0: 'warning',
  1: 'accent',
  2: 'default',
  3: 'success',
  4: 'default',
}
const COMMISSION_COLOR: Record<CommissionStatus, ChipColor> = {
  0: 'warning',
  1: 'accent',
  2: 'success',
  3: 'danger',
}

/** Needs the `admin-orders` namespace. */
export function OrderStatusChip({ status }: { status: OrderStatus }) {
  const t = useT('admin-orders')
  return (
    <Chip size="sm" variant="soft" color={ORDER_COLOR[status] ?? 'default'} className="whitespace-nowrap">
      {t(`status.${status}`)}
    </Chip>
  )
}

export function CommissionStatusChip({ status }: { status: CommissionStatus }) {
  const t = useT('admin-orders')
  return (
    <Chip
      size="sm"
      variant="soft"
      color={COMMISSION_COLOR[status] ?? 'default'}
      className="whitespace-nowrap"
    >
      {t(`commission.${status}`)}
    </Chip>
  )
}

export function OrderTypeLabel({ type }: { type: OrderType }) {
  const t = useT('admin-orders')
  return <>{t(`type.${type}`)}</>
}
