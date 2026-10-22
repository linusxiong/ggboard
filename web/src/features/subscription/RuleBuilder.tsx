/**
 * Rule-set builder: template → source → categories (toggle + target) → policy groups → final/mirror/DNS,
 * saved with `PUT /user/rule-profile`. The draft is local state; `ruleProfile.ts` holds the pure logic.
 */
import { Button, Description, FieldError, Input, Label, NumberField, TextField } from '@heroui/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, ExternalLink, Lock, Plus, Trash2 } from 'lucide-react'
import { useId, useMemo, useRef, useState } from 'react'
import { FormError } from '@/components/FormError'
import { confirm } from '@/components/feedback/confirm'
import { useLocale, useT } from '@/i18n'
import { isApiError } from '@/lib/api/client'
import { qk } from '@/lib/api/queries'
import type { PolicyGroup, RuleProfile, RulesetCatalog, RulesetCategory } from '@/lib/api/types'
import { saveRuleProfile } from '@/lib/api/user'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { OptionSelect, SectionTitle, Toggle, type Option } from './controls'
import {
  addGroup,
  applyTemplate,
  DNS_MODES,
  effectiveSource,
  errorKey,
  finalOptions,
  findTemplate,
  GROUP_TYPES,
  groupCategories,
  isEnabled,
  localized,
  moveGroup,
  needsProbe,
  normalizeProfile,
  PROXY_GROUP,
  removeGroup,
  renameError,
  renameGroup,
  sameProfile,
  setCategoryTarget,
  SITE_MIRROR,
  sortedSources,
  sourceCoverage,
  targetOptions,
  toggleCategory,
  updateGroup,
  validateProfile,
} from './ruleProfile'

export interface RuleBuilderProps {
  catalog: RulesetCatalog
  saved: RuleProfile
  /** Admin disabled customisation (`allow_user_rule_profile=false`). */
  readOnly?: boolean
}

