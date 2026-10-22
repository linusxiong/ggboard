/**
 * Public data for the landing/auth pages WITHOUT React Query: those pages must stay inside the
 * 120 KB first-load budget, and React Query (~11 KB gz) only loads with the authenticated
 * shells. Promises are cached for the session; the shells re-read through React Query.
 *
 *   loader: () => getGuestConfig().catch(() => null)      // route loader
 *   const config = Route.useLoaderData()                   // component
 */
import { useEffect, useState } from 'react'
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

/** Resolves `null` instead of blocking a route loader when the backend is slow. */
export function withDeadline<T>(promise: Promise<T>, ms = 3000): Promise<T | null> {
  return Promise.race([promise, new Promise<null>((resolve) => setTimeout(() => resolve(null), ms))]).catch(() => null)
}

export interface AsyncState<T> {
  data: T | undefined
  error: unknown
  pending: boolean
}

/** Subscribes a component to a (cached) promise. */
export function usePromise<T>(factory: () => Promise<T>): AsyncState<T> {
  const [state, setState] = useState<AsyncState<T>>({ data: undefined, error: undefined, pending: true })
  useEffect(() => {
    let alive = true
    factory().then(
      (data) => alive && setState({ data, error: undefined, pending: false }),
      (error: unknown) => alive && setState({ data: undefined, error, pending: false }),
    )
    return () => {
      alive = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- factories are module-level functions
  }, [])
  return state
}
