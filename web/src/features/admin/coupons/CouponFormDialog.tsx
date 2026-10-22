import { Button, Modal } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { MoneyInput } from '@/features/admin/orders/MoneyInput'
import { MultiSelectInput, NumberInput, TextInput } from '@/features/admin/settings/inputs/Controlled'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import { type AdminCoupon, type AdminPlan, type Coupon, PLAN_PERIODS } from '@/lib/api/types'
import { toast } from '@/lib/toast'
import { couponDefaults as defaults, toCouponInput } from './couponForm'
import { ModalClose } from '@/components/feedback/ModalClose'

interface CouponFormDialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  /** Edit mode when set. */
  coupon?: AdminCoupon | null
  plans: readonly AdminPlan[]
  currency: string
  /** Called with the generated coupons (create mode). */
  onCreated?: (coupons: Coupon[]) => void
}

const err = (meta: { errors: unknown[] }) => {
  const e = meta.errors.find(Boolean)
  return typeof e === 'string' ? e : undefined
}

/** Generate (batch) / edit coupon. */
export function CouponFormDialog(props: CouponFormDialogProps) {
  // Remount per coupon so defaults follow the edited row.
  return <CouponFormDialogInner key={props.isOpen ? (props.coupon?.id ?? 'new') : 'closed'} {...props} />
}

