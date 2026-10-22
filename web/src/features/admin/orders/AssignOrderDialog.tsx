import { Button, Modal } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { useT } from '@/i18n'
import { adminApi, isApiError } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import { type AdminOrderCreateInput, type AdminPlan, PLAN_PERIODS, type PlanPeriod } from '@/lib/api/types'
import { formatMoney } from '@/lib/format'
import { toast } from '@/lib/toast'
import { useLocale } from '@/i18n'
import { MoneyInput } from './MoneyInput'
import { splitUserQuery } from './query'

interface AssignOrderDialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  plans: readonly AdminPlan[]
  currency: string
  symbol?: string
}

interface Values {
  user: string
  plan_id: string | null
  period: string | null
  total_amount: number | null
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const EMPTY: Values = { user: '', plan_id: null, period: null, total_amount: null }

/** "Assign order" (`POST /admin/orders`): creates a pending order for a user by email or id. */
export function AssignOrderDialog({ isOpen, onOpenChange, plans, currency, symbol }: AssignOrderDialogProps) {
  const t = useT(['admin-orders', 'common'])
  const locale = useLocale()
  const qc = useQueryClient()
  const [formError, setFormError] = useState<string>()

  const form = useAppForm({
    defaultValues: EMPTY,
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      const input: AdminOrderCreateInput = {
        ...splitUserQuery(value.user),
        plan_id: Number(value.plan_id),
        period: value.period as PlanPeriod,
        ...(value.total_amount !== null ? { total_amount: value.total_amount } : {}),
      }
      try {
        const order = await adminApi.createOrder(input)
        toast.success(t('assign.created', { tradeNo: order.trade_no }))
        await qc.invalidateQueries({ queryKey: qk.admin.orders() })
        formApi.reset()
        onOpenChange(false)
      } catch (e) {
        // The API reports the user under `email` or `user_id`; the form has one "user" box.
        if (isApiError(e)) {
          const userError = e.fieldError('email') ?? e.fieldError('user_id')
          if (userError)
            formApi.setFieldMeta('user', (m) => ({ ...m, errorMap: { ...m.errorMap, onServer: userError } }))
        }
        setFormError(submitError(formApi, e))
      }
    },
  })

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="md">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{t('assign.title')}</Modal.Heading>
            <p className="text-subhead text-label-secondary">{t('assign.description')}</p>
          </Modal.Header>
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              void form.handleSubmit()
            }}
          >
            <Modal.Body className="flex flex-col gap-4">
              <FormError message={formError} />
              <form.AppField
                name="user"
                validators={{
                  onSubmit: ({ value }) => {
                    const v = value.trim()
                    if (!v) return t('assign.userRequired')
                    if (!/^\d+$/.test(v) && !EMAIL_RE.test(v)) return t('assign.userInvalid')
                    return undefined
                  },
                }}
              >
                {(f) => (
                  <f.TextField
                    label={t('assign.user')}
                    placeholder={t('assign.userPlaceholder')}
                    autoComplete="off"
                    isRequired
                    autoFocus
                  />
                )}
              </form.AppField>
              <form.AppField
                name="plan_id"
                validators={{ onSubmit: ({ value }) => (value ? undefined : t('assign.planRequired')) }}
                listeners={{ onChange: () => form.setFieldValue('period', null) }}
              >
                {(f) => (
                  <f.SelectField
                    label={t('fields.plan')}
                    isRequired
                    placeholder={t('assign.planPlaceholder')}
                    options={plans.map((p) => ({ value: String(p.id), label: p.name }))}
                  />
                )}
              </form.AppField>
              <form.Subscribe selector={(s) => s.values.plan_id}>
                {(planId) => {
                  const plan = plans.find((p) => String(p.id) === planId)
                  const periods = PLAN_PERIODS.filter((p) => plan?.prices[p] !== undefined)
                  return (
                    <form.AppField
                      name="period"
                      validators={{
                        onSubmit: ({ value }) => (value ? undefined : t('assign.periodRequired')),
                      }}
                    >
                      {(f) => (
                        <f.SelectField
                          label={t('fields.period')}
                          isRequired
                          isDisabled={!plan}
                          placeholder={plan ? t('assign.periodPlaceholder') : t('assign.pickPlanFirst')}
                          options={periods.map((p) => ({
                            value: p,
                            label: `${t(`common:periods.${p}`)} · ${formatMoney(plan?.prices[p] ?? 0, currency, locale, symbol !== undefined ? { symbol } : {})}`,
                            textValue: t(`common:periods.${p}`),
                          }))}
                        />
                      )}
                    </form.AppField>
                  )
                }}
              </form.Subscribe>
              <form.Field name="total_amount">
                {(f) => (
                  <MoneyInput
                    label={t('assign.total')}
                    name="total_amount"
                    value={f.state.value}
                    onChange={(v) => f.handleChange(v)}
                    currency={currency}
                    description={t('assign.totalHint')}
                    error={
                      typeof f.state.meta.errorMap.onServer === 'string'
                        ? f.state.meta.errorMap.onServer
                        : undefined
                    }
                  />
                )}
              </form.Field>
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {t('common:actions.cancel')}
              </Button>
              <form.AppForm>
                <form.SubmitButton size="md">{t('assign.submit')}</form.SubmitButton>
              </form.AppForm>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
