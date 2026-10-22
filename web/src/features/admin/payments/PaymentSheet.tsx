import { Alert, Button, Drawer, Spinner } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { type FormEvent, useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import {
  isValidPaymentIcon,
  parsePaymentIcon,
  PaymentIcon,
  type PaymentIconId,
  presetIconValue,
} from '@/components/icons/payments'
import { FormError } from '@/components/form'
import { MoneyInput } from '@/features/admin/orders/MoneyInput'
import { ComboInput } from '@/features/admin/settings/inputs/ComboInput'
import {
  NumberInput,
  SelectInput,
  SwitchInput,
  TextAreaInput,
  TextInput,
} from '@/features/admin/settings/inputs/Controlled'
import { SecretInput } from '@/features/admin/settings/inputs/SecretInput'
import { useT } from '@/i18n'
import { adminApi, isApiError } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { GatewayField, Payment, PaymentGateway, PaymentInput } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import {
  configToStrings,
  defaultConfig,
  defaultIconPreset,
  fieldSuggestions,
  KNOWN_GATEWAYS,
  notifyUrlFor,
  validateConfig,
  WEBHOOK_GATEWAYS,
} from './gateways'
import { DrawerClose } from '@/components/feedback/DrawerClose'
import { CUSTOM_ICON, IconPicker } from './IconPicker'

export interface PaymentSheetProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  gateways: readonly PaymentGateway[]
  /** Edit mode when set. */
  payment?: Payment | null
  currency: string
  appUrl: string
  /** Preselected gateway for a new payment. */
  defaultGateway?: string
}

interface Values {
  gateway: string
  name: string
  /** Icon picker: a preset id, `CUSTOM_ICON` (→ `iconUrl`) or null (no icon). */
  iconChoice: string | null
  iconUrl: string
  notify_domain: string
  handling_fee_fixed: number | null
  handling_fee_percent: number | null
  enabled: boolean
  config: Record<string, string>
}

type Errors = Record<string, string | undefined>

/** The stored `Payment.icon` for the picker state. */
function iconValue(v: Pick<Values, 'iconChoice' | 'iconUrl'>): string {
  if (v.iconChoice === CUSTOM_ICON) return v.iconUrl.trim()
  return v.iconChoice ? presetIconValue(v.iconChoice as PaymentIconId) : ''
}

/** Picker state for a stored icon: known presets select their tile, anything else is "custom". */
function iconState(icon: string): Pick<Values, 'iconChoice' | 'iconUrl'> {
  const parsed = parsePaymentIcon(icon)
  if (parsed.kind === 'preset' && parsed.preset) return { iconChoice: parsed.id, iconUrl: '' }
  return icon.trim() ? { iconChoice: CUSTOM_ICON, iconUrl: icon } : { iconChoice: null, iconUrl: '' }
}

function initialValues(
  payment: Payment | null | undefined,
  gateways: readonly PaymentGateway[],
  gw?: string,
): Values {
  if (payment) {
    return {
      gateway: payment.gateway,
      name: payment.name,
      ...iconState(payment.icon ?? ''),
      notify_domain: payment.notify_domain ?? '',
      handling_fee_fixed: payment.handling_fee_fixed ?? 0,
      handling_fee_percent: payment.handling_fee_percent ?? 0,
      enabled: payment.enabled,
      config: {
        ...defaultConfig(gateways.find((g) => g.name === payment.gateway)),
        ...configToStrings(payment.config),
      },
    }
  }
  const gateway = gateways.find((g) => g.name === gw)
  const config = defaultConfig(gateway)
  return {
    gateway: gateway?.name ?? '',
    name: gateway?.label ?? '',
    iconChoice: gateway ? defaultIconPreset(gateway.name, config) : null,
    iconUrl: '',
    notify_domain: '',
    handling_fee_fixed: 0,
    handling_fee_percent: 0,
    enabled: true,
    config,
  }
}

/** Create/edit sheet for a payment method; the gateway's `fields` drive the config form. */
export function PaymentSheet(props: PaymentSheetProps) {
  return <PaymentSheetInner key={props.isOpen ? `${props.payment?.id ?? 'new'}` : 'closed'} {...props} />
}

