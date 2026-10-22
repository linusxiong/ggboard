/** Payment method picker (`GET /user/payment-methods`): icon, name and the handling-fee hint. */
import { Radio, RadioGroup } from '@heroui/react'
import { CreditCard } from 'lucide-react'
import { useT } from '@/i18n'
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
          <Radio key={method.id} value={String(method.id)} className="group w-full">
            <Radio.Content
              className={cx(
                'flex w-full items-center gap-3 rounded-[14px] bg-fill-quaternary px-4 py-3 ring-1 ring-transparent transition-[box-shadow,background-color]',
                'group-data-selected:bg-accent-soft group-data-selected:ring-2 group-data-selected:ring-tint',
                'data-focus-visible:ring-2 data-focus-visible:ring-tint',
              )}
            >
              <MethodIcon icon={method.icon} />
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

function MethodIcon({ icon }: { icon: string }) {
  const isUrl = /^(https?:)?\/\//.test(icon) || icon.startsWith('/') || icon.startsWith('data:image/')
  return (
    <span
      className="flex size-9 shrink-0 items-center justify-center overflow-hidden rounded-[10px] bg-elevated text-label-secondary"
      aria-hidden="true"
    >
      {isUrl ? (
        <img src={icon} alt="" width={24} height={24} loading="lazy" className="size-6 object-contain" />
      ) : icon && Array.from(icon).length <= 2 ? (
        <span className="text-headline">{icon}</span>
      ) : (
        <CreditCard className="size-5" />
      )}
    </span>
  )
}
