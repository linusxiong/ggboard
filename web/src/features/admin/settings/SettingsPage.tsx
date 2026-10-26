import { Alert, Button, Skeleton } from '@heroui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { MailCheck } from 'lucide-react'
import { type ReactNode, useMemo, useRef, useState } from 'react'
import { focusFirstInvalid } from '@/components/form/focus'
import { TitleWithHelp } from '@/components/HelpTip'
import { SaveBar } from '@/components/form/SaveBar'
import { PageShell } from '@/components/PageShell'
import { useLocale, useT } from '@/i18n'
import { adminApi, isApiError } from '@/lib/api'
import { adminPlansQuery, qk } from '@/lib/api/queries'
import type { AdminSettingsResponse, SettingDef, SettingGroupMeta } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { RULESETS_FULL_KEY } from '@/features/admin/rulesets/catalog'
import { GeoIPDatabase } from './GeoIPDatabase'
import { DEPENDS_ON, FALLBACK_ICON, GROUP_HELP, GROUP_ICON, type RowModel, sectionsOf } from './layout'
import { ServerSnippets } from './ServerSnippets'
import { SettingField, type SettingFieldContext } from './SettingField'
import { buildGroups, dirtyPatch, type Edits, flattenValues, isGroupDirty } from './settingsMeta'
import { TelegramActions } from './TelegramActions'
import { TestMailDialog } from './TestMailDialog'

const SETTINGS_KEY = [...qk.admin.settings(), 'meta'] as const
const asText = (v: unknown) => (typeof v === 'string' ? v : '')

interface SettingsPageProps {
  /** Selected group (URL `?group=`). */
  group?: string
  onGroupChange: (group: string) => void
  /** Rendered with the overall dirty state (the route mounts a leave-page guard there). */
  renderGuard?: (dirty: boolean) => ReactNode
  /** Link to the rule-set catalog shown in the subscribe group (a router `<Link>`). */
  rulesetsLink?: ReactNode
  /** Link to the e-mail templates shown in the email group (a router `<Link>`). */
  mailTemplatesLink?: ReactNode
}

