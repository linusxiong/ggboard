/**
 * Plan sheet → "Apply to current users" (`force_update`): offered only when editing a plan someone holds,
 * confirmed with the number of holders before saving, sent as one PATCH, and never persisted.
 */
import { type QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { useConfirmStore } from '@/components/feedback/confirm'
import { ConfirmHost } from '@/components/feedback/ConfirmHost'
import { makePlan } from '@/features/shop/testing'
import { loadNamespaces } from '@/i18n'
import { qk } from '@/lib/api/queries'
import type { AdminPlan } from '@/lib/api/types'
import { createQueryClient } from '@/lib/queryClient'
import { toastStore } from '@/lib/toast'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import config from '../../../../scripts/fixtures/guest-config.json'
import { planToValues, valuesToPlanInput } from './planForm'
import { PlanSheet } from './PlanSheet'

/** 4 active subscribers; `holders` (every `plan_id` match, expired included) is set per test. */
const plan: AdminPlan = { ...makePlan({ id: 2, name: 'Pro', group_id: 3, conn_limit: 128 }), user_count: 4 }
const holdersKey = qk.admin.users({ plan_id: 2, page_size: 1 })
/** Everything the sheet sends for an untouched plan (the PATCH body carries every field). */
const unchanged = valuesToPlanInput(planToValues(plan, config.currency), config.currency)

let holders = 6
const fetchMock = vi.fn<typeof fetch>()
const keyOf = (input: Parameters<typeof fetch>[0], init?: RequestInit) => {
  const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
  return `${(init?.method ?? 'GET').toUpperCase()} ${url.pathname.replace('/api/v1', '')}`
}
const sent = (key: string) =>
  fetchMock.mock.calls
    .filter(([input, init]) => keyOf(input, init) === key)
    .map(
      ([, init]) =>
        JSON.parse(typeof init?.body === 'string' ? init.body : 'null') as Record<string, unknown> | null,
    )
const patches = () => sent('PATCH /admin/plans/2')

const respond = (body: unknown) =>
  new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })

function renderSheet(sheetPlan: AdminPlan | null, onOpenChange = vi.fn()) {
  const client = createQueryClient()
  // Admin user views cached elsewhere in the console (a list page and a user's detail drawer).
  client.setQueryData(qk.admin.users({ page: 1 }), { items: [], total: 0 })
  client.setQueryData(qk.admin.user(9), { id: 9 })
  const view = render(
    <QueryClientProvider client={client}>
      <PlanSheet plan={sheetPlan} isOpen onOpenChange={onOpenChange} />
      <ConfirmHost />
    </QueryClientProvider>,
  )
  return { client, onOpenChange, view }
}

const syncSwitch = () => screen.findByRole('switch', { name: '同步到现有用户' }, { timeout: 4000 })
const saveButton = () => screen.getByRole('button', { name: '保存' })
const isInvalidated = (client: QueryClient, key: readonly unknown[]) =>
  client.getQueryState(key)?.isInvalidated ?? false

