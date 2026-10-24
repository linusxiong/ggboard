import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { User } from '@/lib/api/types'
import { createQueryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import fixtureUser from '../../../../scripts/fixtures/user.json'
import { KnowledgeDialog } from './KnowledgeDialog'

const TOKEN = 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig'
const SUBSCRIPTION_PLACEHOLDERS = [
  '{{subscribeUrl}}',
  '{{urlEncodeSubscribeUrl}}',
  '{{safeBase64SubscribeUrl}}',
]

function renderDialog(user: Partial<User>) {
  useAuthStore.setState({ token: TOKEN, user: { ...(fixtureUser as User), ...user } })
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <KnowledgeDialog isOpen onOpenChange={() => undefined} categories={new Map()} />
    </QueryClientProvider>,
  )
}

async function toolbar() {
  return within(await screen.findByRole('toolbar', { name: '格式工具' }))
}

describe('knowledge editor placeholders', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['admin-support', 'common'])
  })
  beforeEach(() => vi.stubGlobal('fetch', vi.fn()))
  afterEach(() => {
    vi.unstubAllGlobals()
    useAuthStore.setState({ token: null, user: null })
  })

  it('offers admins every placeholder', async () => {
    renderDialog({ is_admin: true, is_staff: false })
    const bar = await toolbar()
    for (const p of ['{{siteName}}', ...SUBSCRIPTION_PLACEHOLDERS])
      expect(bar.getByRole('button', { name: p })).toBeInTheDocument()
    // The placeholder reference sits behind the toolbar's ⓘ, not on the page.
    expect(screen.queryByText(/订阅占位符仅管理员可用/)).toBeNull()
    await userEvent.click(bar.getByRole('button', { name: '关于「正文」的说明' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('订阅占位符仅管理员可用')
  })

  it('hides the subscription-URL placeholders from staff', async () => {
    renderDialog({ is_admin: false, is_staff: true })
    const bar = await toolbar()
    expect(bar.getByRole('button', { name: '{{siteName}}' })).toBeInTheDocument()
    for (const p of SUBSCRIPTION_PLACEHOLDERS) expect(bar.queryByRole('button', { name: p })).toBeNull()
  })

  it("shows the server's 403 `errors.body` on the body field", async () => {
    const fetchMock = vi.fn<typeof fetch>(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            code: 'forbidden',
            message: 'forbidden',
            errors: { body: ['subscription placeholders are admin-only'] },
          }),
          { status: 403, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderDialog({ is_admin: false, is_staff: true })
    const user = userEvent.setup()
    await user.type(await screen.findByRole('textbox', { name: /^标题/ }), 'Clash 教程')
    await user.click(screen.getByRole('textbox', { name: /^正文/ }))
    await user.paste('导入：{{subscribeUrl}}') // typed by hand: the toolbar no longer offers it
    await user.type(screen.getByRole('combobox', { name: '分类' }), '入门')
    await user.tab() // close the suggestions popover (it hides the rest of the dialog from queries)
    await user.click(screen.getByRole('button', { name: '发布' }))

    expect(await screen.findByText('subscription placeholders are admin-only')).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: /^正文/ })).toHaveAttribute('aria-invalid', 'true')
    const [, init] = fetchMock.mock.calls[0] ?? []
    expect(init?.method).toBe('POST')
  })
})
