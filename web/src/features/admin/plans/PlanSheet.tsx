import { Button } from '@heroui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { confirm } from '@/components/feedback/confirm'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { Markdown } from '@/components/Markdown'
import { SegmentedTabs } from '@/components/SegmentedControl'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { adminGroupsQuery, adminUsersQuery, qk } from '@/lib/api/queries'
import { type AdminPlan, type ID, PLAN_PERIODS } from '@/lib/api/types'
import { FieldGroup } from '../shared/FieldGroup'
import { FormDrawer } from '../shared/FormDrawer'
import { CONN_LIMIT_MAX } from '../shared/units'
import { useAdminMutation } from '../shared/useAdminMutation'
import { useSite } from '../shared/useSite'
import { type PlanPatch, planToValues, RESET_CHOICES, valuesToPlanPatch } from './planForm'

/**
 * Everyone `force_update` rewrites: users whose `plan_id` is this plan, expired ones included (`user_count`
 * only counts unexpired subscriptions). Only the list's `total` is used.
 */
const holdersQuery = (planId: ID | undefined) => adminUsersQuery({ plan_id: planId, page_size: 1 })

/** Every admin user list (an empty filter matches any) and user detail (the detail key without its id). */
const ADMIN_USER_KEYS = [qk.admin.users(), qk.admin.user(0).slice(0, -1)] as const

interface SaveVars {
  body: PlanPatch
  /** Holders that `force_update` rewrote (only set when it was sent). */
  synced?: number
}

interface PlanSheetProps {
  plan: AdminPlan | null
  isOpen: boolean
  onOpenChange: (open: boolean) => void
}

