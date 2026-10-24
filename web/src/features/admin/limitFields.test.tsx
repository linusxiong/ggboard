/**
 * The connection-limit field in the plan sheet and the create-user dialog: loaded from the API,
 * sent on save, cleared to `null`, and explained behind the label's ⓘ (not on the page).
 */
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { makePlan } from '@/features/shop/testing'
import { loadNamespaces } from '@/i18n'
import type { AdminPlan } from '@/lib/api/types'
import { createQueryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import config from '../../../scripts/fixtures/guest-config.json'
import { PlanSheet } from './plans/PlanSheet'
import { CreateUserDialog } from './users/UserDialogs'

const plan: AdminPlan = { ...makePlan({ id: 2, name: 'Pro', group_id: 3, conn_limit: 128 }), user_count: 4 }

const fetchMock = vi.fn<typeof fetch>()
const sent = (method: string, path: string) =>
  fetchMock.mock.calls
    .filter(([input, init]) => {
      const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
      return (init?.method ?? 'GET').toUpperCase() === method && url.pathname === `/api/v1${path}`
    })
    .map(
      ([, init]) =>
        JSON.parse(typeof init?.body === 'string' ? init.body : 'null') as Record<string, unknown>,
    )

function respond(data: unknown) {
  return new Response(JSON.stringify({ data }), { headers: { 'Content-Type': 'application/json' } })
}

function renderWithQuery(node: ReactNode) {
  return render(<QueryClientProvider client={createQueryClient()}>{node}</QueryClientProvider>)
}

const connInput = () => screen.findByRole('textbox', { name: /^连接数限制/ }, { timeout: 4000 })

/** Opens a field's ⓘ and returns its popover (the newest dialog; the drawer is a dialog too). */
async function openHelp(u: ReturnType<typeof userEvent.setup>, subject: string) {
  const before = screen.queryAllByRole('dialog').length
  await u.click(screen.getByRole('button', { name: `关于「${subject}」的说明` }))
  return waitFor(() => {
    const dialogs = screen.getAllByRole('dialog', { hidden: true })
    expect(dialogs.length).toBeGreaterThan(before)
    return dialogs.at(-1)!
  })
}

describe('connection limit fields', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['admin-plans', 'admin-users', 'common'])
  })
  beforeEach(() => {
    useAuthStore.setState({ token: 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig', user: null })
    fetchMock.mockReset()
    fetchMock.mockImplementation((input, init) => {
      const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
      const key = `${(init?.method ?? 'GET').toUpperCase()} ${url.pathname.replace('/api/v1', '')}`
      if (key === 'GET /guest/config') return Promise.resolve(respond(config))
      if (key === 'GET /admin/groups') return Promise.resolve(respond([{ id: 3, name: '默认组' }]))
      if (key === 'GET /admin/plans') return Promise.resolve(respond([plan]))
      if (key === 'PATCH /admin/plans/2') return Promise.resolve(respond(plan))
      if (key === 'POST /admin/users') return Promise.resolve(respond({ id: 99 }))
      return Promise.resolve(new Response('{"code":"not_found","message":"x"}', { status: 404 }))
    })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    useAuthStore.setState({ token: null, user: null })
  })

  it('plan sheet: loads, edits and clears the connection limit', async () => {
    const u = userEvent.setup()
    const view = renderWithQuery(<PlanSheet plan={plan} isOpen onOpenChange={() => undefined} />)
    const input = await connInput()
    expect(input).toHaveValue('128')

    // The explanation sits behind the ⓘ (on the page it is only the input's sr-only description);
    // the device field's ⓘ explains the relay caveat.
    expect(screen.getByText(/仅 soga 执行/)).toHaveClass('sr-only')
    const connHelp = await openHelp(u, '连接数限制')
    expect(connHelp).toHaveTextContent('仅 soga 执行（下发为 tcp_limit），V2bX 与 XrayR 会忽略')
    expect(within(connHelp).queryByRole('button', { name: '保存' })).toBeNull()
    await u.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByText(/仅 soga 执行/, { selector: '[role=dialog]' })).toBeNull())
    expect(await openHelp(u, '设备数')).toHaveTextContent('除非中转发送 PROXY protocol')
    await u.keyboard('{Escape}')

    await u.clear(input)
    await u.type(input, '64')
    await u.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(sent('PATCH', '/admin/plans/2')).toHaveLength(1))
    expect(sent('PATCH', '/admin/plans/2')[0]).toMatchObject({ conn_limit: 64, device_limit: 3 })
    view.unmount()

    // Emptying the field clears the limit: PATCH sends an explicit null.
    renderWithQuery(<PlanSheet plan={plan} isOpen onOpenChange={() => undefined} />)
    await u.clear(await connInput())
    await u.click(screen.getByRole('button', { name: '保存' }))
    await waitFor(() => expect(sent('PATCH', '/admin/plans/2')).toHaveLength(2))
    const cleared = sent('PATCH', '/admin/plans/2')[1]!
    expect(cleared).toHaveProperty('conn_limit', null)
  })

  it('create-user dialog: sends the connection limit only when filled in', async () => {
    const u = userEvent.setup()
    renderWithQuery(<CreateUserDialog isOpen onOpenChange={() => undefined} onCreated={() => undefined} />)
    const dialog = within(await screen.findByRole('dialog', {}, { timeout: 4000 }))
    await u.type(dialog.getByRole('textbox', { name: /^邮箱/ }), 'new@example.com')
    await u.type(dialog.getByLabelText(/^密码/), 'password-123')
    await u.type(await connInput(), '32')
    await u.click(dialog.getByRole('button', { name: '新建' }))
    await waitFor(() => expect(sent('POST', '/admin/users')).toHaveLength(1))
    expect(sent('POST', '/admin/users')[0]).toEqual({
      email: 'new@example.com',
      password: 'password-123',
      conn_limit: 32,
    })
  })
})
