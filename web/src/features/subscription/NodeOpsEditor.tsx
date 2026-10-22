/**
 * Sub-Store-style node operations for one subscription profile (`RuleProfile.nodes`): type / region chips,
 * include / exclude regex lists, ordered rename rules, prefix / suffix, flag emoji, sort, dedupe and limit, with
 * a live preview computed from `GET /user/nodes` (`nodeOps.ts`). Saved through the page's profile endpoint.
 */
import {
  Button,
  Chip,
  Description,
  FieldError,
  Input,
  Label,
  NumberField,
  Tag,
  TagGroup,
  TextField,
} from '@heroui/react'
import { useMutation } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Lock, Plus, Trash2 } from 'lucide-react'
import { type ReactNode, useEffect, useId, useMemo, useRef, useState } from 'react'
import { FormError } from '@/components/FormError'
import { confirm } from '@/components/feedback/confirm'
import { useT } from '@/i18n'
import { isApiError } from '@/lib/api/client'
import { NODE_TYPES } from '@/lib/api/types'
import type { NodeOperations, RuleProfile, UserNode } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { ChipSelect, SectionTitle, Segmented, Toggle } from './controls'
import {
  applyNodeOps,
  compilePattern,
  CORE_REGIONS,
  countMatches,
  LIMITS,
  EMOJI_MODES,
  emptyNodeOps,
  moveItem,
  type NodeOpsIssue,
  type NodeOpsPreview,
  normalizeNodeOps,
  REGIONS,
  regionCounts,
  regionFlag,
  sameNodeOps,
  SORT_MODES,
  TYPE_LABELS,
  validateNodeOps,
} from './nodeOps'
import { errorKey } from './ruleProfile'

type T = (key: string, params?: Record<string, string | number>) => string
/** GB is shown as "UK" (the contract's and most node names' spelling); the payload uses the ISO code. */
const regionLabel = (code: string) => (code === 'GB' ? 'UK' : code)
const NO_NODES: readonly UserNode[] = []

export interface NodeOpsEditorProps {
  /** `RuleProfile.nodes` as stored (normalised here). */
  saved: NodeOperations | undefined
  /** `GET /user/nodes` (preview source); `undefined` while loading. */
  nodes: readonly UserNode[] | undefined
  readOnly?: boolean
  /** Persists the operations (the page merges them into the full profile). */
  save: (nodes: NodeOperations) => Promise<RuleProfile>
  onDirtyChange?: (dirty: boolean) => void
}

