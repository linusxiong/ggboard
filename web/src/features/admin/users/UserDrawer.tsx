/** User detail drawer: summary (stats, credentials, actions) and the full edit form (admins only). */
import { Button, Chip, Tabs } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { Shuffle } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { TrafficBar } from '@/components/TrafficBar'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { adminGroupsQuery, adminPlansQuery, adminUserQuery } from '@/lib/api/queries'
import type { AdminUser, AdminUserDetail, AdminUserUpdateInput } from '@/lib/api/types'
import { toast } from '@/lib/toast'
import { DateTimeField } from '../shared/fields'
import { FormDrawer } from '../shared/FormDrawer'
import { useAdminMutation } from '../shared/useAdminMutation'
import { useFormatters } from '../shared/useFormatters'
import { useNow } from '../shared/useNow'
import { useSite } from '../shared/useSite'
import { LoginAsDialog } from './UserActionsMenu'
import { useUserActions } from './useUserActions'
import { UserSubscriptionTab } from './UserSubscriptionTab'
import { NONE, randomPassword, userToValues, valuesToPatch } from './userForm'

type Tab = 'summary' | 'profile' | 'subscription'

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 px-4 py-3">
      <dt className="shrink-0 text-subhead text-label-secondary">{label}</dt>
      <dd className="min-w-0 text-end text-subhead text-label">{children}</dd>
    </div>
  )
}

