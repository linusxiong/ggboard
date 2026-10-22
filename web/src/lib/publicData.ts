/**
 * Public data for the landing/auth pages WITHOUT React Query: those pages must stay inside the
 * 120 KB first-load budget, and React Query (~11 KB gz) only loads with the authenticated
 * shells. Promises are cached for the session; the shells re-read through React Query.
 *
 *   loader: () => getGuestConfig().catch(() => null)      // route loader
 *   const config = Route.useLoaderData()                   // component
 */
import * as guestApi from '@/lib/api/guest'
import type { GuestConfig, Plan } from '@/lib/api/types'

let configPromise: Promise<GuestConfig> | null = null
let plansPromise: Promise<Plan[]> | null = null

export function getGuestConfig(): Promise<GuestConfig> {
  configPromise ??= guestApi.config().catch((error: unknown) => {
    configPromise = null
    throw error
  })
  return configPromise
}

export function getGuestPlans(): Promise<Plan[]> {
  plansPromise ??= guestApi.plans().catch((error: unknown) => {
    plansPromise = null
    throw error
  })
  return plansPromise
}

let plansOrNull: Promise<Plan[] | null> | null = null
/**
 * Plans for React `use()` (cached, never rejects → `null` on error). Suspending on data and
 * chunk together means one reveal — no skeleton → content swap (CLS 0).
 */
export function guestPlansResource(): Promise<Plan[] | null> {
  plansOrNull ??= getGuestPlans().then(
    (plans) => plans,
    () => {
      plansOrNull = null
      return null
    },
  )
  return plansOrNull
}

/** Resolves `null` instead of blocking a route loader when the backend is slow. */
export function withDeadline<T>(promise: Promise<T>, ms = 3000): Promise<T | null> {
  return Promise.race([promise, new Promise<null>((resolve) => setTimeout(() => resolve(null), ms))]).catch(
    () => null,
  )
}