export function NodeOpsEditor({
  saved,
  nodes,
  readOnly: readOnlyProp = false,
  save,
  onDirtyChange,
}: NodeOpsEditorProps) {
  const t = useT(['subscription', 'common'])
  const baseline = useMemo(() => normalizeNodeOps(saved), [saved])
  const [draft, setDraftState] = useState<NodeOperations>(baseline)
  const [serverErrors, setServerErrors] = useState<Record<string, string>>({})
  const [formError, setFormError] = useState<string>()
  const [forbidden, setForbidden] = useState(false)
  const sectionRef = useRef<HTMLElement>(null)
  const readOnly = readOnlyProp || forbidden

  const setDraft = (next: NodeOperations) => {
    setDraftState(next)
    if (Object.keys(serverErrors).length > 0) setServerErrors({})
    setFormError(undefined)
  }
  const patch = (p: Partial<NodeOperations>) => setDraft({ ...draft, ...p })

  const issues = useMemo(() => validateNodeOps(draft), [draft])
  const patternsFull = draft.include.length + draft.exclude.length >= LIMITS.patterns
  const preview = useMemo(() => (nodes ? applyNodeOps(nodes, draft) : null), [nodes, draft])
  const dirty = !sameNodeOps(draft, baseline)
  const blocking = Object.keys(issues).length > 0
  useEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange])

  const errorFor = (key: string) => {
    const issue: NodeOpsIssue | undefined = issues[key]
    return issue ? t(`nodeOps.issues.${issue}`) : serverErrors[key]
  }

  const mutation = useMutation({
    mutationFn: (ops: NodeOperations) => save(normalizeNodeOps(ops)),
    onSuccess: (result) => {
      setDraftState(normalizeNodeOps(result.nodes ?? draft))
      toast.success(t('nodeOps.saved'))
    },
    onError: (error) => {
      if (isApiError(error) && error.status === 403) {
        setForbidden(true)
        setFormError(t('builder.readOnly'))
        return
      }
      if (isApiError(error) && Object.keys(error.errors).length > 0) {
        const mapped: Record<string, string> = {}
        const other: string[] = []
        for (const [key, messages] of Object.entries(error.errors)) {
          const path = errorKey(key)
          if (path.startsWith('nodes.') && messages[0]) mapped[path.slice('nodes.'.length)] = messages[0]
          else if (messages[0]) other.push(messages[0])
        }
        setServerErrors(mapped)
        setFormError(other[0] ?? t('builder.fixErrors'))
        requestAnimationFrame(() => {
          sectionRef.current
            ?.querySelector<HTMLElement>('[data-invalid="true"], [aria-invalid="true"]')
            ?.scrollIntoView?.({ block: 'center', behavior: 'smooth' })
        })
        return
      }
      setFormError(errorMessage(error))
    },
  })

  const clearAll = async () => {
    const ok = await confirm({
      title: t('nodeOps.clearTitle'),
      description: t('nodeOps.clearHint'),
      confirmLabel: t('nodeOps.clearConfirm'),
    })
    if (ok) setDraft(emptyNodeOps())
  }

  const list = nodes ?? NO_NODES
  const typeCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const n of list) counts.set(n.type, (counts.get(n.type) ?? 0) + 1)
    return counts
  }, [list])
  const regions = useMemo(() => regionCounts(list), [list])
  const regionOptions = REGIONS.filter(
    (r) => CORE_REGIONS.includes(r) || regions.has(r) || draft.regions.includes(r),
  ).map((r) => ({
    value: r,
    textValue: regions.get(r)
      ? `${regionLabel(r)} · ${t('nodeOps.regionCount', { count: regions.get(r) ?? 0 })}`
      : regionLabel(r),
    label: (
      <span className="inline-flex items-center gap-1">
        <span aria-hidden="true">{regionFlag(r)}</span>
        {regionLabel(r)}
        {regions.get(r) ? <span className="text-label-secondary tabular">{regions.get(r)}</span> : null}
      </span>
    ),
  }))
  const typeOptions = NODE_TYPES.map((type) => ({
    value: type,
    textValue: TYPE_LABELS[type] ?? type,
    label: (
      <span className="inline-flex items-center gap-1">
        {TYPE_LABELS[type] ?? type}
        {typeCounts.get(type) ? (
          <span className="text-label-secondary tabular">{typeCounts.get(type)}</span>
        ) : null}
      </span>
    ),
  }))

  return (
    <section
      ref={sectionRef}
      className="surface-card flex flex-col gap-8 p-6"
      aria-labelledby="nodeops-title"
    >
      <div className="flex flex-col gap-1">
        <h2 id="nodeops-title" className="text-title3 text-label">
          {t('nodeOps.title')}
        </h2>
        <p className="text-subhead text-label-secondary">{t('nodeOps.subtitle')}</p>
        <p className="text-footnote text-label-secondary">{t('nodeOps.pipeline')}</p>
      </div>

      {readOnly ? (
        <div className="flex items-start gap-3 rounded-[12px] bg-fill-quaternary px-4 py-3" role="note">
          <Lock className="mt-0.5 size-4 shrink-0 text-label-secondary" aria-hidden="true" />
          <p className="text-subhead text-label">{t('builder.readOnly')}</p>
        </div>
      ) : null}

      <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_minmax(16rem,22rem)]">
        <div className="flex min-w-0 flex-col gap-8">
          {/* ------------------------------------------------------------ filter */}
          <div className="flex flex-col gap-5" role="group" aria-labelledby="nodeops-filter">
            <SectionTitle id="nodeops-filter" title={t('nodeOps.filter')} />
            <ChipSelect
              label={t('nodeOps.types')}
              hint={t('nodeOps.typesHint')}
              value={draft.types}
              options={typeOptions}
              onChange={(types) => patch({ types })}
              isDisabled={readOnly}
            />
            <ChipSelect
              label={t('nodeOps.regions')}
              hint={t('nodeOps.regionsHint')}
              value={draft.regions}
              options={regionOptions}
              onChange={(regions) => patch({ regions })}
              isDisabled={readOnly}
            />
            <PatternList
              label={t('nodeOps.include')}
              hint={t('nodeOps.includeHint')}
              values={draft.include}
              onChange={(include) => patch({ include })}
              nodes={list}
              readOnly={readOnly}
              full={patternsFull}
              errorFor={(i) => errorFor(`include.${i}`)}
            />
            <PatternList
              label={t('nodeOps.exclude')}
              hint={t('nodeOps.excludeHint')}
              values={draft.exclude}
              onChange={(exclude) => patch({ exclude })}
              nodes={list}
              readOnly={readOnly}
              full={patternsFull}
              errorFor={(i) => errorFor(`exclude.${i}`)}
            />
            {errorFor('exclude') ? (
              <p className="text-footnote text-label-red">{errorFor('exclude')}</p>
            ) : patternsFull ? (
              <p className="text-footnote text-label-secondary">
                {t('nodeOps.patternsFull', { max: LIMITS.patterns })}
              </p>
            ) : null}
          </div>

          {/* ------------------------------------------------------------ rename */}
          <div className="flex flex-col gap-3" role="group" aria-labelledby="nodeops-rename">
            <SectionTitle
              id="nodeops-rename"
              title={t('nodeOps.rename')}
              hint={t('nodeOps.renameHint')}
              end={
                readOnly ? null : (
                  <Button
                    size="sm"
                    variant="secondary"
                    isDisabled={draft.rename.length >= LIMITS.renameRules}
                    onPress={() => patch({ rename: [...draft.rename, { match: '', replace: '' }] })}
                  >
                    <Plus className="size-4" aria-hidden="true" />
                    {t('nodeOps.addRule')}
                  </Button>
                )
              }
            />
            {draft.rename.length === 0 ? (
              <p className="text-footnote text-label-secondary">{t('nodeOps.none')}</p>
            ) : (
              <ol className="flex flex-col gap-3">
                {draft.rename.map((rule, index) => (
                  <RenameRow
                    key={index}
                    index={index}
                    count={draft.rename.length}
                    match={rule.match}
                    replace={rule.replace}
                    nodes={list}
                    readOnly={readOnly}
                    error={errorFor(`rename.${index}.match`)}
                    replaceError={errorFor(`rename.${index}.replace`)}
                    onChange={(next) =>
                      patch({ rename: draft.rename.map((r, i) => (i === index ? { ...r, ...next } : r)) })
                    }
                    onMove={(delta) => patch({ rename: moveItem(draft.rename, index, delta) })}
                    onRemove={() => patch({ rename: draft.rename.filter((_, i) => i !== index) })}
                  />
                ))}
              </ol>
            )}
          </div>

          {/* ------------------------------------------------------------ format */}
          <div className="flex flex-col gap-5" role="group" aria-labelledby="nodeops-format">
            <SectionTitle id="nodeops-format" title={t('nodeOps.format')} />
            <div className="grid gap-4 sm:grid-cols-2">
              <TextField
                value={draft.prefix}
                onChange={(prefix) => patch({ prefix })}
                isReadOnly={readOnly}
                isInvalid={Boolean(errorFor('prefix'))}
                validationBehavior="aria"
                variant="secondary"
              >
                <Label>{t('nodeOps.prefix')}</Label>
                <Input autoComplete="off" spellCheck={false} maxLength={LIMITS.affixChars} />
                <FieldError>{errorFor('prefix')}</FieldError>
              </TextField>
              <TextField
                value={draft.suffix}
                onChange={(suffix) => patch({ suffix })}
                isReadOnly={readOnly}
                isInvalid={Boolean(errorFor('suffix'))}
                validationBehavior="aria"
                variant="secondary"
              >
                <Label>{t('nodeOps.suffix')}</Label>
                <Input autoComplete="off" spellCheck={false} maxLength={LIMITS.affixChars} />
                <FieldError>{errorFor('suffix')}</FieldError>
              </TextField>
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <Segmented
                label={t('nodeOps.emoji')}
                value={draft.emoji}
                options={EMOJI_MODES.map((value) => ({ value, label: t(`nodeOps.emojiModes.${value}`) }))}
                onChange={(emoji) => patch({ emoji })}
                isDisabled={readOnly}
              />
              <Segmented
                label={t('nodeOps.sort')}
                value={draft.sort}
                options={SORT_MODES.map((value) => ({ value, label: t(`nodeOps.sortModes.${value}`) }))}
                onChange={(sort) => patch({ sort })}
                isDisabled={readOnly}
              />
            </div>
            <div className="grid items-start gap-4 sm:grid-cols-2">
              <div className="flex flex-col gap-1">
                <Toggle
                  isSelected={draft.dedupe}
                  onChange={(dedupe) => patch({ dedupe })}
                  isDisabled={readOnly}
                >
                  {t('nodeOps.dedupe')}
                </Toggle>
                <p className="pl-1 text-footnote text-label-secondary">{t('nodeOps.dedupeHint')}</p>
              </div>
              <NumberField
                value={draft.limit}
                onChange={(limit) => patch({ limit: Number.isNaN(limit) ? 0 : limit })}
                minValue={0}
                maxValue={LIMITS.maxNodes}
                step={1}
                isReadOnly={readOnly}
                isInvalid={Boolean(errorFor('limit'))}
                validationBehavior="aria"
                variant="secondary"
                formatOptions={{ maximumFractionDigits: 0, useGrouping: false }}
              >
                <Label>{t('nodeOps.limit')}</Label>
                <NumberField.Group>
                  <NumberField.DecrementButton />
                  <NumberField.Input className="w-full" />
                  <NumberField.IncrementButton />
                </NumberField.Group>
                {errorFor('limit') ? null : <Description>{t('nodeOps.limitHint')}</Description>}
                <FieldError>{errorFor('limit')}</FieldError>
              </NumberField>
            </div>
          </div>
        </div>

        <NodePreview preview={preview} total={nodes?.length ?? 0} t={t} />
      </div>

      {readOnly ? null : (
        <div className="glass glass-border-t sticky bottom-20 z-20 -mx-6 -mb-6 flex flex-wrap items-center justify-between gap-3 rounded-b-card px-6 py-4 md:bottom-0">
          <div className="min-w-0 flex-1">
            <FormError message={formError} />
            {formError ? null : (
              <p className="text-footnote text-label-secondary" aria-live="polite">
                {blocking ? t('builder.fixErrors') : dirty ? t('builder.unsaved') : t('builder.upToDate')}
                {preview && nodes && nodes.length > 0 ? (
                  <span className="xl:hidden">
                    {' · '}
                    {t('nodeOps.previewCount', { kept: preview.kept.length, total: nodes.length })}
                  </span>
                ) : null}
              </p>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" variant="ghost" onPress={() => void clearAll()}>
              {t('nodeOps.clear')}
            </Button>
            <Button
              size="sm"
              variant="secondary"
              isDisabled={!dirty || mutation.isPending}
              onPress={() => setDraft(baseline)}
            >
              {t('builder.discard')}
            </Button>
            <Button
              size="sm"
              isDisabled={!dirty || blocking}
              isPending={mutation.isPending}
              onPress={() => mutation.mutate(draft)}
            >
              {t('nodeOps.save')}
            </Button>
          </div>
        </div>
      )}
    </section>
  )
}