export function RuleBuilder({ catalog, saved, readOnly: readOnlyProp = false }: RuleBuilderProps) {
  const t = useT(['subscription', 'common'])
  const locale = useLocale()
  const queryClient = useQueryClient()
  const [draft, setDraftState] = useState<RuleProfile>(() => normalizeProfile(saved))
  const [serverErrors, setServerErrors] = useState<Record<string, string>>({})
  const [formError, setFormError] = useState<string>()
  const [forbidden, setForbidden] = useState(false)
  const sectionRef = useRef<HTMLElement>(null)
  const readOnly = readOnlyProp || forbidden

  const setDraft = (next: RuleProfile) => {
    setDraftState(next)
    if (Object.keys(serverErrors).length > 0) setServerErrors({})
    setFormError(undefined)
  }

  const issues = useMemo(() => validateProfile(draft), [draft])
  const dirty = !sameProfile(draft, saved)
  const template = findTemplate(catalog, draft.template)
  const errorFor = (key: string) => {
    const issue = issues[key]
    return issue ? t(`issues.${issue}`) : serverErrors[key]
  }

  const save = useMutation({
    mutationFn: (profile: RuleProfile) => saveRuleProfile(normalizeProfile(profile)),
    onSuccess: async (result) => {
      queryClient.setQueryData(qk.user.ruleProfile(), result)
      setDraftState(normalizeProfile(result))
      toast.success(t('builder.saved'))
      await queryClient.invalidateQueries({ queryKey: [...qk.user.subscription(), 'preview'] })
    },
    onError: (error) => {
      if (isApiError(error) && error.status === 403) {
        setForbidden(true)
        setFormError(t('builder.readOnly'))
        return
      }
      if (isApiError(error) && Object.keys(error.errors).length > 0) {
        const mapped: Record<string, string> = {}
        for (const [key, messages] of Object.entries(error.errors))
          if (messages[0]) mapped[errorKey(key)] = messages[0]
        setServerErrors(mapped)
        setFormError(t('builder.fixErrors'))
        requestAnimationFrame(() => {
          const el = sectionRef.current?.querySelector<HTMLElement>(
            '[data-invalid="true"], [aria-invalid="true"]',
          )
          // jsdom has no scrollIntoView; browsers always do.
          el?.scrollIntoView?.({ block: 'center', behavior: 'smooth' })
        })
        return
      }
      setFormError(errorMessage(error))
    },
  })

  const chooseTemplate = async (id: string) => {
    const next = findTemplate(catalog, id)
    if (!next || readOnly) return
    const pristine = template ? sameProfile(draft, applyTemplate(template, draft)) : true
    if (!pristine && id !== draft.template) {
      const ok = await confirm({
        title: t('builder.switchTemplateTitle'),
        description: t('builder.switchTemplateHint'),
        confirmLabel: t('builder.switchTemplateConfirm'),
      })
      if (!ok) return
    }
    setDraft(applyTemplate(next, draft))
  }

  const resetToTemplate = async () => {
    if (!template) return
    const ok = await confirm({
      title: t('builder.resetTitle', { name: localized(template.name, locale, template.id) }),
      description: t('builder.resetHint'),
      confirmLabel: t('builder.reset'),
    })
    if (ok) setDraft(applyTemplate(template, draft))
  }

  const targets: Option[] = targetOptions(draft).map((value) => ({ value, label: targetLabel(value, t) }))
  const enabledCount = catalog.categories.filter((c) => isEnabled(draft, c.id)).length
  const blocking = Object.keys(issues).length > 0

  return (
    <section ref={sectionRef} className="surface-card flex flex-col gap-8 p-6" aria-labelledby="rules-title">
      <div className="flex flex-col gap-1">
        <h2 id="rules-title" className="text-title3 text-label">
          {t('builder.title')}
        </h2>
        <p className="text-subhead text-label-secondary">{t('builder.subtitle')}</p>
      </div>

      {readOnly ? (
        <div className="flex items-start gap-3 rounded-[12px] bg-fill-quaternary px-4 py-3" role="note">
          <Lock className="mt-0.5 size-4 shrink-0 text-label-secondary" aria-hidden="true" />
          <p className="text-subhead text-label">{t('builder.readOnly')}</p>
        </div>
      ) : null}

      {/* ------------------------------------------------------------ templates */}
      <fieldset className="flex flex-col gap-3" disabled={readOnly}>
        <legend className="contents">
          <SectionTitle id="tpl-title" title={t('builder.templates')} hint={t('builder.templatesHint')} />
        </legend>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {catalog.templates.map((tpl) => {
            const selected = tpl.id === draft.template
            return (
              <label
                key={tpl.id}
                className={cx(
                  'flex cursor-pointer flex-col gap-1 rounded-[14px] border p-4 transition-colors',
                  'has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-focus',
                  selected ? 'border-tint bg-accent-soft' : 'border-hairline hover:bg-fill-quaternary',
                  readOnly && 'cursor-default opacity-70',
                )}
              >
                <span className="flex items-center gap-2">
                  <input
                    type="radio"
                    name="rule-template"
                    value={tpl.id}
                    checked={selected}
                    onChange={() => void chooseTemplate(tpl.id)}
                    className="size-4 accent-tint"
                  />
                  <span className="text-subhead font-semibold text-label">
                    {localized(tpl.name, locale, tpl.id)}
                  </span>
                </span>
                <span className="text-footnote text-label-secondary">
                  {localized(tpl.description, locale)}
                </span>
                <span className="mt-auto pt-1 text-caption text-label-secondary">
                  {t('builder.templateStats', {
                    categories: Object.keys(tpl.categories ?? {}).length,
                    groups: tpl.groups.length,
                  })}
                </span>
              </label>
            )
          })}
        </div>
      </fieldset>

      {/* ------------------------------------------------------------ sources */}
      <fieldset className="flex flex-col gap-3" disabled={readOnly}>
        <legend className="contents">
          <SectionTitle id="src-title" title={t('builder.source')} hint={t('builder.sourceHint')} />
        </legend>
        <div className="grid gap-3 md:grid-cols-2">
          {sortedSources(catalog).map((id) => {
            const source = catalog.sources[id]
            if (!source) return null
            const selected = draft.source === id
            return (
              <div
                key={id}
                className={cx(
                  'flex flex-col gap-2 rounded-[14px] border p-4 transition-colors',
                  'has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-focus',
                  selected ? 'border-tint bg-accent-soft' : 'border-hairline',
                )}
              >
                <label className={cx('flex flex-col gap-1', !readOnly && 'cursor-pointer')}>
                  <span className="flex items-center gap-2">
                    <input
                      type="radio"
                      name="rule-source"
                      value={id}
                      checked={selected}
                      onChange={() => setDraft({ ...draft, source: id })}
                      className="size-4 accent-tint"
                    />
                    <span className="min-w-0 truncate text-subhead font-semibold text-label">
                      {source.name}
                    </span>
                  </span>
                  <span className="text-footnote text-label-secondary">
                    {localized(source.description, locale)}
                  </span>
                </label>
                <div className="flex flex-wrap items-center justify-between gap-2 text-caption text-label-secondary">
                  <span>
                    {t('builder.coverage', {
                      count: sourceCoverage(catalog, id),
                      total: catalog.categories.length,
                    })}
                  </span>
                  {source.homepage ? (
                    <a
                      href={source.homepage}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 text-label-blue hover:underline"
                    >
                      {t('builder.homepage')}
                      <ExternalLink className="size-3" aria-hidden="true" />
                      <span className="sr-only">{source.name}</span>
                    </a>
                  ) : null}
                </div>
              </div>
            )
          })}
        </div>
        {errorFor('source') ? <p className="text-footnote text-label-red">{errorFor('source')}</p> : null}
      </fieldset>

      {/* ------------------------------------------------------------ categories */}
      <div className="flex flex-col gap-4" role="group" aria-labelledby="cat-title">
        <SectionTitle
          id="cat-title"
          title={t('builder.categories')}
          hint={t('builder.categoriesHint')}
          end={
            <span className="text-footnote text-label-secondary">
              {t('builder.enabledCount', { count: enabledCount, total: catalog.categories.length })}
            </span>
          }
        />
        {groupCategories(catalog.categories).map(({ band, items }) => (
          <div key={band} className="flex flex-col gap-2">
            <h4 className="px-1 text-footnote font-semibold uppercase tracking-wide text-label-secondary">
              {t(`bands.${band}`)}
            </h4>
            <ul className="inset-group bg-fill-quaternary!">
              {items.map((category) => (
                <CategoryRow
                  key={category.id}
                  category={category}
                  catalog={catalog}
                  profile={draft}
                  targets={targets}
                  readOnly={readOnly}
                  error={errorFor(`categories.${category.id}`)}
                  onToggle={(on) => setDraft(toggleCategory(draft, category, on))}
                  onTarget={(target) => setDraft(setCategoryTarget(draft, category.id, target))}
                />
              ))}
            </ul>
          </div>
        ))}
      </div>

      {/* ------------------------------------------------------------ policy groups */}
      <div className="flex flex-col gap-3" role="group" aria-labelledby="grp-title">
        <SectionTitle
          id="grp-title"
          title={t('builder.groups')}
          hint={t('builder.groupsHint')}
          end={
            readOnly ? null : (
              <Button
                size="sm"
                variant="secondary"
                onPress={() => setDraft(addGroup(draft, nextName(draft, t)))}
              >
                <Plus className="size-4" aria-hidden="true" />
                {t('builder.addGroup')}
              </Button>
            )
          }
        />
        {errorFor('groups') ? <p className="text-footnote text-label-red">{errorFor('groups')}</p> : null}
        <ol className="flex flex-col gap-3">
          {draft.groups.map((group, index) => (
            <GroupEditor
              key={index}
              group={group}
              index={index}
              count={draft.groups.length}
              profile={draft}
              readOnly={readOnly}
              errorFor={errorFor}
              onChange={setDraft}
            />
          ))}
        </ol>
      </div>

      {/* ------------------------------------------------------------ other settings */}
      <div className="flex flex-col gap-4" role="group" aria-labelledby="misc-title">
        <SectionTitle id="misc-title" title={t('builder.other')} />
        <div className="grid gap-4 md:grid-cols-3">
          <OptionSelect
            label={t('builder.final')}
            value={draft.final}
            options={finalOptions(draft).map((value) => ({ value, label: targetLabel(value, t) }))}
            onChange={(final) => setDraft({ ...draft, final })}
            isDisabled={readOnly}
            error={errorFor('final')}
          />
          <OptionSelect
            label={t('builder.mirror')}
            value={draft.mirror}
            options={mirrorOptions(catalog, locale, t)}
            onChange={(mirror) => setDraft({ ...draft, mirror })}
            isDisabled={readOnly}
            error={errorFor('mirror')}
          />
          <OptionSelect
            label={t('builder.dns')}
            value={draft.dns}
            options={DNS_MODES.map((value) => ({ value, label: t(`dns.${value}`) }))}
            onChange={(dns) => setDraft({ ...draft, dns: dns as RuleProfile['dns'] })}
            isDisabled={readOnly}
            error={errorFor('dns')}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Toggle
            isSelected={draft.geoip_cn_direct}
            onChange={(geoip_cn_direct) => setDraft({ ...draft, geoip_cn_direct })}
            isDisabled={readOnly}
          >
            {t('builder.geoip')}
          </Toggle>
          <p className="pl-1 text-footnote text-label-secondary">{t('builder.geoipHint')}</p>
        </div>
      </div>

      {/* ------------------------------------------------------------ save bar */}
      {readOnly ? null : (
        <div className="glass glass-border-t sticky bottom-20 z-20 -mx-6 -mb-6 flex flex-wrap items-center justify-between gap-3 rounded-b-card px-6 py-4 md:bottom-0">
          <div className="min-w-0 flex-1">
            <FormError message={formError} />
            {formError ? null : (
              <p className="text-footnote text-label-secondary" aria-live="polite">
                {blocking ? t('builder.fixErrors') : dirty ? t('builder.unsaved') : t('builder.upToDate')}
              </p>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {template ? (
              <Button size="sm" variant="ghost" onPress={() => void resetToTemplate()}>
                {t('builder.reset')}
              </Button>
            ) : null}
            <Button
              size="sm"
              variant="secondary"
              isDisabled={!dirty || save.isPending}
              onPress={() => setDraft(normalizeProfile(saved))}
            >
              {t('builder.discard')}
            </Button>
            <Button
              size="sm"
              isDisabled={!dirty || blocking}
              isPending={save.isPending}
              onPress={() => save.mutate(draft)}
            >
              {t('builder.save')}
            </Button>
          </div>
        </div>
      )}
    </section>
  )
}

type T = (key: string, params?: Record<string, string | number>) => string

function targetLabel(value: string, t: T): string {
  if (value === 'DIRECT') return t('targets.direct')
  if (value === 'REJECT') return t('targets.reject')
  return value
}

function nextName(profile: RuleProfile, t: T): string {
  const base = t('builder.newGroup')
  const taken = new Set(profile.groups.map((g) => g.name.toLowerCase()))
  for (let i = 1; ; i++) {
    const name = i === 1 ? base : `${base} ${i}`
    if (!taken.has(name.toLowerCase())) return name
  }
}

function mirrorOptions(catalog: RulesetCatalog, locale: string, t: T): Option[] {
  const mirrors = (catalog.mirrors ?? []).map((m) => ({
    value: m.prefix,
    label: m.prefix ? localized(m.name, locale, m.prefix) : t('mirrors.direct'),
  }))
  if (!mirrors.some((m) => m.value === '')) mirrors.unshift({ value: '', label: t('mirrors.direct') })
  return [...mirrors, { value: SITE_MIRROR, label: t('mirrors.site') }]
}

function CategoryRow({
  category,
  catalog,
  profile,
  targets,
  readOnly,
  error,
  onToggle,
  onTarget,
}: {
  category: RulesetCategory
  catalog: RulesetCatalog
  profile: RuleProfile
  targets: Option[]
  readOnly: boolean
  error: string | undefined
  onToggle: (on: boolean) => void
  onTarget: (target: string) => void
}) {
  const t = useT(['subscription', 'common'])
  const locale = useLocale()
  const name = localized(category.name, locale, category.id)
  const enabled = isEnabled(profile, category.id)
  const target = profile.categories[category.id]
  const served = effectiveSource(catalog, category, profile.source)
  const fallback = served && served !== profile.source ? (catalog.sources[served]?.name ?? served) : null
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-2.5">
      <Toggle isSelected={enabled} onChange={onToggle} isDisabled={readOnly} className="min-w-0 flex-1">
        <span className="flex min-w-0 flex-col">
          <span className="text-subhead text-label">{name}</span>
          {fallback ? (
            <span className="text-caption text-label-secondary">
              {t('builder.fallbackSource', { source: fallback })}
            </span>
          ) : null}
        </span>
      </Toggle>
      {enabled && typeof target === 'string' ? (
        <OptionSelect
          hideLabel
          label={t('builder.targetFor', { name })}
          value={target}
          options={targets}
          onChange={onTarget}
          isDisabled={readOnly}
          error={error}
          variant="secondary"
          className="w-40 shrink-0"
        />
      ) : null}
    </li>
  )
}

function GroupEditor({
  group,
  index,
  count,
  profile,
  readOnly,
  errorFor,
  onChange,
}: {
  group: PolicyGroup
  index: number
  count: number
  profile: RuleProfile
  readOnly: boolean
  errorFor: (key: string) => string | undefined
  onChange: (next: RuleProfile) => void
}) {
  const t = useT(['subscription', 'common'])
  const id = useId()
  const [name, setName] = useState(group.name)
  const [nameError, setNameError] = useState<string>()
  // Resync the local input when the group at this index changes (move / remove / template switch).
  const [synced, setSynced] = useState(group.name)
  if (synced !== group.name) {
    setSynced(group.name)
    setName(group.name)
    setNameError(undefined)
  }
  const isProxy = group.name === PROXY_GROUP
  const probe = needsProbe(group.type)

  const commitName = () => {
    if (name.trim() === group.name) return setName(group.name)
    const problem = renameError(profile, index, name)
    if (problem) {
      setNameError(t(`issues.groupName.${problem}`))
      setName(group.name)
      return
    }
    onChange(renameGroup(profile, index, name))
  }

  return (
    <li className="flex flex-col gap-3 rounded-[14px] bg-fill-quaternary p-4" aria-labelledby={`${id}-name`}>
      <div className="flex flex-wrap items-start gap-3">
        <TextField
          value={name}
          onChange={(value) => {
            setName(value)
            setNameError(undefined)
          }}
          onBlur={commitName}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              commitName()
            }
          }}
          isReadOnly={readOnly || isProxy}
          isInvalid={Boolean(nameError ?? errorFor(`groups.${index}.name`))}
          validationBehavior="aria"
          variant="secondary"
          className="min-w-40 flex-1"
        >
          <Label id={`${id}-name`}>{t('builder.groupName')}</Label>
          <Input autoComplete="off" spellCheck={false} />
          {isProxy ? <Description>{t('builder.proxyHint')}</Description> : null}
          <FieldError>{nameError ?? errorFor(`groups.${index}.name`)}</FieldError>
        </TextField>
        <OptionSelect
          label={t('builder.groupType')}
          value={group.type}
          options={GROUP_TYPES.map((value) => ({ value, label: t(`groupTypes.${value}`) }))}
          onChange={(type) => onChange(updateGroup(profile, index, { type: type as PolicyGroup['type'] }))}
          isDisabled={readOnly}
          variant="secondary"
          className="w-44"
        />
        {readOnly ? null : (
          <div className="flex items-center gap-1 self-end pb-0.5">
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={t('builder.moveUp', { name: group.name })}
              isDisabled={index === 0}
              onPress={() => onChange(moveGroup(profile, index, -1))}
            >
              <ArrowUp className="size-4" aria-hidden="true" />
            </Button>
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={t('builder.moveDown', { name: group.name })}
              isDisabled={index === count - 1}
              onPress={() => onChange(moveGroup(profile, index, 1))}
            >
              <ArrowDown className="size-4" aria-hidden="true" />
            </Button>
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={t('builder.removeGroup', { name: group.name })}
              isDisabled={isProxy}
              onPress={() => onChange(removeGroup(profile, index))}
            >
              <Trash2 className="size-4" aria-hidden="true" />
            </Button>
          </div>
        )}
      </div>
      <TextField
        value={group.include}
        onChange={(include) => onChange(updateGroup(profile, index, { include }))}
        isReadOnly={readOnly}
        isInvalid={Boolean(errorFor(`groups.${index}.include`))}
        validationBehavior="aria"
        variant="secondary"
        fullWidth
      >
        <Label>{t('builder.include')}</Label>
        <Input autoComplete="off" spellCheck={false} className="font-mono text-footnote" />
        {index === 0 ? <Description>{t('builder.includeHint')}</Description> : null}
        <FieldError>{errorFor(`groups.${index}.include`)}</FieldError>
      </TextField>
      {probe ? (
        <div className="grid gap-3 sm:grid-cols-[1fr_12rem]">
          <TextField
            value={group.url ?? ''}
            onChange={(url) => onChange(updateGroup(profile, index, { url }))}
            isReadOnly={readOnly}
            isInvalid={Boolean(errorFor(`groups.${index}.url`))}
            validationBehavior="aria"
            variant="secondary"
            type="url"
          >
            <Label>{t('builder.testUrl')}</Label>
            <Input autoComplete="off" spellCheck={false} className="font-mono text-footnote" />
            <FieldError>{errorFor(`groups.${index}.url`)}</FieldError>
          </TextField>
          <NumberField
            value={group.interval ?? Number.NaN}
            onChange={(interval) =>
              onChange(updateGroup(profile, index, { interval: Number.isNaN(interval) ? 0 : interval }))
            }
            minValue={10}
            maxValue={86_400}
            step={10}
            isReadOnly={readOnly}
            isInvalid={Boolean(errorFor(`groups.${index}.interval`))}
            validationBehavior="aria"
            variant="secondary"
          >
            <Label>{t('builder.interval')}</Label>
            <NumberField.Group>
              <NumberField.DecrementButton />
              <NumberField.Input className="w-full" />
              <NumberField.IncrementButton />
            </NumberField.Group>
            <FieldError>{errorFor(`groups.${index}.interval`)}</FieldError>
          </NumberField>
        </div>
      ) : null}
    </li>
  )
}
