/**
 * Payment method picker (`GET /user/payment-methods`): icon, name and the handling-fee hint, plus the
 * small "icon + name" views of the chosen method used by the order page.
 * Icons: `preset:<id>` marks, image URLs, emoji or a neutral fallback (`components/icons/payments`).
 */
import { Radio, RadioGroup } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { PaymentIcon } from '@/components/icons/payments'
import { useT } from '@/i18n'
import { paymentMethodsQuery } from '@/lib/api/queries'
import type { ID, PaymentMethod } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { useFormatMoney } from '@/features/shop/site'
import { handlingFee } from './status'

interface PaymentPickerProps {
  methods: readonly PaymentMethod[]
  value: ID | null
  onChange: (id: ID) => void
  /** Order total (cents) for the fee estimate. */
  total: number
  isDisabled?: boolean
}

export function PaymentPicker({ methods, value, onChange, total, isDisabled }: PaymentPickerProps) {
  const t = useT('orders')
  const money = useFormatMoney()
  return (
    <RadioGroup
      aria-label={t('payment.method')}
      value={value === null ? null : String(value)}
      onChange={(v) => onChange(Number(v))}
      isDisabled={isDisabled}
      className="grid gap-2 sm:grid-cols-2"
    >
      {methods.map((method) => {
        const fee = handlingFee(method, total)
        return (
          // mt-0: HeroUI gives vertical-group radios `mt-4`, which doubled the row gap of this grid.
          <Radio key={method.id} value={String(method.id)} className="group mt-0 w-full">
            <Radio.Content
              className={cx(
                'flex w-full items-center gap-3 rounded-[14px] bg-fill-quaternary px-4 py-3 ring-1 ring-transparent transition-[box-shadow,background-color]',
                'group-data-selected:bg-accent-soft group-data-selected:ring-2 group-data-selected:ring-tint',
                'data-focus-visible:ring-2 data-focus-visible:ring-tint',
              )}
            >
              <PaymentIcon icon={method.icon} size={32} />
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate text-subhead font-semibold text-label">{method.name}</span>
                <span className="text-footnote text-label-secondary">
                  {fee > 0 ? t('payment.fee', { fee: money(fee) }) : t('payment.noFee')}
                </span>
              </span>
              <Radio.Control>
                <Radio.Indicator />
              </Radio.Control>
            </Radio.Content>
          </Radio>
        )
      })}
    </RadioGroup>
  )
}

/** "[icon] name" of the method being paid with (checkout result card). */
export function PaymentMethodBadge({
  method,
  className,
}: {
  method: Pick<PaymentMethod, 'name' | 'icon'>
  className?: string
}) {
  return (
    <span className={cx('inline-flex min-w-0 items-center gap-2', className)}>
      <PaymentIcon icon={method.icon} size={24} />
      <span className="truncate text-subhead font-semibold text-label">{method.name}</span>
    </span>
  )
}

/**
 * The order's payment method (summary row): its name, with the icon when the method is still offered
 * (`/user/payment-methods`, cached 5 min and shared with the picker).
 */
export function OrderPaymentMethod({ payment }: { payment: { id: ID; name: string } }) {
  const methods = useQuery(paymentMethodsQuery())
  const icon = methods.data?.find((m) => m.id === payment.id)?.icon
  return (
    <span className="inline-flex max-w-full min-w-0 items-center justify-end gap-2">
      {icon ? <PaymentIcon icon={icon} size={20} /> : null}
      <span className="truncate">{payment.name}</span>
    </span>
  )
}