/** Regex list: removable tags + an input that validates (RE2-compatible) and shows its match count live. */
function PatternList({
  label,
  hint,
  values,
  onChange,
  nodes,
  readOnly,
  full,
  errorFor,
}: {
  label: string
  hint: ReactNode
  values: readonly string[]
  onChange: (values: string[]) => void
  nodes: readonly UserNode[]
  readOnly: boolean
  /** The shared include + exclude budget (32) is used up. */
  full: boolean
  errorFor: (index: number) => string | undefined
}) {
  const t = useT(['subscription', 'common'])
  const id = useId()
  const [draft, setDraft] = useState('')
  const [touched, setTouched] = useState(false)
  const compiled = draft ? compilePattern(draft) : null
  const draftError =
    touched && compiled && !compiled.ok ? t(`nodeOps.issues.regex.${compiled.issue}`) : undefined
  const live = compiled?.ok ? countMatches(nodes, draft) : null

  const add = () => {
    setTouched(true)
    if (!draft || !compiled?.ok || full) return
    if (!values.includes(draft)) onChange([...values, draft])
    setDraft('')
    setTouched(false)
  }
  const itemErrors = values.flatMap((value, i) => {
    const error = errorFor(i)
    return error ? [{ value, error }] : []
  })

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <div>
        <span id={`${id}-label`} className="text-subhead font-medium text-label">
          {label}
        </span>
        <p className="text-footnote text-label-secondary">{hint}</p>
      </div>
      <TagGroup
        aria-labelledby={`${id}-label`}
        onRemove={readOnly ? undefined : (keys) => onChange(values.filter((_, i) => !keys.has(String(i))))}
      >
        <TagGroup.List
          items={values.map((value, i) => ({ id: String(i), value }))}
          renderEmptyState={() => (
            <span className="text-footnote text-label-secondary">{t('nodeOps.none')}</span>
          )}
        >
          {(item) => {
            const invalid = Boolean(errorFor(Number(item.id)))
            const count = countMatches(nodes, item.value)
            return (
              <Tag
                id={item.id}
                textValue={item.value}
                className={cx('font-mono', invalid && 'text-label-red')}
              >
                {item.value}
                {count !== null ? (
                  <span className="font-sans text-label-secondary tabular" aria-hidden="true">
                    {count}
                  </span>
                ) : null}
              </Tag>
            )
          }}
        </TagGroup.List>
      </TagGroup>
      {itemErrors.length > 0 ? (
        <ul className="flex flex-col gap-0.5">
          {itemErrors.map(({ value, error }) => (
            <li key={value} className="text-footnote text-label-red">
              <span className="font-mono">{value}</span> · {error}
            </li>
          ))}
        </ul>
      ) : null}
      {readOnly ? null : (
        <div className="flex items-start gap-2">
          <TextField
            aria-label={t('nodeOps.addTo', { label })}
            value={draft}
            onChange={(value) => {
              setDraft(value)
              setTouched(false)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                add()
              }
            }}
            onBlur={() => setTouched(Boolean(draft))}
            isInvalid={Boolean(draftError)}
            validationBehavior="aria"
            variant="secondary"
            className="min-w-0 flex-1"
          >
            <Input
              placeholder={t('nodeOps.patternPlaceholder')}
              autoComplete="off"
              spellCheck={false}
              autoCapitalize="none"
              className="w-full font-mono text-footnote"
            />
            {draftError ? null : live !== null ? (
              <Description>{t('nodeOps.matches', { count: live })}</Description>
            ) : null}
            <FieldError>{draftError}</FieldError>
          </TextField>
          <Button size="md" variant="secondary" onPress={add} isDisabled={!draft || full}>
            {t('nodeOps.add')}
          </Button>
        </div>
      )}
    </div>
  )
}

