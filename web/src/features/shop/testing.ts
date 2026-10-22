/**
 * Test helpers for the FE-2 pages: render the real route tree at a URL with a signed-in user and a
 * mocked `fetch` (handlers keyed by "METHOD /path", path relative to `/api/v1`).
 */
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { render } from '@testing-library/react'
import { createElement } from 'react'
import { vi } from 'vitest'
import type { Plan, User } from '@/lib/api/types'
import { queryClient } from '@/lib/queryClient'
import { routeTree } from '@/routeTree.gen'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import config from '../../../scripts/fixtures/guest-config.json'
import userFixture from '../../../scripts/fixtures/user.json'

export interface MockRequest {
  method: string
  path: string
  url: URL
  body: unknown
}
export type Handler = (req: MockRequest) => unknown

/** `reply(409, {code, message})` → error body as-is; `reply(201, data)` → `{data}` envelope. */
export function reply(status: number, body: unknown) {
  return { __reply: true as const, status, body }
}

function isReply(value: unknown): value is ReturnType<typeof reply> {
  return typeof value === 'object' && value !== null && '__reply' in value
}

function json(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

export const testUser: User = { ...(userFixture as User), is_admin: false, balance: 1000, discount: null }

export const testConfig = config

export function makePlan(overrides: Partial<Plan> & Pick<Plan, 'id' | 'name'>): Plan {
  return {
    group_id: 1,
    content: '',
    tags: [],
    transfer_enable: 100,
    speed_limit: null,
    device_limit: 3,
    capacity_limit: null,
    prices: { monthly: 1500 },
    reset_traffic_method: 1,
    show: true,
    sell: true,
    renew: true,
    sort: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

/**
 * Stubs `fetch`. Handlers return plain data (wrapped as `{data}` with 200) or `reply(status, …)`.
 * `/guest/config` and `/user/me` have defaults. Returns the mock to inspect calls.
 */
export function mockApi(handlers: Record<string, Handler>, user: User = testUser) {
  const defaults: Record<string, Handler> = {
    'GET /guest/config': () => config,
    'GET /user/me': () => user,
  }
  const all = { ...defaults, ...handlers }
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const url = urlOf(input)
    const method = (init?.method ?? 'GET').toUpperCase()
    const path = url.pathname.replace(/^\/api\/v1/, '')
    const raw = typeof init?.body === 'string' ? init.body : undefined
    const body: unknown = raw ? JSON.parse(raw) : undefined
    const handler = all[`${method} ${path}`]
    if (!handler) return json(404, { code: 'not_found', message: `no mock for ${method} ${path}` })
    const result = await handler({ method, path, url, body })
    if (isReply(result)) {
      return json(result.status, result.status >= 400 ? result.body : { data: result.body })
    }
    return json(200, { data: result })
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

type FetchCall = Parameters<typeof fetch>

function urlOf(input: FetchCall[0]): URL {
  return new URL(
    input instanceof Request ? input.url : input instanceof URL ? input.href : input,
    'http://localhost',
  )
}

/** Parsed JSON body of a recorded fetch call. */
export function bodyOf(call: FetchCall | undefined): unknown {
  const raw = call?.[1]?.body
  return typeof raw === 'string' ? JSON.parse(raw) : undefined
}

/** URL of a recorded fetch call. */
export function urlOfCall(call: FetchCall | undefined): URL | undefined {
  return call ? urlOf(call[0]) : undefined
}

/** Calls made to `METHOD /path` (relative to `/api/v1`). */
export function callsTo(fetchMock: ReturnType<typeof mockApi>, method: string, path: string) {
  return fetchMock.mock.calls.filter(([input, init]) => {
    const url = urlOf(input)
    return (init?.method ?? 'GET').toUpperCase() === method && url.pathname === `/api/v1${path}`
  })
}

/** Renders the app at `path` as a signed-in user. */
export function renderApp(path: string, user: User = testUser) {
  useUiStore.setState({ locale: 'zh-CN', theme: 'system' })
  useAuthStore.setState({ token: 'test-token', user })
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [path] }) })
  const view = render(createElement(RouterProvider, { router }))
  return { router, ...view }
}

/** Reset shared singletons between tests. */
export function resetApp() {
  queryClient.clear()
  vi.unstubAllGlobals()
  useAuthStore.setState({ token: null, user: null })
}
