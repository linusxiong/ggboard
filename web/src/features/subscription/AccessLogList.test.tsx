import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { AdminUser, SubscriptionAccess } from '@/lib/api/types'
import { createQueryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import fixtureUser from '../../../scripts/fixtures/user.json'
import { UserSubscriptionTab } from '../admin/users/UserSubscriptionTab'
import { AccessCard } from './AccessCard'
import { AccessLogList } from './AccessLogList'

const LABELS = {
  ip: 'IP',
  client: '客户端',
  format: '格式',
  time: '时间',
  unknownClient: '未知客户端',
  list: '访问记录',
  location: '归属地',
}

const at = new Date().toISOString()
const ROWS: SubscriptionAccess[] = [
  { ip: '203.0.113.9', client: 'mihomo/1.19.27', format: 'clash-meta', at, location: '中国 广东 深圳 电信' },
  { ip: '2001:db8:1:2::5', client: 'FlClash/0.9.1', format: 'clash-meta', at, location: '' },
  { ip: '198.51.100.7', client: 'sing-box/1.14.4', format: 'singbox', at }, // older server: no field
]

function mockFetch(routes: Record<string, unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
      const data =
        routes[`${(init?.method ?? 'GET').toUpperCase()} ${url.pathname.replace(/^\/api\/v1/, '')}`]
      return Promise.resolve(
        data === undefined
          ? new Response(JSON.stringify({ code: 'not_found', message: 'nope' }), { status: 404 })
          : new Response(JSON.stringify({ data }), { status: 200 }),
      )
    }),
  )
}

describe('AccessLogList locations', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    useAuthStore.setState({ token: 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig', user: null })
    await loadNamespaces(['admin-users', 'subscription', 'common'])
  })
  afterEach(() => vi.unstubAllGlobals())

  it('shows the location as secondary text next to the IP, with a screen-reader label', () => {
    render(<AccessLogList entries={ROWS} labels={LABELS} />)
    const items = within(screen.getByRole('list', { name: '访问记录' })).getAllByRole('listitem')
    expect(items).toHaveLength(3)

    const place = within(items[0]!).getByText('中国 广东 深圳 电信')
    expect(place).toHaveAttribute('title', '中国 广东 深圳 电信')
    expect(place).toHaveClass('text-label-secondary', 'truncate')
    expect(place).not.toHaveClass('font-mono')
    expect(within(place).getByText('归属地:')).toHaveClass('sr-only')
    // Same line as the IP (one wrapping row), not the client line.
    expect(place.parentElement).toBe(within(items[0]!).getByText('203.0.113.9').parentElement)

    // Unknown ("") or absent: nothing rendered, no dangling label.
    for (const item of items.slice(1)) expect(within(item).queryByText('归属地:')).toBeNull()
  })

  it('labels the location in the admin drawer and on the user security card', async () => {
    mockFetch({
      'GET /admin/users/7/subscription/access-log': [ROWS[0]],
      'GET /admin/users/7/subscription/age': {
        enabled: false,
        public_key: null,
        private_key: null,
        created_at: null,
      },
      'GET /user/subscription/access-log': [{ ...ROWS[0], ip: '203.0.113.*' }],
    })
    const client = createQueryClient()
    const user = { ...fixtureUser, id: 7, email: 'u7@example.com' } as AdminUser
    render(
      <QueryClientProvider client={client}>
        <UserSubscriptionTab user={user} />
        <AccessCard />
      </QueryClientProvider>,
    )
    const admin = await screen.findByRole('list', { name: '订阅访问记录' })
    expect(within(within(admin).getByText('中国 广东 深圳 电信')).getByText('归属地:')).toHaveClass('sr-only')
    const own = await screen.findByRole('list', { name: '最近访问' })
    expect(within(own).getByText('203.0.113.*')).toBeInTheDocument()
    expect(within(within(own).getByText('中国 广东 深圳 电信')).getByText('归属地:')).toHaveClass('sr-only')
    client.clear()
  })
})