function Summary({
  user,
  canEdit,
  onDeleted,
}: {
  user: AdminUserDetail
  canEdit: boolean
  onDeleted: () => void
}) {
  const t = useT(['admin-users', 'common'])
  const fmt = useFormatters()
  const { appUrl } = useSite()
  const groups = useQuery(adminGroupsQuery())
  const actions = useUserActions(onDeleted)
  const group = groups.data?.find((g) => g.id === user.group_id)
  const now = useNow()
  const expired = user.expired_at !== null && new Date(user.expired_at).getTime() < now
  const subscribe = user.token ? `${appUrl}/s/${user.token}` : null
  return (
    <div className="flex flex-col gap-5">
      <dl className="grid grid-cols-2 gap-3">
        <div className="rounded-[14px] bg-fill-quaternary p-3">
          <dt className="text-footnote text-label-secondary">{t('detail.ordersTotal')}</dt>
          <dd className="text-headline text-label">{fmt.money(user.stats.orders_total)}</dd>
        </div>
        <div className="rounded-[14px] bg-fill-quaternary p-3">
          <dt className="text-footnote text-label-secondary">{t('detail.commissionTotal')}</dt>
          <dd className="text-headline text-label">{fmt.money(user.stats.commission_total)}</dd>
        </div>
      </dl>

      <TrafficBar used={user.u + user.d} total={user.transfer_enable} />

      <dl className="inset-group">
        <Row label="ID">{user.id}</Row>
        <Row label={t('columns.plan')}>{user.plan?.name ?? t('detail.noPlan')}</Row>
        <Row label={t('columns.group')}>
          {group?.name ?? (user.group_id ? `#${user.group_id}` : t('detail.none'))}
        </Row>
        <Row label={t('columns.expiredAt')}>
          {user.expired_at ? (
            <span className={expired ? 'text-label-red' : undefined}>
              {fmt.dateTime(user.expired_at)}
              {expired ? ` · ${t('expiry.expired')}` : ''}
            </span>
          ) : (
            t('common:values.never')
          )}
        </Row>
        <Row label={t('detail.nextReset')}>{user.next_reset_at ? fmt.dateTime(user.next_reset_at) : '—'}</Row>
        <Row label={t('detail.limits')}>
          {t('detail.limitsValue', {
            speed: user.speed_limit ? `${user.speed_limit} Mbps` : t('common:values.unlimited'),
            devices: user.device_limit ?? t('common:values.unlimited'),
          })}
        </Row>
        <Row label={t('columns.balance')}>{fmt.money(user.balance)}</Row>
        <Row label={t('detail.commissionBalance')}>{fmt.money(user.commission_balance)}</Row>
        <Row label={t('detail.inviter')}>
          {user.invite_user_id
            ? `${user.invite_user_email ?? ''} #${user.invite_user_id}`.trim()
            : t('detail.none')}
        </Row>
        <Row label={t('columns.createdAt')}>{fmt.dateTime(user.created_at)}</Row>
        <Row label={t('detail.lastLogin')}>
          {user.last_login_at
            ? `${fmt.dateTime(user.last_login_at)}${user.last_login_ip ? ` · ${user.last_login_ip}` : ''}`
            : '—'}
        </Row>
        {user.telegram_id ? <Row label="Telegram">{user.telegram_id}</Row> : null}
      </dl>

      <dl className="inset-group">
        {[
          ['UUID', user.uuid],
          [t('detail.token'), user.token ?? ''],
          [t('detail.subscribeUrl'), subscribe ?? ''],
        ].map(([label, value]) =>
          value ? (
            <div key={label} className="flex items-center gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <dt className="text-footnote text-label-secondary">{label}</dt>
                <dd className="break-all font-mono text-footnote text-label">{value}</dd>
              </div>
              <CopyButton value={value} />
            </div>
          ) : null,
        )}
      </dl>

      {user.remarks ? (
        <section>
          <h3 className="mb-1 text-footnote text-label-secondary">{t('fields.remarks')}</h3>
          <p className="whitespace-pre-wrap text-subhead text-label">{user.remarks}</p>
        </section>
      ) : null}

      {canEdit ? (
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="secondary" onPress={() => void actions.run('loginAs', user)}>
            {t('actions.loginAs')}
          </Button>
          <Button size="sm" variant="secondary" onPress={() => void actions.run('resetTraffic', user)}>
            {t('actions.resetTraffic')}
          </Button>
          <Button size="sm" variant="secondary" onPress={() => void actions.run('resetSecret', user)}>
            {t('actions.resetSecret')}
          </Button>
          <Button
            size="sm"
            variant={user.banned ? 'secondary' : 'danger-soft'}
            onPress={() => void actions.run(user.banned ? 'unban' : 'ban', user)}
          >
            {user.banned ? t('actions.unban') : t('actions.ban')}
          </Button>
          <Button size="sm" variant="danger-soft" onPress={() => void actions.run('delete', user)}>
            {t('common:actions.delete')}
          </Button>
        </div>
      ) : null}
      <LoginAsDialog value={actions.loginUrl} onClose={actions.closeLoginUrl} />
    </div>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <fieldset className="flex min-w-0 flex-col gap-4">
      <legend className="mb-3 text-headline text-label">{title}</legend>
      {children}
    </fieldset>
  )
}

