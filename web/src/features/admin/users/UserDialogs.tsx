/** Toolbar dialogs for `/admin/users`: create, batch generate (CSV), bulk mail (current filters). */
import { Button, Chip, Modal } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { Download, Shuffle } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { adminGroupsQuery, adminPlansQuery } from '@/lib/api/queries'
import type { AdminUserGenerateResult } from '@/lib/api/types'
import { toast } from '@/lib/toast'
import { localInputToIso } from '../shared/datetime'
import { downloadText, stampedName } from '../shared/download'
import { DateTimeField } from '../shared/fields'
import { useAdminMutation } from '../shared/useAdminMutation'
import { useSite } from '../shared/useSite'
import type { UserFilterParams } from './filters'
import { createValuesToInput, EMPTY_CREATE, NONE, randomPassword } from './userForm'

interface DialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
}

function DialogFrame({
  isOpen,
  onOpenChange,
  title,
  children,
  footer,
  onSubmit,
  size = 'md',
}: DialogProps & {
  title: ReactNode
  children: ReactNode
  footer: ReactNode
  onSubmit?: () => void
  size?: 'md' | 'lg'
}) {
  const body = (
    <>
      <Modal.Body className="flex flex-col gap-4">{children}</Modal.Body>
      <Modal.Footer>{footer}</Modal.Footer>
    </>
  )
  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange} isDismissable={false}>
      <Modal.Container size={size} scroll="inside">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{title}</Modal.Heading>
          </Modal.Header>
          {onSubmit ? (
            <form
              noValidate
              className="contents"
              onSubmit={(e) => {
                e.preventDefault()
                onSubmit()
              }}
            >
              {body}
            </form>
          ) : (
            body
          )}
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

function usePlanGroupOptions() {
  const t = useT('admin-users')
  const plans = useQuery(adminPlansQuery())
  const groups = useQuery(adminGroupsQuery())
  return {
    plans: plans.data ?? [],
    planOptions: [
      { value: NONE, label: t('detail.noPlan') },
      ...(plans.data ?? []).map((p) => ({ value: String(p.id), label: p.name })),
    ],
    groupOptions: [
      { value: NONE, label: t('detail.none') },
      ...(groups.data ?? []).map((g) => ({ value: String(g.id), label: g.name })),
    ],
  }
}

