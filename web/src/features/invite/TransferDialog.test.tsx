/** Moving commission to the balance is refused (409) while a withdrawal request is open: say why, in the UI language. */
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import { createQueryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import config from '../../../scripts/fixtures/guest-config.json'
import TransferDialog from './TransferDialog'

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

describe('commission transfer dialog', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['invite', 'common'])
  })
  beforeEach(() => {
    useAuthStore.setState({ token: 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig', user: null })
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
        const key = `${(init?.method ?? 'GET').toUpperCase()} ${url.pathname}`
        if (key === 'GET /api/v1/guest/config') return Promise.resolve(json({ data: config }))
        if (key === 'POST /api/v1/user/commission/transfer')
          return Promise.resolve(
            json({ code: 'conflict', message: 'your withdrawal request is still open' }, 409),
          )
        return Promise.resolve(json({ code: 'not_found', message: 'x' }, 404))
      }),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    useAuthStore.setState({ token: null, user: null })
  })

  it('explains an open withdrawal request', async () => {
    render(
      <QueryClientProvider client={createQueryClient()}>
        <TransferDialog isOpen onOpenChange={() => {}} available={10000} />
      </QueryClientProvider>,
    )
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: '确认转入' }))
    expect(
      await screen.findByText('你的提现申请仍在处理中，工单关闭后才能把佣金转入余额。'),
    ).toBeInTheDocument()
    expect(useAuthStore.getState().token).not.toBeNull()
  })
})
