/**
 * `/admin/nodes` through the real route tree (guards, i18n, React Query) with a mocked `fetch`:
 * the show/hide eye toggle, drag-to-reorder through the row handle and the per-node token.
 */
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Node, User } from '@/lib/api/types'
import { queryClient } from '@/lib/queryClient'
import { toastStore } from '@/lib/toast'
import { routeTree } from '@/routeTree.gen'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import config from '../../../../scripts/fixtures/guest-config.json'
import fixtureUser from '../../../../scripts/fixtures/user.json'

const TOKEN = `eyJhbGciOiJub25lIn0.${btoa(JSON.stringify({ sub: 1, exp: 4102444800 })).replace(/=+$/, '')}.test`
const admin = { ...(fixtureUser as User), is_admin: true }

function node(id: number, name: string, show: boolean, extra: Partial<Node> = {}): Node {
  return {
    id,
    type: 'shadowsocks',
    name,
    group_ids: [],
    route_ids: [],
    parent_id: null,
    host: `n${id}.example.com`,
    port: '443',
    server_port: 443,
    tags: [],
    rate: 1,
    show,
    sort: id,
    protocol_settings: { cipher: 'aes-128-gcm' },
    status: { online: 0, last_check_at: null, last_push_at: null, load: null },
    token: '',
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    ...extra,
  }
}

/** A fixed, obviously fake token (32 alphanumerics). */
const FAKE_TOKEN = 'exampleNodeToken0123456789abcdef'

let nodes: Node[] = []
const writes: { method: string; path: string; body: unknown }[] = []

function respond(data: unknown) {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

/** JSON request body (the API client always sends a string). */
const jsonBody = <T,>(init: RequestInit | undefined): T =>
  (typeof init?.body === 'string' ? JSON.parse(init.body) : null) as T

function route(url: URL, init: RequestInit | undefined): Response {
  const p = url.pathname.replace('/api/v1', '')
  const method = init?.method ?? 'GET'
  if (method !== 'GET') writes.push({ method, path: p, body: jsonBody<unknown>(init) })
  const patch = /^\/admin\/nodes\/(\d+)$/.exec(p)
  if (patch && method === 'PATCH') {
    const body = jsonBody<Partial<Node>>(init)
    nodes = nodes.map((n) => (n.id === Number(patch[1]) ? { ...n, ...body } : n))
    return respond(nodes.find((n) => n.id === Number(patch[1])))
  }
  const copy = /^\/admin\/nodes\/(\d+)\/copy$/.exec(p)
  if (copy && method === 'POST') {
    const source = nodes.find((n) => n.id === Number(copy[1]))!
    const id = Math.max(...nodes.map((n) => n.id)) + 1
    const dup = {
      ...source,
      id,
      name: `${source.name} (副本)`,
      token: source.token ? 'freshCopyToken0123456789abcdefgh' : '',
    }
    nodes = [...nodes, dup]
    return respond(dup)
  }
  if (p === '/admin/nodes/sort' && method === 'POST') {
    const { ids } = jsonBody<{ ids: number[] }>(init)
    nodes = ids.map((id, i) => ({ ...nodes.find((n) => n.id === id)!, sort: i + 1 }))
    return respond(true)
  }
  switch (p) {
    case '/guest/config':
      return respond(config)
    case '/user/me':
      return respond(admin)
    case '/admin/nodes':
      return respond(nodes)
    case '/admin/groups':
    case '/admin/routes':
      return respond([])
    default:
      return new Response(JSON.stringify({ code: 'not_found', message: p }), { status: 404 })
  }
}

const fetchMock = vi.fn<typeof fetch>()

function renderAt(url: string) {
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [url] }) })
  render(<RouterProvider router={router} />)
}

