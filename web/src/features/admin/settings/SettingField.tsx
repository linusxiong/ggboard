import { Button } from '@heroui/react'
import { RefreshCw } from 'lucide-react'
import type { ReactNode } from 'react'
import { confirm } from '@/components/feedback/confirm'
import { ChipSelect } from '@/components/form/ChipSelect'
import { useT } from '@/i18n'
import type { RulesetCatalogFull, SettingDef } from '@/lib/api/types'
import { ComboInput } from './inputs/ComboInput'
import { NumberInput, SelectInput, SwitchInput, TextAreaInput, TextInput } from './inputs/Controlled'
import { SecretInput } from './inputs/SecretInput'
import { TagInput } from './inputs/TagInput'
import { DEPENDS_ON, UNITS } from './layout'
import { randomToken } from './settingsMeta'
import { UrlListEditor } from './UrlListEditor'

export interface SettingFieldContext {
  plans: ReadonlyArray<{ id: number; name: string }>
  catalog: RulesetCatalogFull | undefined
  locale: string
  /** Current (draft) values of every key, for dependent fields. */
  values: Record<string, unknown>
  tokenRevealed: boolean
  setTokenRevealed: (v: boolean) => void
}

interface SettingFieldProps {
  def: SettingDef
  value: unknown
  onChange: (value: unknown) => void
  error?: string
  ctx: SettingFieldContext
}

const URL_KEYS = new Set([
  'app_url',
  'logo',
  'tos_url',
  'frontend_background_url',
  'frontend_customer_service_url',
  'windows_download_url',
  'macos_download_url',
  'android_download_url',
  'ios_download_url',
  'telegram_discuss_link',
  'geoip_download_url',
  'geoip_version_url',
])
const TEXTAREA_KEYS = new Set(['app_description', 'frontend_footer_html'])
const MONO_KEYS = new Set(['server_white_list', 'captcha_site_key', 'email_host', 'ruleset_mirror'])

