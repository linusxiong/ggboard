/**
 * Test helpers for the user-portal pages (FE-1): a routed fetch mock, a signed-in session and a router
 * rendered at a URL. Uses `createElement` so this stays a `.ts` module.
 */
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { render } from '@testing-library/react'
import { createElement } from 'react'
import { vi } from 'vitest'
import type { User } from '@/lib/api/types'
import { routeTree } from '@/routeTree.gen'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import config from '../../scripts/fixtures/guest-config.json'
import fixtureUser from '../../scripts/fixtures/user.json'

export const TEST_USER = { ...fixtureUser, is_admin: false } as User
export const GUEST_CONFIG = config

export interface MockRequest {
  method: string
  path: string
  query: URLSearchParams
  body: unknown
}
export type MockHandler = (req: MockRequest) => unknown

/** `{data}` envelope (or a raw `Response` returned by a handler). */
export function json(data: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(status < 400 ? { data } : data), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  })
}

/**
 * Stubs `fetch`: `routes['GET /user/me']` (path relative to `/api/v1`) returns the `data` value, or a
 * `Response` for errors / text. Unknown routes answer 404. Returns the mock for call assertions.
 */
export function mockApi(routes: Record<string, unknown>) {
  const fetchMock = vi.fn<typeof fetch>((input, init) => {
    const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
    const method = (init?.method ?? 'GET').toUpperCase()
    const path = url.pathname.replace(/^\/api\/v1/, '')
    const key = `${method} ${path}`
    const body = typeof init?.body === 'string' ? (JSON.parse(init.body) as unknown) : undefined
    const route = routes[key]
    if (route === undefined)
      return Promise.resolve(json({ code: 'not_found', message: `unmocked ${key}` }, 404))
    const result =
      typeof route === 'function'
        ? (route as MockHandler)({ method, path, query: url.searchParams, body })
        : route
    return Promise.resolve(result instanceof Response ? result : json(result))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

/** Requests sent to `METHOD /path` (parsed JSON bodies). */
export function bodiesOf(fetchMock: ReturnType<typeof mockApi>, key: string): unknown[] {
  return fetchMock.mock.calls
    .filter(([input, init]) => {
      const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
      return `${(init?.method ?? 'GET').toUpperCase()} ${url.pathname.replace(/^\/api\/v1/, '')}` === key
    })
    .map(([, init]) => (typeof init?.body === 'string' ? (JSON.parse(init.body) as unknown) : undefined))
}

export function signIn(user: User = TEST_USER) {
  useAuthStore.setState({ token: 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig', user })
}

export function resetStores() {
  useUiStore.setState({ locale: 'zh-CN', theme: 'system' })
  useAuthStore.setState({ token: null, user: null })
}

export function renderAt(url: string) {
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [url] }) })
  return { router, ...render(createElement(RouterProvider, { router })) }
}
