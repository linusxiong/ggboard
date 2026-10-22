import { Button, Dropdown, Label } from '@heroui/react'
import { Ellipsis } from 'lucide-react'
import { useT } from '@/i18n'
import type { AdminOrder } from '@/lib/api/types'
import { useOrderActions } from './actions'

interface OrderRowActionsProps {
  order: AdminOrder
  onDetails: () => void
}

export function OrderRowActions({ order, onDetails }: OrderRowActionsProps) {
  const t = useT(['admin-orders', 'common'])
  const actions = useOrderActions()
  const pending = order.status === 0
  return (
    <div className="flex items-center gap-1">
      <Button size="sm" variant="tertiary" onPress={onDetails}>
        {t('actions.details')}
      </Button>
      <Dropdown>
        <Button
          size="sm"
          variant="ghost"
          isIconOnly
          aria-label={t('actions.more', { tradeNo: order.trade_no })}
        >
          <Ellipsis className="size-4" aria-hidden />
        </Button>
        <Dropdown.Popover placement="bottom end">
          <Dropdown.Menu
            disabledKeys={pending ? [] : ['paid', 'cancel']}
            onAction={(key) => {
              if (key === 'paid') void actions.markPaid(order)
              else if (key === 'cancel') void actions.cancel(order)
              else if (key === 'details') onDetails()
            }}
          >
            <Dropdown.Item id="details" textValue={t('actions.details')}>
              <Label>{t('actions.details')}</Label>
            </Dropdown.Item>
            <Dropdown.Item id="paid" textValue={t('actions.markPaid')}>
              <Label>{t('actions.markPaid')}</Label>
            </Dropdown.Item>
            <Dropdown.Item id="cancel" textValue={t('actions.cancel')} variant="danger">
              <Label>{t('actions.cancel')}</Label>
            </Dropdown.Item>
          </Dropdown.Menu>
        </Dropdown.Popover>
      </Dropdown>
    </div>
  )
}
