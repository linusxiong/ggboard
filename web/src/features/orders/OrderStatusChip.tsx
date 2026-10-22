import { Chip } from '@heroui/react'
import { useT } from '@/i18n'
import type { OrderStatus } from '@/lib/api/types'
import { ORDER_STATUS_COLOR, ORDER_STATUS_KEY } from './status'

export function OrderStatusChip({ status, size = 'sm' }: { status: OrderStatus; size?: 'sm' | 'md' }) {
  const t = useT('orders')
  return (
    <Chip size={size} variant="soft" color={ORDER_STATUS_COLOR[status]}>
      {t(`status.${ORDER_STATUS_KEY[status]}`)}
    </Chip>
  )
}
