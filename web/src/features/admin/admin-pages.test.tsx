/**
 * Admin pages rendered through the real route tree (guards, lazy QueryClient, i18n) with a
 * mocked `fetch`.
 */
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminOverview, AdminUser, User } from '@/lib/api/types'
import { routeTree } from '@/routeTree.gen'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import config from '../../../scripts/fixtures/guest-config.json'
import fixtureUser from '../../../scripts/fixtures/user.json'

const TOKEN = `eyJhbGciOiJub25lIn0.${btoa(JSON.stringify({ sub: 1, exp: 4102444800 })).replace(/=+$/, '')}.test`
const admin = { ...(fixtureUser as User), is_admin: true }
const GB = 1024 ** 3

const overview: AdminOverview = {
  today: { income: 45600, paid_orders: 18, registers: 23, commission: 2300 },
  month: { income: 1234500, paid_orders: 412, registers: 530 },
  totals: {
    users: 57,
    active_users: 41,
    online_users: 17,
    pending_tickets: 3,
    pending_orders: 6,
    pending_commission: 8800,
  },
  nodes: { total: 10, online: 9 },
}
const series = (n: number) =>
  Array.from({ length: n }, (_, i) => ({
    date: `2026-10-${String(i + 1).padStart(2, '0')}`,
    value: (i + 1) * 1000,
  }))

const listedUser: AdminUser = {
  ...admin,
  id: 42,
  email: 'listed@example.com',
  is_admin: false,
  remarks: '',
  invite_user_id: null,
  last_login_ip: '',
  telegram_id: null,
}

function respond(data: unknown, extra: Record<string, unknown> = {}) {
  return new Response(JSON.stringify({ data, ...extra }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

const fetchMock = vi.fn<typeof fetch>()

function route(url: URL): Response {
  const p = url.pathname.replace('/api/v1', '')
  switch (p) {
    case '/guest/config':
      return respond(config)
    case '/user/me':
      return respond(admin)
    case '/admin/stats/overview':
      return respond(overview)
    case '/admin/stats/series':
      return respond(series(url.searchParams.get('range') === '7d' ? 7 : 30))
    case '/admin/stats/nodes/ranking':
      return respond([{ node_id: 1, name: '香港 01', u: 2 * GB, d: 30 * GB, total: 32 * GB }])
    case '/admin/stats/users/ranking':
      return respond([{ user_id: 42, email: 'top@example.com', u: GB, d: 9 * GB, total: 10 * GB }])
    case '/admin/users':
      return respond([listedUser], { total: 1 })
    case '/admin/plans':
    case '/admin/groups':
      return respond([])
    default:
      return new Response(JSON.stringify({ code: 'not_found', message: p }), { status: 404 })
  }
}

function renderAt(url: string) {
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [url] }) })
  render(<RouterProvider router={router} />)
  return router
}

beforeEach(() => {
  useUiStore.setState({ locale: 'zh-CN', theme: 'system' })
  useAuthStore.setState({ token: TOKEN, user: admin })
  fetchMock.mockImplementation((input) => {
    const raw = input instanceof Request ? input.url : input.toString()
    return Promise.resolve(route(new URL(raw, 'http://localhost')))
  })
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => vi.unstubAllGlobals())

const requested = () =>
  fetchMock.mock.calls.map(([input]) => (input instanceof Request ? input.url : input.toString()))

describe('admin dashboard', () => {
  it('renders overview tiles, the trend chart and rankings from the API', { timeout: 30_000 }, async () => {
    renderAt('/admin/dashboard')
    // first render compiles the lazy route chunks, dictionaries and HeroUI modules
    expect(
      await screen.findByRole('heading', { level: 1, name: '概览' }, { timeout: 15_000 }),
    ).toBeInTheDocument()
    expect(await screen.findByText('¥456.00')).toBeInTheDocument()
    expect(screen.getByText('¥12,345.00')).toBeInTheDocument()
    expect(screen.getByText('9 / 10')).toBeInTheDocument()
    expect(screen.getByText('活跃 41 · 在线 17')).toBeInTheDocument()

    const chart = await screen.findByRole('img', { name: '收入趋势（近 30 天）' })
    expect(chart.tagName.toLowerCase()).toBe('svg')
    const table = screen.getByRole('table', { name: '收入趋势（近 30 天）' })
    expect(within(table).getAllByRole('row')).toHaveLength(31)
    expect(within(table).getByText('¥300.00')).toBeInTheDocument()

    expect(await screen.findByRole('img', { name: /^收入近 7 天趋势/ })).toBeInTheDocument()
    expect(await screen.findByText('香港 01')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'top@example.com' })).toHaveAttribute(
      'href',
      '/admin/users?email=top%40example.com',
    )

    const calls = requested()
    expect(calls.some((u) => u.includes('/admin/stats/series?metric=income&range=30d'))).toBe(true)
    expect(calls.some((u) => /\/admin\/stats\/users\/ranking\?date=\d{4}-\d\d-\d\d&limit=10/.test(u))).toBe(
      true,
    )
  })
})

describe('admin users page', () => {
  it('turns URL filters into the list query', { timeout: 30_000 }, async () => {
    renderAt('/admin/users?banned=true&role=admin&sort=-balance')
    expect(
      await screen.findByRole('heading', { level: 1, name: '用户' }, { timeout: 15_000 }),
    ).toBeInTheDocument()
    expect(await screen.findByText('listed@example.com')).toBeInTheDocument()
    await waitFor(() =>
      expect(requested()).toContain(
        '/api/v1/admin/users?page=1&page_size=20&sort=-balance&banned=true&is_admin=true',
      ),
    )
    expect(screen.getByText('共 1 个用户')).toBeInTheDocument()
  })
})