export function CreateUserDialog({
  isOpen,
  onOpenChange,
  onCreated,
}: DialogProps & { onCreated: (id: number) => void }) {
  const t = useT(['admin-users', 'common'])
  const { currency } = useSite()
  const { plans, planOptions, groupOptions } = usePlanGroupOptions()
  const [formError, setFormError] = useState<string>()
  const create = useAdminMutation({
    mutationFn: (input: ReturnType<typeof createValuesToInput>) => adminApi.createUser(input),
    success: t('create.done'),
    silentError: true,
  })
  const form = useAppForm({
    defaultValues: EMPTY_CREATE,
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      try {
        const user = await create.mutateAsync(createValuesToInput(value, currency))
        onOpenChange(false)
        onCreated(user.id)
      } catch (error) {
        setFormError(submitError(formApi, error))
      }
    },
  })
  return (
    <DialogFrame
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      size="lg"
      title={t('create.title')}
      onSubmit={() => void form.handleSubmit()}
      footer={
        <>
          <Button slot="close" variant="tertiary">
            {t('common:actions.cancel')}
          </Button>
          <form.AppForm>
            <form.SubmitButton size="md">{t('common:actions.create')}</form.SubmitButton>
          </form.AppForm>
        </>
      }
    >
      <FormError message={formError} />
      <div className="grid gap-4 sm:grid-cols-2">
        <form.AppField
          name="email"
          validators={{
            onSubmit: ({ value }) => (/^\S+@\S+\.\S+$/.test(value.trim()) ? undefined : t('errors.email')),
          }}
        >
          {(f) => <f.TextField label={t('fields.email')} type="email" autoComplete="off" isRequired />}
        </form.AppField>
        <form.AppField
          name="password"
          validators={{ onSubmit: ({ value }) => (value.length >= 8 ? undefined : t('errors.password')) }}
        >
          {(f) => (
            <f.TextField
              label={t('fields.password')}
              autoComplete="new-password"
              isRequired
              endContent={
                <Button
                  size="sm"
                  variant="ghost"
                  isIconOnly
                  aria-label={t('fields.generatePassword')}
                  onPress={() => f.handleChange(randomPassword())}
                >
                  <Shuffle className="size-4" aria-hidden="true" />
                </Button>
              }
            />
          )}
        </form.AppField>
        <form.AppField
          name="plan_id"
          listeners={{
            onChange: ({ value }) => {
              const plan = plans.find((p) => String(p.id) === value)
              if (!plan) return
              form.setFieldValue('group_id', String(plan.group_id))
              if (form.getFieldValue('transfer_enable') === null)
                form.setFieldValue('transfer_enable', plan.transfer_enable)
            },
          }}
        >
          {(f) => (
            <f.SelectField
              label={t('fields.plan')}
              options={planOptions}
              description={t('create.planHint')}
            />
          )}
        </form.AppField>
        <form.AppField name="group_id">
          {(f) => <f.SelectField label={t('fields.group')} options={groupOptions} />}
        </form.AppField>
        <form.AppField name="transfer_enable">
          {(f) => (
            <f.NumberField
              label={t('fields.transfer')}
              minValue={0}
              step={1}
              formatOptions={{ maximumFractionDigits: 2 }}
            />
          )}
        </form.AppField>
        <form.AppField name="expired_at">
          {() => (
            <DateTimeField
              label={t('fields.expiredAt')}
              clearLabel={t('common:values.never')}
              description={t('fields.expiredAtHint')}
            />
          )}
        </form.AppField>
        <form.AppField name="speed_limit">
          {(f) => (
            <f.NumberField
              label={t('fields.speedLimit')}
              minValue={0}
              step={1}
              description={t('fields.unlimitedHint')}
            />
          )}
        </form.AppField>
        <form.AppField name="device_limit">
          {(f) => (
            <f.NumberField
              label={t('fields.deviceLimit')}
              minValue={0}
              step={1}
              description={t('fields.unlimitedHint')}
            />
          )}
        </form.AppField>
        <form.AppField name="balance">
          {(f) => (
            <f.NumberField
              label={t('fields.balance')}
              step={0.01}
              formatOptions={{ style: 'currency', currency }}
              steppers={false}
            />
          )}
        </form.AppField>
      </div>
      <form.AppField name="remarks">
        {(f) => <f.TextAreaField label={t('fields.remarks')} rows={2} />}
      </form.AppField>
      <div className="grid gap-3 sm:grid-cols-2">
        <form.AppField name="is_admin">{(f) => <f.SwitchField label={t('fields.isAdmin')} />}</form.AppField>
        <form.AppField name="is_staff">
          {(f) => <f.SwitchField label={t('fields.isStaff')} description={t('fields.isStaffHint')} />}
        </form.AppField>
      </div>
    </DialogFrame>
  )
}

export function GenerateUsersDialog({ isOpen, onOpenChange }: DialogProps) {
  const t = useT(['admin-users', 'common'])
  const { planOptions } = usePlanGroupOptions()
  const [formError, setFormError] = useState<string>()
  const [result, setResult] = useState<AdminUserGenerateResult | null>(null)
  const generate = useAdminMutation({
    mutationFn: (input: Parameters<typeof adminApi.generateUsers>[0]) => adminApi.generateUsers(input),
    success: (r) => t('generate.done', { count: r.count }),
    silentError: true,
  })
  const download = (csv: string) => downloadText(csv, stampedName('users-generated', 'csv'))
  const form = useAppForm({
    defaultValues: {
      email_prefix: '',
      email_suffix: '',
      count: 10,
      plan_id: NONE,
      expired_at: '',
      password: '',
    },
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      try {
        const r = await generate.mutateAsync({
          email_suffix: value.email_suffix.trim().replace(/^@/, ''),
          count: value.count ?? 0,
          ...(value.email_prefix.trim() ? { email_prefix: value.email_prefix.trim() } : {}),
          ...(value.plan_id !== NONE ? { plan_id: Number(value.plan_id) } : {}),
          ...(value.expired_at ? { expired_at: localInputToIso(value.expired_at) } : {}),
          ...(value.password ? { password: value.password } : {}),
        })
        setResult(r)
        download(r.csv)
      } catch (error) {
        setFormError(submitError(formApi, error))
      }
    },
  })
  return (
    <DialogFrame
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      title={t('generate.title')}
      onSubmit={result ? undefined : () => void form.handleSubmit()}
      footer={
        result ? (
          <>
            <Button slot="close" variant="tertiary">
              {t('common:actions.close')}
            </Button>
            <Button onPress={() => download(result.csv)}>
              <Download className="size-4" aria-hidden="true" />
              {t('generate.download')}
            </Button>
          </>
        ) : (
          <>
            <Button slot="close" variant="tertiary">
              {t('common:actions.cancel')}
            </Button>
            <form.AppForm>
              <form.SubmitButton size="md">{t('generate.submit')}</form.SubmitButton>
            </form.AppForm>
          </>
        )
      }
    >
      {result ? (
        <div role="status" className="flex flex-col gap-2 rounded-[14px] bg-fill-quaternary p-4">
          <p className="text-headline text-label">{t('generate.done', { count: result.count })}</p>
          <p className="text-subhead text-label-secondary">{t('generate.doneHint')}</p>
        </div>
      ) : (
        <>
          <FormError message={formError} />
          <div className="grid gap-4 sm:grid-cols-2">
            <form.AppField name="email_prefix">
              {(f) => (
                <f.TextField
                  label={t('generate.prefix')}
                  placeholder="vip"
                  description={t('generate.prefixHint')}
                />
              )}
            </form.AppField>
            <form.AppField
              name="email_suffix"
              validators={{
                onSubmit: ({ value }) =>
                  /^@?[\w.-]+\.[a-z]{2,}$/i.test(value.trim()) ? undefined : t('generate.suffixError'),
              }}
            >
              {(f) => <f.TextField label={t('generate.suffix')} placeholder="example.com" isRequired />}
            </form.AppField>
            <form.AppField
              name="count"
              validators={{
                onSubmit: ({ value }) =>
                  value !== null && value >= 1 && value <= 500 ? undefined : t('generate.countError'),
              }}
            >
              {(f) => (
                <f.NumberField label={t('generate.count')} minValue={1} maxValue={500} step={1} isRequired />
              )}
            </form.AppField>
            <form.AppField name="plan_id">
              {(f) => <f.SelectField label={t('fields.plan')} options={planOptions} />}
            </form.AppField>
            <form.AppField name="expired_at">
              {() => <DateTimeField label={t('fields.expiredAt')} clearLabel={t('common:values.never')} />}
            </form.AppField>
            <form.AppField name="password">
              {(f) => (
                <f.TextField
                  label={t('fields.password')}
                  autoComplete="new-password"
                  description={t('generate.passwordHint')}
                />
              )}
            </form.AppField>
          </div>
        </>
      )}
    </DialogFrame>
  )
}