/** Phones: the group list is a horizontal chip strip — keep the selected chip in view (no page scroll). */
function revealOnPhone(li: HTMLLIElement | null) {
  const strip = li?.parentElement
  if (!li || !strip || strip.scrollWidth <= strip.clientWidth) return
  const offset = li.getBoundingClientRect().left - strip.getBoundingClientRect().left + strip.scrollLeft
  const left = offset - (strip.clientWidth - li.offsetWidth) / 2
  strip.scrollTo({ left: Math.max(0, left), behavior: 'instant' })
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

/**
 * Admin settings, System Settings style: a sticky group list on the left (220 px), the selected
 * group on the right as titled sections of grouped rows, and a sticky save bar. Each group saves on
 * its own (`PUT /admin/settings` with only the changed keys); drafts survive switching groups.
 */
export function SettingsPage({
  group,
  onGroupChange,
  renderGuard,
  rulesetsLink,
  mailTemplatesLink,
}: SettingsPageProps) {
  const t = useT(['admin-settings', 'common'])
  const locale = useLocale()
  const qc = useQueryClient()
  const formRef = useRef<HTMLFormElement>(null)
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
      toast.error(t('validation.fix'))
      focusFirstInvalid(formRef.current)
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
      toast.success(t('save.saved', { group: groupTitle(g.name) }))
      await qc.invalidateQueries({ queryKey: qk.admin.settings() })
    } catch (e) {
      if (isApiError(e) && Object.keys(e.errors).length) {
        const fieldErrors: Record<string, string> = {}
        for (const [k, msgs] of Object.entries(e.errors)) if (msgs[0]) fieldErrors[k] = msgs[0]
        setErrors((prev) => ({ ...prev, ...fieldErrors }))
        const known = new Set(g.keys.map((d) => d.key))
        const stray = Object.keys(fieldErrors).filter((k) => !known.has(k))
        toast.error(stray.length ? `${stray.join(', ')}: ${fieldErrors[stray[0]!]}` : t('validation.fix'))
        focusFirstInvalid(formRef.current)
      } else {
        toast.error(errorMessage(e))
      }
    } finally {
      setSaving(null)
    }
  }

  const optional = (key: string) => {
    const text = t(key)
    return text === key || text === '' ? undefined : text
  }
  const groupTitle = (name: string) => optional(`groups.${name}.title`) ?? name
  const groupHelp = (name: string) => (GROUP_HELP.has(name) ? optional(`groups.${name}.help`) : undefined)

  const renderField = (def: SettingDef) => (
    <SettingField
      key={def.key}
      def={def}
      value={values[def.key]}
      onChange={(v) => setValue(def.key, v)}
      error={errors[def.key]}
      ctx={ctx}
    />
  )

  /** Extra content under a row: warnings about the current configuration, with a shortcut to fix it. */
  const rowExtra = (key: string): ReactNode => {
    if (key === 'email_verify' && values.email_verify === true && values.email_driver !== 'smtp')
      return <EmailLogOnlyNote onOpenEmail={() => onGroupChange('email')} />
    return null
  }

  const renderRow = ({ def, children }: RowModel) => {
    const shown = children.filter((c) => DEPENDS_ON[c.key]?.when(values) ?? true)
    return (
      <div key={def.key} className="flex flex-col gap-4 px-5 py-4 sm:px-6">
        {renderField(def)}
        {rowExtra(def.key)}
        {shown.length > 0 ? (
          <div
            className={cx(
              'grid gap-4 animate-enter',
              shown.length > 1 && 'sm:grid-cols-2',
              shown.length === 3 && 'lg:grid-cols-3',
            )}
          >
            {shown.map(renderField)}
          </div>
        ) : null}
      </div>
    )
  }

  return (
    <PageShell title={t('title')} help={t('help')}>
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
        <div className="grid gap-8 md:grid-cols-[220px_minmax(0,1fr)]" aria-busy="true">
          <Skeleton className="hidden h-96 w-full rounded-card md:block" />
          <div className="flex flex-col gap-6">
            <Skeleton className="h-8 w-48" />
            <Skeleton className="h-64 w-full rounded-card" />
            <Skeleton className="h-48 w-full rounded-card" />
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-[minmax(0,1fr)] items-start gap-6 md:grid-cols-[220px_minmax(0,1fr)] md:gap-8">
          {/* ------------------------------------------------ group list */}
          <nav aria-label={t('groupsLabel')} className="-mx-4 md:sticky md:top-[5.5rem] md:mx-0">
            <ul className="no-scrollbar flex gap-1.5 overflow-x-auto px-4 md:flex-col md:gap-0.5 md:overflow-visible md:px-0">
              {groups.map((g) => {
                const { icon: Icon, tint } = GROUP_ICON[g.name] ?? FALLBACK_ICON
                const selected = g.name === active.name
                return (
                  <li key={g.name} className="shrink-0" ref={selected ? revealOnPhone : undefined}>
                    <button
                      type="button"
                      aria-current={selected ? 'page' : undefined}
                      onClick={() => onGroupChange(g.name)}
                      className={cx(
                        'flex w-full items-center gap-2.5 rounded-[10px] py-1.5 ps-1.5 pe-3 text-start text-subhead text-label transition-colors duration-150',
                        'max-md:rounded-full max-md:bg-fill-quaternary max-md:py-1 max-md:ps-1',
                        selected
                          ? 'bg-selection font-semibold text-label-blue-soft max-md:bg-selection'
                          : 'hover:bg-fill-quaternary',
                      )}
                    >
                      <span
                        className={cx(
                          'flex size-7 shrink-0 items-center justify-center rounded-[8px] text-white max-md:size-6 max-md:rounded-full',
                          tint,
                        )}
                        aria-hidden="true"
                      >
                        <Icon className="size-4 max-md:size-3.5" />
                      </span>
                      <span className="flex-1 whitespace-nowrap md:truncate">{groupTitle(g.name)}</span>
                      {dirtyGroups.has(g.name) ? (
                        <span
                          className="size-2 shrink-0 rounded-full bg-system-orange"
                          aria-label={t('unsaved')}
                          role="img"
                        />
                      ) : null}
                    </button>
                  </li>
                )
              })}
            </ul>
          </nav>

          {/* ------------------------------------------------ selected group */}
          <form
            ref={formRef}
            key={active.name}
            noValidate
            aria-labelledby={`settings-${active.name}`}
            className="flex min-w-0 flex-col gap-8"
            onSubmit={(e) => {
              e.preventDefault()
              void save(active)
            }}
            onKeyDown={(e) => {
              if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
                e.preventDefault()
                void save(active)
              }
            }}
          >
            <header className="flex flex-wrap items-end justify-between gap-3">
              <TitleWithHelp help={groupHelp(active.name)} subject={groupTitle(active.name)}>
                <h2 id={`settings-${active.name}`} className="min-w-0 text-title2 text-label">
                  {groupTitle(active.name)}
                </h2>
              </TitleWithHelp>
              {active.name === 'email' ? (
                <div className="flex flex-wrap gap-2">
                  {mailTemplatesLink}
                  <Button variant="secondary" size="sm" onPress={() => setTestOpen(true)}>
                    <MailCheck className="size-4" aria-hidden />
                    {t('email.test')}
                  </Button>
                </div>
              ) : active.name === 'subscribe' ? (
                rulesetsLink
              ) : null}
            </header>

            {sectionsOf(active).map((section) => {
              const base = section.id ? `sections.${active.name}.${section.id}` : null
              const title =
                section.id === 'other' ? optional('sections.other.title') : base && optional(`${base}.title`)
              const help = base && section.help ? optional(`${base}.help`) : undefined
              const headingId = `settings-${active.name}-${section.id ?? 'main'}`
              return (
                <section
                  key={section.id ?? 'main'}
                  aria-labelledby={title ? headingId : undefined}
                  className="flex flex-col gap-2"
                >
                  {title ? (
                    <div className="px-1">
                      <TitleWithHelp help={help} subject={title}>
                        <h3 id={headingId} className="text-headline text-label">
                          {title}
                        </h3>
                      </TitleWithHelp>
                    </div>
                  ) : null}
                  <div className="inset-group shadow-card [--separator-inset:20px] sm:[--separator-inset:24px]">
                    {section.rows.map(renderRow)}
                  </div>
                  {active.name === 'subscribe' && section.id === 'geoip' ? (
                    <GeoIPDatabase dirty={dirtyGroups.has('subscribe')} />
                  ) : null}
                </section>
              )
            })}

            {active.name === 'server' ? (
              <ServerSnippets
                appUrl={asText(values.app_url)}
                token={asText(values.server_token)}
                revealed={tokenRevealed}
              />
            ) : null}
            {active.name === 'telegram' ? (
              <TelegramActions
                dirty={dirtyGroups.has('telegram')}
                onFieldErrors={(fieldErrors) => {
                  setErrors((prev) => ({ ...prev, ...fieldErrors }))
                  focusFirstInvalid(formRef.current)
                }}
              />
            ) : null}

            <SaveBar
              dirty={dirtyGroups.has(active.name)}
              saving={saving === active.name}
              dirtyLabel={t('save.dirty')}
              cleanLabel={t('save.clean')}
              hint={t('save.shortcut')}
              saveLabel={t('save.save')}
              discardLabel={t('save.discard')}
              onDiscard={() => discard(active)}
            />
          </form>
        </div>
      )}
      <TestMailDialog isOpen={testOpen} onOpenChange={setTestOpen} unsaved={dirtyGroups.has('email')} />
    </PageShell>
  )
}

/**
 * Under "邮箱验证" while mail delivery is "log only": codes would never reach anyone. One line plus a
 * shortcut to the Email group (the SMTP advice itself is in the switch's ⓘ).
 */
function EmailLogOnlyNote({ onOpenEmail }: { onOpenEmail: () => void }) {
  const t = useT('admin-settings')
  return (
    <div
      className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-[12px] bg-warning-soft px-3.5 py-2.5 text-footnote text-label-orange-soft"
      role="note"
    >
      <p className="min-w-0 flex-1 basis-60">{t('register.logWarning')}</p>
      <Button size="sm" variant="secondary" className="shrink-0" onPress={onOpenEmail}>
        {t('register.openEmail')}
      </Button>
    </div>
  )
}
