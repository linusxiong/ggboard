/**
 * Subscription profiles (several rule profiles per user, each with its own `?profile=<slug>` link).
 * Pure helpers; the UI is `ProfileTabs.tsx` / `SubscriptionPage.tsx`.
 */
import type { RuleProfileSummary, UserSubscription } from '@/lib/api/types'
import { withSubscribeQuery } from './importLinks'

/** Contract: at most 10 profiles per user. */
export const MAX_PROFILES = 10
export const MAX_NAME = 64
export const SLUG_RE = /^[a-z0-9][a-z0-9_-]{0,31}$/

/** Stand-in when the server lists no profile yet (or predates profiles): the default via `/user/rule-profile`. */
export const VIRTUAL_DEFAULT: RuleProfileSummary = {
  id: 0,
  slug: '',
  name: '',
  is_default: true,
  updated_at: null,
}

/** React Aria keys must not be empty (MISTAKES #83). */
export const DEFAULT_TAB = '__default__'
export const tabKey = (p: RuleProfileSummary) => p.slug || DEFAULT_TAB

/** Default profile first, then the server order. */
export function orderProfiles(list: readonly RuleProfileSummary[] | undefined): RuleProfileSummary[] {
  if (!list || list.length === 0) return [VIRTUAL_DEFAULT]
  const defaults = list.filter((p) => p.is_default)
  return [
    ...(defaults.length ? defaults.slice(0, 1) : [VIRTUAL_DEFAULT]),
    ...list.filter((p) => !defaults.includes(p)),
  ]
}

/** The profile named by `?profile=` (slug), else the default. */
export function pickActive(list: readonly RuleProfileSummary[], requested: string | undefined) {
  return (requested ? list.find((p) => p.slug === requested) : undefined) ?? list[0] ?? VIRTUAL_DEFAULT
}

/** Server-provided link for a profile, else the base link (default) or base + `profile=<slug>`. */
export function profileUrl(sub: UserSubscription, profile: RuleProfileSummary): string {
  const listed = profile.slug ? sub.profiles?.find((p) => p.slug === profile.slug)?.subscribe_url : undefined
  if (listed) return listed
  if (profile.is_default || !profile.slug) return sub.subscribe_url
  return withSubscribeQuery(sub.subscribe_url, { profile: profile.slug })
}

/** `"My phone"` → `"my-phone"`; non-ASCII names give `""` (the server picks a slug). */
export function slugify(name: string): string {
  return name
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 28)
    .replace(/-+$/, '')
}

export type ProfileIssue = 'nameRequired' | 'nameTooLong' | 'slugFormat' | 'slugTaken' | 'slugReserved'

/**
 * Create/rename form checks. `reserved` = template ids (`?profile=` also accepts a template id, so a slug
 * must never shadow one).
 */
export function profileIssues(
  input: { name: string; slug?: string },
  existing: readonly RuleProfileSummary[],
  reserved: readonly string[],
  self?: RuleProfileSummary,
): Partial<Record<'name' | 'slug', ProfileIssue>> {
  const issues: Partial<Record<'name' | 'slug', ProfileIssue>> = {}
  const name = input.name.trim()
  if (!name) issues.name = 'nameRequired'
  else if ([...name].length > MAX_NAME) issues.name = 'nameTooLong'
  const slug = input.slug?.trim() ?? ''
  if (slug) {
    if (!SLUG_RE.test(slug)) issues.slug = 'slugFormat'
    else if (reserved.includes(slug)) issues.slug = 'slugReserved'
    else if (existing.some((p) => p.slug === slug && p !== self)) issues.slug = 'slugTaken'
  }
  return issues
}

/** `slugify(name)` made unique (`-2`, `-3`, …) and kept off reserved ids; `""` = let the server choose. */
export function suggestSlug(
  name: string,
  existing: readonly RuleProfileSummary[],
  reserved: readonly string[],
): string {
  const base = slugify(name)
  if (!base) return ''
  const taken = (s: string) => reserved.includes(s) || existing.some((p) => p.slug === s)
  if (!taken(base)) return base
  for (let i = 2; i < 100; i++) {
    const candidate = `${base.slice(0, 29)}-${i}`
    if (!taken(candidate)) return candidate
  }
  return ''
}