export function MailUsersDialog({
  isOpen,
  onOpenChange,
  filters,
  matching,
  filterSummary,
}: DialogProps & { filters: UserFilterParams; matching: number | undefined; filterSummary: string[] }) {
  const t = useT(['admin-users', 'common'])
  const [formError, setFormError] = useState<string>()
  const send = useAdminMutation({
    mutationFn: (input: { subject: string; content: string }) => adminApi.mailUsers({ ...filters, ...input }),
    invalidate: [],
    silentError: true,
  })
  const form = useAppForm({
    defaultValues: { subject: '', content: '' },
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      try {
        const { queued } = await send.mutateAsync({ subject: value.subject.trim(), content: value.content })
        toast.success(t('mail.queued', { count: queued }))
        onOpenChange(false)
      } catch (error) {
        setFormError(submitError(formApi, error))
      }
    },
  })
  return (
    <DialogFrame
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      size="lg"
      title={t('mail.title')}
      onSubmit={() => void form.handleSubmit()}
      footer={
        <>
          <Button slot="close" variant="tertiary">
            {t('common:actions.cancel')}
          </Button>
          <form.AppForm>
            <form.SubmitButton size="md">{t('mail.send')}</form.SubmitButton>
          </form.AppForm>
        </>
      }
    >
      <div className="flex flex-col gap-2 rounded-[14px] bg-fill-quaternary p-3">
        <p className="text-subhead text-label">
          {matching === undefined ? t('mail.recipientsUnknown') : t('mail.recipients', { count: matching })}
        </p>
        {filterSummary.length ? (
          <span className="flex flex-wrap gap-1">
            {filterSummary.map((s) => (
              <Chip key={s} size="sm" variant="secondary">
                {s}
              </Chip>
            ))}
          </span>
        ) : (
          <p className="text-footnote text-label-orange">{t('mail.allUsers')}</p>
        )}
      </div>
      <FormError message={formError} />
      <form.AppField
        name="subject"
        validators={{ onSubmit: ({ value }) => (value.trim() ? undefined : t('errors.required')) }}
      >
        {(f) => <f.TextField label={t('mail.subject')} isRequired />}
      </form.AppField>
      <form.AppField
        name="content"
        validators={{ onSubmit: ({ value }) => (value.trim() ? undefined : t('errors.required')) }}
      >
        {(f) => (
          <f.TextAreaField
            label={t('mail.content')}
            rows={8}
            description={t('mail.contentHint')}
            isRequired
          />
        )}
      </form.AppField>
    </DialogFrame>
  )
}