describe('plan sheet: apply to current users', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['admin-plans', 'common'])
  })
  beforeEach(() => {
    holders = 6
    toastStore.clear()
    useConfirmStore.setState({ current: null, queue: [] })
    useAuthStore.setState({ token: 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig', user: null })
    fetchMock.mockReset()
    fetchMock.mockImplementation((input, init) => {
      const key = keyOf(input, init)
      if (key === 'GET /guest/config') return Promise.resolve(respond({ data: config }))
      if (key === 'GET /admin/groups') return Promise.resolve(respond({ data: [{ id: 3, name: '默认组' }] }))
      if (key === 'GET /admin/users') return Promise.resolve(respond({ data: [], total: holders }))
      if (key === 'PATCH /admin/plans/2') return Promise.resolve(respond({ data: plan }))
      if (key === 'POST /admin/plans') return Promise.resolve(respond({ data: { ...plan, id: 5 } }))
      return Promise.resolve(new Response('{"code":"not_found","message":"x"}', { status: 404 }))
    })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    useAuthStore.setState({ token: null, user: null })
  })

  it('is not offered when creating a plan', async () => {
    renderSheet(null)
    await screen.findByRole('textbox', { name: /^名称/ }, { timeout: 4000 })
    expect(screen.queryByRole('switch', { name: '同步到现有用户' })).toBeNull()
    // Nothing to count either.
    expect(sent('GET /admin/users')).toHaveLength(0)
  })

  it('is not offered when nobody holds the plan', async () => {
    holders = 0
    const { client } = renderSheet({ ...plan, user_count: 0 })
    await waitFor(() => expect(client.getQueryState(holdersKey)?.status).toBe('success'))
    expect(screen.queryByRole('switch', { name: '同步到现有用户' })).toBeNull()
    expect(screen.getByRole('switch', { name: '显示' })).toBeInTheDocument()
  })

  it('is offered when only expired users still hold the plan (they are overwritten too)', async () => {
    holders = 2
    renderSheet({ ...plan, user_count: 0 })
    expect(await syncSwitch()).not.toBeChecked()
  })

  it('explains what gets overwritten behind the ⓘ, not on the page', async () => {
    const u = userEvent.setup()
    renderSheet(plan)
    await syncSwitch()
    expect(screen.getByText(/覆盖到所有持有此套餐的用户/)).toHaveClass('sr-only')
    // The popover is the newest dialog (the drawer is one too); see MISTAKES #217.
    const before = screen.queryAllByRole('dialog').length
    await u.click(screen.getByRole('button', { name: '关于「同步到现有用户」的说明' }))
    const popover = await waitFor(() => {
      const dialogs = screen.getAllByRole('dialog', { hidden: true })
      expect(dialogs.length).toBeGreaterThan(before)
      return dialogs.at(-1)!
    })
    expect(popover).toHaveTextContent(
      '保存时把权限组、流量额度、限速、设备数和连接数覆盖到所有持有此套餐的用户（含已过期的），已用流量和到期时间不变。',
    )
    expect(within(popover).queryByRole('button', { name: '保存' })).toBeNull()
  })

  it('asks first, naming every holder, then sends one PATCH with force_update (even with nothing changed)', async () => {
    const u = userEvent.setup()
    const { client, onOpenChange } = renderSheet(plan)
    await u.click(await syncSwitch())
    await u.click(saveButton())

    const dialog = await screen.findByRole('alertdialog', { name: '同步到 6 个用户？' })
    expect(dialog).toHaveTextContent('权限组、流量额度、限速、设备数和连接数都将改为此套餐的设置')
    expect(dialog).toHaveTextContent('单独为用户调整过的值（如手动增加的流量）会丢失')
    expect(patches()).toHaveLength(0)

    await u.click(within(dialog).getByRole('button', { name: '保存并同步' }))
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(patches()).toEqual([{ ...unchanged, force_update: true }])
    expect(toastStore.getSnapshot().map((t) => t.message)).toContain('套餐已保存，已同步到 6 个用户')
    // The holders' group, quota and limits changed: cached user lists and details are refetched.
    expect(isInvalidated(client, qk.admin.users({ page: 1 }))).toBe(true)
    expect(isInvalidated(client, qk.admin.user(9))).toBe(true)
  })

  it('sends the edited fields and force_update together', async () => {
    const u = userEvent.setup()
    renderSheet(plan)
    const conn = await screen.findByRole('textbox', { name: /^连接数限制/ }, { timeout: 4000 })
    await u.clear(conn)
    await u.type(conn, '64')
    await u.click(await syncSwitch())
    await u.click(saveButton())
    const dialog = await screen.findByRole('alertdialog', { name: '同步到 6 个用户？' })
    await u.click(within(dialog).getByRole('button', { name: '保存并同步' }))
    await waitFor(() => expect(patches()).toHaveLength(1))
    expect(patches()[0]).toEqual({ ...unchanged, conn_limit: 64, force_update: true })
  })

  it('cancelling the confirmation saves nothing and keeps the sheet open', async () => {
    const u = userEvent.setup()
    const { onOpenChange } = renderSheet(plan)
    await u.click(await syncSwitch())
    await u.click(saveButton())
    const dialog = await screen.findByRole('alertdialog', { name: '同步到 6 个用户？' })
    await u.click(within(dialog).getByRole('button', { name: '取消' }))
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
    expect(patches()).toHaveLength(0)
    expect(onOpenChange).not.toHaveBeenCalled()
    await waitFor(() => expect(saveButton()).toBeEnabled())
    expect(await syncSwitch()).toBeChecked()
  })

  it('switched off (the default) saves without force_update and without asking', async () => {
    const u = userEvent.setup()
    const { client, onOpenChange } = renderSheet(plan)
    expect(await syncSwitch()).not.toBeChecked()
    await u.click(saveButton())
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(patches()).toEqual([unchanged])
    expect(patches()[0]).not.toHaveProperty('force_update')
    expect(toastStore.getSnapshot().map((t) => t.message)).toContain('套餐已保存')
    expect(isInvalidated(client, qk.admin.users({ page: 1 }))).toBe(false)
  })

  it('saves normally if nobody holds the plan any more by the time it is saved', async () => {
    const u = userEvent.setup()
    const { client, onOpenChange } = renderSheet(plan)
    await u.click(await syncSwitch())
    // The count shown when the sheet opened is stale; the save recounts and finds no one.
    holders = 0
    client.setQueryData(holdersKey, { items: [], total: 6 }, { updatedAt: 0 })
    await u.click(saveButton())
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(patches()).toEqual([unchanged])
  })

  it('starts off every time the sheet opens', async () => {
    const u = userEvent.setup()
    const first = renderSheet(plan)
    await u.click(await syncSwitch())
    expect(await syncSwitch()).toBeChecked()
    first.view.unmount()
    renderSheet(plan)
    expect(await syncSwitch()).not.toBeChecked()
  })
})
