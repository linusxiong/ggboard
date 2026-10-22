import { Alert, Button, Disclosure, Spinner } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { Braces, Plus, RotateCcw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { ComboInput } from '@/features/admin/settings/inputs/ComboInput'
import {
  NumberInput,
  SelectInput,
  TextAreaInput,
  TextInput,
} from '@/features/admin/settings/inputs/Controlled'
import { useT } from '@/i18n'
import { adminApi, isApiError } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { RulesetCatalogFull, RulesetRuleRefFull, RulesetsCustom } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { formatCustom, type Issue, parseCustom, upsertCategory, upsertMirror } from './custom'

interface CustomEditorProps {
  /** Saved overrides (`GET /admin/rulesets` → `custom`). */
  custom: RulesetsCustom | null
  /** Effective catalog (for source ids and target groups in the forms). */
  catalog: RulesetCatalogFull
}

/** JSON editor for `rulesets_custom` + helper forms that write into the JSON. */
export function CustomEditor(props: CustomEditorProps) {
  // Reset the draft whenever the saved overrides change (after save / refetch).
  return <CustomEditorInner key={formatCustom(props.custom)} {...props} />
}

function CustomEditorInner({ custom, catalog }: CustomEditorProps) {
  const t = useT(['admin-settings', 'common'])
  const qc = useQueryClient()
  const saved = formatCustom(custom)
  const [text, setText] = useState(saved)
  const [serverIssues, setServerIssues] = useState<string[]>([])
  const [saving, setSaving] = useState(false)
  const parsed = useMemo(() => parseCustom(text), [text])
  const dirty = text !== saved

  const issueText = (i: Issue) => `${i.path}: ${t(`rulesets.issues.${i.code}`, { detail: i.detail ?? '' })}`

  const apply = (fn: (c: RulesetsCustom) => RulesetsCustom) => {
    if (!parsed.value) return
    setText(formatCustom(fn(parsed.value)))
    setServerIssues([])
  }

  const save = async () => {
    if (!parsed.value) return
    setSaving(true)
    setServerIssues([])
    try {
      await adminApi.saveRulesetsCustom(parsed.value)
      toast.success(t('rulesets.saved'))
      await qc.invalidateQueries({ queryKey: qk.admin.rulesets() })
    } catch (e) {
      if (isApiError(e) && Object.keys(e.errors).length)
        setServerIssues(Object.entries(e.errors).flatMap(([k, msgs]) => msgs.map((m) => `${k}: ${m}`)))
      else toast.danger(errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="surface-card flex flex-col gap-3 p-5">
        <div>
          <h3 className="text-headline text-label">{t('rulesets.customTitle')}</h3>
          <p className="mt-1 text-subhead text-label-secondary">{t('rulesets.customHint')}</p>
        </div>
        <TextAreaInput
          label={t('rulesets.customJson')}
          name="rulesets_custom"
          value={text}
          onChange={(v) => {
            setText(v)
            setServerIssues([])
          }}
          rows={16}
          monospace
          error={parsed.issues.length ? t('rulesets.invalid', { count: parsed.issues.length }) : undefined}
        />
        {parsed.issues.length || serverIssues.length ? (
          <Alert status="danger">
            <Alert.Indicator />
            <Alert.Content>
              <Alert.Title>
                {serverIssues.length ? t('rulesets.serverRejected') : t('rulesets.fixIssues')}
              </Alert.Title>
              <Alert.Description>
                <ul className="mt-1 list-disc ps-5 font-mono text-footnote">
                  {[...parsed.issues.map(issueText), ...serverIssues].slice(0, 20).map((msg, i) => (
                    <li key={i}>{msg}</li>
                  ))}
                </ul>
              </Alert.Description>
            </Alert.Content>
          </Alert>
        ) : null}
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Button
            variant="tertiary"
            isDisabled={!parsed.value}
            onPress={() => parsed.value && setText(formatCustom(parsed.value))}
          >
            <Braces className="size-4" aria-hidden />
            {t('rulesets.format')}
          </Button>
          <Button variant="tertiary" isDisabled={!dirty || saving} onPress={() => setText(saved)}>
            <RotateCcw className="size-4" aria-hidden />
            {t('rulesets.revert')}
          </Button>
          <Button isDisabled={!dirty || !parsed.value} isPending={saving} onPress={() => void save()}>
            {({ isPending }) => (
              <>
                {isPending ? <Spinner color="current" size="sm" /> : null}
                {t('rulesets.save')}
              </>
            )}
          </Button>
        </div>
      </div>

      <AddMirrorForm disabled={!parsed.value} onAdd={(m) => apply((c) => upsertMirror(c, m))} />
      <AddCategoryForm
        catalog={catalog}
        disabled={!parsed.value}
        onAdd={(cat) => apply((c) => upsertCategory(c, cat))}
      />
    </div>
  )
}

// ---------------------------------------------------------------- add mirror

function AddMirrorForm({
  disabled,
  onAdd,
}: {
  disabled: boolean
  onAdd: (m: { id: string; name: { 'zh-CN': string; en: string }; prefix: string }) => void
}) {
  const t = useT('admin-settings')
  const [id, setId] = useState('')
  const [zh, setZh] = useState('')
  const [en, setEn] = useState('')
  const [prefix, setPrefix] = useState('')
  const [errors, setErrors] = useState<Record<string, string>>({})
  const submit = () => {
    const e: Record<string, string> = {}
    if (!/^[a-z0-9][a-z0-9_-]*$/.test(id)) e.id = t('rulesets.form.idInvalid')
    let p = prefix.trim()
    if (p && !p.endsWith('/')) p += '/'
    if (!/^https?:\/\/\S+\/$/.test(p)) e.prefix = t('rulesets.form.prefixInvalid')
    setErrors(e)
    if (Object.keys(e).length) return
    onAdd({ id, name: { 'zh-CN': zh.trim() || id, en: en.trim() || zh.trim() || id }, prefix: p })
    setId('')
    setZh('')
    setEn('')
    setPrefix('')
    toast.success(t('rulesets.form.added'))
  }
  return (
    <Disclosure className="surface-card">
      <Disclosure.Heading>
        <Disclosure.Trigger className="flex w-full items-center justify-between gap-2 p-5 text-start">
          <span className="flex items-center gap-2 text-headline text-label">
            <Plus className="size-4" aria-hidden />
            {t('rulesets.form.mirrorTitle')}
          </span>
          <Disclosure.Indicator />
        </Disclosure.Trigger>
      </Disclosure.Heading>
      <Disclosure.Content>
        <form
          noValidate
          className="grid gap-4 px-5 pb-5 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <TextInput
            label={t('rulesets.form.id')}
            name="mirror_id"
            value={id}
            onChange={setId}
            monospace
            isRequired
            error={errors.id}
            description={t('rulesets.form.idHint')}
          />
          <TextInput
            label={t('rulesets.form.prefix')}
            name="mirror_prefix"
            type="url"
            value={prefix}
            onChange={setPrefix}
            placeholder="https://ghfast.top/"
            monospace
            isRequired
            error={errors.prefix}
            description={t('rulesets.form.prefixHint')}
          />
          <TextInput label={t('rulesets.form.nameZh')} name="mirror_name_zh" value={zh} onChange={setZh} />
          <TextInput label={t('rulesets.form.nameEn')} name="mirror_name_en" value={en} onChange={setEn} />
          <div className="flex justify-end sm:col-span-2">
            <Button type="submit" variant="secondary" isDisabled={disabled}>
              {t('rulesets.form.addToJson')}
            </Button>
          </div>
        </form>
      </Disclosure.Content>
    </Disclosure>
  )
}

// ---------------------------------------------------------------- add category

type RefDraft = { name: string; geosite: string; geoip: string; behavior: string }

function AddCategoryForm({
  catalog,
  disabled,
  onAdd,
}: {
  catalog: RulesetCatalogFull
  disabled: boolean
  onAdd: (c: Parameters<typeof upsertCategory>[1]) => void
}) {
  const t = useT('admin-settings')
  const sources = useMemo(
    () => Object.entries(catalog.sources).sort(([, a], [, b]) => (a.order ?? 0) - (b.order ?? 0)),
    [catalog.sources],
  )
  const targets = useMemo(() => {
    const full =
      catalog.templates.find((tp) => tp.id === 'full') ?? catalog.templates[catalog.templates.length - 1]
    return ['DIRECT', 'REJECT', ...(full?.groups.map((g) => g.name) ?? [])]
  }, [catalog.templates])
  const [id, setId] = useState('')
  const [zh, setZh] = useState('')
  const [en, setEn] = useState('')
  const [target, setTarget] = useState('Proxy')
  const [order, setOrder] = useState<number | null>(75)
  const [refs, setRefs] = useState<Record<string, RefDraft>>({})
  const [errors, setErrors] = useState<Record<string, string>>({})

  /** Which RuleRef fields a source's URL templates consume (`url_meta[*].field`). */
  const fieldsOf = (sourceId: string) => {
    const meta = catalog.sources[sourceId]?.url_meta
    const fields = new Set(Object.values(meta ?? {}).map((m) => m.field))
    if (fields.size === 0) fields.add('name')
    return fields
  }
  const setRef = (src: string, key: keyof RefDraft, value: string) =>
    setRefs((r) => ({
      ...r,
      [src]: { name: '', geosite: '', geoip: '', behavior: '', ...r[src], [key]: value },
    }))

  const submit = () => {
    const e: Record<string, string> = {}
    if (!/^[a-z0-9][a-z0-9_-]*$/.test(id)) e.id = t('rulesets.form.idInvalid')
    if (!target.trim()) e.target = t('rulesets.form.targetRequired')
    const rules: Record<string, RulesetRuleRefFull> = {}
    for (const [src, r] of Object.entries(refs)) {
      const ref: RulesetRuleRefFull = {}
      if (r.name.trim()) ref.name = r.name.trim()
      if (r.geosite.trim()) ref.geosite = r.geosite.trim()
      if (r.geoip.trim()) ref.geoip = r.geoip.trim()
      if (r.behavior && ref.name) ref.behavior = r.behavior as RulesetRuleRefFull['behavior']
      if (Object.keys(ref).length) rules[src] = ref
    }
    if (Object.keys(rules).length === 0) e.rules = t('rulesets.form.rulesRequired')
    setErrors(e)
    if (Object.keys(e).length) return
    onAdd({
      id,
      name: { 'zh-CN': zh.trim() || id, en: en.trim() || zh.trim() || id },
      default_target: target.trim(),
      order: order ?? 75,
      rules,
    })
    setId('')
    setZh('')
    setEn('')
    setRefs({})
    toast.success(t('rulesets.form.added'))
  }

  return (
    <Disclosure className="surface-card">
      <Disclosure.Heading>
        <Disclosure.Trigger className="flex w-full items-center justify-between gap-2 p-5 text-start">
          <span className="flex items-center gap-2 text-headline text-label">
            <Plus className="size-4" aria-hidden />
            {t('rulesets.form.categoryTitle')}
          </span>
          <Disclosure.Indicator />
        </Disclosure.Trigger>
      </Disclosure.Heading>
      <Disclosure.Content>
        <form
          noValidate
          className="flex flex-col gap-4 px-5 pb-5"
          onSubmit={(ev) => {
            ev.preventDefault()
            submit()
          }}
        >
          <p className="text-footnote text-label-secondary">{t('rulesets.form.categoryHint')}</p>
          <div className="grid gap-4 sm:grid-cols-2">
            <TextInput
              label={t('rulesets.form.id')}
              name="category_id"
              value={id}
              onChange={setId}
              monospace
              isRequired
              error={errors.id}
              description={t('rulesets.form.categoryIdHint')}
            />
            <ComboInput
              label={t('rulesets.form.target')}
              name="category_target"
              value={target}
              onChange={setTarget}
              options={targets}
              isRequired
              error={errors.target}
              description={t('rulesets.form.targetHint')}
            />
            <TextInput
              label={t('rulesets.form.nameZh')}
              name="category_name_zh"
              value={zh}
              onChange={setZh}
            />
            <TextInput
              label={t('rulesets.form.nameEn')}
              name="category_name_en"
              value={en}
              onChange={setEn}
            />
            <NumberInput
              label={t('rulesets.form.order')}
              name="category_order"
              value={order}
              onChange={setOrder}
              minValue={0}
              step={1}
              description={t('rulesets.form.orderHint')}
            />
          </div>
          <fieldset className="flex flex-col gap-3">
            <legend className="mb-2 text-subhead font-semibold text-label">{t('rulesets.form.refs')}</legend>
            {sources.map(([src, s]) => {
              const fields = fieldsOf(src)
              const r = refs[src]
              return (
                <div
                  key={src}
                  className="grid items-start gap-3 rounded-[12px] bg-fill-quaternary p-3 sm:grid-cols-[9rem_1fr_1fr_1fr]"
                >
                  <div className="pt-1">
                    <p className="font-mono text-subhead text-label">{src}</p>
                    <p className="truncate text-caption text-label-secondary" title={s.name}>
                      {s.name}
                    </p>
                  </div>
                  {fields.has('name') ? (
                    <>
                      <TextInput
                        label={t('rulesets.form.refName')}
                        name={`ref_${src}_name`}
                        value={r?.name ?? ''}
                        onChange={(v) => setRef(src, 'name', v)}
                        monospace
                      />
                      <SelectInput
                        label={t('rulesets.form.refBehavior')}
                        name={`ref_${src}_behavior`}
                        value={r?.behavior || 'classical'}
                        onChange={(v) => setRef(src, 'behavior', v ?? '')}
                        options={['classical', 'domain', 'ipcidr'].map((b) => ({ value: b, label: b }))}
                      />
                    </>
                  ) : null}
                  {fields.has('geosite') ? (
                    <TextInput
                      label="geosite"
                      name={`ref_${src}_geosite`}
                      value={r?.geosite ?? ''}
                      onChange={(v) => setRef(src, 'geosite', v)}
                      monospace
                    />
                  ) : null}
                  {fields.has('geoip') ? (
                    <TextInput
                      label="geoip"
                      name={`ref_${src}_geoip`}
                      value={r?.geoip ?? ''}
                      onChange={(v) => setRef(src, 'geoip', v)}
                      monospace
                    />
                  ) : null}
                </div>
              )
            })}
            {errors.rules ? (
              <p role="alert" className="text-footnote text-danger">
                {errors.rules}
              </p>
            ) : null}
          </fieldset>
          <div className="flex justify-end">
            <Button type="submit" variant="secondary" isDisabled={disabled}>
              {t('rulesets.form.addToJson')}
            </Button>
          </div>
        </form>
      </Disclosure.Content>
    </Disclosure>
  )
}