export function PlanSheet({ plan, isOpen, onOpenChange }: PlanSheetProps) {
  const t = useT(['admin-plans', 'common'])
  const { currency, symbol } = useSite()
  const queryClient = useQueryClient()
  const groups = useQuery(adminGroupsQuery())
  const holders = useQuery({
    ...holdersQuery(plan?.id),
    enabled: plan !== null,
    select: (page) => page.total,
  })
  // The switch is offered only when someone holds the plan (until the full count arrives, the active one decides).
  const canSync = plan !== null && (holders.data ?? plan.user_count) > 0
  const [formError, setFormError] = useState<string>()
  const [contentTab, setContentTab] = useState<'edit' | 'preview'>('edit')
  const save = useAdminMutation({
    mutationFn: ({ body }: SaveVars) =>
      plan ? adminApi.updatePlan(plan.id, body) : adminApi.createPlan(body),
    success: (_data, { synced }) =>
      synced === undefined ? (plan ? t('saved') : t('created')) : t('sync.done', { count: synced }),
    invalidate: [qk.admin.plans(), qk.admin.groups()],
    onSuccess: (_data, { synced }) => {
      // The holders' group, quota and limits changed: user lists and details are stale now.
      if (synced !== undefined)
        for (const queryKey of ADMIN_USER_KEYS) void queryClient.invalidateQueries({ queryKey })
    },
    silentError: true,
  })

  const form = useAppForm({
    // `force_update` is part of this save only, not of the plan: it starts off every time the sheet opens
    // (PlansPage mounts a fresh sheet per open).
    defaultValues: { ...planToValues(plan, currency), force_update: false },
    onSubmit: async ({ value: { force_update, ...values }, formApi }) => {
      setFormError(undefined)
      try {
        let synced: number | undefined
        if (plan && force_update) {
          // Count right before asking, so the dialog names everyone who is about to be overwritten.
          const { total } = await queryClient.fetchQuery(holdersQuery(plan.id))
          if (total > 0) {
            const yes = await confirm({
              title: t('sync.confirmTitle', { count: total }),
              description: t('sync.confirmBody'),
              confirmLabel: t('sync.confirm'),
              tone: 'danger',
            })
            if (!yes) return
            synced = total
          }
        }
        await save.mutateAsync({ body: valuesToPlanPatch(values, currency, synced !== undefined), synced })
        onOpenChange(false)
      } catch (error) {
        setFormError(submitError(formApi, error))
      }
    },
  })

  const required = (label: string) => ({
    onSubmit: ({ value }: { value: unknown }) =>
      value === null || value === undefined || value === ''
        ? t('errors.required', { field: label })
        : undefined,
  })
  // Decimal currency units (15 → 15.00), never cents; the symbol is an adornment.
  const money = { minimumFractionDigits: 2, maximumFractionDigits: 2, useGrouping: false } as const

  return (
    <FormDrawer
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      isDismissable={false}
      onSubmit={() => void form.handleSubmit()}
      size="xl"
      title={plan ? t('edit', { name: plan.name }) : t('create')}
      subtitle={plan ? t('usersOnPlan', { count: plan.user_count }) : undefined}
      footer={
        <>
          <Button variant="tertiary" onPress={() => onOpenChange(false)}>
            {t('common:actions.cancel')}
          </Button>
          <form.AppForm>
            <form.SubmitButton size="md">{t('common:actions.save')}</form.SubmitButton>
          </form.AppForm>
        </>
      }
    >
      <div className="flex flex-col gap-8 pb-4">
        <FormError message={formError} />
        <FieldGroup title={t('sections.basic')}>
          <div className="grid gap-4 sm:grid-cols-2">
            <form.AppField name="name" validators={required(t('fields.name'))}>
              {(f) => <f.TextField label={t('fields.name')} isRequired />}
            </form.AppField>
            <form.AppField name="group_id" validators={required(t('fields.group'))}>
              {(f) => (
                <f.SelectField
                  label={t('fields.group')}
                  placeholder={t('fields.groupPlaceholder')}
                  options={(groups.data ?? []).map((g) => ({ value: String(g.id), label: g.name }))}
                  description={t('fields.groupHint')}
                  isRequired
                />
              )}
            </form.AppField>
          </div>
          <form.AppField name="tags">
            {(f) => (
              <f.TagsField
                label={t('fields.tags')}
                placeholder={t('fields.tagsPlaceholder')}
                description={t('fields.tagsHint')}
                removeLabel={(item) => t('fields.tagRemove', { item })}
              />
            )}
          </form.AppField>
          <div className="separated flex flex-col rounded-[14px] bg-fill-quaternary px-4 [&>*]:py-3">
            <form.AppField name="show">
              {(f) => <f.SwitchField label={t('fields.show')} description={t('fields.showHint')} />}
            </form.AppField>
            <form.AppField name="sell">
              {(f) => <f.SwitchField label={t('fields.sell')} description={t('fields.sellHint')} />}
            </form.AppField>
            <form.AppField name="renew">
              {(f) => <f.SwitchField label={t('fields.renew')} description={t('fields.renewHint')} />}
            </form.AppField>
          </div>
        </FieldGroup>

        <FieldGroup title={t('sections.traffic')}>
          <div className="grid gap-4 sm:grid-cols-2">
            <form.AppField name="transfer_enable" validators={required(t('fields.transfer'))}>
              {(f) => (
                <f.NumberField
                  label={t('fields.transfer')}
                  minValue={0}
                  step={1}
                  unit="GB"
                  description={t('fields.transferHint')}
                  isRequired
                />
              )}
            </form.AppField>
            <form.AppField name="reset_traffic_method">
              {(f) => (
                <f.SelectField
                  label={t('fields.resetMethod')}
                  options={RESET_CHOICES.map((c) => ({ value: c, label: t(`reset.${c}`) }))}
                />
              )}
            </form.AppField>
            <form.AppField name="speed_limit">
              {(f) => (
                <f.NumberField
                  label={t('fields.speedLimit')}
                  minValue={0}
                  step={1}
                  unit="Mbps"
                  placeholder={t('common:values.unlimited')}
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
                  unit={t('units.devices')}
                  placeholder={t('common:values.unlimited')}
                  description={t('fields.deviceLimitHint')}
                />
              )}
            </form.AppField>
            <form.AppField name="conn_limit">
              {(f) => (
                <f.NumberField
                  label={t('fields.connLimit')}
                  minValue={0}
                  maxValue={CONN_LIMIT_MAX}
                  step={1}
                  unit={t('units.connections')}
                  placeholder={t('common:values.unlimited')}
                  description={t('fields.connLimitHint')}
                />
              )}
            </form.AppField>
            <form.AppField name="capacity_limit">
              {(f) => (
                <f.NumberField
                  label={t('fields.capacityLimit')}
                  minValue={0}
                  step={1}
                  unit={t('units.users')}
                  placeholder={t('common:values.unlimited')}
                  description={t('fields.capacityHint')}
                />
              )}
            </form.AppField>
          </div>
          {canSync ? (
            <div className="rounded-[14px] bg-fill-quaternary px-4 py-3">
              <form.AppField name="force_update">
                {(f) => <f.SwitchField label={t('sync.label')} description={t('sync.hint')} />}
              </form.AppField>
            </div>
          ) : null}
        </FieldGroup>

        <FieldGroup title={t('sections.prices')} help={t('pricesHint')}>
          <div className="grid gap-4 sm:grid-cols-2">
            {PLAN_PERIODS.map((period) => (
              <form.AppField key={period} name={`prices.${period}`}>
                {(f) => (
                  <f.NumberField
                    label={t(`common:periods.${period}`)}
                    minValue={0}
                    step={0.01}
                    prefix={symbol ?? currency}
                    placeholder={t('priceOff')}
                    formatOptions={money}
                  />
                )}
              </form.AppField>
            ))}
          </div>
        </FieldGroup>

        <FieldGroup title={t('sections.content')}>
          <form.Subscribe selector={(s) => s.values.content}>
            {(content) => (
              <SegmentedTabs
                value={contentTab}
                onChange={(k) => setContentTab(k === 'preview' ? 'preview' : 'edit')}
                className="flex flex-col gap-3"
              >
                <SegmentedTabs.List
                  aria-label={t('fields.content')}
                  size="sm"
                  items={[
                    { id: 'edit', label: t('contentEdit') },
                    { id: 'preview', label: t('contentPreview') },
                  ]}
                />
                <SegmentedTabs.Panel id="edit">
                  <form.AppField name="content">
                    {(f) => (
                      <f.TextAreaField
                        label={t('fields.content')}
                        rows={10}
                        description={t('fields.contentHint')}
                      />
                    )}
                  </form.AppField>
                </SegmentedTabs.Panel>
                <SegmentedTabs.Panel id="preview">
                  {content.trim() ? (
                    <Markdown source={content} className="rounded-[12px] bg-fill-quaternary p-4" />
                  ) : (
                    <p className="text-subhead text-label-secondary">{t('contentEmpty')}</p>
                  )}
                </SegmentedTabs.Panel>
              </SegmentedTabs>
            )}
          </form.Subscribe>
        </FieldGroup>
      </div>
    </FormDrawer>
  )
}
