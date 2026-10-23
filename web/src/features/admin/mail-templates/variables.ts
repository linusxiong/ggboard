/**
 * Placeholder helpers for the mail template editor. Placeholders are written like the knowledge base's:
 * `{{name}}`, no spaces, case-sensitive (docs/API.md "Mail templates").
 */
import type { TFunction } from '@/i18n'
import type { MailTemplateVariable } from '@/lib/api/types'

/** Accepted by every template, whether or not the list endpoint repeats them. */
export const COMMON_VARIABLES = ['siteName', 'siteUrl', 'year'] as const

export const placeholder = (name: string) => `{{${name}}}`

/**
 * A placeholder as the preview reports it: the API lists them as written (`{{foo}}`, also near misses such
 * as `{{ code }}`, which must stay visible as typed); a bare name is wrapped.
 */
export function asPlaceholder(raw: string): string {
  const s = raw.trim()
  return s.startsWith('{{') && s.endsWith('}}') ? s : placeholder(s)
}

/**
 * The editor's variable buttons: the template's variables plus the common ones (deduplicated), required
 * first, then the template's own, then the common ones; the server order is kept inside each group.
 */
export function editorVariables(vars: readonly MailTemplateVariable[]): MailTemplateVariable[] {
  const byName = new Map<string, MailTemplateVariable>()
  for (const v of vars) if (v.name && !byName.has(v.name)) byName.set(v.name, v)
  for (const name of COMMON_VARIABLES) if (!byName.has(name)) byName.set(name, { name, required: false })
  const common = new Set<string>(COMMON_VARIABLES)
  const rank = (v: MailTemplateVariable) => (v.required ? 0 : common.has(v.name) ? 2 : 1)
  return [...byName.values()].sort((a, b) => rank(a) - rank(b))
}

/**
 * Names with strings in `admin-settings.json` (`mailTemplates.vars.*`, `templates.*`, `templateVars.*.url`).
 * Anything else shows its raw name without a dictionary lookup (a miss would log an i18n warning).
 */
export const LABELLED_VARIABLES: ReadonlySet<string> = new Set([
  'siteName',
  'siteUrl',
  'year',
  'code',
  'minutes',
  'expiredAt',
  'url',
  'percent',
  'used',
  'total',
  'tradeNo',
  'amount',
  'planName',
  'period',
  'ticketSubject',
  'message',
  'content',
  'title',
])
export const LABELLED_TEMPLATES: ReadonlySet<string> = new Set([
  'verify_code',
  'remind_expire',
  'remind_traffic',
  'order_paid',
  'ticket_reply',
  'notify',
])
/** Templates whose `{{url}}` points somewhere specific (renewal page, order, ticket …). */
export const TEMPLATE_URL_LABELS: ReadonlySet<string> = new Set([
  'remind_expire',
  'remind_traffic',
  'order_paid',
  'ticket_reply',
  'notify',
])

/** Human label of a variable: per-template wording, then the shared one, then the raw name. */
export function variableLabel(t: TFunction, template: string, name: string): string {
  if (name === 'url' && TEMPLATE_URL_LABELS.has(template))
    return t(`mailTemplates.templateVars.${template}.url`)
  return LABELLED_VARIABLES.has(name) ? t(`mailTemplates.vars.${name}`) : name
}

/** Title / one-line description of a template (unknown templates show their name). */
export function templateTitle(t: TFunction, name: string): string {
  return LABELLED_TEMPLATES.has(name) ? t(`mailTemplates.templates.${name}.title`) : name
}
export function templateDescription(t: TFunction, name: string): string | undefined {
  return LABELLED_TEMPLATES.has(name) ? t(`mailTemplates.templates.${name}.description`) : undefined
}
