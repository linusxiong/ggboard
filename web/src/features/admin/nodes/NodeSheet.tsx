/**
 * Create / edit node sheet: common fields, type-specific protocol form, raw JSON and backend
 * integration snippets. Validation errors (client or server `errors` keyed by JSON path) are
 * shown inline; the sheet switches to the tab that holds the first error.
 */
import { Button, Chip } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { useCallback, useMemo, useState } from 'react'
import { FormError, useAppForm } from '@/components/form'
import { applyServerErrors } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { SegmentedTabs } from '@/components/SegmentedControl'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { ApiError, isApiError } from '@/lib/api/client'
import { adminGroupsQuery, adminRoutesQuery, qk } from '@/lib/api/queries'
import { type Node, type NodeInput, NODE_TYPES } from '@/lib/api/types'
import { FormDrawer } from '../shared/FormDrawer'
import { MultiSelectField } from '../shared/fields'
import { useAdminMutation } from '../shared/useAdminMutation'
import { NodeIntegration } from './NodeIntegration'
import { NodeTokenField } from './NodeTokenField'
import {
  NO_PARENT,
  nodeToValues,
  portError,
  splitNodeErrors,
  tokenError,
  tokenOwner,
  valuesToNodeInput,
} from './nodeForm'
import { defaultSettings, type Settings, validateSettings } from './protocol'
import { ProtocolForm } from './ProtocolForm'
import { RawJsonEditor } from './RawJsonEditor'

type Tab = 'basic' | 'protocol' | 'json' | 'integration'
const BASIC_FIELDS = new Set([
  'type',
  'name',
  'group_ids',
  'route_ids',
  'parent_id',
  'host',
  'port',
  'server_port',
  'tags',
  'rate',
  'show',
  'sort',
  'token',
])

interface NodeSheetProps {
  node: Node | null
  /** All nodes (parent candidates). */
  nodes: readonly Node[]
  isOpen: boolean
  onOpenChange: (open: boolean) => void
}