/** Domains for the email allow-list quick-add chips. */
const DOMAIN_SUGGESTIONS = ['gmail.com', 'outlook.com', 'icloud.com', 'qq.com', '163.com', 'hotmail.com']
const DOMAIN_RE = /^(?=.{1,253}$)([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/i
const domain = (v: string) => v.trim().replace(/^@+/, '').toLowerCase()
/** Telegram chat: numeric id (groups are negative) or a public @username. */
const CHAT_RE = /^(-?\d{1,20}|@[A-Za-z][A-Za-z0-9_]{3,31})$/

const str = (v: unknown): string =>
  v === null || v === undefined
    ? ''
    : typeof v === 'string'
      ? v
      : typeof v === 'number' || typeof v === 'boolean'
        ? String(v)
        : JSON.stringify(v)
const strings = (v: unknown): string[] => (Array.isArray(v) ? v.map(String) : [])
const num = (v: unknown): number | null => (typeof v === 'number' && Number.isFinite(v) ? v : null)
const localized = (text: Record<string, string> | undefined, locale: string, fallback: string) =>
  text?.[locale] ?? text?.en ?? fallback

/** One setting rendered from its metadata, with key-specific editors where it helps. */
export function SettingField({ def, value, onChange, error, ctx }: SettingFieldProps) {
  const t = useT('admin-settings')
  const labelKey = `keys.${def.key}.label`
  const hintKey = `keys.${def.key}.hint`
  const translatedLabel = t(labelKey)
  const label: ReactNode = translatedLabel === labelKey ? def.key : translatedLabel
  const translatedHint = t(hintKey)
  const description: ReactNode =
    translatedHint === hintKey ? def.description || undefined : translatedHint || def.description || undefined
  const dependency = DEPENDS_ON[def.key]
  const isDisabled = dependency ? !dependency.when(ctx.values) : false
  const common = { label, name: def.key, description, error, isDisabled }
  const unit = UNITS[def.key] ? t(`units.${UNITS[def.key]}`) : undefined

  // ------------------------------------------------------------ key-specific editors
  switch (def.key) {
    case 'stop_register':
      // Shown as the positive "开放注册" switch: on = registration open = stop_register false.
      return <SwitchInput {...common} value={value !== true} onChange={(open) => onChange(!open)} />
    case 'email_whitelist_suffix':
      return (
        <TagInput
          {...common}
          value={strings(value)}
          onChange={onChange}
          normalize={domain}
          validate={(d) => (DOMAIN_RE.test(d) ? undefined : t('register.domainInvalid'))}
          suggestions={DOMAIN_SUGGESTIONS}
          placeholder="example.com"
        />
      )
    case 'captcha_provider':
      return (
        <SelectInput
          {...common}
          value={str(value || 'none')}
          onChange={(v) => onChange(v ?? 'none')}
          options={(def.options?.length ? def.options : ['none', 'turnstile', 'recaptcha', 'hcaptcha']).map(
            (o) => ({
              value: o,
              label: t(`enums.captcha_provider.${o}`),
              description: t(`enumHints.captcha_provider.${o}`),
            }),
          )}
        />
      )
    case 'commission_withdraw_limit':
      return (
        <NumberInput
          {...common}
          value={num(value)}
          onChange={onChange}
          minValue={def.min}
          prefix={str(ctx.values.currency_symbol) || undefined}
          step={1}
          formatOptions={{ maximumFractionDigits: 0, useGrouping: false }}
        />
      )
    case 'reset_traffic_method':
      return (
        <SelectInput
          {...common}
          value={str(value)}
          onChange={(v) => onChange(v === null ? null : Number(v))}
          options={[0, 1, 2, 3, 4].map((n) => ({
            value: String(n),
            label: t(`enums.reset_traffic_method.${n}`),
          }))}
        />
      )
    case 'device_limit_mode':
      return (
        <SelectInput
          {...common}
          value={str(value)}
          onChange={(v) => onChange(v === null ? null : Number(v))}
          options={[0, 1].map((n) => ({ value: String(n), label: t(`enums.device_limit_mode.${n}`) }))}
        />
      )
    case 'try_out_plan_id':
      return (
        <SelectInput
          {...common}
          value={str(value ?? 0)}
          onChange={(v) => onChange(v === null ? 0 : Number(v))}
          options={[
            { value: '0', label: t('enums.try_out_plan_id.0') },
            ...ctx.plans.map((p) => ({ value: String(p.id), label: p.name })),
            ...(num(value) && !ctx.plans.some((p) => p.id === value)
              ? [{ value: str(value), label: `#${str(value)}` }]
              : []),
          ]}
        />
      )
    case 'subscribe_url':
      return (
        <UrlListEditor
          {...common}
          value={strings(value)}
          onChange={onChange}
          placeholder={str(ctx.values.app_url) || 'https://sub.example.com'}
        />
      )
    case 'ruleset_default_template':
      if (ctx.catalog?.templates?.length)
        return (
          <SelectInput
            {...common}
            value={str(value)}
            onChange={(v) => onChange(v ?? '')}
            options={ctx.catalog.templates.map((tp) => ({
              value: tp.id,
              label: `${localized(tp.name, ctx.locale, tp.id)} (${tp.id})`,
            }))}
          />
        )
      break
    case 'ruleset_default_source':
      if (ctx.catalog?.sources)
        return (
          <SelectInput
            {...common}
            value={str(value)}
            onChange={(v) => onChange(v ?? '')}
            options={Object.entries(ctx.catalog.sources)
              .sort(([, a], [, b]) => (a.order ?? 0) - (b.order ?? 0))
              .map(([id, s]) => ({ value: id, label: `${s.name} (${id})`, textValue: s.name }))}
          />
        )
      break
    case 'ruleset_mirror':
      return (
        <ComboInput
          {...common}
          value={str(value)}
          onChange={onChange}
          monospace
          placeholder={t('keys.ruleset_mirror.placeholder')}
          options={(ctx.catalog?.mirrors ?? []).map((m) => ({
            value: m.prefix,
            label: m.prefix || t('keys.ruleset_mirror.direct'),
            hint: localized(m.name, ctx.locale, m.id),
          }))}
        />
      )
    case 'telegram_admin_chat_ids':
      return (
        <TagInput
          {...common}
          value={strings(value)}
          onChange={onChange}
          monospace
          validate={(c) => (CHAT_RE.test(c) ? undefined : t('telegram.chatInvalid'))}
          placeholder="123456789"
        />
      )
    case 'subscribe_leak_action':
      return (
        <SelectInput
          {...common}
          value={str(value || 'notify')}
          onChange={(v) => onChange(v ?? 'notify')}
          options={(def.options?.length ? def.options : ['notify', 'rotate']).map((o) => ({
            value: o,
            label: t(`enums.subscribe_leak_action.${o}`),
            description: t(`enumHints.subscribe_leak_action.${o}`),
          }))}
        />
      )
    case 'telegram_webhook_secret':
      return (
        <SecretInput
          {...common}
          value={str(value)}
          onChange={onChange}
          actions={
            <Button
              size="sm"
              variant="ghost"
              isIconOnly
              aria-label={t('telegram.regenerateSecret')}
              onPress={() =>
                void confirm({
                  title: t('telegram.regenerateSecretTitle'),
                  description: t('telegram.regenerateSecretHint'),
                  confirmLabel: t('telegram.regenerateSecret'),
                  tone: 'danger',
                }).then((ok) => {
                  if (ok) onChange(randomToken(32))
                })
              }
            >
              <RefreshCw className="size-4" aria-hidden />
            </Button>
          }
        />
      )
    case 'server_token':
      return (
        <SecretInput
          {...common}
          value={str(value)}
          onChange={onChange}
          revealed={ctx.tokenRevealed}
          onRevealedChange={ctx.setTokenRevealed}
          actions={
            <Button
              size="sm"
              variant="ghost"
              isIconOnly
              aria-label={t('server.regenerate')}
              onPress={() =>
                void confirm({
                  title: t('server.regenerateTitle'),
                  description: t('server.regenerateHint'),
                  confirmLabel: t('server.regenerate'),
                  tone: 'danger',
                }).then((ok) => {
                  if (ok) {
                    onChange(randomToken(32))
                    ctx.setTokenRevealed(true)
                  }
                })
              }
            >
              <RefreshCw className="size-4" aria-hidden />
            </Button>
          }
        />
      )
    case 'frontend_accent':
      return (
        <TextInput
          {...common}
          value={str(value)}
          onChange={onChange}
          monospace
          placeholder="#007AFF"
          endContent={
            <input
              type="color"
              aria-label={t('keys.frontend_accent.picker')}
              value={/^#[0-9a-fA-F]{6}$/.test(str(value)) ? str(value) : '#007aff'}
              onChange={(e) => onChange(e.target.value.toUpperCase())}
              className="size-7 cursor-pointer rounded-[8px] border-0 bg-transparent p-0"
            />
          }
        />
      )
  }

  // ------------------------------------------------------------ by type
  switch (def.type) {
    case 'bool':
      return <SwitchInput {...common} value={value === true} onChange={onChange} />
    case 'int':
      return (
        <NumberInput
          {...common}
          value={num(value)}
          onChange={onChange}
          minValue={def.min}
          maxValue={def.max !== undefined && def.max < 2_147_483_647 ? def.max : undefined}
          step={1}
          unit={unit}
          formatOptions={{ maximumFractionDigits: 0, useGrouping: false }}
        />
      )
    case 'strings':
      // A fixed option set (e.g. telegram_events) is a multi-select of toggle chips.
      if (def.options?.length)
        return (
          <ChipSelect
            {...common}
            value={strings(value)}
            onChange={onChange}
            options={def.options.map((o) => {
              const k = `enums.${def.key}.${o}`
              const text = t(k)
              return { value: o, label: text === k ? o : text }
            })}
          />
        )
      return (
        <TagInput {...common} value={strings(value)} onChange={onChange} monospace={MONO_KEYS.has(def.key)} />
      )
    case 'json':
      return (
        <TextAreaInput
          {...common}
          value={typeof value === 'string' ? value : JSON.stringify(value ?? null, null, 2)}
          onChange={onChange}
          rows={8}
          monospace
        />
      )
    default:
      if (def.options?.length)
        return (
          <SelectInput
            {...common}
            value={str(value)}
            onChange={(v) => onChange(v ?? '')}
            options={def.options.map((o) => {
              const k = `enums.${def.key}.${o}`
              const text = t(k)
              return { value: o, label: text === k ? o : text, textValue: text === k ? o : text }
            })}
          />
        )
      if (def.secret) return <SecretInput {...common} value={str(value)} onChange={onChange} />
      if (TEXTAREA_KEYS.has(def.key))
        return (
          <TextAreaInput
            {...common}
            value={str(value)}
            onChange={onChange}
            rows={def.key === 'frontend_footer_html' ? 5 : 3}
            monospace={def.key === 'frontend_footer_html'}
          />
        )
      return (
        <TextInput
          {...common}
          type={URL_KEYS.has(def.key) ? 'url' : def.key === 'email_from_address' ? 'email' : 'text'}
          value={str(value)}
          onChange={onChange}
          monospace={MONO_KEYS.has(def.key)}
          placeholder={URL_KEYS.has(def.key) ? 'https://' : undefined}
        />
      )
  }
}