beforeEach(() => {
  nodes = [node(1, 'HK01', true), node(2, 'JP02', true), node(3, 'SG03', false)]
  writes.length = 0
  // Each test sets its own node list: drop the previous test's cached `/admin/nodes`.
  queryClient.clear()
  toastStore.clear()
  useUiStore.setState({ locale: 'zh-CN', theme: 'system' })
  useAuthStore.setState({ token: TOKEN, user: admin })
  fetchMock.mockImplementation((input, init) => {
    const raw = input instanceof Request ? input.url : input.toString()
    return Promise.resolve(route(new URL(raw, 'http://localhost'), init))
  })
  vi.stubGlobal('fetch', fetchMock)
  // Desktop layout (the DataTable renders the HeroUI table at ≥ 768 px).
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((query: string) => ({
      matches: query.includes('min-width'),
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  )
})
afterEach(() => vi.unstubAllGlobals())

/** Integration snippets (the intro sentence also names the placeholder). */
const CODE = { selector: 'code' }

/** Body of the last PATCH to `path` (the sheet sends the whole node). */
const lastPatch = (path: string) => writes.filter((w) => w.method === 'PATCH' && w.path === path).at(-1)?.body

const nameOrder = () =>
  within(screen.getByRole('grid', { name: '节点' }))
    .getAllByRole('button', { name: /^编辑节点 / })
    .map((b) => b.textContent)

describe('admin nodes page', () => {
  it('shows visibility as an eye toggle and saves it with PATCH {show}', { timeout: 30_000 }, async () => {
    const user = userEvent.setup()
    renderAt('/admin/nodes')
    const hk = await screen.findByRole('button', { name: '对用户显示「HK01」' }, { timeout: 15_000 })
    const sg = screen.getByRole('button', { name: '对用户显示「SG03」' })
    expect(hk).toHaveAttribute('aria-pressed', 'true')
    expect(sg).toHaveAttribute('aria-pressed', 'false')
    // No text chip any more; the hidden node's name is dimmed instead.
    expect(screen.queryByText('已隐藏')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '编辑节点 SG03' })).toHaveClass('text-label-secondary')
    expect(screen.getByRole('button', { name: '编辑节点 HK01' })).toHaveClass('text-label-blue')

    await user.click(hk)
    await waitFor(() => expect(hk).toHaveAttribute('aria-pressed', 'false'))
    expect(writes).toContainEqual({ method: 'PATCH', path: '/admin/nodes/1', body: { show: false } })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '编辑节点 HK01' })).toHaveClass('text-label-secondary'),
    )

    await user.click(sg)
    await waitFor(() => expect(sg).toHaveAttribute('aria-pressed', 'true'))
    expect(writes).toContainEqual({ method: 'PATCH', path: '/admin/nodes/3', body: { show: true } })

    // The "…" menu no longer duplicates the toggle.
    await user.click(screen.getByRole('button', { name: 'JP02 的更多操作' }))
    expect(await screen.findByRole('menuitem', { name: '复制节点' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: /对用户隐藏|向用户显示/ })).not.toBeInTheDocument()
  })

  it(
    'reorders by keyboard through the drag handle and saves the full order',
    { timeout: 30_000 },
    async () => {
      const user = userEvent.setup()
      renderAt('/admin/nodes')
      const handle = await screen.findByRole('button', { name: '拖动以排序：HK01' }, { timeout: 15_000 })
      expect(handle).toBeEnabled()
      expect(screen.queryByRole('button', { name: /上移|下移/ })).not.toBeInTheDocument()
      expect(nameOrder()).toEqual(['HK01', 'JP02', 'SG03'])

      // Grid keyboard model: focus the first row, → into its first cell, Tab to the handle.
      within(screen.getByRole('grid', { name: '节点' }))
        .getAllByRole('row')[1]
        ?.focus()
      await user.keyboard('{ArrowRight}')
      await user.tab()
      await waitFor(() => expect(handle).toHaveFocus())
      await user.keyboard('{Enter}')
      await waitFor(() => expect(document.activeElement).toHaveAttribute('role', 'button'))
      await user.keyboard('{ArrowDown}{Enter}')

      await waitFor(() =>
        expect(writes).toContainEqual({
          method: 'POST',
          path: '/admin/nodes/sort',
          body: { ids: [2, 1, 3] },
        }),
      )
      await waitFor(() => expect(nameOrder()).toEqual(['JP02', 'HK01', 'SG03']))
    },
  )

  it('disables reordering while a filter is active', { timeout: 30_000 }, async () => {
    renderAt('/admin/nodes?q=0')
    // `?q=0` (a node ID typed into the URL) is parsed as a number: it must filter, not crash.
    expect(
      await screen.findByRole('button', { name: '编辑节点 HK01' }, { timeout: 15_000 }),
    ).toBeInTheDocument()
    const handle = await screen.findByRole('button', { name: '拖动以排序：HK01' }, { timeout: 15_000 })
    expect(handle).toBeDisabled()
    expect(screen.getByRole('button', { name: '拖动以排序：SG03' })).toBeDisabled()
    expect(handle.closest('[role="row"]')).not.toHaveAttribute('draggable', 'true')
  })

  it(
    'generates a node token, shows <node_token> in the snippets and sends it in the PATCH',
    { timeout: 30_000 },
    async () => {
      const user = userEvent.setup()
      renderAt('/admin/nodes')
      await user.click(await screen.findByRole('button', { name: '编辑节点 HK01' }, { timeout: 15_000 }))
      const sheet = await screen.findByRole('dialog', { name: /编辑节点「HK01」/ })

      // Empty = the shared server_token; a masked, never-autofilled input.
      const input = within(sheet).getByLabelText('节点密钥')
      expect(input).toHaveAttribute('type', 'password')
      expect(input).toHaveAttribute('autocomplete', 'new-password')
      expect(input).toHaveAttribute('name', 'token')
      expect(input).toHaveValue('')
      expect(input).toHaveAttribute('placeholder', '使用共享通讯密钥')
      expect(within(sheet).queryByRole('button', { name: '改用共享通讯密钥' })).not.toBeInTheDocument()
      // The explanation sits behind the label's ⓘ (only an sr-only copy on the page).
      expect(within(sheet).getByText(/共享通讯密钥对本节点失效/)).toHaveClass('sr-only')
      expect(within(sheet).getByRole('button', { name: '关于「节点密钥」的说明' })).toBeInTheDocument()
      expect(within(sheet).getAllByText(/<server_token>/, CODE)).toHaveLength(3)
      expect(within(sheet).queryByText(/<node_token>/, CODE)).not.toBeInTheDocument()

      await user.click(within(sheet).getByRole('button', { name: '生成随机密钥' }))
      const token = (input as HTMLInputElement).value
      expect(token).toMatch(/^[A-Za-z0-9]{32}$/)
      expect(input).toHaveAttribute('type', 'text') // revealed after generating
      expect(within(sheet).getByRole('button', { name: '改用共享通讯密钥' })).toBeInTheDocument()

      await user.click(within(sheet).getByRole('tab', { name: '对接' }))
      expect(within(sheet).getAllByText(/<node_token>/, CODE)).toHaveLength(3)
      expect(within(sheet).queryByText(/<server_token>/, CODE)).not.toBeInTheDocument()
      expect(within(sheet).getByText(/<node_token> 为本节点的节点密钥/)).toBeInTheDocument()
      expect(sheet.textContent).not.toContain(token) // placeholders only, never the secret

      await user.click(within(sheet).getByRole('button', { name: '保存' }))
      await waitFor(() => expect(lastPatch('/admin/nodes/1')).toMatchObject({ parent_id: null, token }))
    },
  )

  it('rejects a malformed token and clears back to the shared one', { timeout: 30_000 }, async () => {
    nodes[0] = node(1, 'HK01', true, { token: FAKE_TOKEN })
    const user = userEvent.setup()
    renderAt('/admin/nodes')
    await user.click(await screen.findByRole('button', { name: '编辑节点 HK01' }, { timeout: 15_000 }))
    const sheet = await screen.findByRole('dialog', { name: /编辑节点「HK01」/ })
    const input = within(sheet).getByLabelText('节点密钥')
    expect(input).toHaveValue(FAKE_TOKEN)
    expect(within(sheet).getAllByText(/<node_token>/, CODE)).toHaveLength(3)

    await user.clear(input)
    await user.type(input, 'too short')
    await user.click(within(sheet).getByRole('button', { name: '保存' }))
    expect(await within(sheet).findByText(/需为 16–64 个可见 ASCII 字符/)).toBeInTheDocument()
    expect(writes.filter((w) => w.method === 'PATCH')).toHaveLength(0)

    await user.click(within(sheet).getByRole('button', { name: '改用共享通讯密钥' }))
    expect(input).toHaveValue('')
    expect(within(sheet).getAllByText(/<server_token>/, CODE)).toHaveLength(3)
    await user.click(within(sheet).getByRole('button', { name: '保存' }))
    await waitFor(() => expect(lastPatch('/admin/nodes/1')).toMatchObject({ token: '' }))
  })

  it(
    "hides the token for child nodes, sends '' and points the snippets at the parent's token",
    {
      timeout: 30_000,
    },
    async () => {
      nodes[0] = node(1, 'HK01', true, { token: FAKE_TOKEN })
      nodes[2] = node(3, 'SG03', false, { token: 'standaloneToken0123456789' })
      const user = userEvent.setup()
      renderAt('/admin/nodes')
      await user.click(await screen.findByRole('button', { name: '编辑节点 SG03' }, { timeout: 15_000 }))
      const sheet = await screen.findByRole('dialog', { name: /编辑节点「SG03」/ })
      expect(within(sheet).getByLabelText('节点密钥')).toHaveValue('standaloneToken0123456789')

      // The parent Select's trigger shows the current choice.
      await user.click(within(sheet).getByRole('button', { name: /无（独立节点）/ }))
      await user.click(await screen.findByRole('option', { name: 'HK01 (#1)' }))
      await waitFor(() => expect(within(sheet).queryByLabelText('节点密钥')).not.toBeInTheDocument())
      expect(within(sheet).getAllByText(/<node_token>/, CODE)).toHaveLength(3)
      expect(
        within(sheet).getByText('在后端填写以下配置。<node_token> 为父节点「HK01」的节点密钥。'),
      ).toBeInTheDocument()

      await user.click(within(sheet).getByRole('button', { name: '保存' }))
      await waitFor(() => expect(lastPatch('/admin/nodes/3')).toMatchObject({ parent_id: 1, token: '' }))
    },
  )

  it('says when a copy got a fresh node token', { timeout: 30_000 }, async () => {
    nodes[0] = node(1, 'HK01', true, { token: FAKE_TOKEN })
    const user = userEvent.setup()
    renderAt('/admin/nodes')
    for (const [name, toast] of [
      ['HK01', '已复制为「HK01 (副本)」，副本使用新的节点密钥'],
      ['JP02', '已复制为「JP02 (副本)」'],
    ] as const) {
      await user.click(await screen.findByRole('button', { name: `${name} 的更多操作` }, { timeout: 15_000 }))
      await user.click(await screen.findByRole('menuitem', { name: '复制节点' }))
      await waitFor(() => expect(toastStore.getSnapshot().map((t) => t.message)).toContain(toast))
    }
  })
})
