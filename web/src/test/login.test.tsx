import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { routeTree } from '@/routeTree.gen'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import config from '../../scripts/fixtures/guest-config.json'

function json(data: unknown, status = 200) {
  return new Response(JSON.stringify(status === 200 ? { data } : data), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('login page (smoke)', () => {
  const fetchMock = vi.fn<typeof fetch>()

  beforeEach(() => {
    useUiStore.setState({ locale: 'zh-CN', theme: 'system' })
    useAuthStore.setState({ token: null, user: null })
    fetchMock.mockImplementation((input) => {
      const url = input instanceof Request ? input.url : input.toString()
      if (url.endsWith('/guest/config')) return Promise.resolve(json(config))
      if (url.endsWith('/passport/login'))
        return Promise.resolve(json({ code: 'unauthorized', message: '邮箱或密码错误' }, 401))
      return Promise.resolve(json({ code: 'not_found', message: 'nope' }, 404))
    })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  it('renders the form, validates and shows the server error', async () => {
    const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: ['/login'] }) })
    render(<RouterProvider router={router} />)

    expect(await screen.findByRole('heading', { level: 1, name: '登录' })).toBeInTheDocument()
    const email = screen.getByLabelText(/邮箱/)
    const password = screen.getByLabelText(/密码/)
    expect(email).toHaveAttribute('autocomplete', 'username webauthn')
    expect(password).toHaveAttribute('type', 'password')
    expect(screen.getByRole('link', { name: '立即注册' })).toHaveAttribute('href', '/register')

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '登录' }))
    expect(await screen.findByText('请输入邮箱')).toBeInTheDocument()
    expect(email).toHaveAttribute('aria-invalid', 'true')

    await user.type(email, 'a@example.com')
    await user.type(password, 'wrong-password')
    await user.click(screen.getByRole('button', { name: '登录' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('邮箱或密码错误')
    await waitFor(() => expect(useAuthStore.getState().token).toBeNull())
  })
})
