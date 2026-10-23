/**
 * URL state of `/admin/mail-templates` (`?name=verify_code&locale=zh-CN`, both optional). Keep this module
 * dependency-free: the route's `validateSearch` imports it eagerly (it stays in the entry chunk, MISTAKES #84).
 */
import { optEnum, optString } from '@/features/admin/orders/search'
import type { Locale } from '@/lib/api/types'

/** Locales a template can be customised in (docs/API.md "Mail templates"). */
export const MAIL_LOCALES: readonly Locale[] = ['zh-CN', 'en']

export interface MailTemplatesSearch {
  /** Template name; unknown names fall back to the first template of the list. */
  name?: string
  locale?: Locale
}

export function parseMailTemplatesSearch(raw: Record<string, unknown>): MailTemplatesSearch {
  const name = optString(raw.name)
  const locale = optEnum(raw.locale, MAIL_LOCALES)
  return { ...(name ? { name } : {}), ...(locale ? { locale } : {}) }
}