function EditForm({ user, onSaved }: { user: AdminUser; onSaved: (user: AdminUser) => void }) {
  const t = useT(['admin-users', 'common'])
  const { currency } = useSite()
  const plans = useQuery(adminPlansQuery())
  const groups = useQuery(adminGroupsQuery())
  const [formError, setFormError] = useState<string>()
  const [initial, setInitial] = useState(() => userToValues(user, currency))
  const save = useAdminMutation({
    mutationFn: (patch: AdminUserUpdateInput) => adminApi.updateUser(user.id, patch),
    success: t('saved'),
    silentError: true,
  })
  const form = useAppForm({
    defaultValues: initial,
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      const patch = valuesToPatch(value, initial, currency)
      if (Object.keys(patch).length === 0) {
        toast.info(t('noChanges'))
        return
      }
      try {
        const saved = await save.mutateAsync(patch)
        const next = userToValues(saved, currency)
        setInitial(next)
        formApi.reset(next)
        onSaved(saved)
      } catch (error) {
        setFormError(submitError(formApi, error))
      }
    },
  })
  const money = { style: 'currency', currency } as const
  const gb = { maximumFractionDigits: 2 } as const
  const planOptions = [
    { value: NONE, label: t('detail.noPlan') },
    ...(plans.data ?? []).map((p) => ({ value: String(p.id), label: p.name })),
  ]
  const groupOptions = [
    { value: NONE, label: t('detail.none') },
    ...(groups.data ?? []).map((g) => ({ value: String(g.id), label: g.name })),
  ]

  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        void form.handleSubmit()
      }}
      className="flex flex-col gap-8 pb-2"
    >
      <FormError message={formError} />
      <Section title={t('sections.account')}>
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
          validators={{
            onSubmit: ({ value }) => (value && value.length < 8 ? t('errors.password') : undefined),
          }}
        >
          {(f) => (
            <f.TextField
              label={t('fields.newPassword')}
              autoComplete="new-password"
              description={t('fields.newPasswordHint')}
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
        <form.AppField name="invite_user_id">
          {(f) => (
            <f.NumberField
              label={t('fields.inviter')}
              minValue={1}
              step={1}
              steppers={false}
              formatOptions={{ useGrouping: false }}
              description={
                user.invite_user_email
                  ? t('fields.inviterHint', { email: user.invite_user_email })
                  : t('fields.inviterEmpty')
              }
            />
          )}
        </form.AppField>
        <form.AppField name="remarks">
          {(f) => <f.TextAreaField label={t('fields.remarks')} rows={3} />}
        </form.AppField>
      </Section>

      <Section title={t('sections.subscription')}>
        <div className="grid gap-4 sm:grid-cols-2">
          <form.AppField name="plan_id">
            {(f) => <f.SelectField label={t('fields.plan')} options={planOptions} />}
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
                formatOptions={gb}
                description={t('fields.gbHint')}
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
          <form.AppField name="u">
            {(f) => <f.NumberField label={t('fields.upload')} minValue={0} step={1} formatOptions={gb} />}
          </form.AppField>
          <form.AppField name="d">
            {(f) => <f.NumberField label={t('fields.download')} minValue={0} step={1} formatOptions={gb} />}
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
        </div>
      </Section>

      <Section title={t('sections.finance')}>
        <div className="grid gap-4 sm:grid-cols-2">
          <form.AppField name="balance">
            {(f) => (
              <f.NumberField label={t('fields.balance')} step={0.01} formatOptions={money} steppers={false} />
            )}
          </form.AppField>
          <form.AppField name="commission_balance">
            {(f) => (
              <f.NumberField
                label={t('fields.commissionBalance')}
                minValue={0}
                step={0.01}
                formatOptions={money}
                steppers={false}
              />
            )}
          </form.AppField>
          <form.AppField name="commission_type">
            {(f) => (
              <f.SelectField
                label={t('fields.commissionType')}
                options={(['0', '1', '2'] as const).map((v) => ({
                  value: v,
                  label: t(`commissionTypes.${v}`),
                }))}
              />
            )}
          </form.AppField>
          <form.AppField name="commission_rate">
            {(f) => (
              <f.NumberField
                label={t('fields.commissionRate')}
                minValue={0}
                maxValue={100}
                step={1}
                description={t('fields.followSystemHint')}
              />
            )}
          </form.AppField>
          <form.AppField name="discount">
            {(f) => (
              <f.NumberField
                label={t('fields.discount')}
                minValue={0}
                maxValue={100}
                step={1}
                description={t('fields.discountHint')}
              />
            )}
          </form.AppField>
        </div>
      </Section>

      <Section title={t('sections.flags')}>
        <div className="grid gap-3 sm:grid-cols-2">
          <form.AppField name="banned">
            {(f) => <f.SwitchField label={t('fields.banned')} description={t('fields.bannedHint')} />}
          </form.AppField>
          <form.AppField name="is_admin">
            {(f) => <f.SwitchField label={t('fields.isAdmin')} />}
          </form.AppField>
          <form.AppField name="is_staff">
            {(f) => <f.SwitchField label={t('fields.isStaff')} description={t('fields.isStaffHint')} />}
          </form.AppField>
          <form.AppField name="remind_expire">
            {(f) => <f.SwitchField label={t('fields.remindExpire')} />}
          </form.AppField>
          <form.AppField name="remind_traffic">
            {(f) => <f.SwitchField label={t('fields.remindTraffic')} />}
          </form.AppField>
        </div>
      </Section>

      <div className="sticky bottom-0 flex justify-end gap-2 bg-overlay py-3">
        <form.Subscribe selector={(s) => s.isDirty}>
          {(dirty) => (
            <Button variant="tertiary" isDisabled={!dirty} onPress={() => form.reset(initial)}>
              {t('common:actions.reset')}
            </Button>
          )}
        </form.Subscribe>
        <form.AppForm>
          <form.SubmitButton size="md">{t('common:actions.save')}</form.SubmitButton>
        </form.AppForm>
      </div>
    </form>
  )
}

