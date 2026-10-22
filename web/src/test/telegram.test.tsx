/**
 * FE-S: profile → Telegram card. Binding fetches a one-time code (deep link + QR), polls `GET /user/telegram`
 * every 3 s only while the dialog is open (fake timers), and closes itself once the account is bound.
 */
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConfirmHost } from '@/components/feedback/ConfirmHost'
import { TELEGRAM_POLL_MS, TelegramCard } from '@/features/security/TelegramCard'
import { loadNamespaces } from '@/i18n'
import { createQueryClient } from '@/lib/queryClient'
import { bodiesOf, json, mockApi, resetStores, signIn } from './fe1-helpers'

type Status = {
  enabled: boolean
  bound: boolean
  username?: string
  bot_username?: string
  discuss_link: string
}

const CODE = {
  code: 'K7Q2M9XA',
  bot_username: 'gg_bot',
  deep_link: 'https://t.me/gg_bot?start=K7Q2M9XA',
}

function setup(initial: Status, bindCode: unknown = CODE) {
  let status = initial
  const fetchMock = mockApi({
    'GET /user/telegram': () => status,
    'POST /user/telegram/bind-code': bindCode,
    'DELETE /user/telegram': () => {
      status = { ...status, bound: false }
      delete status.username
      return true
    },
  })
  const client = createQueryClient()
  render(
    <QueryClientProvider client={client}>
      <TelegramCard />
      <ConfirmHost />
    </QueryClientProvider>,
  )
  return {
    fetchMock,
    client,
    polls: () => bodiesOf(fetchMock, 'GET /user/telegram').length,
    bind: (username: string) => {
      status = { ...status, bound: true, username }
    },
  }
}

describe('Telegram binding', () => {
  beforeAll(async () => {
    resetStores()
    await loadNamespaces(['security', 'common'])
  })
  beforeEach(() => {
    signIn()
    vi.useFakeTimers({
      shouldAdvanceTime: true,
      toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'],
    })
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('polls every 3 s while the bind dialog is open and closes once bound', async () => {
    const t = setup({
      enabled: true,
      bound: false,
      bot_username: 'gg_bot',
      discuss_link: 'https://t.me/example_group',
    })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    expect(await screen.findByText('未绑定')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /加入讨论群/ })).toHaveAttribute(
      'href',
      'https://t.me/example_group',
    )

    // Closed dialog: no polling.
    const idle = t.polls()
    await vi.advanceTimersByTimeAsync(TELEGRAM_POLL_MS * 3)
    expect(t.polls()).toBe(idle)

    await user.click(screen.getByRole('button', { name: '绑定 Telegram' }))
    const dialog = await screen.findByRole('dialog', { name: '绑定 Telegram' })
    expect(await within(dialog).findByRole('link', { name: '在 Telegram 中打开' })).toHaveAttribute(
      'href',
      'https://t.me/gg_bot?start=K7Q2M9XA',
    )
    expect(within(dialog).getByText('/start K7Q2M9XA')).toBeInTheDocument()
    expect(await within(dialog).findByRole('img', { name: 'Telegram 绑定二维码' })).toBeInTheDocument()
    expect(bodiesOf(t.fetchMock, 'POST /user/telegram/bind-code')).toHaveLength(1)

    const open = t.polls()
    await vi.advanceTimersByTimeAsync(TELEGRAM_POLL_MS)
    await waitFor(() => expect(t.polls()).toBe(open + 1))
    await vi.advanceTimersByTimeAsync(TELEGRAM_POLL_MS)
    await waitFor(() => expect(t.polls()).toBe(open + 2))
    expect(screen.getByRole('dialog', { name: '绑定 Telegram' })).toBeInTheDocument()

    // The user pressed Start in Telegram: the next poll sees it and the dialog closes.
    t.bind('tg_user')
    await vi.advanceTimersByTimeAsync(TELEGRAM_POLL_MS)
    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: '绑定 Telegram' })).not.toBeInTheDocument(),
    )
    expect(screen.getByText('已绑定 @tg_user')).toBeInTheDocument()

    // Polling stops with the dialog.
    const after = t.polls()
    await vi.advanceTimersByTimeAsync(TELEGRAM_POLL_MS * 3)
    expect(t.polls()).toBe(after)
    t.client.clear()
  })

  it('unbinds after confirmation', async () => {
    const t = setup({
      enabled: true,
      bound: true,
      username: 'tg_user',
      bot_username: 'gg_bot',
      discuss_link: '',
    })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await user.click(await screen.findByRole('button', { name: '解除绑定' }))
    const confirm = await screen.findByRole('alertdialog', { name: '解除 Telegram 绑定？' })
    await user.click(within(confirm).getByRole('button', { name: '解除绑定' }))
    expect(await screen.findByText('未绑定')).toBeInTheDocument()
    expect(bodiesOf(t.fetchMock, 'DELETE /user/telegram')).toHaveLength(1)
    expect(screen.queryByRole('link', { name: /加入讨论群/ })).not.toBeInTheDocument()
    t.client.clear()
  })

  it('shows a muted state instead of the bind button when no bot is configured', async () => {
    const t = setup({ enabled: false, bound: false, discuss_link: '' })
    expect(await screen.findByText('管理员尚未启用 Telegram。')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '绑定 Telegram' })).not.toBeInTheDocument()
    t.client.clear()
  })

  it('explains a Telegram outage (502 telegram_unavailable) and offers a retry', async () => {
    const t = setup(
      { enabled: true, bound: false, bot_username: 'gg_bot', discuss_link: '' },
      json({ code: 'telegram_unavailable', message: 'telegram is unreachable' }, 502),
    )
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await user.click(await screen.findByRole('button', { name: '绑定 Telegram' }))
    const dialog = await screen.findByRole('dialog', { name: '绑定 Telegram' })
    expect(await within(dialog).findByText(/暂时无法连接 Telegram/)).toHaveAttribute('role', 'alert')
    expect(within(dialog).getByRole('button', { name: '重新获取绑定码' })).toBeInTheDocument()
    t.client.clear()
  })

  it('renders nothing when the server has no Telegram endpoint', async () => {
    mockApi({ 'GET /user/telegram': json({ code: 'not_found', message: 'not found' }, 404) })
    const client = createQueryClient()
    const { container } = render(
      <QueryClientProvider client={client}>
        <TelegramCard />
      </QueryClientProvider>,
    )
    await waitFor(() => expect(container).toBeEmptyDOMElement())
    client.clear()
  })
})
