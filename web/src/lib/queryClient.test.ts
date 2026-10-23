import { isCancelledError, type QueryClient, QueryObserver } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/lib/api/client'
import type { User } from '@/lib/api/types'
import { AUTH_STORAGE_KEY, useAuthStore } from '@/stores/auth'
import { bindSessionCache, createQueryClient, isPublicQuery, queryClient } from './queryClient'

// `exp` = 2100-01-01, so the cross-tab rehydrate keeps them.
const TOKEN_A = 'a.eyJleHAiOjQxMDI0NDQ4MDB9.sig'
const TOKEN_B = 'b.eyJleHAiOjQxMDI0NDQ4MDB9.sig'
const userA = { id: 1, email: 'a@example.com', is_admin: true, is_staff: false } as User
const userB = { id: 2, email: 'b@example.com', is_admin: false, is_staff: false } as User

const ME = ['user', 'me'] as const
const SUBSCRIPTION = ['user', 'subscription'] as const
const ADMIN_USERS = ['admin', 'users', {}] as const
const CONFIG = ['guest', 'config'] as const

let client: QueryClient
let unbind: () => void

beforeEach(() => {
  useAuthStore.setState({ token: TOKEN_A, user: userA })
  client = createQueryClient()
  unbind = bindSessionCache(client)
})

afterEach(() => {
  unbind()
  client.clear()
  queryClient.clear()
  vi.unstubAllGlobals()
  useAuthStore.setState({ token: null, user: null })
})

/** What user A's session leaves behind: private reads, a public read and a mutation result. */
async function seedAccountA() {
  client.setQueryData(ME, userA)
  client.setQueryData(SUBSCRIPTION, { subscribe_url: 'https://panel.example.com/s/token-of-a' })
  client.setQueryData(ADMIN_USERS, { items: [userA], total: 1 })
  client.setQueryData(CONFIG, { app_name: 'GG Bond' })
  await client
    .getMutationCache()
    .build(client, { mutationFn: () => Promise.resolve({ url: '/login?token=one-time' }) })
    .execute(undefined)
}

function expectAccountADropped() {
  expect(client.getQueryData(ME)).toBeUndefined()
  expect(client.getQueryData(SUBSCRIPTION)).toBeUndefined()
  expect(client.getQueryData(ADMIN_USERS)).toBeUndefined()
  expect(client.getMutationCache().getAll()).toHaveLength(0)
  // Public `/guest/*` data is the same for everyone: kept, so the shell does not flash.
  expect(client.getQueryData(CONFIG)).toEqual({ app_name: 'GG Bond' })
}

function storageEvent(state: { token: string | null; user: User | null }) {
  localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify({ state, version: 1 }))
  window.dispatchEvent(new StorageEvent('storage', { key: AUTH_STORAGE_KEY }))
}

