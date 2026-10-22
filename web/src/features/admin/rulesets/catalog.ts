import { qk } from '@/lib/api/queries'
import type { LocalizedText } from '@/lib/api/types'

/** Shared cache key of `GET /admin/rulesets` (settings + rule-set pages). */
export const RULESETS_FULL_KEY = [...qk.admin.rulesets(), 'full'] as const

/** `{"zh-CN": …, "en": …}` → text for the locale (falls back to en, zh-CN, then `fallback`). */
export const localized = (text: LocalizedText | undefined, locale: string, fallback: string) =>
  text?.[locale] ?? text?.en ?? text?.['zh-CN'] ?? fallback
