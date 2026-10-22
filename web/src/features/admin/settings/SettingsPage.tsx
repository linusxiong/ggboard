import { Alert, Button, Skeleton, Spinner, Tabs } from '@heroui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { MailCheck } from 'lucide-react'
import { type ReactNode, useMemo, useState, useSyncExternalStore } from 'react'
import { PageShell } from '@/components/PageShell'
import { useLocale, useT } from '@/i18n'
import { adminApi, isApiError } from '@/lib/api'
import { adminPlansQuery, qk } from '@/lib/api/queries'
import type { AdminSettingsResponse, SettingGroupMeta } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { RULESETS_FULL_KEY } from '@/features/admin/rulesets/catalog'
import { ServerSnippets } from './ServerSnippets'
import { SettingField, type SettingFieldContext } from './SettingField'
import { buildGroups, dirtyPatch, type Edits, flattenValues, isGroupDirty } from './settingsMeta'
import { TestMailDialog } from './TestMailDialog'

const SETTINGS_KEY = [...qk.admin.settings(), 'meta'] as const
const asText = (v: unknown) => (typeof v === 'string' ? v : '')

const MD = '(min-width: 768px)'
const subscribeMd = (cb: () => void) => {
  const mql = window.matchMedia?.(MD)
  mql?.addEventListener('change', cb)
  return () => mql?.removeEventListener('change', cb)
}
const isDesktop = () => window.matchMedia?.(MD).matches ?? true

interface SettingsPageProps {
  /** Selected group (URL `?group=`). */
  group?: string
  onGroupChange: (group: string) => void
  /** Rendered with the overall dirty state (the route mounts a leave-page guard there). */
  renderGuard?: (dirty: boolean) => ReactNode
  /** Link to the rule-set catalog shown in the subscribe group (a router `<Link>`). */
  rulesetsLink?: ReactNode
}

/** Writes a saved patch into the cached `GET /admin/settings` response (no flash before refetch). */
function applyPatch(
  resp: AdminSettingsResponse,
  groups: readonly SettingGroupMeta[],
  patch: Record<string, unknown>,
) {
  const next: AdminSettingsResponse = { ...resp }
  for (const g of groups) {
    const keys = g.keys.filter((d) => d.key in patch)
    if (keys.length === 0) continue
    const values = { ...((resp[g.name] as Record<string, unknown> | undefined) ?? {}) }
    for (const d of keys) values[d.key] = patch[d.key]
    next[g.name] = values
  }
  return next
}

