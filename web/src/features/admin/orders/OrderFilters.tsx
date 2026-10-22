import { Button } from '@heroui/react'
import { RotateCcw } from 'lucide-react'
import { useCallback } from 'react'
import { SelectInput, TextInput } from '@/features/admin/settings/inputs/Controlled'
import { useT } from '@/i18n'
import type { AdminPlan, CommissionStatus, OrderStatus, Payment } from '@/lib/api/types'
import { DebouncedSearch } from './DebouncedSearch'
import { activeOrderFilters } from './query'
import { COMMISSION_STATUSES, type OrderSearch, optDate, ORDER_STATUSES } from './search'

export type OrderFilterPatch = Partial<Omit<OrderSearch, 'page' | 'page_size' | 'sort'>>

interface OrderFiltersProps {
  value: OrderSearch
  /** Called with the changed filters (the page resets to 1 upstream). */
  onChange: (patch: OrderFilterPatch) => void
  plans: readonly Pick<AdminPlan, 'id' | 'name'>[]
  payments: readonly Pick<Payment, 'id' | 'name'>[]
}

const ALL = '__all'

/** Filter bar of `/admin/orders`; every control writes straight to the URL search params. */
export function OrderFilters({ value, onChange, plans, payments }: OrderFiltersProps) {
  const t = useT(['admin-orders', 'common'])
  const active = activeOrderFilters(value)
  const commitTradeNo = useCallback((trade_no: string | undefined) => onChange({ trade_no }), [onChange])
  const commitQ = useCallback((q: string | undefined) => onChange({ q }), [onChange])

  const allOption = { value: ALL, label: t('filters.all') }
  return (
    <section aria-label={t('filters.title')} className="surface-card flex flex-col gap-3 p-4">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <DebouncedSearch
          label={t('filters.tradeNo')}
          value={value.trade_no}
          onCommit={commitTradeNo}
          placeholder={t('filters.tradeNoPlaceholder')}
          monospace
        />
        <DebouncedSearch
          label={t('filters.user')}
          value={value.q}
          onCommit={commitQ}
          placeholder={t('filters.userPlaceholder')}
        />
        <SelectInput
          label={t('filters.status')}
          name="status"
          value={value.status === undefined ? ALL : String(value.status)}
          onChange={(v) =>
            onChange({ status: v === null || v === ALL ? undefined : (Number(v) as OrderStatus) })
          }
          options={[allOption, ...ORDER_STATUSES.map((s) => ({ value: String(s), label: t(`status.${s}`) }))]}
        />
        <SelectInput
          label={t('filters.commission')}
          name="commission_status"
          value={value.commission_status === undefined ? ALL : String(value.commission_status)}
          onChange={(v) =>
            onChange({
              commission_status: v === null || v === ALL ? undefined : (Number(v) as CommissionStatus),
            })
          }
          options={[
            allOption,
            ...COMMISSION_STATUSES.map((s) => ({ value: String(s), label: t(`commission.${s}`) })),
          ]}
        />
        <SelectInput
          label={t('filters.plan')}
          name="plan_id"
          value={value.plan_id === undefined ? ALL : String(value.plan_id)}
          onChange={(v) => onChange({ plan_id: v === null || v === ALL ? undefined : Number(v) })}
          options={[allOption, ...plans.map((p) => ({ value: String(p.id), label: p.name }))]}
        />
        <SelectInput
          label={t('filters.payment')}
          name="payment_id"
          value={value.payment_id === undefined ? ALL : String(value.payment_id)}
          onChange={(v) => onChange({ payment_id: v === null || v === ALL ? undefined : Number(v) })}
          options={[allOption, ...payments.map((p) => ({ value: String(p.id), label: p.name }))]}
        />
        <TextInput
          type="date"
          label={t('filters.dateFrom')}
          name="date_from"
          value={value.date_from ?? ''}
          onChange={(v) => onChange({ date_from: optDate(v) })}
        />
        <TextInput
          type="date"
          label={t('filters.dateTo')}
          name="date_to"
          value={value.date_to ?? ''}
          onChange={(v) => onChange({ date_to: optDate(v) })}
        />
      </div>
      <div className="flex justify-end">
        <Button
          size="sm"
          variant="tertiary"
          isDisabled={active === 0}
          onPress={() =>
            onChange({
              trade_no: undefined,
              q: undefined,
              status: undefined,
              commission_status: undefined,
              plan_id: undefined,
              payment_id: undefined,
              date_from: undefined,
              date_to: undefined,
            })
          }
        >
          <RotateCcw className="size-4" aria-hidden />
          {active > 0 ? t('filters.resetCount', { n: active }) : t('filters.reset')}
        </Button>
      </div>
    </section>
  )
}
