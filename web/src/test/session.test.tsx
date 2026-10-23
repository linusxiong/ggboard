/**
 * Session hand-over in one browser tab: whatever ends account A's session (a 401 from the API, the user menu),
 * the next account must never be served A's cached data, and the shell must not refetch without a token.
 */
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setUnauthorizedHandler } from '@/lib/api/client'
import type { User } from '@/lib/api/types'
import { queryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { GUEST_CONFIG, json, renderAt, resetStores, TEST_USER } from './fe1-helpers'

const TOKEN_A = 'a.eyJleHAiOjQxMDI0NDQ4MDB9.sig'
const TOKEN_B = 'b.eyJleHAiOjQxMDI0NDQ4MDB9.sig'
const userA = { ...TEST_USER, id: 1, email: 'alice@example.com', is_admin: false } as User
const userB = { ...TEST_USER, id: 2, email: 'bob@example.com', is_admin: false } as User

function subscription(plan: string, token: string) {
  return {
    subscribe_url: `https://panel.example.com/s/${token}`,
    token,
    plan: { id: 2, name: plan },
    transfer_enable: 100 * 1024 ** 3,
    u: 0,
    d: 0,
    expired_at: null,
    next_reset_at: null,
    reset_day: null,
    device_limit: 3,
    speed_limit: null,
  }
}

const accounts: Record<string, { user: User; subscription: ReturnType<typeof subscription> }> = {
  [TOKEN_A]: { user: userA, subscription: subscription('Plan of Alice', 'sub-alice') },
  [TOKEN_B]: { user: userB, subscription: subscription('Plan of Bob', 'sub-bob') },
}

interface Call {
  method: string
  path: string
  token: string | null
}

/** Answers by bearer token: a revoked or missing token gets 401 on `/user/*`. */
function mockServer() {
  const calls: Call[] = []
  const revoked = new Set<string>()
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>((input, init) => {
      const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
      const path = url.pathname.replace(/^\/api\/v1/, '')
      const method = (init?.method ?? 'GET').toUpperCase()
      const auth = (init?.headers as Record<string, string> | undefined)?.Authorization
      const token = auth ? auth.replace(/^Bearer /, '') : null
      calls.push({ method, path, token })
      const reply = (data: unknown, status = 200) => Promise.resolve(json(data, status))
      if (path === '/guest/config') return reply(GUEST_CONFIG)
      if (method === 'POST' && path === '/passport/login') return reply({ token: TOKEN_B, user: userB })
      const account = token && !revoked.has(token) ? accounts[token] : undefined
      if (!account) return reply({ code: 'unauthorized', message: 'token expired' }, 401)
      switch (path) {
        case '/user/me':
          return reply(account.user)
        case '/user/subscription':
          return reply(account.subscription)
        case '/user/stats':
          return reply({ pending_orders: 0, open_tickets: 0, notices_count: 0, invite_count: 0 })
        case '/user/logout':
          return reply(true)
        default:
          return reply([])
      }
    }),
  )
  return { calls, revoke: (token: string) => revoked.add(token) }
}

const privateKeys = () =>
  queryClient
    .getQueryCache()
    .getAll()
    .filter((q) => q.queryKey[0] !== 'guest')
    .map((q) => JSON.stringify(q.queryKey))

describe('session hand-over', () => {
  beforeEach(() => {
    resetStores()
    useAuthStore.getState().login(TOKEN_A, userA)
  })
  afterEach(() => {
    setUnauthorizedHandler(null)
    queryClient.clear()
    vi.unstubAllGlobals()
  })

  it('after a 401 eviction the next account sees only its own data', async () => {
    const server = mockServer()
    const { router } = renderAt('/dashboard')
    // What src/router.tsx registers for the app.
    setUnauthorizedHandler(() => {
      void router.navigate({ to: '/login', search: { redirect: '/dashboard' }, replace: true })
    })
    expect(
      await screen.findByRole('heading', { level: 2, name: 'Plan of Alice' }, { timeout: 8000 }),
    ).toBeInTheDocument()

    // The server revokes A's token; the next request answers 401 and the client ends the session.
    server.revoke(TOKEN_A)
    await act(() => queryClient.refetchQueries({ queryKey: ['user', 'stats'] }))
    expect(useAuthStore.getState().token).toBeNull()
    const email = await screen.findByLabelText('邮箱', undefined, { timeout: 8000 })
    expect(privateKeys()).toEqual([]) // the shell unmounted: nothing of A is left

    const user = userEvent.setup()
    await user.type(email, 'bob@example.com')
    await user.type(screen.getByLabelText('密码'), 'correct-horse')
    await user.click(screen.getByRole('button', { name: '登录' }))

    expect(
      await screen.findByRole('heading', { level: 2, name: 'Plan of Bob' }, { timeout: 8000 }),
    ).toBeInTheDocument()
    expect(screen.queryByText('Plan of Alice')).not.toBeInTheDocument()
    expect(server.calls).toContainEqual({ method: 'GET', path: '/user/subscription', token: TOKEN_B })
    // No view refetched without a token while the shell was leaving.
    expect(server.calls.filter((c) => c.path.startsWith('/user/') && c.token === null)).toEqual([])
  })

  it('logging out from the account menu leaves nothing behind and sends no unauthenticated refetch', async () => {
    const server = mockServer()
    const { router } = renderAt('/dashboard')
    expect(
      await screen.findByRole('heading', { level: 2, name: 'Plan of Alice' }, { timeout: 8000 }),
    ).toBeInTheDocument()

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '账户菜单' }))
    await user.click(await screen.findByRole('menuitem', { name: '退出登录' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/login'), { timeout: 8000 })
    expect(useAuthStore.getState().token).toBeNull()
    await waitFor(() => expect(privateKeys()).toEqual([]))
    expect(server.calls.filter((c) => c.path.startsWith('/user/') && c.token === null)).toEqual([])
  })
})