export function NodeSheet({ node, nodes, isOpen, onOpenChange }: NodeSheetProps) {
  const t = useT(['admin-nodes', 'common'])
  const groups = useQuery(adminGroupsQuery())
  const routes = useQuery(adminRoutesQuery())
  const [tab, setTab] = useState<Tab>('basic')
  /** Bumped whenever the JSON tab opens so the editor re-reads the current settings. */
  const [jsonKey, setJsonKey] = useState(0)
  const selectTab = (next: Tab) => {
    if (next === 'json' && tab !== 'json') setJsonKey((k) => k + 1)
    setTab(next)
  }
  const [psErrors, setPsErrors] = useState<Record<string, string>>({})
  const [formError, setFormError] = useState<string>()

  const save = useAdminMutation({
    mutationFn: (input: NodeInput) =>
      node ? adminApi.updateNode(node.id, input) : adminApi.createNode(input),
    success: node ? t('nodes.saved') : t('nodes.created'),
    invalidate: [qk.admin.nodes(), qk.admin.groups()],
    silentError: true,
  })

  const issueText = useCallback(
    (key: string, params?: Record<string, string | number>) => t(`nodes.ps.validation.${key}`, params),
    [t],
  )

  const form = useAppForm({
    defaultValues: nodeToValues(node),
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      const issues = validateSettings(value.type, value.protocol_settings)
      if (Object.keys(issues).length) {
        setPsErrors(
          Object.fromEntries(Object.entries(issues).map(([path, i]) => [path, issueText(i.key, i.params)])),
        )
        setTab('protocol')
        setFormError(t('nodes.fixProtocol'))
        return
      }
      setPsErrors({})
      try {
        await save.mutateAsync(valuesToNodeInput(value))
        onOpenChange(false)
      } catch (error) {
        if (isApiError(error) && Object.keys(error.errors).length) {
          const { protocol, rest } = splitNodeErrors(error.errors)
          setPsErrors(protocol)
          const restKeys = Object.keys(rest)
          const unmatched = restKeys.length
            ? applyServerErrors(
                formApi,
                new ApiError(error.status, { code: error.code, message: error.message, errors: rest }),
              )
            : undefined
          if (Object.keys(protocol).length) setTab('protocol')
          else if (restKeys.some((k) => BASIC_FIELDS.has(k))) setTab('basic')
          setFormError(unmatched ?? error.message)
        } else {
          setFormError(submitError(formApi, error))
        }
      }
    },
  })

  const onEditSetting = useCallback((path: string) => {
    setPsErrors((prev) => {
      if (!(path in prev) && !('' in prev)) return prev
      const next = { ...prev }
      delete next[path]
      delete next['']
      return next
    })
  }, [])

  const groupOptions = useMemo(
    () => (groups.data ?? []).map((g) => ({ value: String(g.id), label: g.name })),
    [groups.data],
  )
  const routeOptions = useMemo(
    () =>
      (routes.data ?? []).map((r) => ({
        value: String(r.id),
        label: `${r.remarks || `#${r.id}`} · ${t(`routes.actions.${r.action}`)}`,
        textValue: r.remarks || `#${r.id}`,
      })),
    [routes.data, t],
  )

  const panelClass = 'data-[inert=true]:hidden'

  return (
    <FormDrawer
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      isDismissable={false}
      onSubmit={() => void form.handleSubmit()}
      size="xl"
      title={node ? t('nodes.edit', { name: node.name }) : t('nodes.create')}
      subtitle={node ? `ID ${node.id}` : undefined}
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
      <div className="flex flex-col gap-4 pb-4">
        <FormError message={formError} />
        <form.Subscribe selector={(s) => s.values.type}>
          {(type) =>
            node ? (
              <p className="flex items-center gap-2 text-subhead text-label-secondary">
                {t('nodes.fields.type')}
                <Chip size="sm" variant="soft" color="accent">
                  {t(`nodes.types.${type}`)}
                </Chip>
              </p>
            ) : (
              <form.AppField
                name="type"
                listeners={{
                  onChange: ({ value }) => {
                    form.setFieldValue('protocol_settings', defaultSettings(value))
                    form.setFieldValue('parent_id', NO_PARENT)
                    setPsErrors({})
                  },
                }}
              >
                {(f) => (
                  <f.SelectField
                    label={t('nodes.fields.type')}
                    options={NODE_TYPES.map((nt) => ({ value: nt, label: t(`nodes.types.${nt}`) }))}
                    description={t('nodes.fields.typeHint')}
                  />
                )}
              </form.AppField>
            )
          }
        </form.Subscribe>

        <SegmentedTabs value={tab} onChange={selectTab} className="flex flex-col gap-4">
          <SegmentedTabs.List
            aria-label={t('nodes.sections')}
            items={(['basic', 'protocol', 'json', 'integration'] as const).map((id) => ({
              id,
              label: t(`nodes.tabs.${id}`),
              adornment:
                id === 'protocol' && Object.keys(psErrors).length ? (
                  <span
                    role="img"
                    className="inline-block size-2 rounded-full bg-system-red"
                    aria-label={t('nodes.hasErrors')}
                  />
                ) : undefined,
            }))}
          />

          <SegmentedTabs.Panel id="basic" shouldForceMount className={panelClass}>
            <div className="flex flex-col gap-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <form.AppField
                  name="name"
                  validators={{
                    onSubmit: ({ value }) => (value.trim() ? undefined : t('nodes.errors.required')),
                  }}
                >
                  {(f) => <f.TextField label={t('nodes.fields.name')} isRequired />}
                </form.AppField>
                <form.AppField name="tags">
                  {(f) => (
                    <f.TextField
                      label={t('nodes.fields.tags')}
                      placeholder="HK, Netflix"
                      description={t('nodes.fields.tagsHint')}
                    />
                  )}
                </form.AppField>
                <form.AppField
                  name="host"
                  validators={{
                    onSubmit: ({ value }) => (value.trim() ? undefined : t('nodes.errors.required')),
                  }}
                >
                  {(f) => (
                    <f.TextField
                      label={t('nodes.fields.host')}
                      placeholder="hk1.example.com"
                      description={t('nodes.fields.hostHint')}
                      isRequired
                    />
                  )}
                </form.AppField>
                <form.AppField
                  name="port"
                  validators={{
                    onSubmit: ({ value }) => {
                      const issue = portError(value)
                      return issue ? t(`nodes.errors.${issue}`) : undefined
                    },
                  }}
                >
                  {(f) => (
                    <f.TextField
                      label={t('nodes.fields.port')}
                      placeholder="443 / 20000-30000"
                      description={t('nodes.fields.portHint')}
                      isRequired
                    />
                  )}
                </form.AppField>
                <form.AppField
                  name="server_port"
                  validators={{
                    onSubmit: ({ value }) =>
                      value !== null && Number.isInteger(value) && value >= 1 && value <= 65535
                        ? undefined
                        : t('nodes.errors.serverPort'),
                  }}
                >
                  {(f) => (
                    <f.NumberField
                      label={t('nodes.fields.serverPort')}
                      minValue={1}
                      maxValue={65535}
                      formatOptions={{ useGrouping: false }}
                      description={t('nodes.fields.serverPortHint')}
                      isRequired
                    />
                  )}
                </form.AppField>
                <form.AppField name="rate">
                  {(f) => (
                    <f.NumberField
                      label={t('nodes.fields.rate')}
                      minValue={0}
                      step={0.1}
                      formatOptions={{ maximumFractionDigits: 2 }}
                      description={t('nodes.fields.rateHint')}
                    />
                  )}
                </form.AppField>
              </div>
              <form.AppField name="group_ids">
                {() => (
                  <MultiSelectField
                    label={t('nodes.fields.groups')}
                    options={groupOptions}
                    placeholder={t('nodes.fields.groupsPlaceholder')}
                    description={t('nodes.fields.groupsHint')}
                  />
                )}
              </form.AppField>
              <form.AppField name="route_ids">
                {() => (
                  <MultiSelectField
                    label={t('nodes.fields.routes')}
                    options={routeOptions}
                    placeholder={t('nodes.fields.routesPlaceholder')}
                    description={t('nodes.fields.routesHint')}
                  />
                )}
              </form.AppField>
              <form.Subscribe selector={(s) => s.values.type}>
                {(type) => (
                  <form.AppField name="parent_id">
                    {(f) => (
                      <f.SelectField
                        label={t('nodes.fields.parent')}
                        options={[
                          { value: NO_PARENT, label: t('nodes.fields.noParent') },
                          ...nodes
                            .filter((n) => n.type === type && !n.parent_id && n.id !== node?.id)
                            .map((n) => ({ value: String(n.id), label: `${n.name} (#${n.id})` })),
                        ]}
                        description={t('nodes.fields.parentHint')}
                      />
                    )}
                  </form.AppField>
                )}
              </form.Subscribe>
              {/* Child nodes authenticate with their parent's token: no field, `token: ''` is sent. */}
              <form.Subscribe selector={(s) => s.values.parent_id === NO_PARENT}>
                {(standalone) =>
                  standalone ? (
                    <form.AppField
                      name="token"
                      validators={{
                        onSubmit: ({ value }) => (tokenError(value) ? t('nodes.errors.token') : undefined),
                      }}
                    >
                      {() => <NodeTokenField saved={node?.token ?? ''} />}
                    </form.AppField>
                  ) : null
                }
              </form.Subscribe>
              <div className="grid gap-4 sm:grid-cols-2">
                <form.AppField name="sort">
                  {(f) => (
                    <f.NumberField
                      label={t('nodes.fields.sort')}
                      minValue={0}
                      step={1}
                      description={t('nodes.fields.sortHint')}
                    />
                  )}
                </form.AppField>
                <form.AppField name="show">
                  {(f) => (
                    <f.SwitchField label={t('nodes.fields.show')} description={t('nodes.fields.showHint')} />
                  )}
                </form.AppField>
              </div>
            </div>
          </SegmentedTabs.Panel>

          <SegmentedTabs.Panel id="protocol" shouldForceMount className={panelClass}>
            <form.Subscribe selector={(s) => s.values.type}>
              {(type) => (
                <form.Field name="protocol_settings">
                  {(field) => (
                    <ProtocolForm
                      type={type}
                      value={field.state.value}
                      onChange={(next: Settings) => field.handleChange(next)}
                      onEdit={onEditSetting}
                      errors={psErrors}
                    />
                  )}
                </form.Field>
              )}
            </form.Subscribe>
          </SegmentedTabs.Panel>

          <SegmentedTabs.Panel id="json" shouldForceMount className={panelClass}>
            <form.Field name="protocol_settings">
              {(field) => (
                <RawJsonEditor
                  key={jsonKey}
                  value={field.state.value}
                  onChange={(next) => {
                    field.handleChange(next)
                    setPsErrors({})
                  }}
                  issues={psErrors}
                />
              )}
            </form.Field>
          </SegmentedTabs.Panel>

          <SegmentedTabs.Panel id="integration" shouldForceMount className={panelClass}>
            <form.Subscribe
              selector={(s) =>
                [
                  s.values.type,
                  s.values.protocol_settings.version,
                  s.values.parent_id,
                  s.values.token,
                ] as const
              }
            >
              {([type, version, parent, token]) => (
                <NodeIntegration
                  type={type}
                  nodeId={node?.id ?? null}
                  version={version}
                  isChild={parent !== NO_PARENT}
                  tokenOwner={tokenOwner(parent, token, nodes)}
                />
              )}
            </form.Subscribe>
          </SegmentedTabs.Panel>
        </SegmentedTabs>
      </div>
    </FormDrawer>
  )
}