export function UserDrawer({
  userId,
  canEdit,
  onClose,
}: {
  userId: number
  canEdit: boolean
  onClose: () => void
}) {
  const t = useT(['admin-users', 'common'])
  const detail = useQuery(adminUserQuery(userId))
  const [tab, setTab] = useState<Tab>('summary')
  const user = detail.data
  return (
    <FormDrawer
      isOpen
      onOpenChange={(open) => !open && onClose()}
      size="xl"
      title={user?.email ?? t('detail.loading')}
      subtitle={
        user ? (
          <span className="flex flex-wrap items-center gap-1.5">
            ID {user.id}
            {user.is_admin ? (
              <Chip size="sm" variant="soft" color="accent">
                {t('roles.admin')}
              </Chip>
            ) : null}
            {user.is_staff ? (
              <Chip size="sm" variant="soft" color="accent">
                {t('roles.staff')}
              </Chip>
            ) : null}
            {user.banned ? (
              <Chip size="sm" variant="soft" color="danger">
                {t('filters.banned')}
              </Chip>
            ) : null}
          </span>
        ) : undefined
      }
    >
      {detail.isError && !user ? (
        <p role="alert" className="text-subhead text-label-red">
          {t('detail.loadError')}
        </p>
      ) : !user ? (
        <div className="flex flex-col gap-3" aria-busy="true">
          {Array.from({ length: 6 }, (_, i) => (
            <span key={i} className="block h-10 animate-pulse rounded-[10px] bg-fill-tertiary" />
          ))}
        </div>
      ) : canEdit ? (
        <Tabs
          selectedKey={tab}
          onSelectionChange={(k) => setTab(k === 'profile' || k === 'subscription' ? k : 'summary')}
        >
          <Tabs.ListContainer>
            <Tabs.List aria-label={t('detail.tabs')}>
              <Tabs.Tab id="summary">
                {t('detail.summary')}
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="profile">
                {t('detail.profile')}
                <Tabs.Indicator />
              </Tabs.Tab>
              <Tabs.Tab id="subscription">
                {t('subscription.tab')}
                <Tabs.Indicator />
              </Tabs.Tab>
            </Tabs.List>
          </Tabs.ListContainer>
          <Tabs.Panel id="summary" shouldForceMount className="pt-4 data-[inert=true]:hidden">
            <Summary user={user} canEdit onDeleted={onClose} />
          </Tabs.Panel>
          <Tabs.Panel id="profile" shouldForceMount className="pt-4 data-[inert=true]:hidden">
            <EditForm user={user} onSaved={() => setTab('profile')} />
          </Tabs.Panel>
          <Tabs.Panel id="subscription" className="pt-4">
            {tab === 'subscription' ? <UserSubscriptionTab user={user} /> : null}
          </Tabs.Panel>
        </Tabs>
      ) : (
        <Summary user={user} canEdit={false} onDeleted={onClose} />
      )}
    </FormDrawer>
  )
}