function CouponFormDialogInner({
  isOpen,
  onOpenChange,
  coupon,
  plans,
  currency,
  onCreated,
}: CouponFormDialogProps) {
  const t = useT(['admin-orders', 'common'])
  const qc = useQueryClient()
  const isEdit = Boolean(coupon)
  const [formError, setFormError] = useState<string>()

  const form = useAppForm({
    defaultValues: defaults(coupon),
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      const input = toCouponInput(value, isEdit)
      try {
        if (coupon) {
          await adminApi.updateCoupon(coupon.id, input)
          toast.success(t('coupons.saved'))
        } else {
          const created = await adminApi.createCoupons(input)
          onCreated?.(created)
          if (created.length === 1) toast.success(t('coupons.createdOne', { code: created[0]?.code ?? '' }))
        }
        await qc.invalidateQueries({ queryKey: qk.admin.coupons() })
        onOpenChange(false)
      } catch (e) {
        setFormError(submitError(formApi, e))
      }
    },
  })

  const required = (msg: string) => ({
    onSubmit: ({ value }: { value: unknown }) => (value ? undefined : msg),
  })

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="lg" scroll="inside">
        <Modal.Dialog>
          <ModalClose />
          <Modal.Header>
            <Modal.Heading>{isEdit ? t('coupons.editTitle') : t('coupons.createTitle')}</Modal.Heading>
          </Modal.Header>
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              void form.handleSubmit()
            }}
          >
            <Modal.Body className="grid gap-4 sm:grid-cols-2">
              <FormError message={formError} className="sm:col-span-2" />
              <form.AppField name="name" validators={required(t('coupons.nameRequired'))}>
                {(f) => <f.TextField label={t('coupons.fields.name')} isRequired autoFocus />}
              </form.AppField>
              <form.Subscribe selector={(s) => s.values.count}>
                {(count) => (
                  <form.AppField name="code">
                    {(f) => (
                      <f.TextField
                        label={t('coupons.fields.code')}
                        placeholder={t('coupons.codePlaceholder')}
                        isDisabled={!isEdit && (count ?? 1) > 1}
                        description={
                          !isEdit && (count ?? 1) > 1 ? t('coupons.codeBatchHint') : t('coupons.codeHint')
                        }
                        autoComplete="off"
                      />
                    )}
                  </form.AppField>
                )}
              </form.Subscribe>
              {!isEdit ? (
                <form.AppField
                  name="count"
                  validators={{
                    onSubmit: ({ value }) =>
                      value === null || value < 1 || value > 500 ? t('coupons.countInvalid') : undefined,
                  }}
                >
                  {(f) => (
                    <f.NumberField
                      label={t('coupons.fields.count')}
                      minValue={1}
                      maxValue={500}
                      step={1}
                      description={t('coupons.countHint')}
                    />
                  )}
                </form.AppField>
              ) : null}
              <form.AppField name="type" listeners={{ onChange: () => form.setFieldValue('value', null) }}>
                {(f) => (
                  <f.SelectField
                    label={t('coupons.fields.type')}
                    isRequired
                    options={[
                      { value: '1', label: t('coupons.type.1') },
                      { value: '2', label: t('coupons.type.2') },
                    ]}
                  />
                )}
              </form.AppField>
              <form.Subscribe selector={(s) => s.values.type}>
                {(type) => (
                  <form.Field
                    name="value"
                    validators={{
                      onSubmit: ({ value }) => {
                        if (value === null || value <= 0) return t('coupons.valueRequired')
                        if (type === '2' && value > 100) return t('coupons.percentMax')
                        return undefined
                      },
                    }}
                  >
                    {(f) =>
                      type === '1' ? (
                        <MoneyInput
                          label={t('coupons.fields.amount')}
                          name="value"
                          value={f.state.value}
                          onChange={(v) => f.handleChange(v)}
                          currency={currency}
                          isRequired
                          error={err(f.state.meta)}
                        />
                      ) : (
                        <NumberInput
                          label={t('coupons.fields.percent')}
                          name="value"
                          value={f.state.value}
                          onChange={(v) => f.handleChange(v === null ? null : Math.round(v))}
                          minValue={1}
                          maxValue={100}
                          step={1}
                          unit="%"
                          isRequired
                          description={t('coupons.percentHint')}
                          error={err(f.state.meta)}
                        />
                      )
                    }
                  </form.Field>
                )}
              </form.Subscribe>
              <form.AppField name="limit_use">
                {(f) => (
                  <f.NumberField
                    label={t('coupons.fields.limit_use')}
                    minValue={1}
                    step={1}
                    description={t('coupons.unlimitedHint')}
                  />
                )}
              </form.AppField>
              <form.AppField name="limit_use_with_user">
                {(f) => (
                  <f.NumberField
                    label={t('coupons.fields.limit_use_with_user')}
                    minValue={1}
                    step={1}
                    description={t('coupons.unlimitedHint')}
                  />
                )}
              </form.AppField>
              <form.Field name="limit_plan_ids">
                {(f) => (
                  <MultiSelectInput
                    label={t('coupons.fields.limit_plan_ids')}
                    name="limit_plan_ids"
                    value={f.state.value}
                    onChange={(v) => f.handleChange(v)}
                    options={plans.map((p) => ({ value: String(p.id), label: p.name }))}
                    placeholder={t('coupons.anyPlan')}
                    error={err(f.state.meta)}
                  />
                )}
              </form.Field>
              <form.Field name="limit_periods">
                {(f) => (
                  <MultiSelectInput
                    label={t('coupons.fields.limit_periods')}
                    name="limit_periods"
                    value={f.state.value}
                    onChange={(v) => f.handleChange(v)}
                    options={PLAN_PERIODS.map((p) => ({ value: p, label: t(`common:periods.${p}`) }))}
                    placeholder={t('coupons.anyPeriod')}
                    error={err(f.state.meta)}
                  />
                )}
              </form.Field>
              <form.Field name="started_at" validators={required(t('coupons.startRequired'))}>
                {(f) => (
                  <TextInput
                    type="datetime-local"
                    label={t('coupons.fields.started_at')}
                    name="started_at"
                    value={f.state.value}
                    onChange={(v) => f.handleChange(v)}
                    isRequired
                    error={err(f.state.meta)}
                  />
                )}
              </form.Field>
              <form.Field
                name="ended_at"
                validators={{
                  onSubmit: ({ value, fieldApi }) => {
                    if (!value) return t('coupons.endRequired')
                    const start = fieldApi.form.getFieldValue('started_at')
                    return start && Date.parse(value) <= Date.parse(start)
                      ? t('coupons.endAfterStart')
                      : undefined
                  },
                }}
              >
                {(f) => (
                  <TextInput
                    type="datetime-local"
                    label={t('coupons.fields.ended_at')}
                    name="ended_at"
                    value={f.state.value}
                    onChange={(v) => f.handleChange(v)}
                    isRequired
                    error={err(f.state.meta)}
                  />
                )}
              </form.Field>
              <form.AppField name="show">
                {(f) => (
                  <f.SwitchField
                    label={t('coupons.fields.show')}
                    description={t('coupons.showHint')}
                    className="sm:col-span-2"
                  />
                )}
              </form.AppField>
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {t('common:actions.cancel')}
              </Button>
              <form.AppForm>
                <form.SubmitButton size="md">
                  {isEdit ? t('common:actions.save') : t('coupons.generate')}
                </form.SubmitButton>
              </form.AppForm>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
