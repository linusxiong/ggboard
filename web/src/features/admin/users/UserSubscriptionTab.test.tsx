import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { ConfirmHost } from '@/components/feedback/ConfirmHost'
import { loadNamespaces } from '@/i18n'
import type { AdminUser } from '@/lib/api/types'
import { createQueryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import fixtureUser from '../../../../scripts/fixtures/user.json'
import { UserSubscriptionTab } from './UserSubscriptionTab'

const USER = {
  ...fixtureUser,
  id: 7,
  email: 'u7@example.com',
  remarks: '',
  invite_user_id: null,
  last_login_ip: '',
  telegram_id: null,
} as AdminUser

function mockFetch(routes: Record<string, unknown>) {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
      const key = `${(init?.method ?? 'GET').toUpperCase()} ${url.pathname.replace(/^\/api\/v1/, '')}`
      calls.push(key)
      const data = routes[key]
      return Promise.resolve(
        data === undefined
          ? new Response(JSON.stringify({ code: 'not_found', message: 'nope' }), { status: 404 })
          : new Response(JSON.stringify({ data }), { status: 200 }),
      )
    }),
  )
  return calls
}

describe('admin user drawer → subscription tab', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    useAuthStore.setState({ token: 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig', user: null })
    await loadNamespaces(['admin-users', 'common'])
  })
  afterEach(() => vi.unstubAllGlobals())

  it('shows full IPs and resets the age key / rotates via reset-secret after confirmation', async () => {
    const calls = mockFetch({
      'GET /admin/users/7/subscription/access-log': [
        {
          ip: '203.0.113.45',
          client: 'clash-verge/v2.2.3',
          format: 'clash-meta',
          at: new Date().toISOString(),
          ip_bucket: '203.0.113.45',
          token_tag: '3f9a1c07',
        },
      ],
      'GET /admin/users/7/subscription/age': {
        enabled: true,
        public_key: 'age1adminviewxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        private_key: null,
        created_at: '2026-10-22T08:00:00Z',
      },
      'POST /admin/users/7/subscription/age/reset': {
        enabled: true,
        public_key: 'age1newkeyxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
        private_key: null,
        created_at: '2026-10-22T09:00:00Z',
      },
      'POST /admin/users/7/reset-secret': USER,
    })
    const client = createQueryClient()
    render(
      <QueryClientProvider client={client}>
        <UserSubscriptionTab user={USER} />
        <ConfirmHost />
      </QueryClientProvider>,
    )
    const user = userEvent.setup()
    const list = await screen.findByRole('list', { name: '订阅访问记录' })
    expect(within(list).getByText('203.0.113.45')).toBeInTheDocument()
    expect(screen.getByText('近 24 小时 1 个不同 IP')).toBeInTheDocument()
    expect(within(list).getByText('#3f9a1c07')).toBeInTheDocument()
    expect(await screen.findByText(/age1adminview/)).toBeInTheDocument()
    expect(screen.getByText('已开启')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '重置 age 密钥' }))
    await user.click(
      within(await screen.findByRole('alertdialog', { name: '重置 u7@example.com 的 age 密钥？' })).getByRole(
        'button',
        { name: '重置 age 密钥' },
      ),
    )
    await waitFor(() => expect(calls).toContain('POST /admin/users/7/subscription/age/reset'))

    await user.click(screen.getByRole('button', { name: '轮换订阅链接' }))
    const confirm = await screen.findByRole('alertdialog', { name: '轮换 u7@example.com 的订阅链接？' })
    expect(within(confirm).getByText(/此操作等同于“重置订阅密钥”/)).toBeInTheDocument()
    await user.click(within(confirm).getByRole('button', { name: '轮换订阅链接' }))
    await waitFor(() => expect(calls).toContain('POST /admin/users/7/reset-secret'))
    client.clear()
  })

  it('says so when the server has no access-log endpoint', async () => {
    mockFetch({})
    const client = createQueryClient()
    render(
      <QueryClientProvider client={client}>
        <UserSubscriptionTab user={USER} />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('后端未提供访问记录接口。')).toBeInTheDocument()
    // No age endpoint either: the reset button stays usable and the status section is hidden.
    expect(screen.queryByText('订阅加密')).not.toBeInTheDocument()
    client.clear()
  })
})