describe('session-bound query cache', () => {
  it('classifies only `guest` keys as public', () => {
    expect(isPublicQuery({ queryKey: ['guest', 'config'] })).toBe(true)
    expect(isPublicQuery({ queryKey: ['user', 'me'] })).toBe(false)
    expect(isPublicQuery({ queryKey: ['admin', 'users', {}] })).toBe(false)
  })

  it('switching to another account drops every private query and mutation of the previous one', async () => {
    await seedAccountA()
    expect(client.getMutationCache().getAll()).toHaveLength(1)
    useAuthStore.getState().login(TOKEN_B, userB) // useFinishAuth: login, register, MFA, passkey, "log in as"
    expectAccountADropped()
  })

  it('signing in starts from an empty cache even if something survived the sign-out', async () => {
    useAuthStore.getState().logout()
    await seedAccountA() // e.g. written while signed out
    useAuthStore.getState().login(TOKEN_B, userB)
    expectAccountADropped()
  })

  it('a late answer for the previous account never lands in the cache', async () => {
    let answer: (user: User) => void = () => undefined
    const inFlight = client
      .query({ queryKey: ME, queryFn: () => new Promise<User>((resolve) => (answer = resolve)) })
      .catch((error: unknown) => error)
    useAuthStore.getState().login(TOKEN_B, userB)
    answer(userA)
    expect(isCancelledError(await inFlight)).toBe(true)
    expect(client.getQueryData(ME)).toBeUndefined()
  })

  it('signing out drops unobserved data now and the still-mounted views as they unmount, without refetching', async () => {
    const queryFn = vi.fn(() => Promise.resolve(userA))
    const observer = new QueryObserver(client, { queryKey: ME, queryFn, staleTime: Infinity })
    const unsubscribe = observer.subscribe(() => undefined)
    await vi.waitFor(() => expect(client.getQueryData(ME)).toEqual(userA))
    await seedAccountA()

    useAuthStore.getState().logout()
    expect(client.getQueryData(SUBSCRIPTION)).toBeUndefined()
    expect(client.getQueryData(ADMIN_USERS)).toBeUndefined()
    expect(client.getMutationCache().getAll()).toHaveLength(0)
    expect(client.getQueryData(ME)).toEqual(userA) // the shell is still on screen until the router leaves it…
    expect(queryFn).toHaveBeenCalledTimes(1) // …and is not refetched without a token

    unsubscribe() // shell unmounted
    expect(client.getQueryData(ME)).toBeUndefined()
    expect(client.getQueryData(CONFIG)).toEqual({ app_name: 'GG Bond' })
    expect(queryFn).toHaveBeenCalledTimes(1)
  })

  it('a 401 from the API (expired or revoked token) ends the session and drops the cache', async () => {
    await seedAccountA()
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ code: 'unauthorized', message: 'token expired' }), {
            status: 401,
            headers: { 'Content-Type': 'application/json' },
          }),
        ),
      ),
    )
    await expect(api.get('/user/stats')).rejects.toMatchObject({ status: 401 })
    expect(useAuthStore.getState().token).toBeNull()
    expectAccountADropped()
  })

  it('follows another tab signing out or signing in as someone else', async () => {
    await seedAccountA()
    storageEvent({ token: null, user: null })
    expect(useAuthStore.getState().token).toBeNull()
    expectAccountADropped()

    useAuthStore.getState().login(TOKEN_A, userA)
    await seedAccountA()
    storageEvent({ token: TOKEN_B, user: userB })
    expect(useAuthStore.getState().token).toBe(TOKEN_B)
    expectAccountADropped()
  })

  it('a view still on screen when another tab switches accounts is blanked and refetched for the new one', async () => {
    const queryFn = vi.fn(() => Promise.resolve(useAuthStore.getState().token === TOKEN_B ? userB : userA))
    const observer = new QueryObserver(client, { queryKey: ME, queryFn, staleTime: Infinity })
    const unsubscribe = observer.subscribe(() => undefined)
    await vi.waitFor(() => expect(observer.getCurrentResult().data).toEqual(userA))

    storageEvent({ token: TOKEN_B, user: userB })
    expect(client.getQueryData(ME)).toBeUndefined() // A's data is gone at once…
    await vi.waitFor(() => expect(observer.getCurrentResult().data).toEqual(userB)) // …and B's fetched
    expect(queryFn).toHaveBeenCalledTimes(2)
    unsubscribe()
  })

  it('keeps the cache when the session does not change hands (profile refresh)', async () => {
    await seedAccountA()
    useAuthStore.getState().setUser({ ...userA, email: 'renamed@example.com' })
    expect(client.getQueryData(ME)).toEqual(userA)
    expect(client.getQueryData(SUBSCRIPTION)).toBeDefined()
  })

  it('the app-wide client is bound to the session', () => {
    queryClient.setQueryData(SUBSCRIPTION, { subscribe_url: 'https://panel.example.com/s/token-of-a' })
    queryClient.setQueryData(CONFIG, { app_name: 'GG Bond' })
    useAuthStore.getState().login(TOKEN_B, userB)
    expect(queryClient.getQueryData(SUBSCRIPTION)).toBeUndefined()
    expect(queryClient.getQueryData(CONFIG)).toEqual({ app_name: 'GG Bond' })
  })
})
