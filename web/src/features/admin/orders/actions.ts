import { useQueryClient } from '@tanstack/react-query'
import { confirm } from '@/components/feedback/confirm'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { CommissionStatus, Order } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'

type OrderRef = Pick<Order, 'id' | 'trade_no'>

/** Mark paid / cancel / commission status, with confirmation, toasts and cache refresh. */
export function useOrderActions() {
  const t = useT('admin-orders')
  const qc = useQueryClient()

  const done = async (order: Order) => {
    qc.setQueryData(qk.admin.order(order.id), (prev: Order | undefined) =>
      prev ? { ...prev, ...order } : order,
    )
    await qc.invalidateQueries({ queryKey: qk.admin.orders() })
  }

  const run = async (fn: () => Promise<Order>, success: string) => {
    try {
      const order = await fn()
      toast.success(success)
      await done(order)
      return true
    } catch (e) {
      toast.danger(errorMessage(e))
      return false
    }
  }

  return {
    async markPaid(order: OrderRef) {
      const ok = await confirm({
        title: t('actions.markPaidTitle'),
        description: t('actions.markPaidHint', { tradeNo: order.trade_no }),
        confirmLabel: t('actions.markPaid'),
      })
      return ok ? run(() => adminApi.markOrderPaid(order.id), t('actions.markedPaid')) : false
    },
    async cancel(order: OrderRef) {
      const ok = await confirm({
        title: t('actions.cancelTitle'),
        description: t('actions.cancelHint', { tradeNo: order.trade_no }),
        confirmLabel: t('actions.cancel'),
        tone: 'danger',
      })
      return ok ? run(() => adminApi.cancelOrder(order.id), t('actions.cancelled')) : false
    },
    setCommission(order: OrderRef, commission_status: CommissionStatus) {
      return run(() => adminApi.updateOrder(order.id, { commission_status }), t('actions.commissionSaved'))
    },
  }
}