export function SettingsPage({ group, onGroupChange, renderGuard, rulesetsLink }: SettingsPageProps) {
  const t = useT(['admin-settings', 'common'])
  const locale = useLocale()
  const qc = useQueryClient()
  const desktop = useSyncExternalStore(subscribeMd, isDesktop, () => true)
  const settings = useQuery({
    queryKey: SETTINGS_KEY,
    queryFn: ({ signal }) => adminApi.settingsWithMeta(signal),
  })
  const rulesets = useQuery({
    queryKey: RULESETS_FULL_KEY,
    queryFn: ({ signal }) => adminApi.rulesetsFull(signal),
    staleTime: 5 * 60_000,
  })
  const plans = useQuery(adminPlansQuery())

  const groups = useMemo(() => (settings.data ? buildGroups(settings.data) : []), [settings.data])
  const initial = useMemo(
    () => (settings.data ? flattenValues(settings.data, groups) : {}),
    [settings.data, groups],
  )
  const [edits, setEdits] = useState<Edits>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState<string | null>(null)
  const [tokenRevealed, setTokenRevealed] = useState(false)
  const [testOpen, setTestOpen] = useState(false)

  const values = useMemo(() => ({ ...initial, ...edits }), [initial, edits])
  const active = groups.find((g) => g.name === group) ?? groups[0]
  const dirtyGroups = useMemo(
    () => new Set(groups.filter((g) => isGroupDirty(g, initial, edits)).map((g) => g.name)),
    [groups, initial, edits],
  )

  const ctx: SettingFieldContext = {
    plans: plans.data ?? [],
    catalog: rulesets.data?.effective ?? rulesets.data?.builtin,
    locale,
    values,
    tokenRevealed,
    setTokenRevealed,
  }

  const setValue = (key: string, value: unknown) => {
    setEdits((e) => ({ ...e, [key]: value }))
    setErrors((e) => {
      if (!e[key]) return e
      const next = { ...e }
      delete next[key]
      return next
    })
  }

  const discard = (g: SettingGroupMeta) => {
    const keys = new Set(g.keys.map((d) => d.key))
    setEdits((e) => Object.fromEntries(Object.entries(e).filter(([k]) => !keys.has(k))))
    setErrors((e) => Object.fromEntries(Object.entries(e).filter(([k]) => !keys.has(k))))
  }

  const save = async (g: SettingGroupMeta) => {
    const { patch, errors: problems } = dirtyPatch(g, initial, edits, {
      json: t('validation.json'),
      number: t('validation.number'),
      min: (min) => t('validation.min', { min }),
      max: (max) => t('validation.max', { max }),
    })
    if (Object.keys(problems).length) {
      setErrors((e) => ({ ...e, ...problems }))
      toast.danger(t('validation.fix'))
      return
    }
    if (Object.keys(patch).length === 0) {
      discard(g)
      toast(t('save.nothing'))
      return
    }
    setSaving(g.name)
    try {
      await adminApi.patchSettings(patch)
      qc.setQueryData<AdminSettingsResponse>(SETTINGS_KEY, (old) =>
        old ? applyPatch(old, groups, patch) : old,
      )
      discard(g)
      toast.success(t('save.saved', { group: t(`groups.${g.name}.title`) }))
      await qc.invalidateQueries({ queryKey: qk.admin.settings() })
    } catch (e) {
      if (isApiError(e) && Object.keys(e.errors).length) {
        const fieldErrors: Record<string, string> = {}
        for (const [k, msgs] of Object.entries(e.errors)) if (msgs[0]) fieldErrors[k] = msgs[0]
        setErrors((prev) => ({ ...prev, ...fieldErrors }))
        const known = new Set(g.keys.map((d) => d.key))
        const stray = Object.keys(fieldErrors).filter((k) => !known.has(k))
        toast.danger(stray.length ? `${stray.join(', ')}: ${fieldErrors[stray[0]!]}` : t('validation.fix'))
      } else {
        toast.danger(errorMessage(e))
      }
    } finally {
      setSaving(null)
    }
  }

  const groupTitle = (name: string) => {
    const key = `groups.${name}.title`
    const text = t(key)
    return text === key ? name : text
  }
  const groupHint = (name: string) => {
    const key = `groups.${name}.hint`
    const text = t(key)
    return text === key ? undefined : text
  }

  return (
    <PageShell title={t('title')} description={t('description')}>
      {renderGuard?.(dirtyGroups.size > 0)}
      {settings.isError ? (
        <Alert status="danger">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>{t('loadFailed')}</Alert.Title>
            <Alert.Description>{errorMessage(settings.error)}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : !active ? (
        <div className="grid gap-6 md:grid-cols-[13rem_1fr]" aria-busy="true">
          <Skeleton className="h-72 w-full rounded-card" />
          <Skeleton className="h-[32rem] w-full rounded-card" />
        </div>
      ) : (
        <Tabs
          orientation={desktop ? 'vertical' : 'horizontal'}
          selectedKey={active.name}
          onSelectionChange={(k) => onGroupChange(String(k))}
          className={cx(desktop && 'grid grid-cols-[13rem_1fr] items-start gap-6')}
        >
          <Tabs.ListContainer className={cx(desktop && 'sticky top-20')}>
            <Tabs.List aria-label={t('groupsLabel')}>
              {groups.map((g) => (
                <Tabs.Tab key={g.name} id={g.name} className="justify-between gap-2 whitespace-nowrap">
                  {groupTitle(g.name)}
                  {dirtyGroups.has(g.name) ? (
                    <span
                      className="size-2 rounded-full bg-system-orange"
                      aria-label={t('unsaved')}
                      role="img"
                    />
                  ) : null}
                  <Tabs.Indicator />
                </Tabs.Tab>
              ))}
            </Tabs.List>
          </Tabs.ListContainer>
          {groups.map((g) => (
            <Tabs.Panel key={g.name} id={g.name} className="min-w-0">
              <form
                noValidate
                aria-labelledby={`settings-${g.name}`}
                className="surface-card flex flex-col"
                onSubmit={(e) => {
                  e.preventDefault()
                  void save(g)
                }}
                onKeyDown={(e) => {
                  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
                    e.preventDefault()
                    void save(g)
                  }
                }}
              >
                <div className="flex flex-wrap items-start justify-between gap-3 p-5 hairline-b">
                  <div className="min-w-0">
                    <h2 id={`settings-${g.name}`} className="text-title3 text-label">
                      {groupTitle(g.name)}
                    </h2>
                    {groupHint(g.name) ? (
                      <p className="mt-1 text-subhead text-label-secondary">{groupHint(g.name)}</p>
                    ) : null}
                  </div>
                  {g.name === 'email' ? (
                    <Button size="sm" variant="secondary" onPress={() => setTestOpen(true)}>
                      <MailCheck className="size-4" aria-hidden />
                      {t('email.test')}
                    </Button>
                  ) : g.name === 'subscribe' ? (
                    rulesetsLink
                  ) : null}
                </div>

                <div className="flex max-w-2xl flex-col gap-6 p-5">
                  {g.keys.map((def) => (
                    <SettingField
                      key={def.key}
                      def={def}
                      value={values[def.key]}
                      onChange={(v) => setValue(def.key, v)}
                      error={errors[def.key]}
                      ctx={ctx}
                    />
                  ))}
                  {g.name === 'server' ? (
                    <ServerSnippets
                      appUrl={asText(values.app_url)}
                      token={asText(values.server_token)}
                      revealed={tokenRevealed}
                    />
                  ) : null}
                </div>

                <div className="glass glass-border-t sticky bottom-[calc(3.75rem+env(safe-area-inset-bottom))] z-10 flex items-center justify-end gap-2 rounded-b-card px-5 py-3 md:bottom-0">
                  {dirtyGroups.has(g.name) ? (
                    <span className="me-auto text-footnote text-label-secondary">{t('unsavedHint')}</span>
                  ) : null}
                  <Button
                    variant="tertiary"
                    isDisabled={!dirtyGroups.has(g.name) || saving !== null}
                    onPress={() => discard(g)}
                  >
                    {t('save.discard')}
                  </Button>
                  <Button type="submit" isDisabled={!dirtyGroups.has(g.name)} isPending={saving === g.name}>
                    {({ isPending }) => (
                      <>
                        {isPending ? <Spinner color="current" size="sm" /> : null}
                        {t('save.save')}
                      </>
                    )}
                  </Button>
                </div>
              </form>
            </Tabs.Panel>
          ))}
        </Tabs>
      )}
      <TestMailDialog isOpen={testOpen} onOpenChange={setTestOpen} unsaved={dirtyGroups.has('email')} />
    </PageShell>
  )
}