function PaymentSheetInner({
  isOpen,
  onOpenChange,
  gateways,
  payment: initialPayment,
  currency,
  appUrl,
  defaultGateway,
}: PaymentSheetProps) {
  const t = useT(['admin-payments', 'common'])
  const qc = useQueryClient()
  // After a successful create the sheet stays open on the saved payment (to show its notify URL).
  const [saved, setSaved] = useState<Payment | null>(initialPayment ?? null)
  const [values, setValues] = useState<Values>(() => initialValues(initialPayment, gateways, defaultGateway))
  const [errors, setErrors] = useState<Errors>({})
  const [formError, setFormError] = useState<string>()
  const [submitting, setSubmitting] = useState(false)
  const [justCreated, setJustCreated] = useState(false)
  // A new payment's icon follows the gateway (and its config) until the admin picks one.
  const [iconAuto, setIconAuto] = useState(!initialPayment)

  const gateway = gateways.find((g) => g.name === values.gateway)
  const isEdit = saved !== null

  const set = <K extends keyof Values>(key: K, value: Values[K]) => {
    setValues((v) => ({ ...v, [key]: value }))
    setErrors((e) => (e[key] ? { ...e, [key]: undefined } : e))
  }
  const setConfig = (key: string, value: string) => {
    setValues((v) => {
      const config = { ...v.config, [key]: value }
      return iconAuto && !saved
        ? { ...v, config, iconChoice: defaultIconPreset(v.gateway, config) }
        : { ...v, config }
    })
    setErrors((e) => (e[`config.${key}`] ? { ...e, [`config.${key}`]: undefined } : e))
  }
  const chooseGateway = (name: string | null) => {
    const next = gateways.find((g) => g.name === name)
    const config = defaultConfig(next)
    setValues((v) => ({
      ...v,
      gateway: next?.name ?? '',
      name: !v.name || gateways.some((g) => g.label === v.name) ? (next?.label ?? '') : v.name,
      config,
      ...(iconAuto ? { iconChoice: next ? defaultIconPreset(next.name, config) : null, iconUrl: '' } : {}),
    }))
    setErrors({})
  }
  const chooseIcon = (choice: string) => {
    setIconAuto(false)
    setValues((v) => ({ ...v, iconChoice: choice }))
    setErrors((e) => (e.icon ? { ...e, icon: undefined } : e))
  }
  const setIconUrl = (url: string) => {
    setValues((v) => ({ ...v, iconUrl: url }))
    setErrors((e) => (e.icon ? { ...e, icon: undefined } : e))
  }

  const validate = (): Errors => {
    const e: Errors = {}
    if (!values.gateway) e.gateway = t('form.gatewayRequired')
    if (!values.name.trim()) e.name = t('form.nameRequired')
    if (values.iconChoice === CUSTOM_ICON && !isValidPaymentIcon(values.iconUrl))
      e.icon = t('icons.customInvalid')
    if (values.notify_domain && !/^https?:\/\/[^/\s]+$/.test(values.notify_domain.replace(/\/+$/, '')))
      e.notify_domain = t('form.domainInvalid')
    const pct = values.handling_fee_percent ?? 0
    if (pct < 0 || pct > 100) e.handling_fee_percent = t('form.percentInvalid')
    for (const [key, problem] of Object.entries(validateConfig(gateway?.fields ?? [], values.config))) {
      e[`config.${key}`] = problem === 'required' ? t('form.required') : t('form.numberInvalid')
    }
    return e
  }

  /** Server `errors` → fields: `config.url`, bare config keys, or top-level payment fields. */
  const applyServer = (error: unknown): string | undefined => {
    if (!isApiError(error) || Object.keys(error.errors).length === 0) return errorMessage(error)
    const next: Errors = {}
    let unmatched: string | undefined
    const configKeys = new Set(gateway?.fields.map((f) => f.key) ?? [])
    for (const [key, messages] of Object.entries(error.errors)) {
      const msg = messages[0]
      if (!msg) continue
      if (key.startsWith('config.')) next[key] = msg
      else if (configKeys.has(key)) next[`config.${key}`] = msg
      else if (key in values || key === 'icon') next[key] = msg
      else unmatched ??= `${key}: ${msg}`
    }
    setErrors(next)
    return unmatched ?? (Object.keys(next).length ? undefined : error.message)
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setFormError(undefined)
    const problems = validate()
    if (Object.values(problems).some(Boolean)) {
      setErrors(problems)
      return
    }
    const input: PaymentInput = {
      gateway: values.gateway,
      name: values.name.trim(),
      icon: iconValue(values),
      config: Object.fromEntries(Object.entries(values.config).map(([k, v]) => [k, v.trim()])),
      notify_domain: values.notify_domain.trim().replace(/\/+$/, ''),
      handling_fee_fixed: values.handling_fee_fixed ?? 0,
      handling_fee_percent: values.handling_fee_percent ?? 0,
      enabled: values.enabled,
    }
    setSubmitting(true)
    try {
      const result = saved
        ? await adminApi.updatePayment(saved.id, input)
        : await adminApi.createPayment(input)
      await qc.invalidateQueries({ queryKey: qk.admin.payments() })
      if (saved) {
        toast.success(t('form.saved'))
        onOpenChange(false)
      } else {
        toast.success(t('form.created'))
        setSaved(result)
        setJustCreated(true)
      }
    } catch (err) {
      setFormError(applyServer(err))
    } finally {
      setSubmitting(false)
    }
  }

  const notifyUrl = saved
    ? notifyUrlFor({ ...saved, notify_domain: values.notify_domain || saved.notify_domain }, appUrl)
    : null
  const helpKey = (KNOWN_GATEWAYS as readonly string[]).includes(values.gateway)
    ? `help.${values.gateway}`
    : null

  return (
    <Drawer.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Drawer.Content placement="right">
        <Drawer.Dialog className="sm:w-[34rem]">
          <DrawerClose />
          <Drawer.Header>
            <Drawer.Heading>{isEdit ? t('form.editTitle') : t('form.createTitle')}</Drawer.Heading>
          </Drawer.Header>
          <form noValidate onSubmit={(e) => void onSubmit(e)} className="flex min-h-0 flex-1 flex-col">
            <Drawer.Body className="flex flex-col gap-5 pt-4">
              {justCreated ? (
                <Alert status="success">
                  <Alert.Indicator />
                  <Alert.Content>
                    <Alert.Title>{t('form.createdTitle')}</Alert.Title>
                    <Alert.Description>{t('form.createdHint')}</Alert.Description>
                  </Alert.Content>
                </Alert>
              ) : null}
              <FormError message={formError} />

              <SelectInput
                label={t('form.gateway')}
                name="gateway"
                value={values.gateway || null}
                onChange={chooseGateway}
                options={gateways.map((g) => ({ value: g.name, label: g.label, textValue: g.label }))}
                placeholder={t('form.gatewayPlaceholder')}
                isRequired
                isDisabled={isEdit}
                error={errors.gateway}
                description={isEdit ? t('form.gatewayLocked') : undefined}
              />

              {helpKey ? (
                <Alert status={WEBHOOK_GATEWAYS.has(values.gateway) ? 'warning' : 'accent'}>
                  <Alert.Indicator />
                  <Alert.Content>
                    <Alert.Title>
                      {t('form.helpTitle', { gateway: gateway?.label ?? values.gateway })}
                    </Alert.Title>
                    <Alert.Description className="whitespace-pre-line">{t(helpKey)}</Alert.Description>
                  </Alert.Content>
                </Alert>
              ) : null}

              {notifyUrl ? (
                <div className="flex flex-col gap-1">
                  <TextInput
                    label={t('form.notifyUrl')}
                    name="notify_url"
                    value={notifyUrl}
                    onChange={() => undefined}
                    isReadOnly
                    monospace
                    endContent={<CopyButton value={notifyUrl} variant="ghost" />}
                    description={
                      WEBHOOK_GATEWAYS.has(values.gateway) ? t('form.notifyWebhook') : t('form.notifyAuto')
                    }
                  />
                </div>
              ) : null}

              {gateway ? (
                <fieldset className="flex flex-col gap-4">
                  <legend className="mb-3 text-headline text-label">{t('form.configTitle')}</legend>
                  {gateway.fields.map((field) => (
                    <GatewayFieldInput
                      key={field.key}
                      gateway={gateway.name}
                      field={field}
                      value={values.config[field.key] ?? ''}
                      onChange={(v) => setConfig(field.key, v)}
                      error={errors[`config.${field.key}`]}
                    />
                  ))}
                  {gateway.fields.length === 0 ? (
                    <p className="text-subhead text-label-secondary">{t('form.noConfig')}</p>
                  ) : null}
                </fieldset>
              ) : null}

              <fieldset className="flex flex-col gap-4">
                <legend className="mb-3 text-headline text-label">{t('form.displayTitle')}</legend>
                <TextInput
                  label={t('form.name')}
                  name="name"
                  value={values.name}
                  onChange={(v) => set('name', v)}
                  isRequired
                  error={errors.name}
                  description={t('form.nameHint')}
                  // Live preview: the icon users will see next to this name.
                  startContent={<PaymentIcon icon={iconValue(values)} size={24} />}
                />
                <IconPicker
                  choice={values.iconChoice}
                  customValue={values.iconUrl}
                  onChoiceChange={chooseIcon}
                  onCustomChange={setIconUrl}
                  error={errors.icon}
                />
                <TextInput
                  label={t('form.notifyDomain')}
                  name="notify_domain"
                  type="url"
                  value={values.notify_domain}
                  onChange={(v) => set('notify_domain', v)}
                  placeholder={appUrl || 'https://panel.example.com'}
                  error={errors.notify_domain}
                  description={t('form.notifyDomainHint')}
                  monospace
                />
                <div className="grid gap-4 sm:grid-cols-2">
                  <MoneyInput
                    label={t('form.feeFixed')}
                    name="handling_fee_fixed"
                    value={values.handling_fee_fixed}
                    onChange={(v) => set('handling_fee_fixed', v)}
                    currency={currency}
                    error={errors.handling_fee_fixed}
                  />
                  <NumberInput
                    label={t('form.feePercent')}
                    name="handling_fee_percent"
                    value={values.handling_fee_percent}
                    onChange={(v) => set('handling_fee_percent', v)}
                    minValue={0}
                    maxValue={100}
                    step={0.01}
                    steppers={false}
                    formatOptions={{ maximumFractionDigits: 2 }}
                    error={errors.handling_fee_percent}
                  />
                </div>
                <p className="-mt-2 text-footnote text-label-secondary">{t('form.feeHint')}</p>
                <SwitchInput
                  label={t('form.enabled')}
                  name="enabled"
                  value={values.enabled}
                  onChange={(v) => set('enabled', v)}
                  description={t('form.enabledHint')}
                />
              </fieldset>
            </Drawer.Body>
            <Drawer.Footer>
              <Button slot="close" variant="secondary">
                {justCreated ? t('common:actions.close') : t('common:actions.cancel')}
              </Button>
              <Button type="submit" isPending={submitting}>
                {({ isPending }) => (
                  <>
                    {isPending ? <Spinner color="current" size="sm" /> : null}
                    {isEdit ? t('common:actions.save') : t('form.create')}
                  </>
                )}
              </Button>
            </Drawer.Footer>
          </form>
        </Drawer.Dialog>
      </Drawer.Content>
    </Drawer.Backdrop>
  )
}

interface GatewayFieldInputProps {
  gateway: string
  field: GatewayField
  value: string
  onChange: (value: string) => void
  error?: string
}

/** One config field rendered from the gateway's `Field` schema. */
export function GatewayFieldInput({ gateway, field, value, onChange, error }: GatewayFieldInputProps) {
  const name = `config.${field.key}`
  const common = {
    label: field.label || field.key,
    name,
    description: field.description || undefined,
    error,
    isRequired: field.required,
  }
  switch (field.type) {
    case 'password':
      return <SecretInput {...common} value={value} onChange={onChange} />
    case 'textarea':
      return <TextAreaInput {...common} value={value} onChange={onChange} rows={5} monospace />
    case 'number':
      return <TextInput {...common} value={value} onChange={onChange} inputMode="decimal" monospace />
    case 'select':
      return (
        <ComboInput
          {...common}
          value={value}
          onChange={onChange}
          options={fieldSuggestions(gateway, field)}
          monospace
        />
      )
    default: {
      const suggestions = fieldSuggestions(gateway, field)
      return suggestions.length ? (
        <ComboInput {...common} value={value} onChange={onChange} options={suggestions} monospace />
      ) : (
        <TextInput {...common} value={value} onChange={onChange} monospace />
      )
    }
  }
}
