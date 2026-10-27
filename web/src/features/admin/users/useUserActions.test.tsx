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
import { useUserActions } from './useUserActions'

const USER = {
  ...fixtureUser,
  id: 7,
  email: 'u7@example.com',
  remarks: '',
  invite_user_id: null,
  last_login_ip: '',
  telegram_id: null,
} as AdminUser

function ResetButton() {
  const actions = useUserActions()
  return (
    <button type="button" onClick={() => void actions.run('resetSecret', USER)}>
      reset
    </button>
  )
}

function mockFetch() {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
      calls.push(`${(init?.method ?? 'GET').toUpperCase()} ${url.pathname.replace(/^\/api\/v1/, '')}`)
      return Promise.resolve(new Response(JSON.stringify({ data: USER }), { status: 200 }))
    }),
  )
  return calls
}

async function openResetConfirm(title: string) {
  const client = createQueryClient()
  render(
    <QueryClientProvider client={client}>
      <ResetButton />
      <ConfirmHost />
    </QueryClientProvider>,
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'reset' }))
  return { client, user, dialog: await screen.findByRole('alertdialog', { name: title }) }
}

describe('admin user actions → reset subscription secret', () => {
  beforeAll(async () => {
    useAuthStore.setState({ token: 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig', user: null })
    await loadNamespaces(['admin-users', 'common'], 'zh-CN')
    await loadNamespaces(['admin-users', 'common'], 'en')
  })
  afterEach(() => vi.unstubAllGlobals())

  it('tells the admin the user is signed out and notified by email and Telegram', async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    const calls = mockFetch()
    const { client, user, dialog } = await openResetConfirm('重置 u7@example.com 的订阅密钥？')
    expect(
      within(dialog).getByText(
        'UUID 与订阅 Token 会重新生成，用户会被登出，并收到邮件和 Telegram（如已绑定）通知，需要重新导入订阅。',
      ),
    ).toBeInTheDocument()
    expect(calls).toEqual([])
    await user.click(within(dialog).getByRole('button', { name: '重置订阅密钥' }))
    await waitFor(() => expect(calls).toContain('POST /admin/users/7/reset-secret'))
    client.clear()
  })

  it('says the same in English', async () => {
    useUiStore.setState({ locale: 'en' })
    mockFetch()
    const { client, dialog } = await openResetConfirm('Reset the subscription secret of u7@example.com?')
    expect(within(dialog).getByText(/notified by email and Telegram \(if linked\)/)).toBeInTheDocument()
    expect(within(dialog).getByText(/the user is signed out/)).toBeInTheDocument()
    client.clear()
  })
})
