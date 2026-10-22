import { Button, Tabs } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { type ReactNode, useState } from 'react'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { Markdown } from '@/components/Markdown'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { adminGroupsQuery, qk } from '@/lib/api/queries'
import { type AdminPlan, PLAN_PERIODS, type PlanInput } from '@/lib/api/types'
import { FormDrawer } from '../shared/FormDrawer'
import { useAdminMutation } from '../shared/useAdminMutation'
import { useSite } from '../shared/useSite'
import { planToValues, RESET_CHOICES, valuesToPlanInput } from './planForm'

interface PlanSheetProps {
  plan: AdminPlan | null
  isOpen: boolean
  onOpenChange: (open: boolean) => void
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <fieldset className="flex flex-col gap-4">
      <legend className="mb-3 text-headline text-label">{title}</legend>
      {children}
    </fieldset>
  )
}

export function PlanSheet({ plan, isOpen, onOpenChange }: PlanSheetProps) {
  const t = useT(['admin-plans', 'common'])
  const { currency } = useSite()
  const groups = useQuery(adminGroupsQuery())
  const [formError, setFormError] = useState<string>()
  const [contentTab, setContentTab] = useState<'edit' | 'preview'>('edit')
  const save = useAdminMutation({
    mutationFn: (input: PlanInput) =>
      plan ? adminApi.updatePlan(plan.id, input) : adminApi.createPlan(input),
    success: plan ? t('saved') : t('created'),
    invalidate: [qk.admin.plans(), qk.admin.groups()],
    silentError: true,
  })

  const form = useAppForm({
    defaultValues: planToValues(plan, currency),
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      try {
        await save.mutateAsync(valuesToPlanInput(value, currency))
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
  const money = { style: 'currency', currency } as const

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
        <Section title={t('sections.basic')}>
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
              <f.TextField
                label={t('fields.tags')}
                placeholder={t('fields.tagsPlaceholder')}
                description={t('fields.tagsHint')}
              />
            )}
          </form.AppField>
          <div className="grid gap-3 sm:grid-cols-3">
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
        </Section>

        <Section title={t('sections.traffic')}>
          <div className="grid gap-4 sm:grid-cols-2">
            <form.AppField name="transfer_enable" validators={required(t('fields.transfer'))}>
              {(f) => (
                <f.NumberField
                  label={t('fields.transfer')}
                  minValue={0}
                  step={1}
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
            <form.AppField name="capacity_limit">
              {(f) => (
                <f.NumberField
                  label={t('fields.capacityLimit')}
                  minValue={0}
                  step={1}
                  description={t('fields.capacityHint')}
                />
              )}
            </form.AppField>
          </div>
        </Section>

        <Section title={t('sections.prices')}>
          <p className="-mt-2 text-footnote text-label-secondary">{t('pricesHint')}</p>
          <div className="grid gap-4 sm:grid-cols-2">
            {PLAN_PERIODS.map((period) => (
              <form.AppField key={period} name={`prices.${period}`}>
                {(f) => (
                  <f.NumberField
                    label={t(`common:periods.${period}`)}
                    minValue={0}
                    step={0.01}
                    formatOptions={money}
                    steppers={false}
                  />
                )}
              </form.AppField>
            ))}
          </div>
        </Section>

        <Section title={t('sections.content')}>
          <form.Subscribe selector={(s) => s.values.content}>
            {(content) => (
              <Tabs
                selectedKey={contentTab}
                onSelectionChange={(k) => setContentTab(k === 'preview' ? 'preview' : 'edit')}
              >
                <Tabs.ListContainer>
                  <Tabs.List aria-label={t('fields.content')}>
                    <Tabs.Tab id="edit">
                      {t('contentEdit')}
                      <Tabs.Indicator />
                    </Tabs.Tab>
                    <Tabs.Tab id="preview">
                      {t('contentPreview')}
                      <Tabs.Indicator />
                    </Tabs.Tab>
                  </Tabs.List>
                </Tabs.ListContainer>
                <Tabs.Panel id="edit" className="pt-3">
                  <form.AppField name="content">
                    {(f) => (
                      <f.TextAreaField
                        label={t('fields.content')}
                        rows={10}
                        description={t('fields.contentHint')}
                      />
                    )}
                  </form.AppField>
                </Tabs.Panel>
                <Tabs.Panel id="preview" className="pt-3">
                  {content.trim() ? (
                    <Markdown source={content} className="rounded-[12px] bg-fill-quaternary p-4" />
                  ) : (
                    <p className="text-subhead text-label-secondary">{t('contentEmpty')}</p>
                  )}
                </Tabs.Panel>
              </Tabs>
            )}
          </form.Subscribe>
        </Section>
      </div>
    </FormDrawer>
  )
}
