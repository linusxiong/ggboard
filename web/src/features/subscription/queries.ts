/**
 * Query factories for the subscription page (profiles, age key, access log) and the Telegram card.
 * Feature-local (like `features/security/api.ts`) so the shared `queries.ts` stays untouched; keys live
 * under `qk.user.*` prefixes so the existing invalidations keep working.
 */
import { type QueryKey, queryOptions } from '@tanstack/react-query'
import { isApiError } from '@/lib/api/client'
import { qk } from '@/lib/api/queries'
import type { RuleProfile, RuleProfileSummary } from '@/lib/api/types'
import {
  ageKey,
  ruleProfile,
  ruleProfileBySlug,
  ruleProfiles,
  subscriptionAccessLog,
  telegram,
} from '@/lib/api/user'

export const profilesKey = () => [...qk.user.all, 'rule-profiles'] as const
/** Under `qk.user.ruleProfile()` so invalidating the default profile refreshes every profile. */
export const profileKey = (slug: string) => [...qk.user.ruleProfile(), 'slug', slug] as const
export const ageKeyKey = () => [...qk.user.subscription(), 'age'] as const
export const accessLogKey = () => [...qk.user.subscription(), 'access-log'] as const
export const telegramKey = () => [...qk.user.all, 'telegram'] as const

/** 404/405 = the server predates the endpoint (feature hidden / legacy behaviour). */
export const isMissing = (error: unknown) =>
  isApiError(error) && (error.status === 404 || error.status === 405)

export const ruleProfilesQuery = () =>
  queryOptions({ queryKey: profilesKey(), queryFn: ({ signal }) => ruleProfiles(signal) })

export const ruleProfileBySlugQuery = (slug: string) =>
  queryOptions({ queryKey: profileKey(slug), queryFn: ({ signal }) => ruleProfileBySlug(slug, signal) })

/** The default profile goes through the `/user/rule-profile` alias (same cache as `ruleProfileQuery()`). */
const usesAlias = (p: Pick<RuleProfileSummary, 'slug' | 'is_default'>) => p.is_default || !p.slug

/** Cache key of a profile's stored rules. */
export const profileDetailKey = (p: Pick<RuleProfileSummary, 'slug' | 'is_default'>): QueryKey =>
  usesAlias(p) ? qk.user.ruleProfile() : profileKey(p.slug)

/** Stored profile of a tab: the default via the alias, the others by slug. */
export const profileDetailQuery = (p: Pick<RuleProfileSummary, 'slug' | 'is_default'>) =>
  queryOptions<RuleProfile, Error, RuleProfile, QueryKey>({
    queryKey: profileDetailKey(p),
    queryFn: ({ signal }) => (usesAlias(p) ? ruleProfile(signal) : ruleProfileBySlug(p.slug, signal)),
  })

/** `null` = no key yet (the endpoint answers 404). */
export const ageKeyQuery = () =>
  queryOptions({
    queryKey: ageKeyKey(),
    queryFn: async ({ signal }) => {
      try {
        return await ageKey(signal)
      } catch (error) {
        if (isApiError(error) && error.status === 404 && error.code === 'not_found') return null
        throw error
      }
    },
  })

export const accessLogQuery = () =>
  queryOptions({ queryKey: accessLogKey(), queryFn: ({ signal }) => subscriptionAccessLog(signal) })

export const telegramQuery = () =>
  queryOptions({ queryKey: telegramKey(), queryFn: ({ signal }) => telegram(signal) })
