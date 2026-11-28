import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import { createQueryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { MailUsersDialog } from './UserDialogs'

type Reply = { status: number; body: unknown }

function mockFetch(reply: Reply) {
  const posts: { body: Record<string, unknown>; lang: string | null }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
      if (init?.method === 'POST' && url.pathname === '/api/v1/admin/users/mail') {
        posts.push({
          body: JSON.parse(typeof init.body === 'string' ? init.body : '{}') as Record<string, unknown>,
          lang: new Headers(init.headers).get('Accept-Language'),
        })
        return Promise.resolve(new Response(JSON.stringify(reply.body), { status: reply.status }))
      }
      return Promise.resolve(new Response(JSON.stringify({ code: 'not_found', message: 'nope' }), { status: 404 }))
    }),
  )
  return posts
}

function renderDialog(onOpenChange = vi.fn()) {
  render(
    <QueryClientProvider client={createQueryClient()}>
      <MailUsersDialog
        isOpen
        onOpenChange={onOpenChange}
        filters={{ email: 'vip' }}
        matching={3}
        filterSummary={['邮箱：vip']}
      />
    </QueryClientProvider>,
  )
  return onOpenChange
}

describe('admin bulk message dialog', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    useAuthStore.setState({ token: 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig', user: null })
    await loadNamespaces(['admin-users', 'common', 'admin-support'])
  })
  afterEach(() => vi.unstubAllGlobals())

  it('sends Markdown with the chosen channels and the current filters', async () => {
    const posts = mockFetch({ status: 200, body: { data: { queued: 3, telegram: 2 } } })
    const onOpenChange = renderDialog()
    const user = userEvent.setup()
    await user.type(screen.getByRole('textbox', { name: /主题/ }), '维护通知')
    await user.type(screen.getByRole('textbox', { name: /内容/ }), '今晚 **23:00** 维护')
    await user.click(screen.getByRole('switch', { name: '同时发到 Telegram' }))
    await user.click(screen.getByRole('button', { name: '加入发送队列' }))
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(posts).toEqual([
      {
        body: {
          email: 'vip',
          subject: '维护通知',
          content: '今晚 **23:00** 维护',
          markdown: true,
          send_email: true,
          send_telegram: true,
        },
        lang: 'zh-CN',
      },
    ])
  })

  it('shows the server field error for the Telegram switch', async () => {
    mockFetch({
      status: 400,
      body: {
        code: 'validation_failed',
        message: '请检查填写的内容',
        errors: { send_telegram: ['未配置 Telegram 机器人（telegram_bot_token）'] },
      },
    })
    const onOpenChange = renderDialog()
    const user = userEvent.setup()
    await user.type(screen.getByRole('textbox', { name: /主题/ }), 's')
    await user.type(screen.getByRole('textbox', { name: /内容/ }), 'c')
    await user.click(screen.getByRole('switch', { name: '同时发到 Telegram' }))
    await user.click(screen.getByRole('button', { name: '加入发送队列' }))
    expect(await screen.findByText('未配置 Telegram 机器人（telegram_bot_token）')).toBeInTheDocument()
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })
})
