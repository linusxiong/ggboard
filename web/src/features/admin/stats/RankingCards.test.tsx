/**
 * The ranking date is a server calendar day: an admin whose browser is a day behind the server
 * (New York vs. a `TZ=Asia/Shanghai` server) gets the server's today as default and maximum, and
 * every ranking request names that day.
 */
import { QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { AdminOverview } from '@/lib/api/types'
import { createQueryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import config from '../../../../scripts/fixtures/guest-config.json'
import { AdminDashboard } from './Dashboard'
import { offsetName } from './zone'

const GB = 1024 ** 3
/** The browser's clock: Oct 26, 18:55 local time, whatever zone the test runs in. */
const BROWSER_NOW = new Date(2026, 9, 26, 18, 55)

const overview = (date: string): AdminOverview => ({
  today: { date, income: 0, paid_orders: 0, registers: 0, commission: 0 },
  month: { income: 0, paid_orders: 0, registers: 0 },
  totals: {
    users: 1,
    active_users: 1,
    online_users: 0,
    pending_tickets: 0,
    pending_orders: 0,
    pending_commission: 0,
  },
  nodes: { total: 1, online: 1 },
  timezone: { name: 'Asia/Shanghai', offset: 8 * 3600 },
})

function respond(data: unknown) {
  return new Response(JSON.stringify({ data }), { headers: { 'Content-Type': 'application/json' } })
}

const fetchMock = vi.fn<typeof fetch>()
const urlOf = (input: Parameters<typeof fetch>[0]) =>
  new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
const rankingRequests = () =>
  fetchMock.mock.calls.map(([input]) => urlOf(input)).filter((u) => u.pathname.includes('/ranking'))
const rankingDates = () => new Set(rankingRequests().map((u) => u.searchParams.get('date')))

/** `overviewAnswer` answers `/admin/stats/overview` (a Response, or a promise of one). */
let overviewAnswer: () => Response | Promise<Response>

function renderDashboard() {
  const rootRoute = createRootRoute({ component: AdminDashboard })
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory() })
  render(
    <QueryClientProvider client={createQueryClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const dateInput = () => screen.findByLabelText<HTMLInputElement>('日期', {}, { timeout: 10_000 })

describe('ranking date', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['admin-stats', 'common'])
  })
  beforeEach(() => {
    // Only Date is faked: the browser's calendar day is the 26th; timers and promises run normally.
    vi.useFakeTimers({ toFake: ['Date'], now: BROWSER_NOW })
    useAuthStore.setState({ token: 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig', user: null })
    overviewAnswer = () => respond(overview('2026-10-27'))
    fetchMock.mockReset()
    fetchMock.mockImplementation((input) => {
      const url = urlOf(input)
      switch (url.pathname.replace('/api/v1', '')) {
        case '/guest/config':
          return Promise.resolve(respond(config))
        case '/admin/stats/overview':
          return Promise.resolve(overviewAnswer())
        case '/admin/stats/series':
          return Promise.resolve(respond([]))
        case '/admin/stats/nodes/ranking':
          return Promise.resolve(respond([{ node_id: 1, name: '香港 01', u: GB, d: 2 * GB, total: 3 * GB }]))
        case '/admin/stats/users/ranking':
          return Promise.resolve(respond([]))
        default:
          return Promise.resolve(new Response('{"code":"not_found","message":"x"}', { status: 404 }))
      }
    })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    useAuthStore.setState({ token: null, user: null })
  })

  it('defaults to and stops at the server day, not the browser day', { timeout: 30_000 }, async () => {
    let release!: () => void
    const answered = new Promise<void>((resolve) => (release = resolve))
    overviewAnswer = () => answered.then(() => respond(overview('2026-10-27')))
    renderDashboard()

    const input = await dateInput()
    expect(input).toHaveAttribute('type', 'date')
    // Before the overview answers, no ranking is requested for a guessed (browser) day.
    expect(input).toHaveValue('')
    expect(rankingRequests()).toHaveLength(0)

    release()
    await waitFor(() => expect(input).toHaveValue('2026-10-27'))
    expect(input).toHaveAttribute('max', '2026-10-27')
    expect(await screen.findByText('香港 01')).toBeInTheDocument()
    await waitFor(() =>
      expect(rankingRequests().map((u) => u.pathname + u.search)).toEqual(
        expect.arrayContaining([
          '/api/v1/admin/stats/nodes/ranking?date=2026-10-27',
          '/api/v1/admin/stats/users/ranking?date=2026-10-27&limit=10',
        ]),
      ),
    )
    expect(rankingDates()).toEqual(new Set(['2026-10-27']))
  })

  it(
    'keeps a picked day across refreshes while the maximum follows the server',
    { timeout: 30_000 },
    async () => {
      const u = userEvent.setup()
      renderDashboard()
      const input = await dateInput()
      await waitFor(() => expect(input).toHaveValue('2026-10-27'))
      const refresh = () => u.click(screen.getByRole('button', { name: '刷新' }))

      // Midnight on the server: the unpicked date follows to the new day.
      overviewAnswer = () => respond(overview('2026-10-28'))
      await refresh()
      await waitFor(() => expect(input).toHaveAttribute('max', '2026-10-28'))
      expect(input).toHaveValue('2026-10-28')
      await waitFor(() => expect(rankingDates()).toContain('2026-10-28'))

      // A picked past day stays through the next refresh and rollover; only the maximum moves.
      fireEvent.change(input, { target: { value: '2026-10-26' } })
      await waitFor(() => expect(rankingDates()).toContain('2026-10-26'))
      overviewAnswer = () => respond(overview('2026-10-29'))
      await refresh()
      await waitFor(() => expect(input).toHaveAttribute('max', '2026-10-29'))
      expect(input).toHaveValue('2026-10-26')

      // Clearing the field goes back to the server's today.
      fireEvent.change(input, { target: { value: '' } })
      await waitFor(() => expect(input).toHaveValue('2026-10-29'))
      await waitFor(() => expect(rankingDates()).toContain('2026-10-29'))
      expect(rankingDates()).not.toContain(null)
    },
  )

  it('names the server time zone behind the ranking ⓘ', { timeout: 30_000 }, async () => {
    const u = userEvent.setup()
    renderDashboard()
    await waitFor(async () => expect(await dateInput()).toHaveValue('2026-10-27'))
    const before = screen.queryAllByRole('dialog').length
    await u.click(screen.getByRole('button', { name: '关于「流量排行」的说明' }))
    const popover = await waitFor(() => {
      const dialogs = screen.getAllByRole('dialog')
      expect(dialogs.length).toBeGreaterThan(before)
      return dialogs.at(-1)!
    })
    expect(popover).toHaveTextContent(
      '统计日期以服务器时区为准：Asia/Shanghai（UTC+08:00）。今日、趋势和排行都按服务器的日期计算，可能与你的本地日期不同。',
    )
  })

  it('ranks the server today without a date when the overview fails', { timeout: 30_000 }, async () => {
    overviewAnswer = () => new Response('{"code":"forbidden","message":"x"}', { status: 403 })
    renderDashboard()
    const input = await dateInput()
    expect(await screen.findByText('香港 01')).toBeInTheDocument()
    expect(rankingDates()).toEqual(new Set([null]))
    expect(input).toHaveValue('')
    expect(input).not.toHaveAttribute('max')
  })
})

describe('offsetName', () => {
  it('formats offsets like the server does for unnamed zones', () => {
    expect(offsetName(0)).toBe('UTC')
    expect(offsetName(8 * 3600)).toBe('UTC+08:00')
    expect(offsetName(-4 * 3600)).toBe('UTC-04:00')
    expect(offsetName(-(3 * 3600 + 30 * 60))).toBe('UTC-03:30')
    expect(offsetName(5 * 3600 + 45 * 60)).toBe('UTC+05:45')
  })
})