function RenameRow({
  index,
  count,
  match,
  replace,
  nodes,
  readOnly,
  error,
  replaceError,
  onChange,
  onMove,
  onRemove,
}: {
  index: number
  count: number
  match: string
  replace: string
  nodes: readonly UserNode[]
  readOnly: boolean
  error: string | undefined
  replaceError: string | undefined
  onChange: (next: { match?: string; replace?: string }) => void
  onMove: (delta: -1 | 1) => void
  onRemove: () => void
}) {
  const t = useT(['subscription', 'common'])
  const id = useId()
  const n = index + 1
  const affected = match ? countMatches(nodes, match) : null
  return (
    <li className="flex flex-col gap-3 rounded-[14px] bg-fill-quaternary p-4" aria-labelledby={`${id}-title`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h4 id={`${id}-title`} className="text-subhead font-semibold text-label">
          {t('nodeOps.ruleN', { n })}
          {affected !== null ? (
            <span className="ml-2 text-footnote font-normal text-label-secondary">
              {t('nodeOps.ruleAffects', { count: affected })}
            </span>
          ) : null}
        </h4>
        {readOnly ? null : (
          <div className="flex items-center gap-1">
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={t('nodeOps.moveUp', { n })}
              isDisabled={index === 0}
              onPress={() => onMove(-1)}
            >
              <ArrowUp className="size-4" aria-hidden="true" />
            </Button>
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={t('nodeOps.moveDown', { n })}
              isDisabled={index === count - 1}
              onPress={() => onMove(1)}
            >
              <ArrowDown className="size-4" aria-hidden="true" />
            </Button>
            <Button
              isIconOnly
              size="sm"
              variant="ghost"
              aria-label={t('nodeOps.removeRule', { n })}
              onPress={onRemove}
            >
              <Trash2 className="size-4" aria-hidden="true" />
            </Button>
          </div>
        )}
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <TextField
          value={match}
          onChange={(value) => onChange({ match: value })}
          isReadOnly={readOnly}
          isInvalid={Boolean(error)}
          validationBehavior="aria"
          variant="secondary"
        >
          <Label>{t('nodeOps.match')}</Label>
          <Input
            autoComplete="off"
            spellCheck={false}
            autoCapitalize="none"
            className="font-mono text-footnote"
          />
          <FieldError>{error}</FieldError>
        </TextField>
        <TextField
          value={replace}
          onChange={(value) => onChange({ replace: value })}
          isReadOnly={readOnly}
          isInvalid={Boolean(replaceError)}
          validationBehavior="aria"
          variant="secondary"
        >
          <Label>{t('nodeOps.replace')}</Label>
          <Input
            autoComplete="off"
            spellCheck={false}
            autoCapitalize="none"
            placeholder={t('nodeOps.replacePlaceholder')}
            className="font-mono text-footnote"
          />
          <FieldError>{replaceError}</FieldError>
        </TextField>
      </div>
    </li>
  )
}

function NodePreview({
  preview,
  total,
  t,
}: {
  /** `null` while the node list loads. */
  preview: NodeOpsPreview | null
  total: number
  t: T
}) {
  return (
    <aside
      aria-labelledby="nodeops-preview-title"
      className="flex min-w-0 flex-col gap-3 self-start rounded-[14px] bg-fill-quaternary p-4 xl:sticky xl:top-20"
    >
      <div>
        <h3 id="nodeops-preview-title" className="text-headline text-label">
          {t('nodeOps.preview')}
        </h3>
        <p className="text-footnote text-label-secondary">{t('nodeOps.previewHint')}</p>
      </div>
      {!preview ? (
        <div className="h-40 animate-pulse rounded-[10px] bg-fill-tertiary" aria-busy="true" />
      ) : total === 0 ? (
        <p className="text-subhead text-label-secondary">{t('nodeOps.previewEmpty')}</p>
      ) : (
        <>
          <p className="text-subhead font-medium text-label" aria-live="polite" data-testid="nodeops-count">
            {t('nodeOps.previewCount', { kept: preview.kept.length, total })}
          </p>
          {preview.kept.length === 0 ? (
            <p className="text-footnote text-label-orange">{t('nodeOps.previewNone')}</p>
          ) : (
            <ol
              aria-label={t('nodeOps.previewList')}
              className="flex max-h-[26rem] flex-col divide-y divide-hairline overflow-auto rounded-[10px] bg-elevated"
            >
              {preview.kept.map((node) => (
                <li key={node.id} className="flex items-start justify-between gap-2 px-3 py-2">
                  <span className="min-w-0">
                    <span className="block break-words text-subhead text-label">{node.name}</span>
                    {node.name !== node.original ? (
                      <span className="block break-words text-caption text-label-secondary">
                        {t('nodeOps.renamedFrom', { name: node.original })}
                      </span>
                    ) : null}
                  </span>
                  <span className="shrink-0 text-caption text-label-secondary">
                    {TYPE_LABELS[node.type] ?? node.type}
                  </span>
                </li>
              ))}
            </ol>
          )}
          {preview.dropped.length > 0 ? (
            <details className="group">
              <summary className="cursor-pointer text-footnote text-label-secondary">
                {t('nodeOps.dropped', { count: preview.dropped.length })}
              </summary>
              <ul className="mt-2 flex flex-col gap-1.5">
                {preview.dropped.map((node) => (
                  <li key={`${node.id}-${node.reason}`} className="flex items-center justify-between gap-2">
                    <span className="min-w-0 truncate text-footnote text-label-secondary line-through">
                      {node.original}
                    </span>
                    <Chip size="sm" variant="soft">
                      {t(`nodeOps.reasons.${node.reason}`)}
                    </Chip>
                  </li>
                ))}
              </ul>
            </details>
          ) : null}
        </>
      )}
    </aside>
  )
}
